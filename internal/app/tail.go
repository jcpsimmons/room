package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/git"
)

type TailOptions struct {
	WorkingDir string
	ConfigPath string
}

type TailReport struct {
	RepoRoot             string                `json:"repo_root"`
	RunDir               string                `json:"run_dir"`
	BundleMode           string                `json:"bundle_mode,omitempty"`
	BundleIntegrity      string                `json:"bundle_integrity,omitempty"`
	BundleRecovery       string                `json:"bundle_recovery,omitempty"`
	BundleIntegrityHints []BundleIntegrityHint `json:"bundle_integrity_hints,omitempty"`
	Execution            *ExecutionReport      `json:"execution,omitempty"`
	Progress             *ProgressReport       `json:"progress,omitempty"`
	Prompt               string                `json:"prompt"`
	Result               *agent.Result         `json:"result,omitempty"`
	Diff                 git.DiffStats         `json:"diff"`
	Lines                []string              `json:"lines"`
}

func (s *Service) Tail(ctx context.Context, opts TailOptions) (TailReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return TailReport{}, err
	}
	_, paths, err := s.loadConfig(ctx, repoRoot, opts.ConfigPath)
	if err != nil {
		return TailReport{}, err
	}

	assessment, err := assessNewestBundle(paths.RunsDir)
	if err != nil {
		return TailReport{}, err
	}
	if assessment.RunDir == "" {
		return TailReport{}, fmt.Errorf("no ROOM run bundles found in %s", paths.RunsDir)
	}
	runDir := assessment.RunDir

	promptPath := filepath.Join(runDir, "prompt.txt")
	promptBody, promptOK, err := readTailPrompt(promptPath)
	if err != nil {
		return TailReport{}, err
	}

	result, hasResult, resultWarn, err := readTailResultLenient(filepath.Join(runDir, "result.json"))
	if err != nil {
		return TailReport{}, err
	}
	if resultWarn != nil {
		appendArtifactDecodeWarning(&assessment, "newest bundle", "result.json", resultWarn)
	}
	execution, hasExecution, executionWarn, err := readExecutionArtifactLenient(filepath.Join(runDir, "execution.json"))
	if err != nil {
		return TailReport{}, err
	}
	if executionWarn != nil {
		appendArtifactDecodeWarning(&assessment, "newest bundle", "execution.json", executionWarn)
	}
	progress, hasProgress, progressWarn, err := readProgressArtifactLenient(filepath.Join(runDir, "progress.jsonl"))
	if err != nil {
		return TailReport{}, err
	}
	if progressWarn != nil {
		appendArtifactDecodeWarning(&assessment, "newest bundle", "progress.jsonl", progressWarn)
	}
	stats, hasStats, err := readTailDiffStats(filepath.Join(runDir, "diff.patch"))
	if err != nil {
		return TailReport{}, err
	}

	lines := []string{
		fmt.Sprintf("Latest ROOM bundle: %s", runDir),
		fmt.Sprintf("Bundle mode: %s", assessment.Mode),
		fmt.Sprintf("Bundle integrity: %s", assessment.Integrity),
	}
	if assessment.Hint != "" {
		lines = append(lines, assessment.Hint)
	}
	if len(assessment.Hints) > 0 {
		lines = append(lines, fmt.Sprintf("Bundle integrity hints: %s", manifestHintsJSON(assessment.Hints)))
	}
	if assessment.Recovery != "" {
		lines = append(lines, fmt.Sprintf("Stale-lock recovery: %s", assessment.Recovery))
	}
	lines = append(lines, executionLines(execution, hasExecution)...)
	lines = append(lines, progressLines(progress, hasProgress)...)
	lines = append(lines,
		"Prompt:",
	)
	if promptOK {
		lines = append(lines, indent(strings.TrimSpace(string(promptBody))))
	} else {
		lines = append(lines, indent("unavailable"))
	}
	lines = append(lines,
		"Result:",
	)
	if hasResult {
		lines = append(lines,
			indent(fmt.Sprintf("summary: %s", result.Summary)),
			indent(fmt.Sprintf("status: %s", result.Status)),
			indent(fmt.Sprintf("next instruction: %s", result.NextInstruction)),
			indent(fmt.Sprintf("commit message: %s", result.CommitMessage)),
		)
	} else {
		lines = append(lines, indent("unavailable"))
	}
	lines = append(lines, "Diff stats:")
	if hasStats {
		lines = append(lines,
			indent(fmt.Sprintf("files changed: %d", stats.Files)),
			indent(fmt.Sprintf("insertions: %d", stats.Added)),
			indent(fmt.Sprintf("deletions: %d", stats.Deleted)),
		)
	} else {
		lines = append(lines, indent("unavailable"))
	}

	return TailReport{
		RepoRoot:             repoRoot,
		RunDir:               runDir,
		BundleMode:           string(assessment.Mode),
		BundleIntegrity:      assessment.Integrity,
		BundleRecovery:       assessment.Recovery,
		BundleIntegrityHints: assessment.Hints,
		Execution:            executionReportIfPresent(execution, hasExecution),
		Progress:             progressReportIfPresent(progress, hasProgress),
		Prompt:               strings.TrimSpace(string(promptBody)),
		Result:               resultIfPresent(result, hasResult),
		Diff:                 statsIfPresent(stats, hasStats),
		Lines:                lines,
	}, nil
}

