package ui

import (
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

func accentBadge(fg, bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(fg).
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
		accentBadge(textColor, accent).Render(" "+label+" "),
		" ",
		subtitleStyle().Render(value),
	)
}

func framed(title, body string, accent lipgloss.Color) string {
	head := lipgloss.JoinHorizontal(
		lipgloss.Top,
		accentBadge(textColor, accent).Render(" "+title+" "),
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
		return accentBadge(bgColor, accentLime).Render(" DONE ")
	case "dry_run":
		return accentBadge(bgColor, accentGold).Render(" DRY ")
	case "pivot":
		return accentBadge(bgColor, accentGold).Render(" PIVOT ")
	case "failed", "failure":
		return accentBadge(bgColor, accentRed).Render(" FAIL ")
	case "running":
		return accentBadge(bgColor, accentCyan).Render(" RUN ")
	default:
		return accentBadge(bgColor, accentViolet).Render(" " + strings.ToUpper(kind) + " ")
	}
}
