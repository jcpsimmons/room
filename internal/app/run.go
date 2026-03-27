package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/prompt"
	"github.com/jcpsimmons/room/internal/state"
)

type RunOptions struct {
	WorkingDir      string
	Iterations      int
	UntilDone       bool
	UntilDoneSet    bool
	MaxFailures     int
	NoCommit        bool
	AllowDirty      bool
	AllowDirtySet   bool
	DryRun          bool
	Verbose         bool
	VerboseSet      bool
	JSON            bool
	JSONSet         bool
	InstructionFile string
	ConfigPath      string
	CommitPrefix    string
	Progress        func(RunProgressEvent)
}

type RunProgressPhase string

const (
	RunProgressPhaseRunStart            RunProgressPhase = "run_start"
	RunProgressPhaseIterationStart      RunProgressPhase = "iteration_start"
	RunProgressPhaseAgentExecutionStart RunProgressPhase = "agent_execution_start"
	RunProgressPhaseIterationSuccess    RunProgressPhase = "iteration_success"
	RunProgressPhaseIterationFailure    RunProgressPhase = "iteration_failure"
	RunProgressPhaseRunFinish           RunProgressPhase = "run_finish"
)

type RunProgressEvent struct {
	Phase               RunProgressPhase `json:"phase"`
	RepoRoot            string           `json:"repo_root"`
	Provider            string           `json:"provider"`
	Model               string           `json:"model"`
	RequestedIterations int              `json:"requested_iterations"`
	CompletedIterations int              `json:"completed_iterations"`
	Failures            int              `json:"failures"`
	Iteration           int              `json:"iteration"`
	RunDir              string           `json:"run_dir"`
	PromptPath          string           `json:"prompt_path"`
	Status              string           `json:"status"`
	StoppedOnDone       bool             `json:"stopped_on_done"`
	Summary             string           `json:"summary"`
	NextInstruction     string           `json:"next_instruction"`
	CommitMessage       string           `json:"commit_message"`
	StdoutFragment      string           `json:"stdout_fragment,omitempty"`
	StderrFragment      string           `json:"stderr_fragment,omitempty"`
	DryRun              bool             `json:"dry_run"`
	CommitEnabled       bool             `json:"commit_enabled"`
	Err                 error            `json:"-"`
	EventAt             time.Time        `json:"event_at,omitempty"`
	RunStartedAt        time.Time        `json:"run_started_at,omitempty"`
	RunElapsedMS        int64            `json:"run_elapsed_ms,omitempty"`
	PhaseStartedAt      time.Time        `json:"phase_started_at,omitempty"`
	PhaseFinishedAt     time.Time        `json:"phase_finished_at,omitempty"`
	PhaseLatencyMS      int64            `json:"phase_latency_ms,omitempty"`
	StartedAt           time.Time        `json:"started_at"`
	FinishedAt          time.Time        `json:"finished_at"`
	Duration            time.Duration    `json:"duration"`
}

type RunReport struct {
	RepoRoot            string    `json:"repo_root"`
	Provider            string    `json:"provider"`
	RequestedIterations int       `json:"requested_iterations"`
	CompletedIterations int       `json:"completed_iterations"`
	Failures            int       `json:"failures"`
	LastStatus          string    `json:"last_status"`
	StoppedOnDone       bool      `json:"stopped_on_done"`
	LastRunDir          string    `json:"last_run_dir"`
	StartedAt           time.Time `json:"started_at,omitempty"`
	FinishedAt          time.Time `json:"finished_at,omitempty"`
	DurationMS          int64     `json:"duration_ms,omitempty"`
	Lines               []string  `json:"lines"`
}

type runProgressEmitter struct {
	now             Clock
	fn              func(RunProgressEvent)
	runStartedAt    time.Time
	previousEventAt time.Time
}

func newRunProgressEmitter(now Clock, fn func(RunProgressEvent)) *runProgressEmitter {
	return &runProgressEmitter{now: now, fn: fn}
}

