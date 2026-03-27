package app

import (
	"context"
	"strings"

	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/prompt"
)

type preparedPrompt struct {
	currentInstruction string
	recoveryHint       string
	recentSummaries    []logs.SummaryEntry
	priorInstructions  []string
	recentCommits      []git.Commit
	gitStatus          string
	body               string
	stats              prompt.BuildReport
}

func (s *Service) preparePrompt(ctx context.Context, repoRoot string, cfg config.Config, paths config.Paths, requireInstruction bool) (preparedPrompt, error) {
	var (
		currentInstruction string
		instructionHint    string
		err                error
	)
	if requireInstruction {
		currentInstruction, err = requireInstructionSignal(paths.InstructionPath)
		if err != nil {
			return preparedPrompt{}, err
		}
	} else {
		instructionSignal, loadErr := loadInstructionSignal(paths.InstructionPath)
		if loadErr != nil {
			return preparedPrompt{}, loadErr
		}
		currentInstruction = instructionSignal.Body
		instructionHint = instructionSignal.Hint
	}

	recentSummaries, err := logs.ReadRecentSummaries(paths.SummariesPath, cfg.Prompt.MaxRecentSummaries)
	if err != nil {
		return preparedPrompt{}, err
	}
	priorInstructions, err := logs.ReadSeenInstructions(paths.SeenInstructionsPath, cfg.Prompt.MaxSeenInstructions)
	if err != nil {
		return preparedPrompt{}, err
	}
	assessment, err := assessNewestBundle(paths.RunsDir)
	if err != nil {
		return preparedPrompt{}, err
	}
	recentCommits, err := s.git.RecentCommits(ctx, repoRoot, 10)
	if err != nil {
		return preparedPrompt{}, err
	}
	gitStatus, err := s.git.StatusShort(ctx, repoRoot)
	if err != nil {
		return preparedPrompt{}, err
	}

	recoveryHint := strings.TrimSpace(assessment.Hint)
	if strings.TrimSpace(instructionHint) != "" {
		if recoveryHint == "" {
			recoveryHint = instructionHint
		} else {
			recoveryHint += "\n" + instructionHint
		}
	}
	promptHistoryHint, promptHistoryReplacement := promptHistorySignal(currentInstruction, priorInstructions, recentSummaries, recentCommits)
	if promptHistoryHint != "" {
		if recoveryHint != "" {
			recoveryHint += "\n"
		}
		recoveryHint += promptHistoryHint
		if strings.TrimSpace(promptHistoryReplacement) != "" {
			recoveryHint += "\n" + promptHistoryReplacement
		}
	}

	body, stats := prompt.BuildDetailed(prompt.BuildInput{
		CurrentInstruction: currentInstruction,
		RecoveryHint:       recoveryHint,
		RecentSummaries:    recentSummaries,
		PriorInstructions:  priorInstructions,
		RecentCommits:      recentCommits,
		GitStatus:          gitStatus,
		RepoPath:           repoRoot,
	})

	return preparedPrompt{
		currentInstruction: currentInstruction,
		recoveryHint:       recoveryHint,
		recentSummaries:    recentSummaries,
		priorInstructions:  priorInstructions,
		recentCommits:      recentCommits,
		gitStatus:          gitStatus,
		body:               body,
		stats:              stats,
	}, nil
}
