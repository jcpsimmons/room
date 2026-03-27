package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/state"
	"github.com/jcpsimmons/room/internal/version"
)

func TestStatusHintsAtIncompleteNewestBundle(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "older prompt", &agent.Result{
		Summary:         "Closed the loop",
		NextInstruction: "Listen for drift",
		Status:          "continue",
		CommitMessage:   "close the loop",
	}, strings.TrimSpace(`
diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1,2 @@
-old
+old
+new
`))
	writeTailBundle(t, paths.RunsDir, "0002", "newest prompt", nil, "")

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.LatestRunDir != filepath.Join(paths.RunsDir, "0002") {
		t.Fatalf("latest run dir = %q", report.LatestRunDir)
	}
	if !strings.Contains(report.LatestBundleHint, "missing result.json and diff.patch") {
		t.Fatalf("latest bundle hint = %q", report.LatestBundleHint)
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, report.LatestBundleHint) {
		t.Fatalf("status lines missing hint:\n%s", joined)
	}
}

func TestStatusSkipsHintForCompleteNewestBundle(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "complete prompt", &agent.Result{
		Summary:         "Closed the loop",
		NextInstruction: "Listen for drift",
		Status:          "continue",
		CommitMessage:   "close the loop",
	}, strings.TrimSpace(`
diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1,2 @@
-old
+old
+new
`))

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.LatestBundleHint != "" {
		t.Fatalf("expected no recovery hint, got %q", report.LatestBundleHint)
	}
	for _, line := range report.Lines {
		if strings.Contains(line, "Hint: newest bundle") {
			t.Fatalf("unexpected recovery hint in status lines:\n%s", strings.Join(report.Lines, "\n"))
		}
	}
}

func TestStatusSurvivesMissingInstructionFile(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.Remove(paths.InstructionPath); err != nil {
		t.Fatalf("remove instruction: %v", err)
	}
	writeTailBundle(t, paths.RunsDir, "0001", "newest prompt", nil, "")

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.CurrentInstruction != "Current instruction unavailable: missing instruction.txt." {
		t.Fatalf("current instruction = %q", report.CurrentInstruction)
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Current instruction unavailable: missing instruction.txt.",
		"Current instruction:",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("status lines missing %q:\n%s", want, joined)
		}
	}
}

func TestStatusSurfacesPromptHistoryStagnation(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.InstructionPath, []byte("Improve parser resilience\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Improve parser resilience"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}
	if err := logs.AppendSummary(paths.SummariesPath, logs.SummaryEntry{
		Iteration: 1,
		Timestamp: time.Date(2026, 3, 25, 12, 30, 0, 0, time.UTC),
		Status:    "continue",
		Summary:   "Kept circling the parser",
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.PromptHistoryHint == "" {
		t.Fatal("expected prompt history hint")
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Prompt history is stagnating: exact duplicate next instruction.",
		"Matched prior instruction \"Improve parser resilience\".",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("status lines missing %q:\n%s", want, joined)
		}
	}
}

func TestStatusSurfacesStaleLockRecoveryState(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "resurrected prompt", nil, "")
	if err := writeBundleManifest(filepath.Join(paths.RunsDir, "0001"), bundleModeDryRun, []string{"prompt.txt"}, &bundleLockRecovery{
		PID:       4242,
		StartedAt: time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.LatestBundleRecovery == "" {
		t.Fatal("expected stale-lock recovery state")
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Stale-lock recovery: Reclaimed stale run lock from pid 4242 started 2026-03-25T11:00:00Z.") {
		t.Fatalf("status lines missing recovery state:\n%s", joined)
	}
}

func TestStatusSurfacesLastFailureContext(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastStatus = "failed"
	snapshot.LastFailure = "Wrapper drift detected: Claude output envelope was malformed."
	snapshot.LastFailureRunDirectory = filepath.Join(paths.RunsDir, "0007")
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.LastFailure == "" {
		t.Fatal("expected last failure context")
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Last failure in " + filepath.Join(paths.RunsDir, "0007") + ": Wrapper drift detected: Claude output envelope was malformed.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("status lines missing %q:\n%s", want, joined)
		}
	}
}

func TestStatusSurfacesMalformedRunLockHint(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(runLockPath(paths.RoomDir), []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write malformed run lock: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if !strings.Contains(report.LatestLockHint, "unreadable run lock") {
		t.Fatalf("latest lock hint = %q", report.LatestLockHint)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "unreadable run lock") {
		t.Fatalf("status lines missing unreadable lock hint:\n%s", strings.Join(report.Lines, "\n"))
	}
}

func TestStatusSurfacesMalformedRoomIgnoreHint(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, _ = prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(filepath.Join(repoRoot, ".roomignore"), []byte("[\n"), 0o644); err != nil {
		t.Fatalf("write malformed roomignore: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.RoomIgnoreHint == "" {
		t.Fatal("expected malformed .roomignore hint")
	}
	if !strings.Contains(report.RoomIgnoreHint, "malformed .roomignore") {
		t.Fatalf("room ignore hint = %q", report.RoomIgnoreHint)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "malformed .roomignore") {
		t.Fatalf("status lines missing malformed roomignore hint:\n%s", strings.Join(report.Lines, "\n"))
	}
}
