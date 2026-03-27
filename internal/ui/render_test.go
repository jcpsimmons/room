package ui

import (
	"strings"
	"testing"
)

func TestAsProgress(t *testing.T) {
	ev := ProgressEvent{Kind: ProgressStart, Title: "boot"}

	got, ok := AsProgress(ev)
	if !ok {
		t.Fatalf("expected progress event")
	}
	if got.Kind != ProgressStart || got.Title != "boot" {
		t.Fatalf("unexpected event: %#v", got)
	}

	got, ok = AsProgress(ProgressMsg{Event: ev})
	if !ok {
		t.Fatalf("expected progress msg")
	}
	if got.Kind != ProgressStart {
		t.Fatalf("unexpected msg event: %#v", got)
	}
}

func TestRenderersReturnText(t *testing.T) {
	initOut := RenderInit(InitSummary{
		RepoRoot:       "/tmp/repo",
		RoomDir:        "/tmp/repo/.room",
		NextSteps:      []string{"room doctor", "room run --iterations 5"},
		MissingIgnore:  true,
		IgnoreAdvisory: "add .room/ to .gitignore if you want git status to stay clean",
	})
	if initOut == "" {
		t.Fatal("expected init render")
	}

	statusOut := RenderStatus(StatusSummary{
		RepoRoot:           "/tmp/repo",
		Provider:           "codex",
		Iteration:          12,
		LastRun:            "2026-03-25T12:00:00Z",
		LastStatus:         "continue",
		BundleHint:         "Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
		RecoveryHint:       "Hint: reclaimed stale run lock from pid 4242 started 2026-03-25T11:00:00Z.",
		Dirty:              false,
		CurrentInstruction: "make the UI feel alive",
		RecentCommits:      []string{"abc123 add UI polish"},
		RecentSummaries:    []string{"#12 continue make the UI feel alive"},
	})
	if statusOut == "" {
		t.Fatal("expected status render")
	}
	if !strings.Contains(statusOut, "newest bundle 0002") {
		t.Fatalf("expected bundle hint to render, got:\n%s", statusOut)
	}
	if !strings.Contains(statusOut, "reclaimed stale run lock") {
		t.Fatalf("expected stale-lock recovery to render, got:\n%s", statusOut)
	}

	doctorOut := RenderDoctor(DoctorSummary{
		RepoRoot: "/tmp/repo",
		Checks: []Check{
			{Name: "git", OK: true, Message: "available"},
			{Name: "provider", OK: false, Message: "missing"},
		},
	})
	if doctorOut == "" {
		t.Fatal("expected doctor render")
	}

	runOut := RenderRun(RunSummary{
		RepoRoot:            "/tmp/repo",
		Provider:            "codex",
		RequestedIterations: 5,
		CompletedIterations: 3,
		Failures:            1,
		LastStatus:          "continue",
		LastRunDir:          "/tmp/repo/.room/runs/0003",
		Timeline: []string{
			"ROOM run in /tmp/repo",
			"Iteration 3 [continue]: pushed the UI harder",
		},
	})
	if runOut == "" {
		t.Fatal("expected run render")
	}
}
