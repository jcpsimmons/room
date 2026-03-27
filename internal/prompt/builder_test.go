package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
)

func TestBuildIncludesRelevantContext(t *testing.T) {
	t.Parallel()

	body := Build(BuildInput{
		CurrentInstruction: "Harden the config loader",
		RecoveryHint:       "Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
		RecentSummaries: []logs.SummaryEntry{{
			Iteration: 3,
			Timestamp: time.Now(),
			Status:    "continue",
			Summary:   "Added config parsing tests",
		}},
		PriorInstructions: []string{"Improve config validation"},
		RecentCommits: []git.Commit{{
			Hash:    "abc123",
			Subject: "room: tighten config errors",
		}},
		GitStatus: " M internal/config/config.go",
		RepoPath:  "/tmp/repo",
	})

	for _, want := range []string{
		"Harden the config loader",
		"Added config parsing tests",
		"Improve config validation",
		"room: tighten config errors",
		"Artifact fault signal",
		"missing result.json and diff.patch",
		"M internal/config/config.go",
		"No repeated patch notes",
		"fully patched and humming",
		"Return only JSON",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}
