package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
)

type ProgressKind string

const (
	ProgressBoot        ProgressKind = "boot"
	ProgressStart       ProgressKind = "start"
	ProgressStep        ProgressKind = "step"
	ProgressComplete    ProgressKind = "complete"
	ProgressFailure     ProgressKind = "failure"
	ProgressPivot       ProgressKind = "pivot"
	ProgressDone        ProgressKind = "done"
	ProgressMessageKind ProgressKind = "message"
)

type ProgressEvent struct {
	Kind         ProgressKind
	Title        string
	Detail       string
	Iteration    int
	Total        int
	Completed    int
	Failures     int
	Percent      float64
	HasIteration bool
	HasTotal     bool
	HasCompleted bool
	HasFailures  bool
	HasPercent   bool
	When         time.Time
	Meta         map[string]any
}

type ProgressCarrier interface {
	ProgressEvent() ProgressEvent
}

type ProgressMsg struct {
	Event ProgressEvent
}

func (m ProgressMsg) ProgressEvent() ProgressEvent { return m.Event }

func EventMsg(ev ProgressEvent) tea.Msg { return ProgressMsg{Event: ev} }

func AsProgress(msg any) (ProgressEvent, bool) {
	switch value := msg.(type) {
	case ProgressEvent:
		return value, true
	case ProgressMsg:
		return value.Event, true
	case ProgressCarrier:
		return value.ProgressEvent(), true
	case *ProgressMsg:
		if value == nil {
			return ProgressEvent{}, false
		}
		return value.Event, true
	case interface{ ProgressEvent() ProgressEvent }:
		return value.ProgressEvent(), true
	default:
		return ProgressEvent{}, false
	}
}

type frameMsg time.Time

type RunModel struct {
	title     string
	subtitle  string
	total     int
	completed int
	failures  int
	percent   float64
	status    ProgressKind
	headline  string
	detail    string
	events    []ProgressEvent

	width  int
	height int

	bar     progress.Model
	spin    spinner.Model
	springX harmonica.Spring
	springY harmonica.Spring

	orbX  float64
	orbY  float64
	orbVX float64
	orbVY float64
	phase float64
	trail []trailPoint

	lastFrame time.Time
	started   time.Time
}

type trailPoint struct {
	X float64
	Y float64
}

func NewRunModel(total int) RunModel {
	bar := progress.New(
		progress.WithGradient("#35F2FF", "#FF4FD8"),
		progress.WithFillCharacters('█', '░'),
		progress.WithoutPercentage(),
	)
	bar.Width = 36

	spin := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(titleStyle().Foreground(accentCyan)),
	)

	now := time.Now()
	return RunModel{
		title:     "ROOM live run",
		subtitle:  "orchestrating repo improvement in real time",
		total:     total,
		bar:       bar,
		spin:      spin,
		springX:   harmonica.NewSpring(harmonica.FPS(30), 9.0, 0.62),
		springY:   harmonica.NewSpring(harmonica.FPS(30), 8.0, 0.58),
		orbX:      10,
		orbY:      4,
		trail:     []trailPoint{{X: 10, Y: 4}},
		width:     100,
		height:    30,
		lastFrame: now,
		started:   now,
		status:    ProgressBoot,
		headline:  "warming the hyperdrive",
		detail:    "booting the neon loop",
	}
}

func (m RunModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.frameCmd())
}

func (m RunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = value.Width
		m.height = value.Height
		barWidth := value.Width - 26
		if barWidth < 12 {
			barWidth = 12
		}
		m.bar.Width = barWidth
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(value)
		return m, cmd
	case frameMsg:
		m.stepFrame(time.Time(value))
		if m.percent > 1 {
			m.percent = 1
		}
		return m, m.frameCmd()
	case ProgressMsg:
		return m.consume(value.Event), nil
	case ProgressEvent:
		return m.consume(value), nil
	case *ProgressMsg:
		if value == nil {
			return m, nil
		}
		return m.consume(value.Event), nil
	case ProgressCarrier:
		return m.consume(value.ProgressEvent()), nil
	}
	return m, nil
}

func (m RunModel) View() string {
	innerWidth := m.width
	if innerWidth < 80 {
		innerWidth = 80
	}

	leftWidth := innerWidth / 2
	if leftWidth < 36 {
		leftWidth = 36
	}
	rightWidth := innerWidth - leftWidth - 2
	if rightWidth < 30 {
		rightWidth = 30
	}

	left := strings.Join([]string{
		rainbow(strings.ToUpper(m.title)),
		subtitleStyle().Render(m.subtitle),
		"",
		kvLine("status", string(m.status), accentPink),
		kvLine("progress", fmt.Sprintf("%d/%d", m.completed, m.total), accentCyan),
		kvLine("failures", fmt.Sprintf("%d", m.failures), accentGold),
		"",
		m.spin.View() + " " + subtitleStyle().Render(m.headline),
		"",
		m.bar.ViewAs(m.progressValue()),
		"",
		subtitleStyle().Render(m.detail),
	}, "\n")

	right := strings.Join([]string{
		m.renderPhysicsPanel(rightWidth),
		"",
		m.renderEventStream(),
	}, "\n")

	frame := lipglossJoinHorizontal(
		neonPanel(accentCyan).Width(leftWidth).Render(left),
		neonPanel(accentPink).Width(rightWidth).Render(right),
	)

	footer := framed("HINTS", strings.Join([]string{
		statusBadge(string(m.status)),
		subtitleStyle().Render("ctrl+c stops the run"),
	}, "  "), accentViolet)

	return lipglossJoinVertical(
		framed("ROOM LIVE", strings.Join([]string{
			rainbow("NEON ITERATION ENGINE"),
			subtitleStyle().Render(fmt.Sprintf("running for %s", time.Since(m.started).Round(time.Millisecond))),
		}, "\n"), accentCyan),
		frame,
		footer,
	)
}

