package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/state"
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

func TestPrunePreservesStateReferencedBundle(t *testing.T) {
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

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastRunDirectory = filepath.Join(paths.RunsDir, "0002")
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Prune(context.Background(), PruneOptions{
		WorkingDir: repoRoot,
		Keep:       2,
	})
	if err != nil {
		t.Fatalf("prune with protected bundle: %v", err)
	}
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], filepath.Join("0001")) {
		t.Fatalf("removed bundles = %#v", report.Removed)
	}
	if len(report.Kept) != 3 {
		t.Fatalf("kept bundles = %#v", report.Kept)
	}
	for _, name := range []string{"0002", "0003", "0004"} {
		if _, err := os.Stat(filepath.Join(paths.RunsDir, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "0001")); !os.IsNotExist(err) {
		t.Fatalf("expected 0001 to be pruned, stat err=%v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"removed " + filepath.Join(paths.RunsDir, "0001"),
		"kept " + filepath.Join(paths.RunsDir, "0002") + " (referenced by state.json last run)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prune output missing %q:\n%s", want, joined)
		}
	}
}

func TestPrunePreservesLastFailureReferencedBundle(t *testing.T) {
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
		Status:          "failed",
		CommitMessage:   "two",
	}, "")
	writeTailBundle(t, paths.RunsDir, "0003", "prompt three", &agent.Result{
		Summary:         "three",
		NextInstruction: "four",
		Status:          "continue",
		CommitMessage:   "three",
	}, "")

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastFailureRunDirectory = filepath.Join(paths.RunsDir, "0002")
	snapshot.LastFailure = "wrapper drift"
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Prune(context.Background(), PruneOptions{
		WorkingDir: repoRoot,
		Keep:       1,
	})
	if err != nil {
		t.Fatalf("prune with last failure protection: %v", err)
	}
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], filepath.Join("0001")) {
		t.Fatalf("removed bundles = %#v", report.Removed)
	}
	if len(report.Kept) != 2 {
		t.Fatalf("kept bundles = %#v", report.Kept)
	}
	for _, name := range []string{"0002", "0003"} {
		if _, err := os.Stat(filepath.Join(paths.RunsDir, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "0001")); !os.IsNotExist(err) {
		t.Fatalf("expected 0001 to be pruned, stat err=%v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"removed " + filepath.Join(paths.RunsDir, "0001"),
		"kept " + filepath.Join(paths.RunsDir, "0002") + " (referenced by state.json last failure)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prune output missing %q:\n%s", want, joined)
		}
	}
}

func TestPruneKeepsNewestVerifiedRecoveryBundleWhenNewerTapeDrifts(t *testing.T) {
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
	runDir0002 := filepath.Join(paths.RunsDir, "0002")
	writeExecutionArtifactForTest(t, runDir0002, 1200, false, "")
	if err := os.WriteFile(filepath.Join(runDir0002, "stdout.log"), []byte("stdout\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir0002, "stderr.log"), []byte("stderr\n"), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir0002, "diff.patch"), []byte("diff --git a/a b/a\n"), 0o644); err != nil {
		t.Fatalf("write diff patch: %v", err)
	}
	if err := writeBundleManifest(runDir0002, bundleModeExecuted, []string{
		"prompt.txt",
		"execution.json",
		"stdout.log",
		"stderr.log",
		"result.json",
		"diff.patch",
	}); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}
	writeTailBundle(t, paths.RunsDir, "0003", "prompt three", &agent.Result{
		Summary:         "three",
		NextInstruction: "four",
		Status:          "continue",
		CommitMessage:   "three",
	}, "")

	if err := os.Remove(filepath.Join(paths.RunsDir, "0003", "result.json")); err != nil {
		t.Fatalf("remove result.json: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Prune(context.Background(), PruneOptions{
		WorkingDir: repoRoot,
		Keep:       1,
	})
	if err != nil {
		t.Fatalf("prune with recovery anchor: %v", err)
	}
	if report.RecoveryBundle == "" || !strings.HasSuffix(report.RecoveryBundle, filepath.Join("0002")) {
		t.Fatalf("recovery bundle = %q", report.RecoveryBundle)
	}
	if report.RecoveryBundleDrift != 1 {
		t.Fatalf("recovery drift = %d", report.RecoveryBundleDrift)
	}
	if len(report.Removed) != 1 || !strings.HasSuffix(report.Removed[0], filepath.Join("0001")) {
		t.Fatalf("removed bundles = %#v", report.Removed)
	}
	if len(report.Kept) != 2 {
		t.Fatalf("kept bundles = %#v", report.Kept)
	}
	for _, name := range []string{"0002", "0003"} {
		if _, err := os.Stat(filepath.Join(paths.RunsDir, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Recovery anchor: keeping " + filepath.Join(paths.RunsDir, "0002") + " because 1 newer bundle(s) drifted out of tune.",
		"kept " + filepath.Join(paths.RunsDir, "0002") + " (referenced by state.json newest verified recovery bundle)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("prune output missing %q:\n%s", want, joined)
		}
	}
}

func TestPruneSkipsRecoveryAnchorWhenNewestBundleIsVerified(t *testing.T) {
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
	runDir0002 := filepath.Join(paths.RunsDir, "0002")
	writeExecutionArtifactForTest(t, runDir0002, 900, false, "")
	if err := os.WriteFile(filepath.Join(runDir0002, "stdout.log"), []byte("stdout\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir0002, "stderr.log"), []byte("stderr\n"), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir0002, "diff.patch"), []byte("diff --git a/a b/a\n"), 0o644); err != nil {
		t.Fatalf("write diff patch: %v", err)
	}
	if err := writeBundleManifest(runDir0002, bundleModeExecuted, []string{
		"prompt.txt",
		"execution.json",
		"stdout.log",
		"stderr.log",
		"result.json",
		"diff.patch",
	}); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Prune(context.Background(), PruneOptions{
		WorkingDir: repoRoot,
		Keep:       1,
	})
	if err != nil {
		t.Fatalf("prune with verified newest bundle: %v", err)
	}
	if report.RecoveryBundle != "" || report.RecoveryBundleDrift != 0 {
		t.Fatalf("unexpected recovery anchor: %#v", report)
	}
	joined := strings.Join(report.Lines, "\n")
	if strings.Contains(joined, "Recovery anchor:") {
		t.Fatalf("unexpected recovery anchor line:\n%s", joined)
	}
}
