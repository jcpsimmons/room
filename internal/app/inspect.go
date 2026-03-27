package app

import (
	"context"
	"strings"

	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/prompt"
)

type InspectOptions struct {
	WorkingDir      string
	ConfigPath      string
	InstructionFile string
}

type InspectReport struct {
	RepoRoot           string              `json:"repo_root"`
	Prompt             string              `json:"prompt"`
	PromptStats        prompt.BuildReport  `json:"prompt_stats"`
	CurrentInstruction string              `json:"current_instruction"`
	RecoveryHint       string              `json:"recovery_hint,omitempty"`
	RecentSummaries    []logs.SummaryEntry `json:"recent_summaries,omitempty"`
	PriorInstructions  []string            `json:"prior_instructions,omitempty"`
	RecentCommits      []git.Commit        `json:"recent_commits,omitempty"`
	GitStatus          string              `json:"git_status,omitempty"`
}

func (s *Service) Inspect(ctx context.Context, opts InspectOptions) (InspectReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return InspectReport{}, err
	}
	cfg, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return InspectReport{}, err
	}
	if opts.InstructionFile != "" {
		cfg.Prompt.InstructionFile = opts.InstructionFile
		paths = config.ResolvePaths(repoRoot, opts.ConfigPath, cfg)
	}
	summaries, err := logs.ReadRecentSummaries(paths.SummariesPath, cfg.Prompt.MaxRecentSummaries)
	if err != nil {
		return InspectReport{}, err
	}
	priorInstructions, err := logs.ReadSeenInstructions(paths.SeenInstructionsPath, cfg.Prompt.MaxSeenInstructions)
	if err != nil {
		return InspectReport{}, err
	}
	assessment, err := assessNewestBundle(paths.RunsDir)
	if err != nil {
		return InspectReport{}, err
	}
	commits, err := s.git.RecentCommits(ctx, repoRoot, 10)
	if err != nil {
		return InspectReport{}, err
	}
	gitStatus, err := s.git.StatusShort(ctx, repoRoot)
	if err != nil {
		return InspectReport{}, err
	}
	instructionBody, err := fsutil.ReadFileIfExists(paths.InstructionPath)
	if err != nil {
		return InspectReport{}, err
	}
	currentInstruction := strings.TrimSpace(string(instructionBody))
	recoveryHint := assessment.Hint
	if !fsutil.FileExists(paths.InstructionPath) {
		missingInstructionHint := "Current instruction unavailable: missing instruction.txt."
		if strings.TrimSpace(recoveryHint) == "" {
			recoveryHint = missingInstructionHint
		} else {
			recoveryHint = recoveryHint + "\n" + missingInstructionHint
		}
	}
	promptHistoryHint, promptHistoryReplacement := promptHistorySignal(currentInstruction, priorInstructions, summaries)
	if promptHistoryHint != "" {
		if strings.TrimSpace(recoveryHint) != "" {
			recoveryHint += "\n"
		}
		recoveryHint += promptHistoryHint
		if strings.TrimSpace(promptHistoryReplacement) != "" {
			recoveryHint += "\n" + promptHistoryReplacement
		}
	}
	promptText, promptStats := prompt.BuildDetailed(prompt.BuildInput{
		CurrentInstruction: currentInstruction,
		RecoveryHint:       recoveryHint,
		RecentSummaries:    summaries,
		PriorInstructions:  priorInstructions,
		RecentCommits:      commits,
		GitStatus:          gitStatus,
		RepoPath:           repoRoot,
	})

	return InspectReport{
		RepoRoot:           repoRoot,
		Prompt:             promptText,
		PromptStats:        promptStats,
		CurrentInstruction: currentInstruction,
		RecoveryHint:       recoveryHint,
		RecentSummaries:    summaries,
		PriorInstructions:  priorInstructions,
		RecentCommits:      commits,
		GitStatus:          gitStatus,
	}, nil
}
