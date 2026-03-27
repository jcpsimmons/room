package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/version"
)

func TestTailReadsNewestBundle(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	t.Run("complete bundle", func(t *testing.T) {
		writeTailBundle(t, paths.RunsDir, "0001", "old prompt", nil, "")
		writeTailBundle(t, paths.RunsDir, "0002", "new prompt", &agent.Result{
			Summary:         "Signal locked in",
			NextInstruction: "Turn the resonance up",
			Status:          "continue",
			CommitMessage:   "tighten the filter",
		}, strings.TrimSpace(`
diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1,2 @@
-old
+old
+new
diff --git a/b.txt b/b.txt
--- /dev/null
+++ b/b.txt
@@ -0,0 +1 @@
+fresh
`))

		report, err := svc.Tail(context.Background(), TailOptions{WorkingDir: repoRoot})
		if err != nil {
			t.Fatalf("tail: %v", err)
		}
		if report.RunDir != filepath.Join(paths.RunsDir, "0002") {
			t.Fatalf("run dir = %q", report.RunDir)
		}
		if report.Prompt != "new prompt" {
			t.Fatalf("prompt = %q", report.Prompt)
		}
		if report.Result == nil {
			t.Fatal("expected result to be present")
		}
		if report.Result.Summary != "Signal locked in" {
			t.Fatalf("result summary = %q", report.Result.Summary)
		}
		if report.Diff.Files != 2 || report.Diff.Added != 3 || report.Diff.Deleted != 1 {
			t.Fatalf("diff stats = %#v", report.Diff)
		}
		joined := strings.Join(report.Lines, "\n")
		for _, want := range []string{
			"Latest ROOM bundle: " + filepath.Join(paths.RunsDir, "0002"),
			"summary: Signal locked in",
			"status: continue",
			"files changed: 2",
			"insertions: 3",
			"deletions: 1",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("tail output missing %q:\n%s", want, joined)
			}
		}
	})

	t.Run("sparse bundle", func(t *testing.T) {
		writeTailBundle(t, paths.RunsDir, "0003", "dry-run prompt", nil, "")

		report, err := svc.Tail(context.Background(), TailOptions{WorkingDir: repoRoot})
		if err != nil {
			t.Fatalf("tail: %v", err)
		}
		if report.RunDir != filepath.Join(paths.RunsDir, "0003") {
			t.Fatalf("run dir = %q", report.RunDir)
		}
		if report.Result != nil {
			t.Fatalf("expected no result, got %#v", report.Result)
		}
		joined := strings.Join(report.Lines, "\n")
		for _, want := range []string{
			"dry-run prompt",
			"unavailable",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("tail output missing %q:\n%s", want, joined)
			}
		}
	})

	t.Run("manifested dry run", func(t *testing.T) {
		runDir := filepath.Join(paths.RunsDir, "0004")
		writeTailBundle(t, paths.RunsDir, "0004", "dry-run prompt", nil, "")
		if err := writeBundleManifest(runDir, bundleModeDryRun, []string{"prompt.txt"}); err != nil {
			t.Fatalf("write bundle manifest: %v", err)
		}

		report, err := svc.Tail(context.Background(), TailOptions{WorkingDir: repoRoot})
		if err != nil {
			t.Fatalf("tail: %v", err)
		}
		if report.BundleMode != string(bundleModeDryRun) {
			t.Fatalf("bundle mode = %q", report.BundleMode)
		}
		if report.BundleIntegrity != bundleIntegrityOK {
			t.Fatalf("bundle integrity = %q", report.BundleIntegrity)
		}
		joined := strings.Join(report.Lines, "\n")
		for _, want := range []string{
			"Bundle mode: dry_run",
			"Bundle integrity: verified",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("tail output missing %q:\n%s", want, joined)
			}
		}
		if strings.Contains(joined, "missing result.json and diff.patch") {
			t.Fatalf("dry-run bundle should not be treated as incomplete:\n%s", joined)
		}
	})
}

func writeTailBundle(t *testing.T, runsDir, name, prompt string, result *agent.Result, patch string) {
	t.Helper()

	runDir := filepath.Join(runsDir, name)
	if err := fsutil.EnsureDir(runDir); err != nil {
		t.Fatalf("ensure run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "prompt.txt"), []byte(prompt+"\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal result: %v", err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "result.json"), append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write result: %v", err)
		}
	}
	if patch != "" {
		if err := os.WriteFile(filepath.Join(runDir, "diff.patch"), []byte(patch+"\n"), 0o644); err != nil {
			t.Fatalf("write diff patch: %v", err)
		}
	}
}

type gitClientForTailTest struct{}

func (gitClientForTailTest) IsRepo(context.Context, string) (bool, error)        { return true, nil }
func (gitClientForTailTest) Root(_ context.Context, dir string) (string, error)  { return dir, nil }
func (gitClientForTailTest) StatusShort(context.Context, string) (string, error) { return "", nil }
func (gitClientForTailTest) IsDirty(context.Context, string) (bool, error)       { return false, nil }
func (gitClientForTailTest) Diff(context.Context, string) (string, error)        { return "", nil }
func (gitClientForTailTest) DiffStats(context.Context, string) (git.DiffStats, error) {
	return git.DiffStats{}, nil
}
func (gitClientForTailTest) CommitAll(context.Context, string, string) (string, error) {
	return "", nil
}
func (gitClientForTailTest) RecentCommits(context.Context, string, int) ([]git.Commit, error) {
	return nil, nil
}
func (gitClientForTailTest) RecentCommitsWithPrefix(context.Context, string, int, string) ([]git.Commit, error) {
	return nil, nil
}
func (gitClientForTailTest) Head(context.Context, string) (string, error) { return "", nil }
