package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
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
