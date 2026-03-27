package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/state"
	"github.com/jcpsimmons/room/internal/version"
)

type fakeRunner struct {
	version   string
	versionFn func(context.Context, string) (string, error)
	calls     int
	prompts   []string
	runs      []fakeRun
}

type fakeRun struct {
	result agent.Result
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Version(ctx context.Context, binary string) (string, error) {
	if f.versionFn != nil {
		return f.versionFn(ctx, binary)
	}
	return f.version, nil
}

func (f *fakeRunner) Run(_ context.Context, prompt agent.Prompt, _ agent.Schema, _ agent.RunOptions, outputPath string) (agent.Execution, error) {
	if f.calls >= len(f.runs) {
		return agent.Execution{}, errors.New("unexpected run call")
	}
	run := f.runs[f.calls]
	f.prompts = append(f.prompts, prompt.Body)
	f.calls++

	if run.err == nil {
		data, err := json.Marshal(run.result)
		if err != nil {
			return agent.Execution{}, err
		}
		if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
			return agent.Execution{}, err
		}
	}

	return agent.Execution{
		Result:     run.result,
		Stdout:     run.stdout,
		Stderr:     run.stderr,
		Command:    []string{"codex", "exec"},
		DurationMS: 1250,
	}, run.err
}

type fakeGit struct {
	root           string
	dirtySeq       []bool
	statusShort    string
	diffSeq        []string
	statsSeq       []git.DiffStats
	commitHashes   []string
	commitMessages []string
	recentCommits  []git.Commit
	roomCommits    []git.Commit
	diffCalls      int
	statsCalls     int
	bundleCalls    int
}

func (f *fakeGit) IsRepo(context.Context, string) (bool, error) { return true, nil }
func (f *fakeGit) Root(context.Context, string) (string, error) { return f.root, nil }
func (f *fakeGit) StatusShort(context.Context, string) (string, error) {
	return f.statusShort, nil
}
func (f *fakeGit) IsDirty(context.Context, string) (bool, error) {
	if len(f.dirtySeq) == 0 {
		return false, nil
	}
	value := f.dirtySeq[0]
	f.dirtySeq = f.dirtySeq[1:]
	return value, nil
}
func (f *fakeGit) Diff(context.Context, string) (string, error) {
	f.diffCalls++
	if len(f.diffSeq) == 0 {
		return "", nil
	}
	value := f.diffSeq[0]
	f.diffSeq = f.diffSeq[1:]
	return value, nil
}
func (f *fakeGit) DiffStats(context.Context, string) (git.DiffStats, error) {
	f.statsCalls++
	if len(f.statsSeq) == 0 {
		return git.DiffStats{}, nil
	}
	value := f.statsSeq[0]
	f.statsSeq = f.statsSeq[1:]
	return value, nil
}
func (f *fakeGit) DiffAndStats(context.Context, string) (string, git.DiffStats, error) {
	f.bundleCalls++
	diff := ""
	if len(f.diffSeq) > 0 {
		diff = f.diffSeq[0]
		f.diffSeq = f.diffSeq[1:]
	}
	stats := git.DiffStats{}
	if len(f.statsSeq) > 0 {
		stats = f.statsSeq[0]
		f.statsSeq = f.statsSeq[1:]
	}
	return diff, stats, nil
}
func (f *fakeGit) CommitAll(_ context.Context, _ string, message string) (string, error) {
	f.commitMessages = append(f.commitMessages, message)
	hash := "deadbeef"
	if len(f.commitHashes) > 0 {
		hash = f.commitHashes[0]
		f.commitHashes = f.commitHashes[1:]
	}
	return hash, nil
}
func (f *fakeGit) RecentCommits(context.Context, string, int) ([]git.Commit, error) {
	return f.recentCommits, nil
}
func (f *fakeGit) RecentCommitsWithPrefix(context.Context, string, int, string) ([]git.Commit, error) {
	return f.roomCommits, nil
}
func (f *fakeGit) Head(context.Context, string) (string, error) { return "deadbeef", nil }

