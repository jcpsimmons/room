package prompt

import (
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
)

func TestDetectStagnationOnDuplicateInstruction(t *testing.T) {
	t.Parallel()

	result := DetectStagnation(DedupeInput{
		NextInstruction:   "Improve parser resilience",
		PriorInstructions: []string{"Improve parser resilience"},
	})

	if !result.ShouldPivot {
		t.Fatalf("expected forced pivot")
	}
	if result.Reasons[0] != "exact duplicate next instruction" {
		t.Fatalf("reason = %q", result.Reasons[0])
	}
	if result.Replacement == "" {
		t.Fatalf("expected replacement instruction")
	}
}

func TestDetectStagnationOnChurnAndTinyDiffs(t *testing.T) {
	t.Parallel()

	result := DetectStagnation(DedupeInput{
		NextInstruction:      "Refactor docs comments in the parser package",
		RecentSummaries:      []logs.SummaryEntry{{Summary: "Refactor docs around parser comments"}, {Summary: "Refresh documentation in parser tests"}},
		ConsecutiveTinyDiffs: 2,
	})

	if !result.ShouldPivot {
		t.Fatalf("expected forced pivot")
	}
}

func TestDetectStagnationOnRepeatedSubsystemFocusAcrossHistory(t *testing.T) {
	t.Parallel()

	result := DetectStagnation(DedupeInput{
		NextInstruction:   "Strengthen config bundle recovery wiring",
		PriorInstructions: []string{"Harden config bundle validation", "Route config bundle drift through doctor"},
		RecentSummaries: []logs.SummaryEntry{
			{Summary: "Guard config bundle collisions before execution"},
		},
		RecentCommits: []git.Commit{
			{Subject: "room: surface config bundle drift in status"},
		},
	})

	if !result.ShouldPivot {
		t.Fatalf("expected forced pivot")
	}
	if got := result.Reasons[0]; got != "repeated subsystem focus across recent runs" {
		t.Fatalf("reason = %q", got)
	}
	for _, want := range []string{
		"Avoid these recently saturated modules:",
		"config",
		"bundle",
	} {
		if !strings.Contains(result.Replacement, want) {
			t.Fatalf("replacement missing %q: %q", want, result.Replacement)
		}
	}
}

func TestDetectStagnationOnRepeatedFocusAreasFromChangedFiles(t *testing.T) {
	t.Parallel()

	result := DetectStagnation(DedupeInput{
		NextInstruction:   "Patch the ui transport and ui diagnostics path",
		PriorInstructions: []string{"Leave the app layer alone and push into ui experiments"},
		RecentSummaries: []logs.SummaryEntry{
			{Summary: "Shifted the viewport signal", FocusAreas: []string{"ui"}},
			{Summary: "Retuned the panel renderer", FocusAreas: []string{"ui"}},
		},
	})

	if !result.ShouldPivot {
		t.Fatalf("expected forced pivot")
	}
	if got := result.Reasons[0]; got != "repeated subsystem focus across recent runs" {
		t.Fatalf("reason = %q", got)
	}
	if !strings.Contains(result.Replacement, "ui") {
		t.Fatalf("replacement missing ui avoid-list: %q", result.Replacement)
	}
}
