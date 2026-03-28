package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/version"
)

func TestBundleInspectsManifestedRun(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	runDir := filepath.Join(paths.RunsDir, "0001")
	writeTailBundle(t, paths.RunsDir, "0001", "bundle prompt", &agent.Result{
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
`))
	writeExecutionArtifactForTest(t, runDir, 2500, false, 0, "", "")
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("stdout\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte("stderr\n"), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := writeBundleManifest(runDir, bundleModeExecuted, []string{
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
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Bundle(context.Background(), BundleOptions{
		WorkingDir: repoRoot,
		RunDir:     "0001",
	})
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}

	if report.RunDir != runDir {
		t.Fatalf("run dir = %q", report.RunDir)
	}
	if !report.ManifestOK {
		t.Fatal("expected verified manifest")
	}
	if report.BundleIntegrity != bundleIntegrityOK {
		t.Fatalf("bundle integrity = %q", report.BundleIntegrity)
	}
	if report.Execution == nil {
		t.Fatal("expected execution details")
	}
	if report.Execution.DurationMS != 2500 || report.Execution.TimedOut || report.Execution.ExitCode != 0 || report.Execution.ExitSignal != "" || report.Execution.Error != "" {
		t.Fatalf("execution report = %#v", report.Execution)
	}
	if len(report.Artifacts) != 6 {
		t.Fatalf("artifact count = %d", len(report.Artifacts))
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"ROOM bundle",
		"Bundle dir: " + runDir,
		"Bundle integrity: verified",
		"duration: 2.5s (2500 ms)",
		"timed out: false",
		"exit: 0",
		"Manifest artifacts:",
		"prompt.txt",
		"result.json",
		"diff.patch",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("bundle output missing %q:\n%s", want, joined)
		}
	}
}

func TestBundleExplainsMissingArtifacts(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "sparse prompt", nil, "")

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Bundle(context.Background(), BundleOptions{
		WorkingDir: repoRoot,
	})
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}

	if report.BundleIntegrity != bundleIntegrityWarn {
		t.Fatalf("bundle integrity = %q", report.BundleIntegrity)
	}
	if report.BundleHint == "" {
		t.Fatal("expected bundle hint")
	}
	if !strings.Contains(report.BundleHint, "missing result.json and diff.patch") {
		t.Fatalf("bundle hint = %q", report.BundleHint)
	}
	if len(report.BundleIntegrityHints) != 2 {
		t.Fatalf("bundle integrity hints = %#v", report.BundleIntegrityHints)
	}
	if report.BundleIntegrityHints[0].Code != bundleIntegrityHintRunArtifactMissing || report.BundleIntegrityHints[1].Code != bundleIntegrityHintRunArtifactMissing {
		t.Fatalf("bundle integrity hint codes = %#v", report.BundleIntegrityHints)
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Bundle integrity hints:") {
		t.Fatalf("bundle lines missing integrity hints:\n%s", joined)
	}
}

func TestBundleSurfacesUnreadableExecutionArtifact(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	runDir := filepath.Join(paths.RunsDir, "0001")
	writeTailBundle(t, paths.RunsDir, "0001", "bundle prompt", &agent.Result{
		Summary:         "Signal locked in",
		NextInstruction: "Turn the resonance up",
		Status:          "continue",
		CommitMessage:   "tighten the filter",
	}, strings.TrimSpace(`
diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
`))
	if err := os.WriteFile(filepath.Join(runDir, "execution.json"), []byte(`{"provider":"","started_at":"2026-03-25T11:00:00Z","finished_at":"2026-03-25T11:00:01Z"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write malformed execution: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("stdout\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte("stderr\n"), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := writeBundleManifest(runDir, bundleModeExecuted, []string{
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
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Bundle(context.Background(), BundleOptions{
		WorkingDir: repoRoot,
		RunDir:     "0001",
	})
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if report.BundleIntegrity != bundleIntegrityWarn {
		t.Fatalf("bundle integrity = %q", report.BundleIntegrity)
	}
	if report.Execution != nil {
		t.Fatalf("expected unreadable execution to be suppressed, got %#v", report.Execution)
	}
	if len(report.BundleIntegrityHints) == 0 {
		t.Fatal("expected bundle integrity hints")
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Bundle integrity: unverified",
		"unreadable execution.json",
		"Execution:",
		"unavailable",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("bundle output missing %q:\n%s", want, joined)
		}
	}
}

func TestBundleInspectsFailedManifestedRun(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	runDir := filepath.Join(paths.RunsDir, "0001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "prompt.txt"), []byte("failed prompt\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	writeExecutionArtifactForTest(t, runDir, 900, false, 9, "killed", "claude wrapper drift detected")
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("stdout fragment\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stderr.log"), []byte("stderr fragment\n"), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := writeBundleManifest(runDir, bundleModeFailed, []string{
		"prompt.txt",
		"execution.json",
		"stdout.log",
		"stderr.log",
	}); err != nil {
		t.Fatalf("write failed bundle manifest: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Bundle(context.Background(), BundleOptions{
		WorkingDir: repoRoot,
		RunDir:     "0001",
	})
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if report.BundleMode != string(bundleModeFailed) {
		t.Fatalf("bundle mode = %q", report.BundleMode)
	}
	if report.BundleIntegrity != bundleIntegrityOK {
		t.Fatalf("bundle integrity = %q", report.BundleIntegrity)
	}
	if !report.ManifestOK {
		t.Fatal("expected failed manifest to verify cleanly")
	}
	if report.Execution == nil || report.Execution.Error != "claude wrapper drift detected" {
		t.Fatalf("execution report = %#v", report.Execution)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Bundle mode: failed",
		"Bundle integrity: verified",
		"error: claude wrapper drift detected",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("bundle output missing %q:\n%s", want, joined)
		}
	}
}

var _ git.Client = gitClientForTailTest{}
