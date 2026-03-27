package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/version"
)

func TestPruneRemovesOlderBundles(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "prompt one", &agent.Result{
		Summary:         "one",
		NextInstruction: "two",
		Status:          "continue",
		CommitMessage:   "one",
	}, "")
	writeTailBundle(t, paths.RunsDir, "0002", "prompt two", &agent.Result{
		Summary:         "two",
		NextInstruction: "three",
		Status:          "continue",
		CommitMessage:   "two",
	}, "")
	writeTailBundle(t, paths.RunsDir, "0003", "prompt three", &agent.Result{
		Summary:         "three",
		NextInstruction: "four",
		Status:          "continue",
		CommitMessage:   "three",
	}, "")
	writeTailBundle(t, paths.RunsDir, "0004", "prompt four", &agent.Result{
		Summary:         "four",
		NextInstruction: "five",
		Status:          "continue",
		CommitMessage:   "four",
	}, "")

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Prune(context.Background(), PruneOptions{
		WorkingDir: repoRoot,
		Keep:       2,
	})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(report.Removed) != 2 {
		t.Fatalf("removed bundles = %#v", report.Removed)
	}
	if !strings.HasSuffix(report.Removed[0], filepath.Join("0002")) || !strings.HasSuffix(report.Removed[1], filepath.Join("0001")) {
		t.Fatalf("removed bundles out of order: %#v", report.Removed)
	}
	for _, name := range []string{"0001", "0002"} {
		if _, err := os.Stat(filepath.Join(paths.RunsDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", name, err)
		}
	}
	for _, name := range []string{"0003", "0004"} {
		if _, err := os.Stat(filepath.Join(paths.RunsDir, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"ROOM prune",
		"Keeping newest 2 bundle(s).",
		"removed " + filepath.Join(paths.RunsDir, "0002"),
		"removed " + filepath.Join(paths.RunsDir, "0001"),
		"kept " + filepath.Join(paths.RunsDir, "0004"),
		"kept " + filepath.Join(paths.RunsDir, "0003"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prune output missing %q:\n%s", want, joined)
		}
	}
}

func TestPruneDryRunLeavesBundlesIntact(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "prompt one", &agent.Result{
		Summary:         "one",
		NextInstruction: "two",
		Status:          "continue",
		CommitMessage:   "one",
	}, "")
	writeTailBundle(t, paths.RunsDir, "0002", "prompt two", &agent.Result{
		Summary:         "two",
		NextInstruction: "three",
		Status:          "continue",
		CommitMessage:   "two",
	}, "")

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Prune(context.Background(), PruneOptions{
		WorkingDir: repoRoot,
		Keep:       1,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("prune dry run: %v", err)
	}
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], filepath.Join("0001")) {
		t.Fatalf("dry-run removed bundles = %#v", report.Removed)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "0001")); err != nil {
		t.Fatalf("expected dry-run to leave 0001 intact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "0002")); err != nil {
		t.Fatalf("expected dry-run to leave 0002 intact: %v", err)
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "would remove "+filepath.Join(paths.RunsDir, "0001")) {
		t.Fatalf("prune dry-run output missing removal preview:\n%s", joined)
	}
	if !strings.Contains(joined, "Dry run only; nothing was deleted.") {
		t.Fatalf("prune dry-run output missing dry-run note:\n%s", joined)
	}
}
