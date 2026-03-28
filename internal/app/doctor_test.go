package app

import (
	"context"
	"errors"
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

func TestDoctorSurfacesBundleRecoveryAndDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeTailBundle(t, paths.RunsDir, "0001", "old prompt", &agent.Result{
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

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.LastRunDirectory = filepath.Join(paths.RunsDir, "0001")
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
		"state points at " + filepath.Join(paths.RunsDir, "0001"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, joined)
		}
	}

	var bundleCheck, driftCheck bool
	for _, check := range report.Checks {
		switch check.Name {
		case "bundle":
			bundleCheck = strings.Contains(check.Message, "Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.")
		case "run_directory":
			driftCheck = strings.Contains(check.Message, "state points at") && strings.Contains(check.Message, "newest bundle")
		}
	}
	if !bundleCheck {
		t.Fatalf("expected bundle recovery check, got %#v", report.Checks)
	}
	if !driftCheck {
		t.Fatalf("expected run-directory drift check, got %#v", report.Checks)
	}
}

func TestDoctorSurfacesRunLockStatus(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := writeRunLock(runLockPath(paths.RoomDir), runLock{
		PID:         4242,
		StartedAt:   time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		RepoRoot:    repoRoot,
		Provider:    "codex",
		RoomVersion: "dev",
	}); err != nil {
		t.Fatalf("write run lock: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
		ProcessAlive: func(int) (bool, error) {
			return false, nil
		},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "stale run lock from pid 4242") {
		t.Fatalf("doctor output missing lock hint:\n%s", joined)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "run_lock" {
			found = true
			if !strings.Contains(check.Message, "stale run lock from pid 4242") {
				t.Fatalf("run_lock check = %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("expected run_lock check, got %#v", report.Checks)
	}
}

func TestDoctorReportsGitInfoExcludeProtection(t *testing.T) {
	repoRoot := initGitRepo(t)
	prepareInitializedRepo(t, repoRoot)

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "git_info_exclude" {
			found = true
			if check.OK {
				t.Fatalf("expected missing exclude protection to fail, got %#v", check)
			}
			if !strings.Contains(check.Message, "does not mention .room/") {
				t.Fatalf("unexpected exclude check message: %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("expected git_info_exclude check, got %#v", report.Checks)
	}

	if err := os.WriteFile(filepath.Join(repoRoot, ".git", "info", "exclude"), []byte(".room/\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	report, err = svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor after exclude: %v", err)
	}

	found = false
	for _, check := range report.Checks {
		if check.Name == "git_info_exclude" {
			found = true
			if !check.OK {
				t.Fatalf("expected exclude protection to pass, got %#v", check)
			}
			if !strings.Contains(check.Message, "already protects .room/") {
				t.Fatalf("unexpected exclude check message: %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("expected git_info_exclude check after writing exclude, got %#v", report.Checks)
	}
}

func TestDoctorReportsWorktreeGitExcludeProtection(t *testing.T) {
	repoRoot := t.TempDir()
	_, _ = prepareInitializedRepo(t, repoRoot)
	excludePath := writeLinkedWorktreeGitDir(t, repoRoot)

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "git_info_exclude" {
			found = true
			if check.OK {
				t.Fatalf("expected missing exclude protection to fail, got %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("expected git_info_exclude check, got %#v", report.Checks)
	}

	if err := os.WriteFile(excludePath, []byte(".room/\n"), 0o644); err != nil {
		t.Fatalf("write exclude: %v", err)
	}

	report, err = svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor after exclude: %v", err)
	}

	found = false
	for _, check := range report.Checks {
		if check.Name == "git_info_exclude" {
			found = true
			if !check.OK {
				t.Fatalf("expected exclude protection to pass, got %#v", check)
			}
			if !strings.Contains(check.Message, "git exclude file already protects .room/") {
				t.Fatalf("unexpected exclude check message: %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("expected git_info_exclude check after writing exclude, got %#v", report.Checks)
	}
}

func TestDoctorSurfacesMalformedSummaryHistory(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.SummariesPath, []byte("{not-json\n"), 0o644); err != nil {
		t.Fatalf("write malformed summaries: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name != "history" {
			continue
		}
		found = true
		if check.OK {
			t.Fatalf("expected malformed history to fail, got %#v", check)
		}
		if !strings.Contains(check.Message, "malformed entrie(s)") {
			t.Fatalf("unexpected history message: %#v", check)
		}
	}
	if !found {
		t.Fatalf("expected history check, got %#v", report.Checks)
	}

	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "summary history log has 1 malformed entrie(s)") {
		t.Fatalf("doctor output missing history hint:\n%s", joined)
	}
}

func TestDoctorSurfacesSchemaContractDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.SchemaPath, []byte("{\"type\":\"object\",\"title\":\"stale\"}\n"), 0o644); err != nil {
		t.Fatalf("write schema drift: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name != "schema" {
			continue
		}
		found = true
		if check.OK {
			t.Fatalf("expected schema drift to fail, got %#v", check)
		}
		if !strings.Contains(check.Message, "drifted from this ROOM build") {
			t.Fatalf("unexpected schema check: %#v", check)
		}
	}
	if !found {
		t.Fatalf("expected schema check, got %#v", report.Checks)
	}
}

func TestDoctorFlagsBlankInstructionFile(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.InstructionPath, []byte("\n \n"), 0o644); err != nil {
		t.Fatalf("blank instruction: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name == "instruction" {
			found = true
			if check.OK {
				t.Fatalf("expected instruction check to fail, got %#v", check)
			}
			if !strings.Contains(check.Message, "instruction.txt is blank") {
				t.Fatalf("unexpected instruction check: %#v", check)
			}
		}
	}
	if !found {
		t.Fatalf("expected instruction check, got %#v", report.Checks)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "instruction.txt is blank") {
		t.Fatalf("doctor lines missing blank-instruction hint:\n%s", strings.Join(report.Lines, "\n"))
	}
}

func TestDoctorSurfacesMalformedRoomIgnore(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, _ = prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(filepath.Join(repoRoot, ".roomignore"), []byte("[\n"), 0o644); err != nil {
		t.Fatalf("write malformed roomignore: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name != "room_ignore" {
			continue
		}
		found = true
		if check.OK {
			t.Fatalf("expected malformed .roomignore to fail, got %#v", check)
		}
		if !strings.Contains(check.Message, "malformed .roomignore") {
			t.Fatalf("unexpected room_ignore message: %#v", check)
		}
	}
	if !found {
		t.Fatalf("expected room_ignore check, got %#v", report.Checks)
	}

	if !strings.Contains(strings.Join(report.Lines, "\n"), "malformed .roomignore") {
		t.Fatalf("doctor output missing malformed roomignore hint:\n%s", strings.Join(report.Lines, "\n"))
	}
}

func TestDoctorSurfacesConfigPathCollisions(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, paths := prepareInitializedRepo(t, repoRoot)

	writeRepoFile(t, paths.ConfigPath, `
[prompt]
instruction_file = ".room/state.json"
`)

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name != "config_paths" {
			continue
		}
		found = true
		if check.OK {
			t.Fatalf("expected config_paths failure, got %#v", check)
		}
		if !strings.Contains(check.Message, "collides with the ROOM state file") {
			t.Fatalf("unexpected config_paths message: %#v", check)
		}
	}
	if !found {
		t.Fatalf("expected config_paths check, got %#v", report.Checks)
	}
}

func TestDoctorSurfacesPromptHistoryStagnation(t *testing.T) {
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
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name != "prompt_history" {
			continue
		}
		found = true
		if check.OK {
			t.Fatalf("expected prompt history check to fail, got %#v", check)
		}
		if !strings.Contains(check.Message, "exact duplicate next instruction") {
			t.Fatalf("unexpected prompt history message: %#v", check)
		}
	}
	if !found {
		t.Fatalf("expected prompt_history check, got %#v", report.Checks)
	}
	if !strings.Contains(report.PromptHistoryHint, "exact duplicate next instruction") {
		t.Fatalf("prompt history hint = %q", report.PromptHistoryHint)
	}
}

func TestDoctorReportsProviderBinaryResolution(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, _ = prepareInitializedRepo(t, repoRoot)

	originalLookPath := lookPath
	t.Cleanup(func() {
		lookPath = originalLookPath
	})

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Version:   version.Info{Version: "dev"},
	})

	lookPath = func(binary string) (string, error) {
		if binary == "codex" {
			return "/opt/room/bin/codex", nil
		}
		return "", errors.New("unexpected lookPath binary")
	}

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor resolved: %v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"configured codex binary: codex",
		"PATH search resolved codex to /opt/room/bin/codex",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, joined)
		}
	}

	lookPath = func(binary string) (string, error) {
		if binary == "codex" {
			return "", errors.New("executable file not found in $PATH")
		}
		return "", errors.New("unexpected lookPath binary")
	}

	report, err = svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor missing: %v", err)
	}

	joined = strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"PATH search for codex failed: executable file not found in $PATH",
		"binary not found on PATH: codex",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, joined)
		}
	}
}

func TestDoctorTimesOutSlowProviderVersionProbe(t *testing.T) {
	repoRoot := initGitRepo(t)
	_, _ = prepareInitializedRepo(t, repoRoot)

	originalTimeout := providerDiagnosticsTimeout
	providerDiagnosticsTimeout = 10 * time.Millisecond
	t.Cleanup(func() {
		providerDiagnosticsTimeout = originalTimeout
	})

	svc := NewService(Dependencies{
		Git: &fakeGit{root: repoRoot},
		Providers: testProviders(&fakeRunner{
			versionFn: func(ctx context.Context, _ string) (string, error) {
				<-ctx.Done()
				return "", ctx.Err()
			},
		}, nil),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Doctor(context.Background(), DoctorOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	found := false
	for _, check := range report.Checks {
		if check.Name != "provider" {
			continue
		}
		found = true
		if check.OK {
			t.Fatalf("expected provider check to fail, got %#v", check)
		}
		if !strings.Contains(check.Message, "version probe timed out after 10ms") {
			t.Fatalf("unexpected provider timeout message: %#v", check)
		}
	}
	if !found {
		t.Fatalf("expected provider check, got %#v", report.Checks)
	}
}
