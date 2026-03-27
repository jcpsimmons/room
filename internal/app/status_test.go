package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
