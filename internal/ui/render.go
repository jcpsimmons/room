package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Check struct {
	Name    string
	OK      bool
	Message string
}

type InitSummary struct {
	RepoRoot       string
	RoomDir        string
	NextSteps      []string
	Notes          []string
	MissingIgnore  bool
	IgnoreAdvisory string
}

type StatusSummary struct {
	RepoRoot           string
	Provider           string
	Iteration          int
	LastRun            string
	LastStatus         string
	BundleHint         string
	RecoveryHint       string
	RoomIgnoreHint     string
	Dirty              bool
	CurrentInstruction string
	RecentCommits      []string
	RecentSummaries    []string
}

type DoctorSummary struct {
	RepoRoot string
	Checks   []Check
	Notes    []string
}

type RunSummary struct {
	RepoRoot            string
	Provider            string
	RequestedIterations int
	CompletedIterations int
	Failures            int
	LastStatus          string
	LastRunDir          string
	Timeline            []string
}

func RenderInit(summary InitSummary) string {
	left := framed("INIT", strings.Join([]string{
		kvLine("repo", summary.RepoRoot, accentCyan),
		kvLine("room", summary.RoomDir, accentPink),
	}, "\n"), accentPink)

	rightItems := append([]string{}, summary.NextSteps...)
	right := framed("NEXT STEPS", bulletLines(rightItems, accentLime), accentLime)

	blocks := []string{left, right}
	if summary.MissingIgnore {
		blocks = append(blocks, framed("IGNORE", summary.IgnoreAdvisory, accentGold))
	}
	if len(summary.Notes) > 0 {
		blocks = append(blocks, framed("NOTES", bulletLines(summary.Notes, accentOrange), accentOrange))
	}

	return lipglossJoinVertical(blocks...)
}

func RenderStatus(summary StatusSummary) string {
	headerLines := []string{
		rainbow("ROOM"),
		subtitleStyle().Render("repository orchestration status"),
		kvLine("repo", summary.RepoRoot, accentCyan),
		kvLine("provider", summary.Provider, accentPink),
		kvLine("iteration", fmt.Sprintf("%d", summary.Iteration), accentLime),
		kvLine("last run", summary.LastRun, accentGold),
		kvLine("dirty", fmt.Sprintf("%t", summary.Dirty), accentRed),
		kvLine("status", summary.LastStatus, accentViolet),
	}
	if strings.TrimSpace(summary.BundleHint) != "" {
		headerLines = append(headerLines, kvLine("bundle", summary.BundleHint, accentGold))
	}
	if strings.TrimSpace(summary.RecoveryHint) != "" {
		headerLines = append(headerLines, kvLine("recovery", summary.RecoveryHint, accentViolet))
	}
	if strings.TrimSpace(summary.RoomIgnoreHint) != "" {
		headerLines = append(headerLines, kvLine("ignore", summary.RoomIgnoreHint, accentGold))
	}
	header := framed("STATUS", strings.Join(headerLines, "\n"), accentCyan)

	instruction := framed("INSTRUCTION", summary.CurrentInstruction, accentPink)
	commits := framed("ROOM COMMITS", bulletLines(summary.RecentCommits, accentCyan), accentCyan)
	summaries := framed("SUMMARIES", bulletLines(summary.RecentSummaries, accentLime), accentLime)

	return lipglossJoinVertical(header, lipglossJoinHorizontal(instruction, commits), summaries)
}

func RenderDoctor(summary DoctorSummary) string {
	var rows []string
	for _, check := range summary.Checks {
		accent := accentLime
		label := "OK"
		if !check.OK {
			accent = accentRed
			label = "FAIL"
		}
		rows = append(rows, lipglossJoinHorizontal(
			accentBadge(accent).Render(" "+label+" "),
			titleStyle().Render(check.Name),
			subtitleStyle().Render(check.Message),
		))
	}

	body := bulletLines(summary.Notes, accentGold)
	if strings.TrimSpace(body) == "" {
		body = subtitleStyle().Render("all systems reporting in")
	}

	checkPanel := framed("CHECKS", strings.Join(rows, "\n"), accentCyan)
	notePanel := framed("NOTES", body, accentGold)

	header := framed("DOCTOR", strings.Join([]string{
		rainbow("ROOM"),
		subtitleStyle().Render("environment and repository verification"),
		kvLine("repo", summary.RepoRoot, accentPink),
	}, "\n"), accentPink)

	return lipglossJoinVertical(header, lipglossJoinHorizontal(checkPanel, notePanel))
}

func RenderRun(summary RunSummary) string {
	header := framed("RUN", strings.Join([]string{
		rainbow("ROOM"),
		subtitleStyle().Render("recursive loop report"),
		kvLine("repo", summary.RepoRoot, accentCyan),
		kvLine("provider", summary.Provider, accentPink),
		kvLine("requested", fmt.Sprintf("%d", summary.RequestedIterations), accentGold),
		kvLine("completed", fmt.Sprintf("%d", summary.CompletedIterations), accentLime),
		kvLine("failures", fmt.Sprintf("%d", summary.Failures), accentRed),
		kvLine("last run", summary.LastRunDir, accentViolet),
	}, "\n"), accentCyan)

	statusPanel := framed("OUTCOME", strings.Join([]string{
		statusBadge(summary.LastStatus),
		subtitleStyle().Render(summary.LastStatus),
	}, "\n"), accentPink)

	timeline := summary.Timeline
	if len(timeline) == 0 {
		timeline = []string{"no run output recorded"}
	}
	stream := framed("TIMELINE", bulletLines(timeline, accentLime), accentLime)

	return lipglossJoinVertical(header, lipglossJoinHorizontal(statusPanel, stream))
}

func lipglossJoinVertical(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, filtered...)
}

func lipglossJoinHorizontal(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, filtered...)
}
