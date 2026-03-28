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

type TapeInstructionEcho struct {
	Instruction string `json:"instruction"`
	Count       int    `json:"count"`
	Iterations  []int  `json:"iterations"`
	Consecutive bool   `json:"consecutive,omitempty"`
}

type TapeReport struct {
	RepoRoot                string                `json:"repo_root"`
	Provider                string                `json:"provider"`
	Limit                   int                   `json:"limit"`
	Entries                 []TapeEntry           `json:"entries"`
	MalformedSummaries      int                   `json:"malformed_summaries,omitempty"`
	MissingNextInstructions int                   `json:"missing_next_instructions,omitempty"`
	InstructionEchoes       []TapeInstructionEcho `json:"instruction_echoes,omitempty"`
	Lines                   []string              `json:"lines"`
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

	instructionEchoes := detectTapeInstructionEchoes(entries)
	if len(instructionEchoes) > 0 {
		lines = append(lines, fmt.Sprintf("Instruction echo: %d repeated next-instruction motif(s) detected in this window.", len(instructionEchoes)))
		for _, echo := range instructionEchoes {
			lines = append(lines, indent(formatTapeInstructionEcho(echo)))
		}
	}

	return TapeReport{
		RepoRoot:                repoRoot,
		Provider:                agent.NormalizeProvider(cfg.Agent.Provider),
		Limit:                   limit,
		Entries:                 entries,
		MalformedSummaries:      malformedSummaries,
		MissingNextInstructions: missingNextInstructions,
		InstructionEchoes:       instructionEchoes,
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

func detectTapeInstructionEchoes(entries []TapeEntry) []TapeInstructionEcho {
	type aggregate struct {
		instruction string
		iterations  []int
		consecutive bool
	}

	aggregates := make(map[string]*aggregate)
	lastByInstruction := make(map[string]int)
	order := make([]string, 0, len(entries))

	for _, entry := range entries {
		normalized := normalizeTapeInstruction(entry.NextInstruction)
		if normalized == "" {
			continue
		}
		group, ok := aggregates[normalized]
		if !ok {
			group = &aggregate{instruction: strings.TrimSpace(entry.NextInstruction)}
			aggregates[normalized] = group
			order = append(order, normalized)
		}
		group.iterations = append(group.iterations, entry.Iteration)
		if previousIteration, ok := lastByInstruction[normalized]; ok && entry.Iteration == previousIteration+1 {
			group.consecutive = true
		}
		lastByInstruction[normalized] = entry.Iteration
	}

	echoes := make([]TapeInstructionEcho, 0, len(order))
	for _, normalized := range order {
		group := aggregates[normalized]
		if len(group.iterations) < 2 {
			continue
		}
		echoes = append(echoes, TapeInstructionEcho{
			Instruction: group.instruction,
			Count:       len(group.iterations),
			Iterations:  append([]int(nil), group.iterations...),
			Consecutive: group.consecutive,
		})
	}

	return echoes
}

func normalizeTapeInstruction(instruction string) string {
	fields := strings.Fields(strings.TrimSpace(instruction))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func formatTapeInstructionEcho(echo TapeInstructionEcho) string {
	shape := fmt.Sprintf("echo x%d on %s", echo.Count, formatTapeIterations(echo.Iterations))
	if echo.Consecutive {
		shape += " (consecutive)"
	}
	return fmt.Sprintf("%s: %s", shape, shortenTapeInstruction(echo.Instruction, 96))
}

func formatTapeIterations(iterations []int) string {
	if len(iterations) == 0 {
		return "unknown steps"
	}
	parts := make([]string, 0, len(iterations))
	for _, iteration := range iterations {
		parts = append(parts, fmt.Sprintf("#%d", iteration))
	}
	return strings.Join(parts, ", ")
}

func shortenTapeInstruction(instruction string, limit int) string {
	instruction = normalizeTapeInstruction(instruction)
	if limit <= 0 || len(instruction) <= limit {
		return instruction
	}
	if limit <= 3 {
		return instruction[:limit]
	}
	return instruction[:limit-3] + "..."
}
