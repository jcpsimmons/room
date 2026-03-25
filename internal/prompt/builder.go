package prompt

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
)

type BuildInput struct {
	CurrentInstruction string
	RecentSummaries    []logs.SummaryEntry
	PriorInstructions  []string
	RecentCommits      []git.Commit
	GitStatus          string
	RepoPath           string
}

func Build(input BuildInput) string {
	var b strings.Builder
	b.WriteString("You are ROOM, a cold-start repository improvement loop.\n")
	b.WriteString("Operate on the current repository without relying on prior conversational state.\n\n")
	b.WriteString("Primary instruction:\n")
	b.WriteString(strings.TrimSpace(input.CurrentInstruction))
	b.WriteString("\n\n")
	b.WriteString("Hard constraints:\n")
	b.WriteString("- Make exactly one concrete improvement.\n")
	b.WriteString("- Apply the change directly instead of stopping at analysis.\n")
	b.WriteString("- Validate the change if practical.\n")
	b.WriteString("- Avoid cosmetic churn.\n")
	b.WriteString("- Do not ask follow-up questions.\n")
	b.WriteString("- Return only JSON that matches the supplied schema.\n\n")
	b.WriteString(fmt.Sprintf("Repository path: %s\n\n", input.RepoPath))

	b.WriteString("Recent summaries:\n")
	if len(input.RecentSummaries) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, summary := range input.RecentSummaries {
			b.WriteString(fmt.Sprintf("- #%d [%s] %s\n", summary.Iteration, summary.Status, summary.Summary))
		}
	}
	b.WriteString("\nPrior next-instructions:\n")
	if len(input.PriorInstructions) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, instruction := range input.PriorInstructions {
			b.WriteString("- ")
			b.WriteString(instruction)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nRecent commits:\n")
	if len(input.RecentCommits) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, commit := range input.RecentCommits {
			b.WriteString(fmt.Sprintf("- %s %s\n", commit.Hash, commit.Subject))
		}
	}
	b.WriteString("\nCurrent git status:\n")
	if strings.TrimSpace(input.GitStatus) == "" {
		b.WriteString("clean\n")
	} else {
		b.WriteString(input.GitStatus)
		b.WriteByte('\n')
	}

	b.WriteString("\nResponse contract:\n")
	b.WriteString("- summary: short description of the improvement you made\n")
	b.WriteString("- next_instruction: the next direction ROOM should try\n")
	b.WriteString("- status: continue, pivot, or done\n")
	b.WriteString("- commit_message: concise commit message body without the ROOM prefix if possible\n")

	return b.String()
}
