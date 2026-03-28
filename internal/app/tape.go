package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/logs"
)

const DefaultTapeLimit = 8

type TapeOptions struct {
	WorkingDir string
	ConfigPath string
	Limit      int
}

type TapeEntry struct {
	Iteration       int      `json:"iteration"`
	Timestamp       string   `json:"timestamp"`
	Status          string   `json:"status"`
	Summary         string   `json:"summary"`
	NextInstruction string   `json:"next_instruction,omitempty"`
	CommitHash      string   `json:"commit_hash,omitempty"`
	ChangedFiles    int      `json:"changed_files"`
	LinesAdded      int      `json:"lines_added"`
	LinesDeleted    int      `json:"lines_deleted"`
	FocusAreas      []string `json:"focus_areas,omitempty"`
}

type TapeReport struct {
	RepoRoot                string      `json:"repo_root"`
	Provider                string      `json:"provider"`
	Limit                   int         `json:"limit"`
	Entries                 []TapeEntry `json:"entries"`
	MalformedSummaries      int         `json:"malformed_summaries,omitempty"`
	MissingNextInstructions int         `json:"missing_next_instructions,omitempty"`
	Lines                   []string    `json:"lines"`
}

func (s *Service) Tape(ctx context.Context, opts TapeOptions) (TapeReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return TapeReport{}, err
	}
	cfg, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return TapeReport{}, err
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultTapeLimit
	}

	summaries, malformedSummaries, err := logs.ReadRecentSummariesDetailed(paths.SummariesPath, limit)
	if err != nil {
		return TapeReport{}, err
	}
	instructions, err := logs.ReadSeenInstructions(paths.SeenInstructionsPath, limit)
	if err != nil {
		return TapeReport{}, err
	}

	alignedInstructions, missingNextInstructions := alignTapeInstructions(summaries, instructions)
	entries := make([]TapeEntry, 0, len(summaries))
	lines := []string{
		"ROOM tape",
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Provider: %s", agent.DisplayName(cfg.Agent.Provider)),
		fmt.Sprintf("Entries: %d (limit %d)", len(summaries), limit),
	}
	if malformedSummaries > 0 {
		lines = append(lines, fmt.Sprintf("Summary log drift: %d malformed entrie(s) ignored while reading the tape.", malformedSummaries))
	}
	if missingNextInstructions > 0 {
		lines = append(lines, fmt.Sprintf("Instruction drift: %d tape step(s) are missing captured next-instruction data.", missingNextInstructions))
	}

	if len(summaries) == 0 {
		lines = append(lines, "No recorded iterations yet.")
	}

	for i, summary := range summaries {
		entry := TapeEntry{
			Iteration:       summary.Iteration,
			Timestamp:       summary.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
			Status:          summary.Status,
			Summary:         summary.Summary,
			NextInstruction: alignedInstructions[i],
			CommitHash:      strings.TrimSpace(summary.CommitHash),
			ChangedFiles:    summary.ChangedFiles,
			LinesAdded:      summary.LinesAdded,
			LinesDeleted:    summary.LinesDeleted,
			FocusAreas:      append([]string(nil), summary.FocusAreas...),
		}
		entries = append(entries, entry)

		lines = append(lines, fmt.Sprintf("#%d %s [%s] %s", entry.Iteration, entry.Timestamp, entry.Status, entry.Summary))
		lines = append(lines, indent(fmt.Sprintf("next: %s", tapeValueOrPlaceholder(entry.NextInstruction, "unavailable"))))
		lines = append(lines, indent(fmt.Sprintf("diff: %d file(s), +%d/-%d", entry.ChangedFiles, entry.LinesAdded, entry.LinesDeleted)))
		if entry.CommitHash != "" {
			lines = append(lines, indent(fmt.Sprintf("commit: %s", shortenCommitHash(entry.CommitHash))))
		}
		if len(entry.FocusAreas) > 0 {
			lines = append(lines, indent(fmt.Sprintf("focus: %s", strings.Join(entry.FocusAreas, ", "))))
		}
	}

	return TapeReport{
		RepoRoot:                repoRoot,
		Provider:                agent.NormalizeProvider(cfg.Agent.Provider),
		Limit:                   limit,
		Entries:                 entries,
		MalformedSummaries:      malformedSummaries,
		MissingNextInstructions: missingNextInstructions,
		Lines:                   lines,
	}, nil
}

func alignTapeInstructions(summaries []logs.SummaryEntry, instructions []string) ([]string, int) {
	aligned := make([]string, len(summaries))
	if len(summaries) == 0 {
		return aligned, 0
	}
	if len(instructions) > len(summaries) {
		instructions = instructions[len(instructions)-len(summaries):]
	}

	missing := len(summaries) - len(instructions)
	if missing < 0 {
		missing = 0
	}
	for i := range summaries {
		if j := i - missing; j >= 0 && j < len(instructions) {
			aligned[i] = strings.TrimSpace(instructions[j])
		}
	}
	return aligned, missing
}

func shortenCommitHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func tapeValueOrPlaceholder(value, placeholder string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return placeholder
	}
	return value
}
