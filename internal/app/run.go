package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/codex"
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
	MaxFailures     int
	NoCommit        bool
	AllowDirty      bool
	DryRun          bool
	Verbose         bool
	JSON            bool
	InstructionFile string
	ConfigPath      string
	CommitPrefix    string
}

type RunReport struct {
	RepoRoot            string   `json:"repo_root"`
	RequestedIterations int      `json:"requested_iterations"`
	CompletedIterations int      `json:"completed_iterations"`
	Failures            int      `json:"failures"`
	LastStatus          string   `json:"last_status"`
	LastRunDir          string   `json:"last_run_dir"`
	Lines               []string `json:"lines"`
}

func (s *Service) Run(ctx context.Context, opts RunOptions) (RunReport, error) {
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
	if !opts.AllowDirty {
		opts.AllowDirty = cfg.Run.AllowDirty
	}
	if !opts.Verbose {
		opts.Verbose = cfg.Output.Verbose
	}
	if !opts.JSON {
		opts.JSON = cfg.Output.JSON
	}
	commitEnabled := cfg.Run.Commit && !opts.NoCommit
	commitPrefix := cfg.Run.CommitPrefix
	if strings.TrimSpace(opts.CommitPrefix) != "" {
		commitPrefix = opts.CommitPrefix
	}

	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		return RunReport{}, err
	}

	dirty, err := s.git.IsDirty(ctx, repoRoot)
	if err != nil {
		return RunReport{}, err
	}
	if dirty && !opts.AllowDirty {
		return RunReport{}, errors.New("repository is dirty; re-run with --allow-dirty to override")
	}

	codexVersion, err := s.runner.Version(ctx, cfg.Codex.Binary)
	if err != nil {
		return RunReport{}, err
	}
	snapshot.LastCodexVersion = codexVersion

	lines := []string{
		fmt.Sprintf("ROOM run in %s", repoRoot),
		fmt.Sprintf("Iterations requested: %d", opts.Iterations),
		fmt.Sprintf("Commit mode: %t", commitEnabled),
	}

	failures := 0
	completed := 0
	for completed < opts.Iterations || opts.UntilDone {
		if ctx.Err() != nil {
			snapshot.LastStatus = "interrupted"
			snapshot.LastRunAt = s.now().UTC()
			if saveErr := state.Save(paths.StatePath, snapshot); saveErr != nil {
				return RunReport{}, errors.Join(ctx.Err(), saveErr)
			}
			return RunReport{
				RepoRoot:            repoRoot,
				RequestedIterations: opts.Iterations,
				CompletedIterations: completed,
				Failures:            failures,
				LastStatus:          snapshot.LastStatus,
				LastRunDir:          snapshot.LastRunDirectory,
				Lines:               append(lines, "Interrupted."),
			}, ctx.Err()
		}

		nextIteration := snapshot.CurrentIteration + 1
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
		if err := state.Save(paths.StatePath, snapshot); err != nil {
			return RunReport{}, err
		}

		if opts.DryRun {
			lines = append(lines, fmt.Sprintf("Dry run prepared prompt for iteration %d at %s", nextIteration, promptPath))
			completed++
			if !opts.UntilDone {
				continue
			}
			break
		}

		resultPath := filepath.Join(runDir, "result.json")
		execution, runErr := s.runner.Run(ctx, codex.Prompt{Body: promptBody}, codex.Schema{Path: paths.SchemaPath}, codex.RunOptions{
			Binary:   cfg.Codex.Binary,
			WorkDir:  repoRoot,
			Model:    cfg.Codex.Model,
			Sandbox:  cfg.Codex.Sandbox,
			Approval: cfg.Codex.Approval,
			Timeout:  time.Duration(cfg.Codex.TimeoutSeconds) * time.Second,
		}, resultPath)

		if err := fsutil.AtomicWriteFile(filepath.Join(runDir, "stdout.log"), []byte(execution.Stdout), 0o644); err != nil {
			return RunReport{}, err
		}
		if err := fsutil.AtomicWriteFile(filepath.Join(runDir, "stderr.log"), []byte(execution.Stderr), 0o644); err != nil {
			return RunReport{}, err
		}

		if runErr != nil {
			failures++
			snapshot.TotalFailures++
			snapshot.LastStatus = "failed"
			if err := state.Save(paths.StatePath, snapshot); err != nil {
				return RunReport{}, errors.Join(runErr, err)
			}
			lines = append(lines, fmt.Sprintf("Iteration %d failed: %v", nextIteration, runErr))
			unsafe, unsafeErr := s.git.IsDirty(ctx, repoRoot)
			if unsafeErr == nil && unsafe {
				lines = append(lines, "Stopping because the repository is dirty after a failed iteration.")
				return RunReport{
					RepoRoot:            repoRoot,
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

		diff, err := s.git.Diff(ctx, repoRoot)
		if err != nil {
			return RunReport{}, err
		}
		if err := fsutil.AtomicWriteFile(filepath.Join(runDir, "diff.patch"), []byte(diff), 0o644); err != nil {
			return RunReport{}, err
		}
		stats, err := s.git.DiffStats(ctx, repoRoot)
		if err != nil {
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
		snapshot.LastSummary = execution.Result.Summary
		snapshot.LastNextInstruction = nextInstruction
		snapshot.CurrentInstructionHash = state.InstructionHash(nextInstruction)
		snapshot.RoomVersion = s.version.Version
		if err := state.Save(paths.StatePath, snapshot); err != nil {
			return RunReport{}, err
		}

		line := fmt.Sprintf("Iteration %d [%s]: %s", nextIteration, statusValue, execution.Result.Summary)
		if opts.Verbose && dedupe.ShouldPivot {
			line += fmt.Sprintf(" | forced pivot: %s", strings.Join(dedupe.Reasons, "; "))
		}
		lines = append(lines, line)

		completed++
		if statusValue == "done" {
			lines = append(lines, "Codex reported done. Stopping.")
			break
		}
		if !opts.UntilDone && completed >= opts.Iterations {
			break
		}
	}

	return RunReport{
		RepoRoot:            repoRoot,
		RequestedIterations: opts.Iterations,
		CompletedIterations: completed,
		Failures:            failures,
		LastStatus:          snapshot.LastStatus,
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
	if !fsutil.FileExists(paths.SchemaPath) {
		if err := codex.WriteSchema(paths.SchemaPath); err != nil {
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

func indent(value string) string {
	if strings.TrimSpace(value) == "" {
		return "  "
	}
	return "  " + strings.ReplaceAll(value, "\n", "\n  ")
}
