package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/codex"
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
	result codex.Result
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Version(_ context.Context, _ string) (string, error) {
	return f.version, nil
}

func (f *fakeRunner) Run(_ context.Context, prompt codex.Prompt, _ codex.Schema, _ codex.RunOptions, outputPath string) (codex.Execution, error) {
	if f.calls >= len(f.runs) {
		return codex.Execution{}, errors.New("unexpected run call")
	}
	run := f.runs[f.calls]
	f.prompts = append(f.prompts, prompt.Body)
	f.calls++

	if run.err == nil {
		data, err := json.Marshal(run.result)
		if err != nil {
			return codex.Execution{}, err
		}
		if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
			return codex.Execution{}, err
		}
	}

	return codex.Execution{
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
	if len(f.diffSeq) == 0 {
		return "", nil
	}
	value := f.diffSeq[0]
	f.diffSeq = f.diffSeq[1:]
	return value, nil
}
func (f *fakeGit) DiffStats(context.Context, string) (git.DiffStats, error) {
	if len(f.statsSeq) == 0 {
		return git.DiffStats{}, nil
	}
	value := f.statsSeq[0]
	f.statsSeq = f.statsSeq[1:]
	return value, nil
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

func TestRunForcesPivotOnDuplicateInstruction(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)
	if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, "Improve parser resilience"); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: codex.Result{
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
		Git:     fakeGit,
		Runner:  runner,
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
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
}

func TestRunStopsOnDoneWithoutChanges(t *testing.T) {
	repoRoot := t.TempDir()
	_, paths := prepareInitializedRepo(t, repoRoot)

	runner := &fakeRunner{
		version: "codex-cli 0.116.0",
		runs: []fakeRun{{
			result: codex.Result{
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
		Git:     fakeGit,
		Runner:  runner,
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Run(context.Background(), RunOptions{
		WorkingDir: repoRoot,
		UntilDone:  true,
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
		Git:     fakeGit,
		Runner:  runner,
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
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
				result: codex.Result{
					Summary:         "Added a reliability check",
					NextInstruction: "Improve diagnostics around failures",
					Status:          "continue",
					CommitMessage:   "add reliability check",
				},
			},
			{
				result: codex.Result{
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
		Git:     fakeGit,
		Runner:  runner,
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
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
	if err := codex.WriteSchema(paths.SchemaPath); err != nil {
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
