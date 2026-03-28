package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/version"
)

func TestTapeReportsRecentSequenceMemory(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	for _, tc := range []struct {
		iteration int
		status    string
		summary   string
		next      string
		commit    string
		focus     []string
		added     int
		deleted   int
		files     int
		when      time.Time
	}{
		{
			iteration: 1,
			status:    "continue",
			summary:   "Rebiased the oscillator bank",
			next:      "Listen for aliasing in the clock divider",
			commit:    "1111111111111111111111111111111111111111",
			focus:     []string{"ui", "audio"},
			added:     8,
			deleted:   2,
			files:     2,
			when:      time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC),
		},
		{
			iteration: 2,
			status:    "pivot",
			summary:   "Patched the clock divider drift",
			next:      "Route the next pass through repo history diagnostics",
			commit:    "2222222222222222222222222222222222222222",
			focus:     []string{"logs"},
			added:     5,
			deleted:   1,
			files:     1,
			when:      time.Date(2026, 3, 25, 12, 2, 0, 0, time.UTC),
		},
		{
			iteration: 3,
			status:    "continue",
			summary:   "Recorded sequence memory as operator tape",
			next:      "Push the doctor path toward install diagnostics",
			commit:    "3333333333333333333333333333333333333333",
			focus:     []string{"cli", "logs"},
			added:     13,
			deleted:   4,
			files:     3,
			when:      time.Date(2026, 3, 25, 12, 3, 0, 0, time.UTC),
		},
	} {
		if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, tc.next); err != nil {
			t.Fatalf("append seen instruction: %v", err)
		}
		if err := logs.AppendSummary(paths.SummariesPath, logs.SummaryEntry{
			Iteration:    tc.iteration,
			Timestamp:    tc.when,
			Status:       tc.status,
			Summary:      tc.summary,
			CommitHash:   tc.commit,
			ChangedFiles: tc.files,
			LinesAdded:   tc.added,
			LinesDeleted: tc.deleted,
			FocusAreas:   tc.focus,
		}); err != nil {
			t.Fatalf("append summary: %v", err)
		}
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Tape(context.Background(), TapeOptions{WorkingDir: repoRoot, Limit: 2})
	if err != nil {
		t.Fatalf("tape: %v", err)
	}

	if report.Limit != 2 {
		t.Fatalf("limit = %d", report.Limit)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("entry count = %d", len(report.Entries))
	}
	if report.Entries[0].Iteration != 2 || report.Entries[1].Iteration != 3 {
		t.Fatalf("iterations = %#v", report.Entries)
	}
	if report.Entries[0].NextInstruction != "Route the next pass through repo history diagnostics" {
		t.Fatalf("entry[0] next instruction = %q", report.Entries[0].NextInstruction)
	}
	if report.Entries[1].CommitHash != "3333333333333333333333333333333333333333" {
		t.Fatalf("entry[1] commit hash = %q", report.Entries[1].CommitHash)
	}
	if got := strings.Join(report.Entries[1].FocusAreas, ","); got != "cli,logs" {
		t.Fatalf("entry[1] focus = %q", got)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"ROOM tape",
		"Entries: 2 (limit 2)",
		"#2 2026-03-25T12:02:00Z [pivot] Patched the clock divider drift",
		"next: Route the next pass through repo history diagnostics",
		"commit: 333333333333",
		"focus: cli, logs",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tape output missing %q:\n%s", want, joined)
		}
	}
}

func TestTapeFlagsInstructionDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := logs.AppendSummary(paths.SummariesPath, logs.SummaryEntry{
		Iteration:    1,
		Timestamp:    time.Date(2026, 3, 25, 12, 1, 0, 0, time.UTC),
		Status:       "continue",
		Summary:      "Recovered a missing instruction pulse",
		ChangedFiles: 1,
		LinesAdded:   3,
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Patch the install script next"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}
	if err := logs.AppendSummary(paths.SummariesPath, logs.SummaryEntry{
		Iteration:    2,
		Timestamp:    time.Date(2026, 3, 25, 12, 2, 0, 0, time.UTC),
		Status:       "pivot",
		Summary:      "Split the tape view out of raw logs",
		ChangedFiles: 2,
		LinesAdded:   7,
		LinesDeleted: 1,
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Tape(context.Background(), TapeOptions{WorkingDir: repoRoot, Limit: 4})
	if err != nil {
		t.Fatalf("tape: %v", err)
	}

	if report.MissingNextInstructions != 1 {
		t.Fatalf("missing next instructions = %d", report.MissingNextInstructions)
	}
	if report.Entries[0].NextInstruction != "" {
		t.Fatalf("entry[0] next instruction = %q", report.Entries[0].NextInstruction)
	}
	if report.Entries[1].NextInstruction != "Patch the install script next" {
		t.Fatalf("entry[1] next instruction = %q", report.Entries[1].NextInstruction)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "Instruction drift: 1 tape step(s) are missing captured next-instruction data.") {
		t.Fatalf("expected instruction drift note:\n%s", strings.Join(report.Lines, "\n"))
	}
}