func TestRunUsesCombinedDiffAndStats(t *testing.T) {
	repoRoot := t.TempDir()
	_, _ = prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Added a tighter filter",
				NextInstruction: "Refine the filter graph",
				Status:          "continue",
				CommitMessage:   "tighten the filter",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 3, Deleted: 1}},
		commitHashes: []string{"abc123"},
	}

	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		NoCommit:   true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if fakeGit.bundleCalls != 1 {
		t.Fatalf("expected combined diff/stat call, got %d", fakeGit.bundleCalls)
	}
	if fakeGit.diffCalls != 0 || fakeGit.statsCalls != 0 {
		t.Fatalf("expected legacy diff calls to stay unused, got diff=%d stats=%d", fakeGit.diffCalls, fakeGit.statsCalls)
	}
}

func TestRunRedactsSensitivePromptContext(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	secretInstruction := strings.TrimSpace(`
export OPENAI_API_KEY=sk-test-1234567890abcdef1234567890
AWS_SECRET_ACCESS_KEY=abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd
github_pat_abcdefghijklmnopqrstuvwxyz0123456789
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASC
-----END PRIVATE KEY-----
`)
	if err := os.WriteFile(paths.InstructionPath, []byte(secretInstruction+"\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
		Providers: testProviders(&fakeRunner{
			version: "codex-cli 0.116.0",
			runs:    nil,
		}, nil),
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		DryRun:     true,
		NoCommit:   true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	promptData, err := os.ReadFile(filepath.Join(report.LastRunDir, "prompt.txt"))
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	promptText := string(promptData)

	for _, want := range []string{
		"<redacted>",
		"<redacted private key>",
	} {
		if !strings.Contains(promptText, want) {
			t.Fatalf("run prompt missing %q:\n%s", want, promptText)
		}
	}
	for _, want := range []string{
		"sk-test-1234567890abcdef1234567890",
		"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd",
		"github_pat_abcdefghijklmnopqrstuvwxyz0123456789",
		"MIIEvQIBADANBgkqhkiG9w0BAQEFAASC",
	} {
		if strings.Contains(promptText, want) {
			t.Fatalf("run prompt leaked %q:\n%s", want, promptText)
		}
	}
}

func TestRunIncludesBundleFaultSignalInPrompt(t *testing.T) {
	repoRoot := t.TempDir()
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
		Git: &fakeGit{
			root:          repoRoot,
			statusShort:   " M a.txt",
			recentCommits: []git.Commit{{Hash: "abc123", Subject: "close the loop"}},
		},
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
		Providers: testProviders(&fakeRunner{
			version: "codex-cli 0.116.0",
		}, nil),
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		DryRun:     true,
		NoCommit:   true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	promptData, err := os.ReadFile(filepath.Join(report.LastRunDir, "prompt.txt"))
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	promptText := string(promptData)

	for _, want := range []string{
		"Artifact fault signal:",
		"Hint: newest bundle 0002 is incomplete; missing result.json and diff.patch.",
	} {
		if !strings.Contains(promptText, want) {
			t.Fatalf("run prompt missing %q:\n%s", want, promptText)
		}
	}
}

func TestRunIncludesPromptHistorySignalInPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.InstructionPath, []byte("Improve parser resilience\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Improve parser resilience"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}

	svc := NewService(Dependencies{
		Git: &fakeGit{
			root:          repoRoot,
			statusShort:   " M a.txt",
			recentCommits: []git.Commit{{Hash: "abc123", Subject: "close the loop"}},
		},
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
		Providers: testProviders(&fakeRunner{
			version: "codex-cli 0.116.0",
		}, nil),
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		DryRun:     true,
		NoCommit:   true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	promptData, err := os.ReadFile(filepath.Join(report.LastRunDir, "prompt.txt"))
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	promptText := string(promptData)

	for _, want := range []string{
		"Prompt history is stagnating: exact duplicate next instruction.",
		"Matched prior instruction \"Improve parser resilience\".",
		"Pivot hard. The prior direction is stagnating because of exact duplicate next instruction.",
	} {
		if !strings.Contains(promptText, want) {
			t.Fatalf("run prompt missing %q:\n%s", want, promptText)
		}
	}
}

func TestRunRejectsBlankInstructionFile(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	if err := os.WriteFile(paths.InstructionPath, []byte(" \n\t\n"), 0o644); err != nil {
		t.Fatalf("write blank instruction: %v", err)
	}

	runner := &fakeRunner{version: "codex-cli 0.116.0"}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
		Providers: testProviders(runner, nil),
	})

	_, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		NoCommit:   true,
	})
	if err == nil {
		t.Fatal("expected blank instruction failure")
	}
	if !strings.Contains(err.Error(), "instruction.txt is blank") {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("runner should not be called, got %d calls", runner.calls)
	}
}