func (m RunModel) consume(ev ProgressEvent) RunModel {
	if ev.When.IsZero() {
		ev.When = time.Now()
	}
	if ev.Title != "" {
		m.headline = ev.Title
	}
	if ev.Detail != "" {
		m.detail = ev.Detail
	}
	if ev.HasIteration && ev.Title == "" {
		m.headline = fmt.Sprintf("iteration %d", ev.Iteration)
	}
	if ev.HasTotal {
		m.total = ev.Total
	}
	if ev.HasCompleted {
		m.completed = ev.Completed
	}
	if ev.HasFailures {
		m.failures = ev.Failures
	}
	if ev.HasPercent {
		m.percent = ev.Percent
	} else if m.total > 0 {
		m.percent = float64(m.completed) / float64(m.total)
	}
	if ev.Kind != "" {
		m.status = ev.Kind
	}
	m.events = append(m.events, ev)
	if len(m.events) > 10 {
		m.events = append([]ProgressEvent(nil), m.events[len(m.events)-10:]...)
	}
	if ev.Kind == ProgressDone {
		m.percent = 1
		m.status = ProgressDone
	}
	return m
}

func (m RunModel) progressValue() float64 {
	switch {
	case m.percent < 0:
		return 0
	case m.percent > 1:
		return 1
	default:
		return m.percent
	}
}

func (m RunModel) stepFrame(now time.Time) RunModel {
	dt := now.Sub(m.lastFrame).Seconds()
	if dt <= 0 {
		dt = 1.0 / 30.0
	}
	m.lastFrame = now
	m.phase += dt * 2.1

	targetX, targetY := m.targetPoint()
	m.orbX, m.orbVX = m.springX.Update(m.orbX, m.orbVX, targetX)
	m.orbY, m.orbVY = m.springY.Update(m.orbY, m.orbVY, targetY)
	m.trail = append(m.trail, trailPoint{X: m.orbX, Y: m.orbY})
	if len(m.trail) > 14 {
		m.trail = append([]trailPoint(nil), m.trail[len(m.trail)-14:]...)
	}
	return m
}

func (m RunModel) targetPoint() (float64, float64) {
	const gridW = 22
	const gridH = 9

	progress := m.progressValue()
	x := 1 + progress*float64(gridW-3)
	y := 1 + (math.Sin(m.phase)*0.5+0.5)*float64(gridH-3)

	switch m.status {
	case ProgressFailure:
		x = 1
		y = float64(gridH - 3)
	case ProgressPivot:
		x = float64(gridW / 2)
		y = 1
	case ProgressDone:
		x = float64(gridW - 3)
		y = 1
	case ProgressBoot:
		x = float64(gridW / 3)
		y = float64(gridH / 2)
	}
	return x, y
}

func (m RunModel) renderPhysicsPanel(width int) string {
	const gridW = 22
	const gridH = 9

	gx := int(math.Round(m.orbX))
	gy := int(math.Round(m.orbY))
	if gx < 0 {
		gx = 0
	}
	if gy < 0 {
		gy = 0
	}
	if gx >= gridW {
		gx = gridW - 1
	}
	if gy >= gridH {
		gy = gridH - 1
	}

	trail := make(map[[2]int]int, len(m.trail))
	for i, pt := range m.trail {
		x := int(math.Round(pt.X))
		y := int(math.Round(pt.Y))
		if x < 0 || y < 0 || x >= gridW || y >= gridH {
			continue
		}
		trail[[2]int{x, y}] = i + 1
	}

	var b strings.Builder
	for y := 0; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			switch {
			case x == gx && y == gy:
				b.WriteString(accentBadge(bgColor, accentCyan).Render("•"))
			case x == gridW/2 && y == gridH/2:
				b.WriteString(accentBadge(bgColor, accentPink).Render("◉"))
			case x == gridW/2 || y == gridH/2:
				b.WriteString(subtitleStyle().Render("·"))
			case trail[[2]int{x, y}] > 0:
				b.WriteString(bulletStyle(accentViolet).Render("·"))
			default:
				b.WriteString(subtitleStyle().Render(" "))
			}
		}
		if y < gridH-1 {
			b.WriteByte('\n')
		}
	}

	title := strings.Join([]string{
		accentBadge(bgColor, accentPink).Render(" ORBIT "),
		subtitleStyle().Render(fmt.Sprintf("x=%.1f y=%.1f", m.orbX, m.orbY)),
	}, " ")

	body := strings.Join([]string{
		title,
		"",
		b.String(),
	}, "\n")

	if width < 36 {
		width = 36
	}
	return neonPanel(accentPink).Width(width).Render(body)
}

func (m RunModel) renderEventStream() string {
	if len(m.events) == 0 {
		return framed("EVENTS", subtitleStyle().Render("waiting for progress messages"), accentOrange)
	}
	var lines []string
	for i := len(m.events) - 1; i >= 0 && len(lines) < 6; i-- {
		ev := m.events[i]
		badge := statusBadge(string(ev.Kind))
		if ev.Title == "" {
			ev.Title = ev.Detail
		}
		lines = append(lines, fmt.Sprintf("%s %s", badge, ev.Title))
		if ev.Detail != "" && ev.Detail != ev.Title {
			lines = append(lines, "  "+subtitleStyle().Render(ev.Detail))
		}
	}
	return framed("EVENTS", strings.Join(lines, "\n"), accentOrange)
}

func (m RunModel) frameCmd() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg {
		return frameMsg(t)
	})
}