func (e *runProgressEmitter) Emit(event RunProgressEvent) {
	if e.fn == nil {
		return
	}
	eventAt := e.now().UTC()
	if e.runStartedAt.IsZero() {
		e.runStartedAt = eventAt
	}
	phaseStartedAt := e.previousEventAt
	if phaseStartedAt.IsZero() {
		phaseStartedAt = eventAt
	}
	event.EventAt = eventAt
	event.RunStartedAt = e.runStartedAt
	event.RunElapsedMS = eventAt.Sub(e.runStartedAt).Milliseconds()
	event.PhaseStartedAt = phaseStartedAt
	event.PhaseFinishedAt = eventAt
	event.PhaseLatencyMS = eventAt.Sub(phaseStartedAt).Milliseconds()
	e.previousEventAt = eventAt
	e.fn(event)
}

func (s *Service) Run(ctx context.Context, opts RunOptions) (report RunReport, err error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return RunReport{}, err
	}
	cfg, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return RunReport{}, err
	}
	if opts.InstructionFile != "" {
		cfg.Prompt.InstructionFile = opts.InstructionFile
		paths = config.ResolvePaths(repoRoot, opts.ConfigPath, cfg)
	}
	if opts.Iterations <= 0 {
		opts.Iterations = cfg.Run.DefaultIterations
	}
	if opts.MaxFailures <= 0 {
		opts.MaxFailures = cfg.Run.MaxFailures
	}
	if !opts.UntilDoneSet {
		opts.UntilDone = cfg.Run.UntilDone
	}
	if !opts.AllowDirtySet {
		opts.AllowDirty = cfg.Run.AllowDirty
	}
	if !opts.VerboseSet {
		opts.Verbose = cfg.Output.Verbose
	}
	if !opts.JSONSet {
		opts.JSON = cfg.Output.JSON
	}
	commitEnabled := cfg.Run.Commit && !opts.NoCommit
	commitPrefix := cfg.Run.CommitPrefix
	if strings.TrimSpace(opts.CommitPrefix) != "" {
		commitPrefix = opts.CommitPrefix
	}
	report = RunReport{
		RepoRoot:            repoRoot,
		Provider:            agent.NormalizeProvider(cfg.Agent.Provider),
		RequestedIterations: opts.Iterations,
	}
	progress := opts.Progress
	emitter := newRunProgressEmitter(s.now, progress)
	runStartedAt := s.now().UTC()
	defer func() {
		finishedAt := s.now().UTC()
		report.StartedAt = runStartedAt
		report.FinishedAt = finishedAt
		report.DurationMS = finishedAt.Sub(runStartedAt).Milliseconds()
		emitter.Emit(RunProgressEvent{
			Phase:               RunProgressPhaseRunFinish,
			RepoRoot:            repoRoot,
			Provider:            report.Provider,
			RequestedIterations: report.RequestedIterations,
			CompletedIterations: report.CompletedIterations,
			Failures:            report.Failures,
			RunDir:              report.LastRunDir,
			Status:              report.LastStatus,
			StoppedOnDone:       report.StoppedOnDone,
			DryRun:              opts.DryRun,
			CommitEnabled:       commitEnabled,
			Err:                 err,
		})
	}()
	model := cfg.Codex.Model
	if agent.NormalizeProvider(cfg.Agent.Provider) == agent.ProviderClaude {
		model = cfg.Claude.Model
	}
	emitter.Emit(RunProgressEvent{
		Phase:               RunProgressPhaseRunStart,
		RepoRoot:            repoRoot,
		Provider:            report.Provider,
		Model:               model,
		RequestedIterations: opts.Iterations,
		CommitEnabled:       commitEnabled,
	})

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		return RunReport{}, err
	}
	snapshot.LastFailure = ""
	snapshot.LastFailureRunDirectory = ""

	dirty, err := s.git.IsDirty(ctx, repoRoot)
	if err != nil {
		return RunReport{}, err
	}
	if dirty && !opts.AllowDirty {
		return RunReport{}, errors.New("repository is dirty; re-run with --allow-dirty to override")
	}

	provider, providerVersion, err := s.agentVersion(ctx, cfg)
	if err != nil {
		return RunReport{}, err
	}
	snapshot.LastProvider = provider
	snapshot.LastProviderVersion = providerVersion

	releaseLock, lockNote, lockRecovery, err := s.acquireRunLock(paths.RoomDir, repoRoot, provider)
	if err != nil {
		return RunReport{}, err
	}
	defer func() {
		if releaseErr := releaseLock(); releaseErr != nil && err == nil {
			err = releaseErr
		}
	}()

	runner, err := s.runnerForProvider(provider)
	if err != nil {
		return RunReport{}, err
	}
	report.Provider = provider

	lines := []string{
		fmt.Sprintf("ROOM run in %s", repoRoot),
		fmt.Sprintf("Provider: %s", agent.DisplayName(provider)),
		fmt.Sprintf("Iterations requested: %d", opts.Iterations),
		fmt.Sprintf("Commit mode: %t", commitEnabled),
	}
	if lockNote != "" {
		lines = append(lines, lockNote)
	}

	failures := 0
	completed := 0
	stoppedOnDone := false
	for completed < opts.Iterations || opts.UntilDone {
		if ctx.Err() != nil {
			snapshot.LastStatus = "interrupted"
			snapshot.LastRunAt = s.now().UTC()
			if saveErr := s.saveState(paths.StatePath, snapshot); saveErr != nil {
				return RunReport{}, errors.Join(ctx.Err(), saveErr)
			}
			return RunReport{
				RepoRoot:            repoRoot,
				Provider:            provider,
				RequestedIterations: opts.Iterations,
				CompletedIterations: completed,
				Failures:            failures,
				LastStatus:          snapshot.LastStatus,
				LastRunDir:          snapshot.LastRunDirectory,
				Lines:               append(lines, "Interrupted."),
			}, ctx.Err()
		}

		nextIteration, archiveNote, err := nextRunIteration(paths.RunsDir, snapshot.CurrentIteration)
		if err != nil {
			return RunReport{}, err
		}
		if archiveNote != "" {
			lines = append(lines, archiveNote)
		}
		emitter.Emit(RunProgressEvent{
			Phase:               RunProgressPhaseIterationStart,
			RepoRoot:            repoRoot,
			Provider:            provider,
			RequestedIterations: opts.Iterations,
			CompletedIterations: completed,
			Failures:            failures,
			Iteration:           nextIteration,
			CommitEnabled:       commitEnabled,
		})
		runDir := filepath.Join(paths.RunsDir, fmt.Sprintf("%04d", nextIteration))
		if err := fsutil.EnsureDir(runDir); err != nil {
			return RunReport{}, err
		}

		currentInstruction, err := readTrimmed(paths.InstructionPath)
		if err != nil {
			return RunReport{}, err
		}
		summaries, err := logs.ReadRecentSummaries(paths.SummariesPath, cfg.Prompt.MaxRecentSummaries)
		if err != nil {
			return RunReport{}, err
		}
		priorInstructions, err := logs.ReadSeenInstructions(paths.SeenInstructionsPath, cfg.Prompt.MaxSeenInstructions)
		if err != nil {
			return RunReport{}, err
		}
		commits, err := s.git.RecentCommits(ctx, repoRoot, 10)
		if err != nil {
			return RunReport{}, err
		}
		gitStatus, err := s.git.StatusShort(ctx, repoRoot)
		if err != nil {
			return RunReport{}, err
		}

		promptBody := prompt.Build(prompt.BuildInput{
			CurrentInstruction: currentInstruction,
			RecentSummaries:    summaries,
			PriorInstructions:  priorInstructions,
			RecentCommits:      commits,
			GitStatus:          gitStatus,
			RepoPath:           repoRoot,
		})
		promptPath := filepath.Join(runDir, "prompt.txt")
		if err := fsutil.AtomicWriteFile(promptPath, []byte(promptBody), 0o644); err != nil {
			return RunReport{}, err
		}

		snapshot.CurrentIteration = nextIteration
		snapshot.LastRunDirectory = runDir
		snapshot.LastRunAt = s.now().UTC()
		if err := s.saveState(paths.StatePath, snapshot); err != nil {
			return RunReport{}, err
		}

		if opts.DryRun {
			lines = append(lines, fmt.Sprintf("Dry run prepared prompt for iteration %d at %s", nextIteration, promptPath))
			completed++
			snapshot.LastStatus = "dry_run"
			if err := s.saveState(paths.StatePath, snapshot); err != nil {
				return RunReport{}, err
			}
			if err := writeBundleManifest(runDir, bundleModeDryRun, []string{"prompt.txt"}, lockRecovery); err != nil {
				return RunReport{}, err
			}
			report.CompletedIterations = completed
			report.Failures = failures
			report.LastStatus = snapshot.LastStatus
			report.LastRunDir = runDir
			emitter.Emit(RunProgressEvent{
				Phase:               RunProgressPhaseIterationSuccess,
				RepoRoot:            repoRoot,
				Provider:            provider,
				RequestedIterations: opts.Iterations,
				CompletedIterations: completed,
				Failures:            failures,
				Iteration:           nextIteration,
				RunDir:              runDir,
				PromptPath:          promptPath,
				Status:              "dry_run",
				DryRun:              true,
				CommitEnabled:       commitEnabled,
			})
			if !opts.UntilDone {
				continue
			}
			break
		}

		resultPath := filepath.Join(runDir, "result.json")
		executionPath := filepath.Join(runDir, "execution.json")
		startedAt := s.now()
		emitter.Emit(RunProgressEvent{
			Phase:               RunProgressPhaseAgentExecutionStart,
			RepoRoot:            repoRoot,
			Provider:            provider,
			RequestedIterations: opts.Iterations,
			CompletedIterations: completed,
			Failures:            failures,
			Iteration:           nextIteration,
			RunDir:              runDir,
			PromptPath:          promptPath,
			StartedAt:           startedAt.UTC(),
			CommitEnabled:       commitEnabled,
		})
		execution, runErr := runner.Run(ctx, agent.Prompt{Body: promptBody}, agent.Schema{Path: paths.SchemaPath}, s.runOptionsForProvider(cfg, repoRoot), resultPath)
		finishedAt := s.now()

		if err := fsutil.AtomicWriteFile(filepath.Join(runDir, "stdout.log"), []byte(execution.Stdout), 0o644); err != nil {
			return RunReport{}, err
		}
		if err := fsutil.AtomicWriteFile(filepath.Join(runDir, "stderr.log"), []byte(execution.Stderr), 0o644); err != nil {
			return RunReport{}, err
		}
		if err := writeExecutionArtifact(executionPath, provider, execution, startedAt, finishedAt, runErr); err != nil {
			return RunReport{}, err
		}

		if ctx.Err() != nil && !execution.TimedOut {
			snapshot.LastStatus = "interrupted"
			snapshot.LastRunAt = finishedAt.UTC()
			if err := s.saveState(paths.StatePath, snapshot); err != nil {
				return RunReport{}, errors.Join(ctx.Err(), err)
			}
			lines = append(lines, fmt.Sprintf("Iteration %d interrupted.", nextIteration))
			return RunReport{
				RepoRoot:            repoRoot,
				Provider:            provider,
				RequestedIterations: opts.Iterations,
				CompletedIterations: completed,
				Failures:            failures,
				LastStatus:          snapshot.LastStatus,
				LastRunDir:          runDir,
				Lines:               lines,
			}, ctx.Err()
		}

		if runErr != nil {
			failures++
			snapshot.TotalFailures++
			snapshot.LastStatus = "failed"
			failureNote := runFailureNote(runErr)
			if failureNote == "" {
				failureNote = runErr.Error()
			}
			snapshot.LastFailure = failureNote
			snapshot.LastFailureRunDirectory = runDir
			if err := s.saveState(paths.StatePath, snapshot); err != nil {
				return RunReport{}, errors.Join(runErr, err)
			}
			emitter.Emit(RunProgressEvent{
				Phase:               RunProgressPhaseIterationFailure,
				RepoRoot:            repoRoot,
				Provider:            provider,
				RequestedIterations: opts.Iterations,
				CompletedIterations: completed,
				Failures:            failures,
				Iteration:           nextIteration,
				RunDir:              runDir,
				PromptPath:          promptPath,
				Status:              "failed",
				StdoutFragment:      faultFragment(execution.Stdout),
				StderrFragment:      faultFragment(execution.Stderr),
				Err:                 runErr,
				CommitEnabled:       commitEnabled,
				StartedAt:           startedAt.UTC(),
				FinishedAt:          finishedAt.UTC(),
				Duration:            finishedAt.Sub(startedAt),
			})
			if note := runFailureNote(runErr); note != "" {
				lines = append(lines, note)
			}
			lines = append(lines, fmt.Sprintf("Iteration %d failed: %v", nextIteration, runErr))
			unsafe, unsafeErr := s.git.IsDirty(ctx, repoRoot)
			if unsafeErr == nil && unsafe {
				lines = append(lines, "Stopping because the repository is dirty after a failed iteration.")
				return RunReport{
					RepoRoot:            repoRoot,
					Provider:            provider,
					RequestedIterations: opts.Iterations,
					CompletedIterations: completed,
					Failures:            failures,
					LastStatus:          snapshot.LastStatus,
					LastRunDir:          runDir,
					Lines:               lines,
				}, runErr
			}
			if failures >= opts.MaxFailures {
				lines = append(lines, fmt.Sprintf("Stopping after %d failures.", failures))
				return RunReport{
					RepoRoot:            repoRoot,
					Provider:            provider,
					RequestedIterations: opts.Iterations,
					CompletedIterations: completed,
					Failures:            failures,
					LastStatus:          snapshot.LastStatus,
					LastRunDir:          runDir,
					Lines:               lines,
				}, runErr
			}
			continue
		}

		diff := ""
		stats := git.DiffStats{}
		if bundle, ok := s.git.(interface {
			DiffAndStats(context.Context, string) (string, git.DiffStats, error)
		}); ok {
			diff, stats, err = bundle.DiffAndStats(ctx, repoRoot)
			if err != nil {
				return RunReport{}, err
			}
		} else {
			diff, err = s.git.Diff(ctx, repoRoot)
			if err != nil {
				return RunReport{}, err
			}
			stats, err = s.git.DiffStats(ctx, repoRoot)
			if err != nil {
				return RunReport{}, err
			}
		}
		if err := fsutil.AtomicWriteFile(filepath.Join(runDir, "diff.patch"), []byte(diff), 0o644); err != nil {
			return RunReport{}, err
		}

		commitHash := ""
		if stats.Files > 0 {
			snapshot.ConsecutiveNoChange = 0
			if stats.Added+stats.Deleted <= 4 {
				snapshot.ConsecutiveTinyDiffs++
			} else {
				snapshot.ConsecutiveTinyDiffs = 0
			}
			if commitEnabled {
				message := git.NormalizeCommitMessage(commitPrefix, execution.Result.CommitMessage)
				commitHash, err = s.git.CommitAll(ctx, repoRoot, message)
				if err != nil {
					return RunReport{}, err
				}
				snapshot.LastCommitHash = strings.TrimSpace(commitHash)
			}
		} else {
			snapshot.ConsecutiveNoChange++
		}

		dedupe := prompt.DetectStagnation(prompt.DedupeInput{
			NextInstruction:      execution.Result.NextInstruction,
			PriorInstructions:    priorInstructions,
			RecentSummaries:      summaries,
			RecentCommits:        commits,
			ConsecutiveNoChange:  snapshot.ConsecutiveNoChange,
			ConsecutiveTinyDiffs: snapshot.ConsecutiveTinyDiffs,
		})
		nextInstruction := strings.TrimSpace(execution.Result.NextInstruction)
		statusValue := execution.Result.Status
		if cfg.Prompt.ForcePivotOnDuplicate && dedupe.ShouldPivot {
			nextInstruction = dedupe.Replacement
			statusValue = "pivot"
		}

		if err := fsutil.AtomicWriteFile(paths.InstructionPath, []byte(nextInstruction+"\n"), 0o644); err != nil {
			return RunReport{}, err
		}
		if err := logs.AppendSeenInstruction(paths.SeenInstructionsPath, nextInstruction); err != nil {
			return RunReport{}, err
		}
		if err := logs.AppendSummary(paths.SummariesPath, logs.SummaryEntry{
			Iteration:    nextIteration,
			Timestamp:    s.now().UTC(),
			Status:       statusValue,
			Summary:      execution.Result.Summary,
			CommitHash:   strings.TrimSpace(commitHash),
			ChangedFiles: stats.Files,
			LinesAdded:   stats.Added,
			LinesDeleted: stats.Deleted,
		}); err != nil {
			return RunReport{}, err
		}

		snapshot.TotalSuccessfulIterations++
		snapshot.LastStatus = statusValue
		snapshot.LastFailure = ""
		snapshot.LastFailureRunDirectory = ""
		snapshot.LastSummary = execution.Result.Summary
		snapshot.LastNextInstruction = nextInstruction
		snapshot.CurrentInstructionHash = state.InstructionHash(nextInstruction)
		snapshot.RoomVersion = s.version.Version
		if err := s.saveState(paths.StatePath, snapshot); err != nil {
			return RunReport{}, err
		}
		if err := writeBundleManifest(runDir, bundleModeExecuted, []string{
			"prompt.txt",
			"execution.json",
			"stdout.log",
			"stderr.log",
			"result.json",
			"diff.patch",
		}, lockRecovery); err != nil {
			return RunReport{}, err
		}

		line := fmt.Sprintf("Iteration %d [%s]: %s", nextIteration, statusValue, execution.Result.Summary)
		if opts.Verbose && dedupe.ShouldPivot {
			line += fmt.Sprintf(" | forced pivot: %s", strings.Join(dedupe.Reasons, "; "))
		}
		lines = append(lines, line)
		completed++
		report.CompletedIterations = completed
		report.Failures = failures
		report.LastStatus = statusValue
		report.StoppedOnDone = false
		report.LastRunDir = runDir
		emitter.Emit(RunProgressEvent{
			Phase:               RunProgressPhaseIterationSuccess,
			RepoRoot:            repoRoot,
			Provider:            provider,
			RequestedIterations: opts.Iterations,
			CompletedIterations: completed,
			Failures:            failures,
			Iteration:           nextIteration,
			RunDir:              runDir,
			PromptPath:          promptPath,
			Status:              statusValue,
			Summary:             execution.Result.Summary,
			NextInstruction:     nextInstruction,
			CommitMessage:       execution.Result.CommitMessage,
			DryRun:              false,
			CommitEnabled:       commitEnabled,
			StartedAt:           startedAt.UTC(),
			FinishedAt:          finishedAt.UTC(),
			Duration:            finishedAt.Sub(startedAt),
		})

		if statusValue == "done" && opts.UntilDone {
			stoppedOnDone = true
			report.StoppedOnDone = true
			lines = append(lines, "Agent reported done. Stopping.")
			break
		}
		if !opts.UntilDone && completed >= opts.Iterations {
			break
		}
	}

	return RunReport{
		RepoRoot:            repoRoot,
		Provider:            provider,
		RequestedIterations: opts.Iterations,
		CompletedIterations: completed,
		Failures:            failures,
		LastStatus:          snapshot.LastStatus,
		StoppedOnDone:       stoppedOnDone,
		LastRunDir:          snapshot.LastRunDirectory,
		Lines:               lines,
	}, nil
}