func TestRunRefusesAnActiveRunLock(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	lockPath := runLockPath(paths.RoomDir)
	if err := writeRunLock(lockPath, runLock{
		PID:         4242,
		StartedAt:   time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		RepoRoot:    repoRoot,
		Provider:    "codex",
		RoomVersion: "dev",
	}); err != nil {
		t.Fatalf("write run lock: %v", err)
	}

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Should never run",
				NextInstruction: "Should never run",
				Status:          "continue",
				CommitMessage:   "should not matter",
			},
		}},
	}
	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot},
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
		ProcessAlive: func(int) (bool, error) {
			return true, nil
		},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
	})
	if err == nil {
		t.Fatalf("expected active lock error")
	}
	if !strings.Contains(err.Error(), "another ROOM run is already active") {
		t.Fatalf("run error = %v", err)
	}
	if report.CompletedIterations != 0 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if runner.calls != 0 {
		t.Fatalf("runner should not be invoked while lock is active")
	}
}

func TestRunReclaimsAStaleRunLock(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	lockPath := runLockPath(paths.RoomDir)
	if err := writeRunLock(lockPath, runLock{
		PID:         4242,
		StartedAt:   time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		RepoRoot:    repoRoot,
		Provider:    "codex",
		RoomVersion: "dev",
	}); err != nil {
		t.Fatalf("write run lock: %v", err)
	}

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Recovered from a stale lock",
				NextInstruction: "Keep going",
				Status:          "continue",
				CommitMessage:   "recover from stale lock",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 2, Deleted: 1}},
		commitHashes: []string{"abc123"},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
		ProcessAlive: func(int) (bool, error) {
			return false, nil
		},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if !containsLine(report.Lines, "Reclaimed stale run lock from pid 4242 started 2026-03-25T11:00:00Z.") {
		t.Fatalf("run lines missing stale lock recovery note:\n%s", strings.Join(report.Lines, "\n"))
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stale lock to be cleaned up, stat err=%v", err)
	}
}

func TestRunReclaimsAMalformedRunLock(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	lockPath := runLockPath(paths.RoomDir)
	if err := os.WriteFile(lockPath, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write malformed lock: %v", err)
	}

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Recovered from a malformed lock",
				NextInstruction: "Keep going",
				Status:          "continue",
				CommitMessage:   "recover from malformed lock",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 2, Deleted: 1}},
		commitHashes: []string{"abc123"},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
		ProcessAlive: func(int) (bool, error) {
			return false, nil
		},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "unreadable run lock") {
		t.Fatalf("run lines missing malformed-lock hint:\n%s", strings.Join(report.Lines, "\n"))
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected malformed lock to be replaced and cleaned up, stat err=%v", err)
	}
}