func readTailPrompt(path string) ([]byte, bool, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, nil
	}
	return data, true, nil
}

func latestRunBundle(runsDir string) (string, error) {
	bundles, err := runBundles(runsDir)
	if err != nil {
		return "", err
	}
	if len(bundles) == 0 {
		return "", fmt.Errorf("no ROOM run bundles found in %s", runsDir)
	}
	return bundles[0].path, nil
}

type runBundle struct {
	name string
	seq  int
	path string
}

func runBundles(runsDir string) ([]runBundle, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var runs []runBundle
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		seq, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		runs = append(runs, runBundle{name: entry.Name(), seq: seq, path: filepath.Join(runsDir, entry.Name())})
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].seq == runs[j].seq {
			return runs[i].name > runs[j].name
		}
		return runs[i].seq > runs[j].seq
	})
	return runs, nil
}

func readTailResult(path string) (*agent.Result, bool, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, nil
	}

	result, err := agent.ParseResult(data)
	if err != nil {
		return nil, false, err
	}
	return &result, true, nil
}

func readTailResultLenient(path string) (*agent.Result, bool, error, error) {
	result, ok, err := readTailResult(path)
	if err == nil {
		return result, ok, nil, nil
	}
	if strings.Contains(err.Error(), "ROOM JSON result") || strings.Contains(err.Error(), "summary is required") || strings.Contains(err.Error(), "next_instruction is required") || strings.Contains(err.Error(), "status must be one of") || strings.Contains(err.Error(), "commit_message is required") {
		return nil, false, err, nil
	}
	return nil, false, nil, err
}

func readTailDiffStats(path string) (git.DiffStats, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return git.DiffStats{}, false, nil
		}
		return git.DiffStats{}, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return git.DiffStats{}, false, err
	}
	if info.Size() == 0 {
		return git.DiffStats{}, false, nil
	}
	return parseDiffPatchStats(file)
}

func statsIfPresent(stats git.DiffStats, ok bool) git.DiffStats {
	if !ok {
		return git.DiffStats{}
	}
	return stats
}

func resultIfPresent(result *agent.Result, ok bool) *agent.Result {
	if !ok {
		return nil
	}
	return result
}

func parseDiffPatchStats(r io.Reader) (git.DiffStats, bool, error) {
	var stats git.DiffStats
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			switch {
			case strings.HasPrefix(line, "diff --git "):
				stats.Files++
			case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
				stats.Added++
			case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
				stats.Deleted++
			}
		}
		if errors.Is(err, io.EOF) {
			return stats, true, nil
		}
		if err != nil {
			return git.DiffStats{}, false, err
		}
	}
}
