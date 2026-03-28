package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jcpsimmons/room/internal/audio"
)

type fluxNode struct {
	label string
	x, y  int
	color lipgloss.Color
}

type fluxPulse struct {
	from, to int
	progress float64
	speed    float64
	color    lipgloss.Color
	glyph    string
}

type fluxFault struct {
	node  int
	age   float64
	ttl   float64
	color lipgloss.Color
	glyph string
}

type fluxCell struct {
	glyph string
	color lipgloss.Color
}

type fluxTopology struct {
	nodes           []fluxNode
	pulses          []fluxPulse
	faults          []fluxFault
	gridW           int
	gridH           int
	elapsed         float64
	lastPhase       string
	lastStatus      string
	lastIteration   int
	lastLatencyMS   int64
	lastRuntimeMS   int64
	lastRunMS       int64
	eventCount      int
	pulseCount      int
	failureCount    int
	pivotCount      int
	completionCount int
}

func newFluxTopology(w, h int) fluxTopology {
	if w < 14 {
		w = 14
	}
	if h < 6 {
		h = 6
	}
	x := func(ratio float64) int {
		return clampInt(int(math.Round(ratio*float64(w-1))), 0, w-1)
	}
	y := func(ratio float64) int {
		return clampInt(int(math.Round(ratio*float64(h-1))), 0, h-1)
	}
	return fluxTopology{
		gridW: w,
		gridH: h,
		nodes: []fluxNode{
			{label: "IN", x: x(0.05), y: y(0.5), color: accentCyan},
			{label: "PROMPT", x: x(0.28), y: y(0.1), color: accentPink},
			{label: "AGENT", x: x(0.5), y: y(0.5), color: accentGold},
			{label: "VERIFY", x: x(0.72), y: y(0.9), color: accentLime},
			{label: "TAPE", x: x(0.95), y: y(0.5), color: accentViolet},
		},
	}
}

func (ft *fluxTopology) ingest(ev ProgressEvent) {
	ft.eventCount++
	ft.lastIteration = ev.Iteration
	ft.lastStatus = string(ev.Kind)
	if status := metaString(ev.Meta, "status"); status != "" {
		ft.lastStatus = status
	}
	ft.lastPhase = metaString(ev.Meta, "phase")
	if value := metaInt64(ev.Meta, "phase_latency_ms"); value > 0 {
		ft.lastLatencyMS = value
	}
	if value := metaInt64(ev.Meta, "execution_elapsed_ms"); value > 0 {
		ft.lastRuntimeMS = value
	}
	if value := metaInt64(ev.Meta, "run_elapsed_ms"); value > 0 {
		ft.lastRunMS = value
	}

	switch ev.Kind {
	case ProgressFailure:
		ft.failureCount++
	case ProgressPivot:
		ft.pivotCount++
	case ProgressDone:
		ft.completionCount++
	}

	ft.spawnForEvent(ev)
}

func (ft *fluxTopology) spawnForEvent(ev ProgressEvent) {
	switch metaString(ev.Meta, "phase") {
	case "run_start":
		ft.addPulse(0, 1, accentCyan, "•", 0.8)
	case "iteration_start":
		ft.addPulse(1, 2, accentPink, "•", 1.0)
	case "agent_execution_start":
		ft.addPulse(2, 3, accentGold, "◦", 1.0)
	case "agent_execution_pulse":
		ft.pulseCount++
		glyph := "≈"
		color := accentGold
		if ft.lastRuntimeMS >= 15000 {
			glyph = "≋"
			color = accentOrange
		}
		ft.addPulse(2, 2, color, glyph, 0.55)
	case "iteration_success":
		ft.addPulse(3, 4, accentLime, "•", 1.2)
		switch ev.Kind {
		case ProgressPivot:
			ft.addFault(4, accentViolet, "⟲", 3.2)
		case ProgressDone:
			ft.addFault(4, accentLime, "◎", 3.6)
		}
	case "iteration_failure":
		ft.addPulse(2, 3, accentRed, "×", 0.85)
		ft.addFault(3, accentRed, "!", 4.0)
	case "run_finish":
		if ev.Kind == ProgressFailure {
			ft.addFault(4, accentRed, "!", 3.5)
			return
		}
		ft.addPulse(4, 0, accentViolet, "◦", 0.7)
	}
}

func (ft *fluxTopology) addPulse(from, to int, color lipgloss.Color, glyph string, speed float64) {
	if from < 0 || from >= len(ft.nodes) || to < 0 || to >= len(ft.nodes) {
		return
	}
	ft.pulses = append(ft.pulses, fluxPulse{
		from:     from,
		to:       to,
		progress: 0,
		speed:    speed,
		color:    color,
		glyph:    glyph,
	})
}

func (ft *fluxTopology) addFault(node int, color lipgloss.Color, glyph string, ttl float64) {
	if node < 0 || node >= len(ft.nodes) {
		return
	}
	ft.faults = append(ft.faults, fluxFault{
		node:  node,
		ttl:   ttl,
		color: color,
		glyph: glyph,
	})
}

