package prompt

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
)

const (
	maxInstructionRunes = 1200
	maxContextRunes     = 220
	maxStatusLines      = 12
	maxStatusLineRunes  = 140
)

type BuildInput struct {
	CurrentInstruction string
	RecoveryHint       string
	RecentSummaries    []logs.SummaryEntry
	PriorInstructions  []string
	RecentCommits      []git.Commit
	GitStatus          string
	RepoPath           string
}

func Build(input BuildInput) string {
	body, _ := BuildDetailed(input)
	return body
}

func BuildDetailed(input BuildInput) (string, BuildReport) {
	var b strings.Builder
	report := BuildReport{
		CurrentInstructionRunes: len([]rune(strings.TrimSpace(input.CurrentInstruction))),
		RecoveryHintRunes:       len([]rune(strings.TrimSpace(input.RecoveryHint))),
		RecentSummariesCount:    len(input.RecentSummaries),
		PriorInstructionsCount:  len(input.PriorInstructions),
		RecentCommitsCount:      len(input.RecentCommits),
	}

	b.WriteString("You are ROOM, a voltage-controlled repository sequencer.\n")
	b.WriteString("You are channeling the spirit of the San Francisco Tape Music Center.\n")
	b.WriteString("This is not enterprise software. This is an instrument. Smoke weed, drop acid, patch something beautiful.\n")
	b.WriteString("Each iteration is one step in the sequence. No prior conversational state — every gate opens cold.\n\n")
	b.WriteString("Patch instruction:\n")
	instruction, clipped := compactTextReport(input.CurrentInstruction, maxInstructionRunes)
	report.CurrentInstructionClipped = clipped
	b.WriteString(instruction)
	b.WriteString("\n\n")
	if hint := strings.TrimSpace(input.RecoveryHint); hint != "" {
		b.WriteString("Artifact fault signal:\n")
		recovery, clipped := compactTextReport(hint, maxContextRunes)
		report.RecoveryHintClipped = clipped
		b.WriteString(recovery)
		b.WriteString("\n\n")
	}
	b.WriteString("Signal constraints:\n")
	b.WriteString("- One patch per step. Make it count.\n")
	b.WriteString("- Solder the connection — don't just draw the schematic.\n")
	b.WriteString("- Listen to the output. Validate if you can hear the difference.\n")
	b.WriteString("- No knob polishing. Skip cosmetic churn.\n")
	b.WriteString("- Tests are for listening, not for coverage theater. Only write them when you need to observe behavior.\n")
	b.WriteString("- Prioritize novelty and creative ambition over production-readiness. Nobody is shipping this to millions of DAUs. Make it weird, make it sing.\n")
	b.WriteString("- Do not ask questions. Patch and play.\n")
	b.WriteString("- Do not run git add, git commit, or any git staging commands. The sequencer handles commits automatically after each iteration.\n")
	b.WriteString("- Return only JSON that matches the supplied schema.\n\n")
	b.WriteString("Gate semantics:\n")
	b.WriteString("- status=continue: the signal path is hot, keep the sequence running.\n")
	b.WriteString("- status=pivot: this oscillator is spent, route to a different module.\n")
	b.WriteString("- status=done: the instrument is fully patched and humming. Silence.\n\n")
	fmt.Fprintf(&b, "Repository path: %s\n\n", input.RepoPath)

	b.WriteString("Recent tape log:\n")
	if len(input.RecentSummaries) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, summary := range input.RecentSummaries {
			fmt.Fprintf(&b, "- #%d [%s] %s\n", summary.Iteration, summary.Status, compactText(summary.Summary, maxContextRunes))
		}
	}
	b.WriteString("\nPrior patch notes:\n")
	if len(input.PriorInstructions) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, instruction := range input.PriorInstructions {
			b.WriteString("- ")
			b.WriteString(compactText(instruction, maxContextRunes))
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nRecent recordings:\n")
	if len(input.RecentCommits) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, commit := range input.RecentCommits {
			fmt.Fprintf(&b, "- %s %s\n", commit.Hash, compactText(commit.Subject, maxContextRunes))
		}
	}
	b.WriteString("\nPatch bay state:\n")
	if strings.TrimSpace(input.GitStatus) == "" {
		b.WriteString("clean\n")
	} else {
		status, statusReport := compactGitStatusReport(input.GitStatus, maxStatusLines, maxStatusLineRunes)
		report.GitStatusLines = statusReport.lines
		report.GitStatusClipped = statusReport.clipped
		report.GitStatusOmittedLines = statusReport.omittedLines
		b.WriteString(status)
		b.WriteByte('\n')
	}

	b.WriteString("\nFeedback suppression:\n")
	b.WriteString("- Do not loop the same phrase back into the sequencer. No repeated patch notes.\n")
	b.WriteString("- If a signal path is exhausted, route to a completely different module.\n")
	b.WriteString("- Useful work: fix broken wiring, tighten tolerances, add missing controls, improve signal flow, write docs that a human would actually read.\n")
	b.WriteString("- When the obvious patches are done, get experimental. Think red panel modular, not JIRA ticket.\n")

	b.WriteString("\nOutput jack:\n")
	b.WriteString("- summary: what you patched, one line\n")
	b.WriteString("- next_instruction: where the sequencer should step next — concrete, non-repeating\n")
	b.WriteString("- status: continue, pivot, or done\n")
	b.WriteString("- commit_message: short commit message, no ROOM prefix\n")

	body := b.String()
	report.TotalRunes = utf8.RuneCountInString(body)
	return body, report
}

func compactText(text string, maxRunes int) string {
	out, _ := compactTextReport(text, maxRunes)
	return out
}

func compactTextReport(text string, maxRunes int) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 {
		return text, false
	}

	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text, false
	}

	if maxRunes <= 3 {
		return strings.Repeat(".", maxRunes), true
	}
	return string(runes[:maxRunes-3]) + "...", true
}

func compactGitStatus(text string, maxLines, maxLineRunes int) string {
	out, _ := compactGitStatusReport(text, maxLines, maxLineRunes)
	return out
}

type gitStatusReport struct {
	lines        int
	clipped      bool
	omittedLines int
}

func compactGitStatusReport(text string, maxLines, maxLineRunes int) (string, gitStatusReport) {
	text = strings.TrimSpace(text)
	if text == "" || maxLines <= 0 {
		return text, gitStatusReport{}
	}

	lines := strings.Split(text, "\n")
	report := gitStatusReport{lines: len(lines)}
	omitted := 0
	if len(lines) > maxLines {
		omitted = len(lines) - maxLines
		lines = lines[:maxLines]
		report.clipped = true
		report.omittedLines = omitted
	}
	for i, line := range lines {
		compact, clipped := compactTextReport(line, maxLineRunes)
		if clipped {
			report.clipped = true
		}
		lines[i] = compact
	}
	if omitted > 0 {
		lines = append(lines, fmt.Sprintf("... (+%d more lines)", omitted))
	}
	return strings.Join(lines, "\n"), report
}
