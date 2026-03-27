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
	version string
	calls   int
	prompts []string
	runs    []fakeRun
}

type fakeRun struct {
	result agent.Result
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Version(_ context.Context, _ string) (string, error) {
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

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