func TestRunForcesPivotOnDuplicateInstruction(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Improve parser resilience"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Added parser guardrails",
				NextInstruction: "Improve parser resilience",
				Status:          "continue",
				CommitMessage:   "add parser guardrails",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 10, Deleted: 2}},
		commitHashes: []string{"abc123"},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if len(fakeGit.commitMessages) != 1 || fakeGit.commitMessages[0] != "room: add parser guardrails" {
		t.Fatalf("commit messages = %#v", fakeGit.commitMessages)
	}

	instruction, err := os.ReadFile(paths.InstructionPath)
	if err != nil {
		t.Fatalf("read instruction: %v", err)
	}
	if !strings.Contains(string(instruction), "Pivot hard.") {
		t.Fatalf("expected pivot instruction, got %q", string(instruction))
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if snapshot.LastStatus != "pivot" {
		t.Fatalf("last status = %q", snapshot.LastStatus)
	}
	if snapshot.LastCommitHash != "abc123" {
		t.Fatalf("last commit hash = %q", snapshot.LastCommitHash)
	}

	data, err := os.ReadFile(filepath.Join(paths.RunsDir, "0001", "execution.json"))
	if err != nil {
		t.Fatalf("read execution artifact: %v", err)
	}
	if !strings.Contains(string(data), "\"duration_ms\": 1250") {
		t.Fatalf("execution artifact missing duration: %s", string(data))
	}
	if !strings.Contains(string(data), "\"provider\": \"codex\"") {
		t.Fatalf("execution artifact missing provider: %s", string(data))
	}
}

func TestRunEmitsProgressEventsOnSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	_, _ = prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Added parser guardrails",
				NextInstruction: "Improve diagnostics around failures",
				Status:          "continue",
				CommitMessage:   "add parser guardrails",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 10, Deleted: 2}},
		commitHashes: []string{"abc123"},
	}
	var events []RunProgressEvent
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		Progress: func(event RunProgressEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}

	wantPhases := []RunProgressPhase{
		RunProgressPhaseRunStart,
		RunProgressPhaseIterationStart,
		RunProgressPhaseAgentExecutionStart,
		RunProgressPhaseIterationSuccess,
		RunProgressPhaseRunFinish,
	}
	if len(events) != len(wantPhases) {
		t.Fatalf("event count = %d, want %d", len(events), len(wantPhases))
	}
	for i, want := range wantPhases {
		if events[i].Phase != want {
			t.Fatalf("event %d phase = %q, want %q", i, events[i].Phase, want)
		}
	}
	if events[1].Iteration != 1 || events[2].Iteration != 1 || events[3].Iteration != 1 {
		t.Fatalf("iteration number not propagated across events: %#v", events[1:4])
	}
	if events[3].Summary != "Added parser guardrails" {
		t.Fatalf("success summary = %q", events[3].Summary)
	}
	if events[3].Status != "continue" {
		t.Fatalf("success status = %q", events[3].Status)
	}
	if events[4].CompletedIterations != 1 {
		t.Fatalf("finish completed iterations = %d", events[4].CompletedIterations)
	}
	if events[4].Status != "continue" || events[4].Err != nil {
		t.Fatalf("finish event = %#v", events[4])
	}
	if events[4].StoppedOnDone {
		t.Fatalf("finish event should not mark stop-on-done: %#v", events[4])
	}
	if report.DurationMS != 0 {
		t.Fatalf("report duration_ms = %d", report.DurationMS)
	}
}