func (s *Service) requireRepo(ctx context.Context, workingDir string) (string, error) {
	ok, err := s.git.IsRepo(ctx, workingDir)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("current directory is not a git repository")
	}
	return s.git.Root(ctx, workingDir)
}

func (s *Service) loadConfig(ctx context.Context, repoRoot, override string) (config.Config, config.Paths, error) {
	path := override
	if path == "" {
		path = filepath.Join(repoRoot, config.DefaultConfigRelPath)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, config.Paths{}, err
	}
	paths := config.ResolvePaths(repoRoot, path, cfg)
	if err := config.ValidatePaths(paths); err != nil {
		return config.Config{}, config.Paths{}, err
	}
	if !fsutil.FileExists(paths.SchemaPath) {
		if err := agent.WriteSchema(paths.SchemaPath); err != nil {
			return config.Config{}, config.Paths{}, err
		}
	}
	return cfg, paths, nil
}

func readTrimmed(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func nextRunIteration(runsDir string, stateIteration int) (int, string, error) {
	next := stateIteration + 1
	bundles, err := runBundles(runsDir)
	if err != nil {
		return 0, "", err
	}
	if len(bundles) == 0 {
		return next, "", nil
	}
	latest := bundles[0].seq
	if latest < next {
		return next, "", nil
	}
	return latest + 1, fmt.Sprintf(
		"Run archive was ahead of state (%d vs bundle %04d); routing this pass to iteration %d to avoid overwriting tape.",
		stateIteration,
		latest,
		latest+1,
	), nil
}

func indent(value string) string {
	if strings.TrimSpace(value) == "" {
		return "  "
	}
	return "  " + strings.ReplaceAll(value, "\n", "\n  ")
}

func runFailureNote(err error) string {
	if errors.Is(err, claude.ErrMalformedOutputEnvelope) {
		return "Wrapper drift detected: Claude output envelope was malformed."
	}
	return ""
}

func faultFragment(raw string) string {
	const (
		maxLines = 6
		maxChars = 280
	)

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r\t ")
	}
	fragment := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(fragment) <= maxChars {
		return fragment
	}
	return strings.TrimSpace(fragment[len(fragment)-maxChars:])
}
