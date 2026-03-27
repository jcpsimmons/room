package app

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/prompt"
)

func promptHistorySignal(currentInstruction string, priorInstructions []string, recentSummaries []logs.SummaryEntry, recentCommits []git.Commit) (string, string) {
	result := prompt.DetectStagnation(prompt.DedupeInput{
		NextInstruction:   strings.TrimSpace(currentInstruction),
		PriorInstructions: priorInstructions,
		RecentSummaries:   recentSummaries,
		RecentCommits:     recentCommits,
	})
	if !result.ShouldPivot {
		return "", ""
	}

	hint := fmt.Sprintf("Prompt history is stagnating: %s.", strings.Join(result.Reasons, "; "))
	if target := strings.TrimSpace(result.MatchedTarget); target != "" {
		hint += fmt.Sprintf(" Matched prior instruction %q.", target)
	}
	return hint, result.Replacement
}
