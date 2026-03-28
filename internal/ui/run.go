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
	"github.com/charmbracelet/lipgloss"
	"github.com/jcpsimmons/room/internal/audio"
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
	Stdout       string
	Stderr       string
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

	// Run params surfaced in the CONFIG panel.
	Provider      string
	Model         string
	RepoRoot      string
	CommitEnabled bool
	DryRun        bool
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
	case *ProgressMsg:
		if value == nil {
			return ProgressEvent{}, false
		}
		return value.Event, true
	case ProgressCarrier:
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
	stdout    string
	stderr    string
	events    []ProgressEvent

	// Run config params for the CONFIG panel.
	provider      string
	model         string
	repoRoot      string
	commitEnabled bool
	dryRun        bool
	audioMuted    bool
	auxPanel      string

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

	// Spark particles launched on status changes.
	sparks []spark

	// Generative visualizations.
	decay     decayChamber
	resonance resonanceField
	flux      fluxTopology

	// Audio synthesis.
	synth      *audio.Synth
	outcomeCue audioCue

	lastFrame time.Time
	started   time.Time
}

type trailPoint struct {
	X, Y float64
}

type spark struct {
	proj  *harmonica.Projectile
	age   float64
	glyph rune
}

func NewRunModel(total int, opts ...RunOption) RunModel {
	bar := progress.New(
		progress.WithGradient("#9E2A1F", "#E8A435"),
		progress.WithFillCharacters('▮', '▯'),
		progress.WithoutPercentage(),
	)
	bar.Width = 36

	spin := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(titleStyle().Foreground(accentCyan)),
	)

	var rc runConfig
	for _, o := range opts {
		o(&rc)
	}

	now := time.Now()
	m := RunModel{
		title:     "ROOM series 100",
		subtitle:  "voltage-controlled repository sequencer",
		total:     total,
		bar:       bar,
		spin:      spin,
		springX:   harmonica.NewSpring(harmonica.FPS(30), 6.0, 0.28),
		springY:   harmonica.NewSpring(harmonica.FPS(30), 5.0, 0.22),
		orbX:      10,
		orbY:      4,
		trail:     []trailPoint{{X: 10, Y: 4}},
		decay:     newDecayChamber(20, 7),
		resonance: newResonanceField(20, 5),
		flux:      newFluxTopology(20, 7),
		width:     100,
		height:    30,
		lastFrame: now,
		started:   now,
		status:    ProgressBoot,
		headline:  "oscillator warming",
		detail:    "filaments reaching operating temperature",
		auxPanel:  "diagnostics",
	}
	if rc.audio {
		m.synth = audio.New()
	}
	return m
}

type runConfig struct {
	audio bool
}

// RunOption configures the run model.
type RunOption func(*runConfig)

// WithAudio enables the FM synthesizer output.
func WithAudio() RunOption {
	return func(c *runConfig) { c.audio = true }
}

func (m RunModel) Init() tea.Cmd {
	if m.synth != nil {
		_ = m.synth.Start()
	}
	return tea.Batch(m.spin.Tick, m.frameCmd())
}