func TestRunProgressEmitterAddsPhaseTiming(t *testing.T) {
	var events []RunProgressEvent
	emitter := newRunProgressEmitter(sequencedClock(
		time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 25, 12, 0, 0, int(150*time.Millisecond), time.UTC),
		time.Date(2026, 3, 25, 12, 0, 0, int(450*time.Millisecond), time.UTC),
		time.Date(2026, 3, 25, 12, 0, 2, int(300*time.Millisecond), time.UTC),
	), func(event RunProgressEvent) {
		events = append(events, event)
	})

	emitter.Emit(RunProgressEvent{Phase: RunProgressPhaseRunStart})
	emitter.Emit(RunProgressEvent{Phase: RunProgressPhaseIterationStart})
	emitter.Emit(RunProgressEvent{Phase: RunProgressPhaseAgentExecutionStart})
	emitter.Emit(RunProgressEvent{Phase: RunProgressPhaseIterationSuccess})

	if len(events) != 4 {
		t.Fatalf("event count = %d", len(events))
	}
	if events[0].PhaseLatencyMS != 0 || events[0].RunElapsedMS != 0 {
		t.Fatalf("run start timing = %#v", events[0])
	}
	if events[1].PhaseLatencyMS != 150 || events[1].RunElapsedMS != 150 {
		t.Fatalf("iteration start timing = %#v", events[1])
	}
	if events[2].PhaseLatencyMS != 300 || events[2].RunElapsedMS != 450 {
		t.Fatalf("agent execution timing = %#v", events[2])
	}
	if events[3].PhaseLatencyMS != 1850 || events[3].RunElapsedMS != 2300 {
		t.Fatalf("iteration success timing = %#v", events[3])
	}
}

func TestRunEmitsProgressEventsOnFailure(t *testing.T) {
	repoRoot := t.TempDir()
	_, _ = prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			err: errors.New("malformed codex JSON"),
		}},
	}
	fakeGit := &fakeGit{
		root:     repoRoot,
		dirtySeq: []bool{false, false},
	}
	var events []RunProgressEvent
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir:  repoRoot,
		Iterations:  1,
		MaxFailures: 1,
		Progress: func(event RunProgressEvent) {
			events = append(events, event)
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if report.Failures != 1 {
		t.Fatalf("failures = %d", report.Failures)
	}

	wantPhases := []RunProgressPhase{
		RunProgressPhaseRunStart,
		RunProgressPhaseIterationStart,
		RunProgressPhaseAgentExecutionStart,
		RunProgressPhaseIterationFailure,
		RunProgressPhaseRunFinish,
	}
	if len(events) != len(wantPhases) {
		t.Fatalf("event count = %d, want %d", len(events), len(wantPhases))
	}
	for i, want := range wantPhases {
		if events[i].Phase != want {
			t.Fatalf("event %d phase = %q, want %q", i, events[i].Phase, want)
		}
	}
	if events[3].Err == nil || events[3].Err.Error() != "malformed codex JSON" {
		t.Fatalf("failure error = %#v", events[3].Err)
	}
	if events[4].Err == nil {
		t.Fatalf("run finish should report the terminal error")
	}
}

func TestRunStopsOnDoneWithoutChanges(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Confirmed no materially useful work remains",
				NextInstruction: "No materially useful work remains.",
				Status:          "done",
				CommitMessage:   "record completion",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:     repoRoot,
		dirtySeq: []bool{false},
		statsSeq: []git.DiffStats{{}},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir:   repoRoot,
		UntilDone:    true,
		UntilDoneSet: true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if report.LastStatus != "done" {
		t.Fatalf("last status = %q", report.LastStatus)
	}
	if !report.StoppedOnDone {
		t.Fatalf("expected stop-on-done report flag")
	}
	if len(fakeGit.commitMessages) != 0 {
		t.Fatalf("expected no commits, got %#v", fakeGit.commitMessages)
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if snapshot.ConsecutiveNoChange != 1 {
		t.Fatalf("consecutive no change = %d", snapshot.ConsecutiveNoChange)
	}
}

func TestRunContinuesPastDoneWhenIterationsRemain(t *testing.T) {
	repoRoot := t.TempDir()
	_, _ = prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{
			{
				result: agent.Result{
					Summary:         "Thought the loop was complete early",
					NextInstruction: "If anything remains, keep pushing on weak spots.",
					Status:          "done",
					CommitMessage:   "record checkpoint",
				},
			},
			{
				result: agent.Result{
					Summary:         "Found one more worthwhile cleanup",
					NextInstruction: "Tighten any remaining rough edges.",
					Status:          "continue",
					CommitMessage:   "tighten rough edges",
				},
			},
		},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"", "diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{}, {Files: 1, Added: 2, Deleted: 1}},
		commitHashes: []string{"abc123"},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 2,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 2 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if report.LastStatus != "continue" {
		t.Fatalf("last status = %q", report.LastStatus)
	}
	if report.StoppedOnDone {
		t.Fatalf("did not expect stop-on-done report flag")
	}
}

func TestRunStopsAfterFailureThreshold(t *testing.T) {
	repoRoot := t.TempDir()
	_, _ = prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			err: errors.New("malformed codex JSON"),
		}},
	}
	fakeGit := &fakeGit{
		root:     repoRoot,
		dirtySeq: []bool{false, false},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir:  repoRoot,
		Iterations:  2,
		MaxFailures: 1,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if report.Failures != 1 {
		t.Fatalf("failures = %d", report.Failures)
	}
}

