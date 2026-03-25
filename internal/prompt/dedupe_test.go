package prompt

import (
	"testing"

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