// Shutdown stops audio output. Call after the program exits.
func (m RunModel) Shutdown() {
	if m.synth != nil {
		m.synth.Stop()
	}
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
	case tea.KeyMsg:
		switch value.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "tab":
			if m.auxPanel == "flux" {
				m.auxPanel = "diagnostics"
			} else {
				m.auxPanel = "flux"
			}
			return m, nil
		case "m":
			if m.synth != nil {
				m.audioMuted = !m.audioMuted
			}
			return m, nil
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(value)
		return m, cmd
	case frameMsg:
		m = m.stepFrame(time.Time(value))
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
	innerHeight := m.height
	if innerHeight < 24 {
		innerHeight = 24
	}

	gapWidth := 1
	leftWidth := int(math.Round(float64(innerWidth) * 0.56))
	if leftWidth < 38 {
		leftWidth = 38
	}
	rightWidth := innerWidth - leftWidth - gapWidth
	if rightWidth < 34 {
		rightWidth = 34
		leftWidth = innerWidth - rightWidth - gapWidth
	}
	if leftWidth < 38 {
		leftWidth = 38
		rightWidth = innerWidth - leftWidth - gapWidth
	}

	headerHeight := 5
	footerHeight := 3
	mainHeight := innerHeight - headerHeight - footerHeight - 2
	if mainHeight < 14 {
		mainHeight = 14
	}

	// Left column: OSCILLATOR + DECAY CHAMBER.
	statusHeight := mainHeight * 2 / 5
	if statusHeight < 10 {
		statusHeight = 10
	}
	decayHeight := mainHeight - statusHeight
	if decayHeight < 8 {
		decayHeight = 8
		statusHeight = mainHeight - decayHeight
	}

	// Right column: SCOPE + RESONANCE + DIAGNOSTICS + EVENTS.
	scopeHeight := 14
	if scopeHeight > mainHeight/3 {
		scopeHeight = mainHeight / 3
	}
	if scopeHeight < 10 {
		scopeHeight = 10
	}
	vizHeight := (mainHeight - scopeHeight) / 3
	if vizHeight < 5 {
		vizHeight = 5
	}
	eventsHeight := mainHeight - scopeHeight - vizHeight*2
	if eventsHeight < 4 {
		eventsHeight = 4
	}

	header := m.renderHeader(innerWidth, headerHeight)
	left := lipglossJoinVertical(
		m.renderStatusPanel(leftWidth, statusHeight),
		m.renderDecayPanel(leftWidth, decayHeight),
	)
	right := lipglossJoinVertical(
		m.renderPhysicsPanel(rightWidth, scopeHeight),
		m.renderResonancePanel(rightWidth, vizHeight),
		m.renderAuxPanel(rightWidth, vizHeight),
		m.renderEventStream(rightWidth, eventsHeight),
	)

	frame := lipglossJoinHorizontal(
		left,
		spacer(gapWidth),
		right,
	)

	footer := renderPanel("MANUAL", strings.Join([]string{
		statusBadge(string(m.status)),
		subtitleStyle().Render(m.manualText()),
	}, "  "), accentRed, innerWidth, footerHeight)

	content := lipglossJoinVertical(
		header,
		frame,
		footer,
	)

	return lipgloss.Place(
		innerWidth,
		innerHeight,
		lipgloss.Left,
		lipgloss.Top,
		content,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(bgColor),
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
	if ev.Stdout != "" || ev.Stderr != "" {
		m.stdout = ev.Stdout
		m.stderr = ev.Stderr
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
	if ev.Provider != "" {
		m.provider = ev.Provider
	}
	if ev.Model != "" {
		m.model = ev.Model
	}
	if ev.RepoRoot != "" {
		m.repoRoot = ev.RepoRoot
	}
	if ev.CommitEnabled {
		m.commitEnabled = true
	}
	if ev.DryRun {
		m.dryRun = true
	}
	if _, ok := outcomeToneRecipe(ev); ok {
		m.outcomeCue.trigger(ev)
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
	m.phase += dt * 2.8
	m.outcomeCue.step(dt)

	targetX, targetY := m.targetPoint()
	prevX, prevY := m.orbX, m.orbY
	m.orbX, m.orbVX = m.springX.Update(m.orbX, m.orbVX, targetX)
	m.orbY, m.orbVY = m.springY.Update(m.orbY, m.orbVY, targetY)

	// Longer, juicier trail.
	m.trail = append(m.trail, trailPoint{X: m.orbX, Y: m.orbY})
	if len(m.trail) > 28 {
		m.trail = append([]trailPoint(nil), m.trail[len(m.trail)-28:]...)
	}

	// Emit sparks when the orb is moving fast (spring overshoot).
	speed := math.Sqrt((m.orbX-prevX)*(m.orbX-prevX) + (m.orbY-prevY)*(m.orbY-prevY))
	if speed > 0.6 && len(m.sparks) < 12 {
		glyphs := []rune{'∿', '≋', '∼', '⏦', '·'}
		g := glyphs[len(m.trail)%len(glyphs)]
		// Fling spark perpendicular to motion.
		dx, dy := m.orbX-prevX, m.orbY-prevY
		m.sparks = append(m.sparks, spark{
			proj: harmonica.NewProjectile(
				harmonica.FPS(30),
				harmonica.Point{X: m.orbX, Y: m.orbY},
				harmonica.Vector{X: -dy * 3, Y: dx * 3},
				harmonica.Vector{X: 0, Y: 0.8},
			),
			glyph: g,
		})
	}

	// Tick sparks and cull dead ones.
	alive := m.sparks[:0]
	for _, s := range m.sparks {
		s.proj.Update()
		s.age += dt
		if s.age < 1.2 {
			alive = append(alive, s)
		}
	}
	m.sparks = alive

	// Step generative visualizations.
	m.decay.step(dt)
	m.resonance.step(dt)
	m.flux.step(dt)

	// Drive the synth from the orbit state.
	if m.synth != nil {
		speed := math.Sqrt(m.orbVX*m.orbVX + m.orbVY*m.orbVY)
		// X position → pitch: map grid range [0,22] to [55, 440] Hz (A1–A4).
		freq := 55.0 * math.Pow(2.0, m.orbX/7.33)
		// Velocity → amplitude with gentle curve. Quiet when still.
		amp := math.Min(speed*0.15, 0.7)
		// Y position → FM modulation depth: deeper at edges.
		modDepth := math.Abs(m.orbY-4.5) * 0.8
		// Modulator at a fifth above fundamental for metallic FM timbre.
		modFreq := freq * 1.5
		// Slight detune for beating.
		detune := 0.5 + math.Sin(m.phase*0.3)*0.3
		base := audio.Params{
			Freq:     freq,
			Amp:      amp,
			ModFreq:  modFreq,
			ModDepth: modDepth,
			Detune:   detune,
		}
		if m.audioMuted {
			base.Amp = 0
			m.synth.UpdateVoice(0, base)
			m.synth.UpdateVoice(1, audio.Params{})
			m.synth.UpdateVoice(2, audio.Params{})
			m.synth.UpdateVoice(3, audio.Params{})
			return m
		}
		m.synth.UpdateVoice(0, m.outcomeCue.audioParams(base))

		// Voice 1: Decay chamber — sub-bass rumble + decay pings.
		m.synth.UpdateVoice(1, m.decay.audioParams())

		// Voice 2: Resonance — crystalline beating tones.
		m.synth.UpdateVoice(2, m.resonance.audioParams())

		// Voice 3: Flux — filtered noise texture.
		m.synth.UpdateVoice(3, m.flux.audioParams())
	}

	return m
}

func (m RunModel) targetPoint() (float64, float64) {
	const gridW = 22
	const gridH = 9

	progress := m.progressValue()

	// Primary: progress sweeps left→right. Secondary: lissajous wobble.
	x := 1 + progress*float64(gridW-3) + math.Sin(m.phase*1.3)*2.0
	y := 1 + (math.Sin(m.phase)*0.5+0.5)*float64(gridH-3) + math.Cos(m.phase*0.7)*1.5

	// Clamp to grid.
	if x < 0 {
		x = 0
	}
	if x > float64(gridW-1) {
		x = float64(gridW - 1)
	}
	if y < 0 {
		y = 0
	}
	if y > float64(gridH-1) {
		y = float64(gridH - 1)
	}

	switch m.status {
	case ProgressFailure:
		x = 1 + math.Sin(m.phase*4)*1.5 // jitter on failure
		y = float64(gridH-3) + math.Cos(m.phase*3)*0.8
	case ProgressPivot:
		x = float64(gridW/2) + math.Sin(m.phase*2)*3
		y = 1 + math.Abs(math.Sin(m.phase*1.5))*2
	case ProgressDone:
		x = float64(gridW-3) + math.Sin(m.phase*0.5)*0.5
		y = 1 + math.Sin(m.phase*0.3)*0.3
	case ProgressBoot:
		x = float64(gridW/3) + math.Sin(m.phase)*2.5
		y = float64(gridH/2) + math.Cos(m.phase*1.4)*2
	}
	return x, y
}

func (m RunModel) renderHeader(width, height int) string {
	// Fake a drifting voltage reading from phase.
	voltage := 3.3 + math.Sin(m.phase*0.9)*1.7 + math.Sin(m.phase*2.3)*0.4
	body := strings.Join([]string{
		titleStyle().Render("SERIES 100 SEQUENCER"),
		subtitleStyle().Render("san francisco, ca"),
		strings.Join([]string{
			kvLine("gate", string(m.status), accentPink),
			kvLine("sequence", fmt.Sprintf("%d/%d", m.completed, maxInt(m.total, 1)), accentCyan),
			kvLine("cv", fmt.Sprintf("%.2fV", voltage), accentGold),
			kvLine("elapsed", time.Since(m.started).Round(time.Millisecond).String(), accentOrange),
		}, "  "),
	}, "\n")
	return renderPanel("PATCH", body, accentPink, width, height)
}

func (m RunModel) renderStatusPanel(width, height int) string {
	panelStyle := neonPanel(accentCyan)
	contentWidth := width - panelStyle.GetHorizontalFrameSize()
	if contentWidth < 12 {
		contentWidth = 12
	}

	bar := m.bar
	bar.Width = contentWidth

	// Oscillating resonance meter.
	resonance := math.Abs(math.Sin(m.phase*1.7)) * 100
	lines := []string{
		m.spin.View() + " " + titleStyle().Render(m.headline),
		lipgloss.NewStyle().Width(contentWidth).Render(m.detail),
		"",
		bar.ViewAs(m.progressValue()),
		"",
		strings.Join([]string{
			kvLine("overload", fmt.Sprintf("%d", m.failures), accentRed),
			kvLine("step", fmt.Sprintf("%d", maxInt(m.completed, m.eventIteration())), accentViolet),
			kvLine("Q", fmt.Sprintf("%.0f%%", resonance), accentGold),
		}, "  "),
	}

	// Fold in config params.
	provider := m.provider
	if provider == "" {
		provider = "—"
	}
	model := m.model
	if model == "" {
		model = "default"
	}
	lines = append(lines, "",
		strings.Join([]string{
			kvLine("source", strings.ToUpper(provider), accentOrange),
			kvLine("voice", model, accentPink),
		}, "  "),
	)
	audioState := "off"
	if m.synth != nil {
		audioState = "live"
		if m.audioMuted {
			audioState = "muted"
		}
	}
	auxState := strings.ToUpper(m.auxPanel)
	lines = append(lines,
		strings.Join([]string{
			kvLine("audio", audioState, accentLime),
			kvLine("aux", auxState, accentCyan),
		}, "  "),
	)

	body := strings.Join(lines, "\n")
	return renderPanel("OSCILLATOR", body, accentCyan, width, height)
}

func (m RunModel) eventIteration() int {
	for i := len(m.events) - 1; i >= 0; i-- {
		if m.events[i].Iteration > 0 {
			return m.events[i].Iteration
		}
	}
	return 0
}

func (m RunModel) renderPhysicsPanel(width, height int) string {
	const gridW = 22
	const gridH = 9

	gx := clampInt(int(math.Round(m.orbX)), 0, gridW-1)
	gy := clampInt(int(math.Round(m.orbY)), 0, gridH-1)

	// Build trail heat-map — phosphor persistence.
	trail := make(map[[2]int]int, len(m.trail))
	for i, pt := range m.trail {
		x := clampInt(int(math.Round(pt.X)), 0, gridW-1)
		y := clampInt(int(math.Round(pt.Y)), 0, gridH-1)
		trail[[2]int{x, y}] = i + 1
	}

	// Spark positions.
	sparkMap := make(map[[2]int]rune, len(m.sparks))
	for _, s := range m.sparks {
		pos := s.proj.Position()
		sx := clampInt(int(math.Round(pos.X)), 0, gridW-1)
		sy := clampInt(int(math.Round(pos.Y)), 0, gridH-1)
		sparkMap[[2]int{sx, sy}] = s.glyph
	}

	// Phosphor decay: recent = bright amber, old = dim red.
	trailColors := []lipgloss.Color{accentRed, accentOrange, accentGold, accentCyan}
	centerX, centerY := gridW/2, gridH/2

	var b strings.Builder
	for y := 0; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			key := [2]int{x, y}
			switch {
			case x == gx && y == gy:
				// Beam head — the electron dot.
				glyphs := []string{"█", "▓", "▒", "▓"}
				gi := int(m.phase*8) % len(glyphs)
				b.WriteString(accentBadge(accentGold).Render(glyphs[gi]))
			case sparkMap[key] != 0:
				b.WriteString(bulletStyle(accentGold).Render(string(sparkMap[key])))
			case x == centerX && y == centerY:
				// Graticule center.
				b.WriteString(bulletStyle(accentPink).Render("+"))
			case trail[key] > 0:
				age := trail[key]
				ci := age * len(trailColors) / (len(m.trail) + 1)
				if ci >= len(trailColors) {
					ci = len(trailColors) - 1
				}
				// Phosphor decay glyphs.
				dots := []string{".", ":", "~", "░"}
				di := age * len(dots) / (len(m.trail) + 1)
				if di >= len(dots) {
					di = len(dots) - 1
				}
				b.WriteString(bulletStyle(trailColors[ci]).Render(dots[di]))
			case x == centerX:
				b.WriteString(subtitleStyle().Render("│"))
			case y == centerY:
				b.WriteString(subtitleStyle().Render("─"))
			default:
				b.WriteString(subtitleStyle().Render(" "))
			}
		}
		if y < gridH-1 {
			b.WriteByte('\n')
		}
	}

	vel := math.Sqrt(m.orbVX*m.orbVX + m.orbVY*m.orbVY)
	freq := 20.0 + vel*120.0 // map velocity to "frequency"
	title := strings.Join([]string{
		accentBadge(accentPink).Render(" TRACE "),
		subtitleStyle().Render(fmt.Sprintf("%.0fHz  %.1fVpp", freq, vel*2.2)),
	}, " ")

	body := strings.Join([]string{
		title,
		"",
		b.String(),
	}, "\n")

	return renderPanel("SCOPE", body, accentGold, width, height)
}

func (m RunModel) renderDecayPanel(width, height int) string {
	activity := float64(len(m.decay.particles))
	epochMin := m.decay.epoch / 60.0
	cyclePos := math.Mod(m.decay.elapsed, m.decay.epoch) / m.decay.epoch * 100

	title := strings.Join([]string{
		accentBadge(accentRed).Render(" EVENTS "),
		subtitleStyle().Render(fmt.Sprintf("%.0f tracks  cycle %.0f%% of %.0fmin", activity, cyclePos, epochMin)),
	}, " ")

	body := strings.Join([]string{
		title,
		"",
		m.decay.render(),
	}, "\n")

	return renderPanel("DECAY CHAMBER", body, accentRed, width, height)
}

func (m RunModel) renderResonancePanel(width, height int) string {
	avgFreq := 0.0
	for _, s := range m.resonance.sources {
		avgFreq += s.freq
	}
	avgFreq /= float64(len(m.resonance.sources))
	epochMin := m.resonance.epoch / 60.0

	title := strings.Join([]string{
		accentBadge(accentLime).Render(" MODES "),
		subtitleStyle().Render(fmt.Sprintf("%d sources  ν̄=%.2f  epoch %.0fmin", len(m.resonance.sources), avgFreq, epochMin)),
	}, " ")

	body := strings.Join([]string{
		title,
		"",
		m.resonance.render(),
	}, "\n")

	return renderPanel("RESONANCE", body, accentLime, width, height)
}

func (m RunModel) renderFluxPanel(width, height int) string {
	activeCharges := 0
	for _, c := range m.flux.charges {
		fade := 0.5 + 0.5*math.Sin(c.fadePhase)
		if fade > 0.1 {
			activeCharges++
		}
	}
	epochMin := m.flux.epoch / 60.0

	title := strings.Join([]string{
		accentBadge(accentViolet).Render(" TOPOLOGY "),
		subtitleStyle().Render(fmt.Sprintf("%d charges  epoch %.0fmin", activeCharges, epochMin)),
	}, " ")

	body := strings.Join([]string{
		title,
		"",
		m.flux.render(),
	}, "\n")

	return renderPanel("FLUX", body, accentViolet, width, height)
}

func (m RunModel) renderDiagnosticsPanel(width, height int) string {
	lines := []string{}
	if strings.TrimSpace(m.stderr) != "" {
		lines = append(lines,
			accentBadge(accentRed).Render(" STDERR "),
			subtitleStyle().Render(m.stderr),
		)
	}
	if strings.TrimSpace(m.stdout) != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines,
			accentBadge(accentGold).Render(" STDOUT "),
			subtitleStyle().Render(m.stdout),
		)
	}
	if len(lines) == 0 {
		lines = []string{subtitleStyle().Render("fault fragments will appear here when a step overloads")}
	}
	return renderPanel("DIAGNOSTICS", strings.Join(lines, "\n"), accentOrange, width, height)
}

