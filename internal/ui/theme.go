package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	bgColor      = lipgloss.Color("#090B16")
	panelColor   = lipgloss.Color("#11182B")
	panelSoft    = lipgloss.Color("#16213A")
	textColor    = lipgloss.Color("#F5F7FF")
	dimColor     = lipgloss.Color("#93A4D7")
	accentCyan   = lipgloss.Color("#35F2FF")
	accentPink   = lipgloss.Color("#FF4FD8")
	accentLime   = lipgloss.Color("#7CFF6B")
	accentGold   = lipgloss.Color("#FFD84D")
	accentOrange = lipgloss.Color("#FF9A3D")
	accentRed    = lipgloss.Color("#FF5F7A")
	accentViolet = lipgloss.Color("#AA7BFF")
)

func basePanel() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(textColor).
		Background(bgColor).
		Padding(0, 1)
}

func neonPanel(accent lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(textColor).
		Background(panelColor).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(accent).
		Padding(0, 1).
		MarginTop(0)
}

func softPanel() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(textColor).
		Background(panelSoft).
		Padding(0, 1)
}

func titleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(textColor).
		Bold(true)
}

func subtitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(dimColor)
}

func accentBadge(bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(badgeForeground(bg)).
		Background(bg).
		Bold(true).
		Padding(0, 1)
}

func bulletStyle(color lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(color).
		Bold(true)
}

func rainbow(text string) string {
	colors := []lipgloss.Color{accentCyan, accentPink, accentLime, accentGold, accentOrange, accentViolet}
	var b strings.Builder
	for i, r := range text {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i%len(colors)]).Bold(true).Render(string(r)))
	}
	return b.String()
}

func wrapLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func bulletLines(items []string, color lipgloss.Color) string {
	if len(items) == 0 {
		return subtitleStyle().Render("none")
	}
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(bulletStyle(color).Render("• "))
		b.WriteString(item)
	}
	return b.String()
}

func kvLine(label, value string, accent lipgloss.Color) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		accentBadge(accent).Render(" "+label+" "),
		" ",
		subtitleStyle().Render(value),
	)
}

func framed(title, body string, accent lipgloss.Color) string {
	head := lipgloss.JoinHorizontal(
		lipgloss.Top,
		accentBadge(accent).Render(" "+title+" "),
	)
	return neonPanel(accent).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		head,
		"",
		body,
	))
}

func statusBadge(kind string) string {
	switch kind {
	case "done", "success":
		return accentBadge(accentLime).Render(" DONE ")
	case "dry_run":
		return accentBadge(accentGold).Render(" DRY ")
	case "pivot":
		return accentBadge(accentGold).Render(" PIVOT ")
	case "failed", "failure":
		return accentBadge(accentRed).Render(" FAIL ")
	case "running":
		return accentBadge(accentCyan).Render(" RUN ")
	default:
		return accentBadge(accentViolet).Render(" " + strings.ToUpper(kind) + " ")
	}
}

func badgeForeground(bg lipgloss.Color) lipgloss.Color {
	if contrastRatio(textColor, bg) >= contrastRatio(bgColor, bg) {
		return textColor
	}
	return bgColor
}

func contrastRatio(a, b lipgloss.Color) float64 {
	ar, ag, ab, ok := parseHexColor(a)
	if !ok {
		return 1
	}
	br, bg, bb, ok := parseHexColor(b)
	if !ok {
		return 1
	}

	al := relativeLuminance(ar, ag, ab)
	bl := relativeLuminance(br, bg, bb)
	if al < bl {
		al, bl = bl, al
	}
	return (al + 0.05) / (bl + 0.05)
}

func parseHexColor(color lipgloss.Color) (int, int, int, bool) {
	raw := strings.TrimSpace(string(color))
	if len(raw) != 7 || raw[0] != '#' {
		return 0, 0, 0, false
	}

	parse := func(value string) (int, bool) {
		n, err := strconv.ParseUint(value, 16, 8)
		if err != nil {
			return 0, false
		}
		return int(n), true
	}

	r, ok := parse(raw[1:3])
	if !ok {
		return 0, 0, 0, false
	}
	g, ok := parse(raw[3:5])
	if !ok {
		return 0, 0, 0, false
	}
	b, ok := parse(raw[5:7])
	if !ok {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

func relativeLuminance(r, g, b int) float64 {
	channel := func(value int) float64 {
		srgb := float64(value) / 255.0
		if srgb <= 0.04045 {
			return srgb / 12.92
		}
		return math.Pow((srgb+0.055)/1.055, 2.4)
	}

	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}
