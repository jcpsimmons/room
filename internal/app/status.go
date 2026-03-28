package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/state"
)

type StatusOptions struct {
	WorkingDir string
	ConfigPath string
}

type StatusReport struct {
	RepoRoot                   string                `json:"repo_root"`
	Provider                   string                `json:"provider"`
	ProviderAuthStatus         string                `json:"provider_auth_status,omitempty"`
	ProviderAuthDrift          string                `json:"provider_auth_drift,omitempty"`
	InstructionDriftHint       string                `json:"instruction_drift_hint,omitempty"`
	State                      state.Snapshot        `json:"state"`
	CurrentInstruction         string                `json:"current_instruction"`
	RecentSummaries            []logs.SummaryEntry   `json:"recent_summaries"`
	RecentCommits              []string              `json:"recent_commits"`
	LatestRunDir               string                `json:"latest_run_dir,omitempty"`
	LatestBundleMode           string                `json:"latest_bundle_mode,omitempty"`
	LatestBundleIntegrity      string                `json:"latest_bundle_integrity,omitempty"`
	LatestBundleHint           string                `json:"latest_bundle_hint,omitempty"`
	LatestBundleRecovery       string                `json:"latest_bundle_recovery,omitempty"`
	LatestBundleIntegrityHints []BundleIntegrityHint `json:"latest_bundle_integrity_hints,omitempty"`
	LatestLockHint             string                `json:"latest_lock_hint,omitempty"`
	RoomIgnoreHint             string                `json:"room_ignore_hint,omitempty"`
	PromptHistoryHint          string                `json:"prompt_history_hint,omitempty"`
	LastFailure                string                `json:"last_failure,omitempty"`
	LastFailureRunDirectory    string                `json:"last_failure_run_directory,omitempty"`
	Dirty                      bool                  `json:"dirty"`
	Lines                      []string              `json:"lines"`
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
	instructionSignal, err := loadInstructionSignal(paths.InstructionPath)
	if err != nil {
		return StatusReport{}, err
	}
	summaries, err := logs.ReadRecentSummaries(paths.SummariesPath, cfg.Prompt.MaxRecentSummaries)
	if err != nil {
		return StatusReport{}, err
	}
	priorInstructions, err := logs.ReadSeenInstructions(paths.SeenInstructionsPath, cfg.Prompt.MaxSeenInstructions)
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
	assessment, err := assessNewestBundle(paths.RunsDir)
	if err != nil {
		return StatusReport{}, err
	}
	if err := appendNewestBundleArtifactWarnings(&assessment); err != nil {
		return StatusReport{}, err
	}
	latestRunDir := assessment.RunDir
	lockHint, err := runLockHint(paths.RoomDir, s.processAlive)
	if err != nil {
		return StatusReport{}, err
	}
	currentInstruction := instructionSignal.Body
	instructionHint := instructionSignal.Hint
	if instructionHint != "" {
		currentInstruction = instructionHint
	}
	drift := inspectInstructionDrift(snapshot, instructionSignal)
	providerDiag := s.providerDiagnostics(ctx, cfg)
	lastRunLine := fmt.Sprintf("Last run: %s", formatTime(snapshot.LastRunAt))
	if providerDiag.AuthDriftInline != "" {
		lastRunLine = fmt.Sprintf("%s (%s)", lastRunLine, providerDiag.AuthDriftInline)
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
		lastRunLine,
		fmt.Sprintf("Last status: %s", snapshot.LastStatus),
		fmt.Sprintf("Dirty worktree: %t", dirty),
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
	if lockHint != "" {
		lines = append(lines, lockHint)
	}
	if snapshot.LastFailure != "" {
		if snapshot.LastFailureRunDirectory != "" {
			lines = append(lines, fmt.Sprintf("Last failure in %s: %s", snapshot.LastFailureRunDirectory, snapshot.LastFailure))
		} else {
			lines = append(lines, fmt.Sprintf("Last failure: %s", snapshot.LastFailure))
		}
	}
	roomIgnoreHint, err := loadRoomIgnoreHint(repoRoot)
	if err != nil {
		return StatusReport{}, err
	}
	if roomIgnoreHint != "" {
		lines = append(lines, roomIgnoreHint)
	}
	if instructionHint != "" {
		lines = append(lines, instructionHint)
	}
	if drift.Message != "" {
		lines = append(lines, drift.Message)
	}
	promptHistoryHint, _ := promptHistorySignal(currentInstruction, priorInstructions, summaries, commits)
	if promptHistoryHint != "" {
		lines = append(lines, promptHistoryHint)
	}
	lines = append(lines,
		"Current instruction:",
		indent(currentInstruction),
		"Recent ROOM commits:",
	)
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
		RepoRoot:                   repoRoot,
		Provider:                   agent.NormalizeProvider(cfg.Agent.Provider),
		ProviderAuthStatus:         providerDiag.AuthStatus,
		ProviderAuthDrift:          providerDiag.AuthDriftInline,
		InstructionDriftHint:       drift.Message,
		State:                      snapshot,
		CurrentInstruction:         currentInstruction,
		RecentSummaries:            summaries,
		RecentCommits:              commitLines,
		LatestRunDir:               latestRunDir,
		LatestBundleMode:           string(assessment.Mode),
		LatestBundleIntegrity:      assessment.Integrity,
		LatestBundleHint:           assessment.Hint,
		LatestBundleRecovery:       assessment.Recovery,
		LatestBundleIntegrityHints: assessment.Hints,
		LatestLockHint:             lockHint,
		RoomIgnoreHint:             roomIgnoreHint,
		PromptHistoryHint:          promptHistoryHint,
		LastFailure:                snapshot.LastFailure,
		LastFailureRunDirectory:    snapshot.LastFailureRunDirectory,
		Dirty:                      dirty,
		Lines:                      lines,
	}, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

func appendNewestBundleArtifactWarnings(assessment *bundleAssessment) error {
	if assessment == nil || strings.TrimSpace(assessment.RunDir) == "" {
		return nil
	}

	if _, _, resultWarn, err := readTailResultLenient(filepath.Join(assessment.RunDir, "result.json")); err != nil {
		return err
	} else if resultWarn != nil {
		appendArtifactDecodeWarning(assessment, "newest bundle", "result.json", resultWarn)
	}

	if _, _, executionWarn, err := readExecutionArtifactLenient(filepath.Join(assessment.RunDir, "execution.json")); err != nil {
		return err
	} else if executionWarn != nil {
		appendArtifactDecodeWarning(assessment, "newest bundle", "execution.json", executionWarn)
	}

	return nil
}

func newestBundleHint(runsDir string) (string, string, error) {
	latestRunDir, err := latestRunBundle(runsDir)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no ROOM run bundles found in ") {
			return "", "", nil
		}
		return "", "", err
	}

	missing := make([]string, 0, 2)
	for _, name := range []string{"result.json", "diff.patch"} {
		if !fileExists(filepath.Join(latestRunDir, name)) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return latestRunDir, "", nil
	}

	return latestRunDir, fmt.Sprintf("Hint: newest bundle %s is incomplete; missing %s.", filepath.Base(latestRunDir), strings.Join(missing, " and ")), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loadRoomIgnoreHint(repoRoot string) (string, error) {
	if err := git.ValidateRoomIgnore(repoRoot); err != nil {
		return fmt.Sprintf("Ignore file: malformed .roomignore; ROOM will skip custom ignore rules until fixed: %v", err), nil
	}
	return "", nil
}
