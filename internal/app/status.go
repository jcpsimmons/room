package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/state"
)

type StatusOptions struct {
	WorkingDir string
	ConfigPath string
}

type StatusReport struct {
	RepoRoot           string              `json:"repo_root"`
	Provider           string              `json:"provider"`
	State              state.Snapshot      `json:"state"`
	CurrentInstruction string              `json:"current_instruction"`
	RecentSummaries    []logs.SummaryEntry `json:"recent_summaries"`
	RecentCommits      []string            `json:"recent_commits"`
	Dirty              bool                `json:"dirty"`
	Lines              []string            `json:"lines"`
}

func (s *Service) Status(ctx context.Context, opts StatusOptions) (StatusReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return StatusReport{}, err
	}
	cfg, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return StatusReport{}, err
	}
	snapshot, err := state.Load(paths.StatePath)
	if err != nil {
		return StatusReport{}, err
	}
	instruction, err := os.ReadFile(paths.InstructionPath)
	if err != nil {
		return StatusReport{}, err
	}
	summaries, err := logs.ReadRecentSummaries(paths.SummariesPath, cfg.Prompt.MaxRecentSummaries)
	if err != nil {
		return StatusReport{}, err
	}
	commits, err := s.git.RecentCommitsWithPrefix(ctx, repoRoot, 5, cfg.Run.CommitPrefix)
	if err != nil {
		return StatusReport{}, err
	}
	dirty, err := s.git.IsDirty(ctx, repoRoot)
	if err != nil {
		return StatusReport{}, err
	}

	var commitLines []string
	for _, commit := range commits {
		commitLines = append(commitLines, fmt.Sprintf("%s %s", commit.Hash, commit.Subject))
	}
	if len(commitLines) == 0 {
		commitLines = append(commitLines, "none")
	}

	lines := []string{
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Provider: %s", agent.DisplayName(cfg.Agent.Provider)),
		fmt.Sprintf("Iteration: %d", snapshot.CurrentIteration),
		fmt.Sprintf("Last run: %s", formatTime(snapshot.LastRunAt)),
		fmt.Sprintf("Last status: %s", snapshot.LastStatus),
		fmt.Sprintf("Dirty worktree: %t", dirty),
		"Current instruction:",
		indent(strings.TrimSpace(string(instruction))),
		"Recent ROOM commits:",
	}
	for _, line := range commitLines {
		lines = append(lines, indent(line))
	}
	lines = append(lines, "Recent summaries:")
	if len(summaries) == 0 {
		lines = append(lines, indent("none"))
	} else {
		for _, summary := range summaries {
			lines = append(lines, indent(fmt.Sprintf("#%d [%s] %s", summary.Iteration, summary.Status, summary.Summary)))
		}
	}

	return StatusReport{
		RepoRoot:           repoRoot,
		Provider:           agent.NormalizeProvider(cfg.Agent.Provider),
		State:              snapshot,
		CurrentInstruction: strings.TrimSpace(string(instruction)),
		RecentSummaries:    summaries,
		RecentCommits:      commitLines,
		Dirty:              dirty,
		Lines:              lines,
	}, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}