func (ft *fluxTopology) step(dt float64) {
	ft.elapsed += dt

	alivePulses := ft.pulses[:0]
	for _, pulse := range ft.pulses {
		pulse.progress += dt * pulse.speed
		if pulse.progress < 1.05 {
			alivePulses = append(alivePulses, pulse)
		}
	}
	ft.pulses = alivePulses

	aliveFaults := ft.faults[:0]
	for _, fault := range ft.faults {
		fault.age += dt
		if fault.age < fault.ttl {
			aliveFaults = append(aliveFaults, fault)
		}
	}
	ft.faults = aliveFaults
}

func (ft *fluxTopology) render() string {
	grid := make([][]fluxCell, ft.gridH)
	for y := range grid {
		grid[y] = make([]fluxCell, ft.gridW)
	}

	for i := 0; i < len(ft.nodes)-1; i++ {
		ft.plotLine(grid, ft.nodes[i].x, ft.nodes[i].y, ft.nodes[i+1].x, ft.nodes[i+1].y, "·", accentViolet)
	}
	ft.plotLine(grid, ft.nodes[len(ft.nodes)-1].x, ft.nodes[len(ft.nodes)-1].y, ft.nodes[0].x, ft.nodes[0].y, "·", accentRed)

	for _, pulse := range ft.pulses {
		from := ft.nodes[pulse.from]
		to := ft.nodes[pulse.to]
		x := lerpInt(from.x, to.x, pulse.progress)
		y := lerpInt(from.y, to.y, pulse.progress)
		grid[y][x] = fluxCell{glyph: pulse.glyph, color: pulse.color}
	}

	for _, fault := range ft.faults {
		node := ft.nodes[fault.node]
		y := node.y - 1
		if y < 0 {
			y = node.y + 1
		}
		glyph := fault.glyph
		if fault.ttl-fault.age < 1 && glyph == "!" {
			glyph = "·"
		}
		grid[y][node.x] = fluxCell{glyph: glyph, color: fault.color}
	}

	for _, node := range ft.nodes {
		grid[node.y][node.x] = fluxCell{glyph: "◉", color: node.color}
	}

	var b strings.Builder
	for y := 0; y < ft.gridH; y++ {
		for x := 0; x < ft.gridW; x++ {
			c := grid[y][x]
			if c.glyph == "" {
				b.WriteString(subtitleStyle().Render(" "))
				continue
			}
			b.WriteString(bulletStyle(c.color).Render(c.glyph))
		}
		if y < ft.gridH-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (ft *fluxTopology) summaryLines() []string {
	latency := formatFluxDuration(ft.lastLatencyMS)
	runtime := formatFluxDuration(ft.lastRuntimeMS)
	runElapsed := formatFluxDuration(ft.lastRunMS)
	phase := strings.ReplaceAll(ft.lastPhase, "_", " ")
	if phase == "" {
		phase = "idle"
	}
	status := ft.lastStatus
	if status == "" {
		status = "listening"
	}

	lineA := fmt.Sprintf("phase %s  iter %d  status %s", phase, max(ft.lastIteration, 0), status)
	lineB := fmt.Sprintf("phase-lat %s  carrier %s  run %s", latency, runtime, runElapsed)
	lineC := fmt.Sprintf("events %d  pulses %d  pivots %d  faults %d  dones %d", ft.eventCount, ft.pulseCount, ft.pivotCount, ft.failureCount, ft.completionCount)
	return []string{lineA, lineB, lineC}
}

func (ft *fluxTopology) audioParams() audio.Params {
	density := float64(len(ft.pulses)+len(ft.faults)) / 8.0
	if density > 1 {
		density = 1
	}
	failureBias := math.Min(float64(ft.failureCount)*0.08, 0.35)
	pivotBias := math.Min(float64(ft.pivotCount)*0.05, 0.2)
	freq := 110.0 + float64(ft.eventCount%9)*18.0 + density*90.0
	modDepth := 0.5 + density*2.5 + failureBias*2.0
	amp := math.Min(0.06+density*0.14+pivotBias*0.06, 0.22)
	return audio.Params{
		Freq:     freq,
		Amp:      amp,
		ModFreq:  freq * (1.2 + failureBias),
		ModDepth: modDepth,
		Detune:   0.12 + pivotBias + failureBias,
	}
}

func (ft *fluxTopology) plotLine(grid [][]fluxCell, x0, y0, x1, y1 int, glyph string, color lipgloss.Color) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if y0 >= 0 && y0 < len(grid) && x0 >= 0 && x0 < len(grid[y0]) && grid[y0][x0].glyph == "" {
			grid[y0][x0] = fluxCell{glyph: glyph, color: color}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func formatFluxDuration(ms int64) string {
	if ms <= 0 {
		return "0ms"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func metaInt64(meta map[string]any, key string) int64 {
	if meta == nil {
		return 0
	}
	switch value := meta[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func lerpInt(a, b int, t float64) int {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return clampInt(int(math.Round(float64(a)+(float64(b-a)*t))), 0, max(a, b))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