func (m RunModel) renderAuxPanel(width, height int) string {
	if m.auxPanel == "flux" {
		return m.renderFluxPanel(width, height)
	}
	return m.renderDiagnosticsPanel(width, height)
}

func (m RunModel) manualText() string {
	parts := []string{"q/esc/ctrl+c close gate", "tab flips aux panel"}
	if m.synth != nil {
		audioState := "mute"
		if m.audioMuted {
			audioState = "unmute"
		}
		parts = append(parts, "m "+audioState+"s audio")
	}
	return strings.Join(parts, "  ")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m RunModel) renderEventStream(width, height int) string {
	if len(m.events) == 0 {
		return renderPanel("SEQUENCE MEMORY", subtitleStyle().Render("awaiting first gate"), accentViolet, width, height)
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
	return renderPanel("SEQUENCE MEMORY", strings.Join(lines, "\n"), accentViolet, width, height)
}

func (m RunModel) frameCmd() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(t time.Time) tea.Msg {
		return frameMsg(t)
	})
}

func renderPanel(title, body string, accent lipgloss.Color, width, height int) string {
	style := neonPanel(accent)
	outerWidth := width - style.GetBorderLeftSize() - style.GetBorderRightSize()
	if outerWidth < 1 {
		outerWidth = 1
	}
	outerHeight := height - style.GetBorderTopSize() - style.GetBorderBottomSize()
	if outerHeight < 1 {
		outerHeight = 1
	}
	bodyWidth := width - style.GetHorizontalFrameSize()
	if bodyWidth < 1 {
		bodyWidth = 1
	}
	bodyHeight := height - style.GetVerticalFrameSize()
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	head := accentBadge(accent).Render(" " + title + " ")
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		head,
		"",
		body,
	)

	return style.
		Width(outerWidth).
		Height(outerHeight).
		Render(
			lipgloss.NewStyle().
				Width(bodyWidth).
				MaxHeight(bodyHeight).
				Render(content),
		)
}

func spacer(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(width).Render("")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
