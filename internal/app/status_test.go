package app

import (
	"context"
	"os"
	"os/exec"
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

func TestStatusSurfacesUnreadableNewestBundleArtifacts(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	runDir := filepath.Join(paths.RunsDir, "0001")
	writeTailBundle(t, paths.RunsDir, "0001", "damaged prompt", &agent.Result{
		Summary:         "Signal smeared",
		NextInstruction: "Trace the fault",
		Status:          "continue",
		CommitMessage:   "follow the crackle",
	}, strings.TrimSpace(`
diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1,2 @@
-old
+old
+new
`))
	writeExecutionArtifactForTest(t, runDir, 900, false, 0, "", "")
	for _, name := range []string{"stdout.log", "stderr.log"} {
		if err := os.WriteFile(filepath.Join(runDir, name), nil, 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
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
	if err := os.WriteFile(filepath.Join(runDir, "result.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("corrupt result artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "execution.json"), []byte(`{"provider":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("corrupt execution artifact: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.LatestBundleIntegrity != bundleIntegrityBad {
		t.Fatalf("latest bundle integrity = %q", report.LatestBundleIntegrity)
	}
	for _, want := range []string{"unreadable result.json", "unreadable execution.json"} {
		if !strings.Contains(report.LatestBundleHint, want) {
			t.Fatalf("latest bundle hint missing %q: %s", want, report.LatestBundleHint)
		}
	}
	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"unreadable result.json",
		"unreadable execution.json",
		"artifact_decode_failed",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("status lines missing %q:\n%s", want, joined)
		}
	}
}

func TestStatusSurfacesActiveRunLockProgress(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)
	writeTailBundle(t, paths.RunsDir, "0001", "active prompt", nil, "")

	if err := writeRunLock(runLockPath(paths.RoomDir), runLock{
		PID:         4242,
		StartedAt:   time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		RepoRoot:    repoRoot,
		Provider:    "codex",
		RoomVersion: "dev",
		Iteration:   12,
		Phase:       string(RunProgressPhaseAgentExecutionPulse),
		RunDir:      filepath.Join(paths.RunsDir, "0012"),
		HeartbeatAt: time.Date(2026, 3, 25, 11, 0, 9, 0, time.UTC),
	}); err != nil {
		t.Fatalf("write run lock: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     gitClientForTailTest{},
		Version: version.Info{Version: "dev"},
		ProcessAlive: func(int) (bool, error) {
			return true, nil
		},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	for _, want := range []string{
		"active run lock held by pid 4242",
		"iteration 12",
		"phase agent_execution_pulse",
		filepath.Join(paths.RunsDir, "0012"),
		"heartbeat 2026-03-25T11:00:09Z",
	} {
		if !strings.Contains(report.LatestLockHint, want) {
			t.Fatalf("latest lock hint missing %q: %s", want, report.LatestLockHint)
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

func TestStatusSurfacesBlankInstructionFile(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.InstructionPath, []byte("\n \n"), 0o644); err != nil {
		t.Fatalf("blank instruction: %v", err)
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

	if report.CurrentInstruction != "Current instruction unavailable: instruction.txt is blank." {
		t.Fatalf("current instruction = %q", report.CurrentInstruction)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "instruction.txt is blank") {
		t.Fatalf("status lines missing blank-instruction hint:\n%s", strings.Join(report.Lines, "\n"))
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

func TestStatusSurfacesInstructionStateDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.InstructionPath, []byte("Patch the orbit meter\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}
	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastNextInstruction = "Tighten parser reliability"
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

	if report.InstructionDriftHint == "" {
		t.Fatal("expected instruction drift hint")
	}
	if !strings.Contains(report.InstructionDriftHint, "state.json no longer matches instruction.txt") {
		t.Fatalf("instruction drift hint = %q", report.InstructionDriftHint)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "Last recorded next instruction diverged from the live file.") {
		t.Fatalf("status lines missing instruction drift detail:\n%s", strings.Join(report.Lines, "\n"))
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

func TestStatusSurfacesProviderAuthDriftBesideLastRun(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	originalLookPath := lookPath
	originalExecCommandContext := execCommandContext
	t.Cleanup(func() {
		lookPath = originalLookPath
		execCommandContext = originalExecCommandContext
	})

	lookPath = func(binary string) (string, error) {
		if binary != "codex" {
			t.Fatalf("unexpected binary lookup: %s", binary)
		}
		return "/opt/room/bin/codex", nil
	}
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestStatusProviderAuthHelperProcess", "--", name}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastRunAt = time.Date(2026, 3, 27, 9, 45, 0, 0, time.UTC)
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       gitClientForTailTest{},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if report.ProviderAuthDrift == "" {
		t.Fatal("expected provider auth drift")
	}
	if !strings.Contains(report.ProviderAuthDrift, "Codex auth drift: login status failed") {
		t.Fatalf("provider auth drift = %q", report.ProviderAuthDrift)
	}
	if report.ProviderAuthStatus == "" {
		t.Fatal("expected provider auth status")
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Last run: 2026-03-27T09:45:00Z (Codex auth drift: login status failed; authenticate separately before running ROOM)") {
		t.Fatalf("status lines missing inline auth drift:\n%s", joined)
	}
}

func TestStatusProviderAuthHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func TestStatusSurfacesTimedOutProviderAuthDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	originalLookPath := lookPath
	originalExecCommandContext := execCommandContext
	originalTimeout := providerDiagnosticsTimeout
	t.Cleanup(func() {
		lookPath = originalLookPath
		execCommandContext = originalExecCommandContext
		providerDiagnosticsTimeout = originalTimeout
	})

	providerDiagnosticsTimeout = 10 * time.Millisecond
	lookPath = func(binary string) (string, error) {
		if binary != "codex" {
			t.Fatalf("unexpected binary lookup: %s", binary)
		}
		return "/opt/room/bin/codex", nil
	}
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "zsh", "-lc", "sleep 5")
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastRunAt = time.Date(2026, 3, 27, 9, 45, 0, 0, time.UTC)
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       gitClientForTailTest{},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if !strings.Contains(report.ProviderAuthStatus, "Codex auth status timed out after 10ms") {
		t.Fatalf("provider auth status = %q", report.ProviderAuthStatus)
	}
	if !strings.Contains(report.ProviderAuthDrift, "Codex auth drift: auth status timed out after 10ms") {
		t.Fatalf("provider auth drift = %q", report.ProviderAuthDrift)
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Last run: 2026-03-27T09:45:00Z (Codex auth drift: auth status timed out after 10ms; check the CLI manually before running ROOM)") {
		t.Fatalf("status lines missing timed-out inline auth drift:\n%s", joined)
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
