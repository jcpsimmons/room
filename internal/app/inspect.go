package app

import (
	"context"

	"github.com/jcpsimmons/room/internal/config"
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
	prepared, err := s.preparePrompt(ctx, repoRoot, cfg, paths, false)
	if err != nil {
		return InspectReport{}, err
	}

	return InspectReport{
		RepoRoot:           repoRoot,
		Prompt:             prepared.body,
		PromptStats:        prepared.stats,
		CurrentInstruction: prepared.currentInstruction,
		RecoveryHint:       prepared.recoveryHint,
		RecentSummaries:    prepared.recentSummaries,
		PriorInstructions:  prepared.priorInstructions,
		RecentCommits:      prepared.recentCommits,
		GitStatus:          prepared.gitStatus,
	}, nil
}