func TestRunSurfacesClaudeWrapperDrift(t *testing.T) {
	repoRoot := t.TempDir()
	cfg, paths := prepareInitializedRepo(t, repoRoot)
	cfg.Agent.Provider = "claude"
	cfg.Claude.PermissionMode = "bypassPermissions"
	if err := config.Save(paths.ConfigPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	runner := &fakeRunner{
		version: "1.0.79",
		runs: []fakeRun{{
			err: errors.Join(errors.New("claude wrapper drift detected"), claude.ErrMalformedOutputEnvelope),
		}},
	}
	fakeGit := &fakeGit{
		root:     repoRoot,
		dirtySeq: []bool{false, false},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(nil, runner),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir:  repoRoot,
		Iterations:  1,
		MaxFailures: 1,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Wrapper drift detected: Claude output envelope was malformed.") {
		t.Fatalf("run output missing wrapper drift note:\n%s", joined)
	}
	if !strings.Contains(joined, "claude wrapper drift detected") {
		t.Fatalf("run output missing wrapper drift failure text:\n%s", joined)
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if snapshot.LastFailure != "Wrapper drift detected: Claude output envelope was malformed." {
		t.Fatalf("last failure = %q", snapshot.LastFailure)
	}
	if snapshot.LastFailureRunDirectory != filepath.Join(paths.RunsDir, "0001") {
		t.Fatalf("last failure run dir = %q", snapshot.LastFailureRunDirectory)
	}

	manifest, ok, hints, err := readBundleManifest(filepath.Join(paths.RunsDir, "0001"))
	if err != nil {
		t.Fatalf("read bundle manifest: %v", err)
	}
	if !ok {
		t.Fatal("expected failed run bundle manifest")
	}
	if len(hints) != 0 {
		t.Fatalf("unexpected manifest hints: %#v", hints)
	}
	if manifest.Mode != bundleModeFailed {
		t.Fatalf("bundle mode = %q", manifest.Mode)
	}
	gotArtifacts := make([]string, 0, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		gotArtifacts = append(gotArtifacts, artifact.Name)
	}
	wantArtifacts := []string{"execution.json", "prompt.txt", "stderr.log", "stdout.log"}
	if strings.Join(gotArtifacts, ",") != strings.Join(wantArtifacts, ",") {
		t.Fatalf("manifest artifacts = %#v", gotArtifacts)
	}
}

func TestRunUsesConfigUntilDoneWhenFlagIsUnset(t *testing.T) {
	repoRoot := t.TempDir()
	cfg, paths := prepareInitializedRepo(t, repoRoot)
	cfg.Run.UntilDone = true
	if err := config.Save(paths.ConfigPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{
			{
				result: agent.Result{
					Summary:         "Added a reliability check",
					NextInstruction: "Improve diagnostics around failures",
					Status:          "continue",
					CommitMessage:   "add reliability check",
				},
			},
			{
				result: agent.Result{
					Summary:         "Confirmed the loop is complete",
					NextInstruction: "No materially useful work remains.",
					Status:          "done",
					CommitMessage:   "record completion",
				},
			},
		},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file", ""},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 8, Deleted: 2}, {}},
		commitHashes: []string{"abc123"},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(runner, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 2 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}
	if report.LastStatus != "done" {
		t.Fatalf("last status = %q", report.LastStatus)
	}
}

func TestRunDryRunIgnoresRoomStateOnCleanRepo(t *testing.T) {
	t.Parallel()

	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	svc := NewService(Dependencies{
		Git:       git.NewClient(),
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	if _, err := svc.Init(context.Background(), InitOptions{WorkingDir: repoRoot}); err != nil {
		t.Fatalf("init: %v", err)
	}

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.CompletedIterations != 1 {
		t.Fatalf("completed iterations = %d", report.CompletedIterations)
	}

	status, err := svc.Status(context.Background(), StatusOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Dirty {
		t.Fatalf("expected ROOM state to be ignored by status")
	}
}

func TestRunSkipsPastArchivedBundlesWhenStateLags(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	snapshot.CurrentIteration = 2
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save lagging state: %v", err)
	}

	existingRunDir := filepath.Join(paths.RunsDir, "0007")
	if err := fsutil.EnsureDir(existingRunDir); err != nil {
		t.Fatalf("ensure archived run dir: %v", err)
	}
	const preservedPrompt = "older tape should stay intact\n"
	if err := os.WriteFile(filepath.Join(existingRunDir, "prompt.txt"), []byte(preservedPrompt), 0o644); err != nil {
		t.Fatalf("write archived prompt: %v", err)
	}

	svc := NewService(Dependencies{
		Git:       &fakeGit{root: repoRoot, dirtySeq: []bool{false}},
		Providers: testProviders(&fakeRunner{version: "codex-cli 0.116.0"}, nil),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
		DryRun:     true,
		NoCommit:   true,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantRunDir := filepath.Join(paths.RunsDir, "0008")
	if report.LastRunDir != wantRunDir {
		t.Fatalf("last run dir = %q, want %q", report.LastRunDir, wantRunDir)
	}
	if !containsLine(report.Lines, "Run archive was ahead of state (2 vs bundle 0007); routing this pass to iteration 8 to avoid overwriting tape.") {
		t.Fatalf("run lines missing archive drift note:\n%s", strings.Join(report.Lines, "\n"))
	}

	archivedPrompt, err := os.ReadFile(filepath.Join(existingRunDir, "prompt.txt"))
	if err != nil {
		t.Fatalf("read archived prompt: %v", err)
	}
	if string(archivedPrompt) != preservedPrompt {
		t.Fatalf("archived prompt was overwritten: %q", string(archivedPrompt))
	}

	if _, err := os.Stat(filepath.Join(wantRunDir, "prompt.txt")); err != nil {
		t.Fatalf("expected new prompt in %s: %v", wantRunDir, err)
	}

	updated, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if updated.CurrentIteration != 8 {
		t.Fatalf("current iteration = %d, want 8", updated.CurrentIteration)
	}
}

func TestRunUsesClaudeProviderWhenConfigured(t *testing.T) {
	repoRoot := t.TempDir()
	cfg, paths := prepareInitializedRepo(t, repoRoot)
	cfg.Agent.Provider = "claude"
	cfg.Claude.Binary = "claude"
	cfg.Claude.PermissionMode = "bypassPermissions"
	if err := config.Save(paths.ConfigPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	claudeRunner := &fakeRunner{
		version: "1.0.79",
		runs: []fakeRun{{
			result: agent.Result{
				Summary:         "Added Claude-specific diagnostics",
				NextInstruction: "Improve failure summaries",
				Status:          "continue",
				CommitMessage:   "add diagnostics",
			},
		}},
	}
	fakeGit := &fakeGit{
		root:         repoRoot,
		dirtySeq:     []bool{false},
		diffSeq:      []string{"diff --git a/file b/file"},
		statsSeq:     []git.DiffStats{{Files: 1, Added: 3, Deleted: 1}},
		commitHashes: []string{"c1aude"},
	}
	svc := NewService(Dependencies{
		Git:       fakeGit,
		Providers: testProviders(nil, claudeRunner),
		Now:       fixedClock(),
		Version:   version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		Iterations: 1,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Provider != "claude" {
		t.Fatalf("provider = %q", report.Provider)
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if snapshot.LastProvider != "claude" {
		t.Fatalf("last provider = %q", snapshot.LastProvider)
	}
	if snapshot.LastProviderVersion != "1.0.79" {
		t.Fatalf("last provider version = %q", snapshot.LastProviderVersion)
	}
}

func prepareInitializedRepo(t *testing.T, repoRoot string) (config.Config, config.Paths) {
	t.Helper()

	cfg := config.Default()
	paths := config.ResolvePaths(repoRoot, filepath.Join(repoRoot, config.DefaultConfigRelPath), cfg)
	for _, dir := range []string{paths.RoomDir, paths.RunsDir} {
		if err := fsutil.EnsureDir(dir); err != nil {
			t.Fatalf("ensure dir %s: %v", dir, err)
		}
	}
	if err := config.Save(paths.ConfigPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := agent.WriteSchema(paths.SchemaPath); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	if err := os.WriteFile(paths.InstructionPath, []byte("Tighten parser reliability\n"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}
	if err := os.WriteFile(paths.SummariesPath, nil, 0o644); err != nil {
		t.Fatalf("write summaries: %v", err)
	}
	if err := os.WriteFile(paths.SeenInstructionsPath, nil, 0o644); err != nil {
		t.Fatalf("write seen instructions: %v", err)
	}

	snapshot := state.New("dev", fixedClock()())
	snapshot.CurrentInstructionHash = state.InstructionHash("Tighten parser reliability")
	if err := state.Save(paths.StatePath, snapshot); err != nil {
		t.Fatalf("save state: %v", err)
	}
	return cfg, paths
}

func fixedClock() Clock {
	return func() time.Time {
		return time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	}
}

func sequencedClock(times ...time.Time) Clock {
	if len(times) == 0 {
		return fixedClock()
	}
	index := 0
	return func() time.Time {
		if index >= len(times) {
			return times[len(times)-1]
		}
		value := times[index]
		index++
		return value
	}
}

func testProviders(codexRunner, claudeRunner agent.Runner) map[string]agent.Runner {
	providers := map[string]agent.Runner{}
	if codexRunner != nil {
		providers["codex"] = codexRunner
	}
	if claudeRunner != nil {
		providers["claude"] = claudeRunner
	}
	return providers
}

func initGitRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	runGit(t, repoRoot, "config", "user.name", "Test User")
	runGit(t, repoRoot, "config", "user.email", "test@example.com")
	return repoRoot
}

func writeRepoFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeLinkedWorktreeGitDir(t *testing.T, repoRoot string) string {
	t.Helper()

	adminRoot := t.TempDir()
	commonDir := filepath.Join(adminRoot, "main.git")
	gitDir := filepath.Join(commonDir, "worktrees", "room")
	if err := os.MkdirAll(filepath.Join(commonDir, "info"), 0o755); err != nil {
		t.Fatalf("mkdir common info: %v", err)
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir gitdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatalf("write commondir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}
	return filepath.Join(commonDir, "info", "exclude")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestFaultFragmentKeepsTail(t *testing.T) {
	raw := "boot\nphase a\nphase b\nphase c\nphase d\nphase e\nphase f\nterminal overload\n"

	got := faultFragment(raw)

	if strings.Contains(got, "boot") {
		t.Fatalf("expected old lines trimmed from %q", got)
	}
	for _, want := range []string{"phase b", "terminal overload"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fragment missing %q in %q", want, got)
		}
	}
}
