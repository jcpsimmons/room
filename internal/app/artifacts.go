package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/prompt"
)

type executionArtifact struct {
	Provider   string        `json:"provider"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Command    []string      `json:"command"`
	DurationMS int64         `json:"duration_ms"`
	TimedOut   bool          `json:"timed_out"`
	ExitCode   int           `json:"exit_code,omitempty"`
	ExitSignal string        `json:"exit_signal,omitempty"`
	Error      string        `json:"error,omitempty"`
	Result     *agent.Result `json:"result,omitempty"`
}

type ExecutionReport struct {
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
	ExitCode   int    `json:"exit_code,omitempty"`
	ExitSignal string `json:"exit_signal,omitempty"`
	Error      string `json:"error,omitempty"`
}

type progressArtifactEntry struct {
	RunProgressEvent
	Error string `json:"error,omitempty"`
}

type ProgressReport struct {
	EventCount         int       `json:"event_count"`
	PulseCount         int       `json:"pulse_count"`
	LastPhase          string    `json:"last_phase,omitempty"`
	LastStatus         string    `json:"last_status,omitempty"`
	FirstEventAt       time.Time `json:"first_event_at,omitempty"`
	LastEventAt        time.Time `json:"last_event_at,omitempty"`
	ExecutionElapsedMS int64     `json:"execution_elapsed_ms,omitempty"`
	RunElapsedMS       int64     `json:"run_elapsed_ms,omitempty"`
}

type recipeArtifact struct {
	Provider        string             `json:"provider"`
	Model           string             `json:"model,omitempty"`
	Binary          string             `json:"binary"`
	CommitEnabled   bool               `json:"commit_enabled"`
	CommitPrefix    string             `json:"commit_prefix,omitempty"`
	ConfigPath      string             `json:"config_path"`
	InstructionPath string             `json:"instruction_path"`
	SchemaPath      string             `json:"schema_path"`
	TimeoutSeconds  int                `json:"timeout_seconds,omitempty"`
	Sandbox         string             `json:"sandbox,omitempty"`
	Approval        string             `json:"approval,omitempty"`
	PermissionMode  string             `json:"permission_mode,omitempty"`
	PromptStats     prompt.BuildReport `json:"prompt_stats"`
}

type RecipeReport struct {
	Provider        string             `json:"provider"`
	Model           string             `json:"model,omitempty"`
	Binary          string             `json:"binary"`
	CommitEnabled   bool               `json:"commit_enabled"`
	ConfigPath      string             `json:"config_path"`
	InstructionPath string             `json:"instruction_path"`
	SchemaPath      string             `json:"schema_path"`
	TimeoutSeconds  int                `json:"timeout_seconds,omitempty"`
	Sandbox         string             `json:"sandbox,omitempty"`
	Approval        string             `json:"approval,omitempty"`
	PermissionMode  string             `json:"permission_mode,omitempty"`
	PromptStats     prompt.BuildReport `json:"prompt_stats"`
}

func writeExecutionArtifact(path, provider string, execution agent.Execution, startedAt, finishedAt time.Time, runErr error) error {
	artifact := executionArtifact{
		Provider:   provider,
		StartedAt:  startedAt.UTC(),
		FinishedAt: finishedAt.UTC(),
		Command:    execution.Command,
		DurationMS: execution.DurationMS,
		TimedOut:   execution.TimedOut,
		ExitCode:   execution.ExitCode,
		ExitSignal: strings.TrimSpace(execution.ExitSignal),
	}
	if runErr != nil {
		artifact.Error = strings.TrimSpace(runErr.Error())
	}
	if execution.Result != (agent.Result{}) {
		result := execution.Result
		artifact.Result = &result
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(path, data, 0o644)
}

func appendProgressArtifact(path string, event RunProgressEvent) (err error) {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := fsutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	entry := progressArtifactEntry{RunProgressEvent: event}
	if event.Err != nil {
		entry.Error = strings.TrimSpace(event.Err.Error())
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func writeRecipeArtifact(path string, cfg config.Config, paths config.Paths, provider, model string, commitEnabled bool, commitPrefix string, stats prompt.BuildReport) error {
	artifact := recipeArtifact{
		Provider:        strings.TrimSpace(provider),
		Model:           strings.TrimSpace(model),
		Binary:          strings.TrimSpace(configuredBinary(cfg, provider)),
		CommitEnabled:   commitEnabled,
		CommitPrefix:    strings.TrimSpace(commitPrefix),
		ConfigPath:      filepath.Clean(paths.ConfigPath),
		InstructionPath: filepath.Clean(paths.InstructionPath),
		SchemaPath:      filepath.Clean(paths.SchemaPath),
		PromptStats:     stats,
	}

	switch artifact.Provider {
	case agent.ProviderClaude:
		artifact.TimeoutSeconds = cfg.Claude.TimeoutSeconds
		artifact.PermissionMode = strings.TrimSpace(cfg.Claude.PermissionMode)
	default:
		artifact.TimeoutSeconds = cfg.Codex.TimeoutSeconds
		artifact.Sandbox = strings.TrimSpace(cfg.Codex.Sandbox)
		artifact.Approval = strings.TrimSpace(cfg.Codex.Approval)
	}

	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(path, data, 0o644)
}

func readExecutionArtifact(path string) (*executionArtifact, bool, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, nil
	}

	var artifact executionArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(artifact.Provider) == "" {
		return nil, false, fmt.Errorf("malformed execution artifact: missing provider")
	}
	if artifact.StartedAt.IsZero() || artifact.FinishedAt.IsZero() {
		return nil, false, fmt.Errorf("malformed execution artifact: missing timestamps")
	}
	return &artifact, true, nil
}

func readExecutionArtifactLenient(path string) (*executionArtifact, bool, error, error) {
	artifact, ok, err := readExecutionArtifact(path)
	if err == nil {
		return artifact, ok, nil, nil
	}
	if strings.HasPrefix(err.Error(), "malformed execution artifact:") {
		return nil, false, err, nil
	}
	return nil, false, nil, err
}

func readProgressArtifact(path string) (ProgressReport, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProgressReport{}, false, nil
		}
		return ProgressReport{}, false, err
	}
	defer file.Close()

	var report ProgressReport
	scanner := bufio.NewScanner(file)
	const maxLine = 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry progressArtifactEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return ProgressReport{}, false, err
		}
		report.EventCount++
		if entry.Phase == RunProgressPhaseAgentExecutionPulse {
			report.PulseCount++
		}
		if report.FirstEventAt.IsZero() && !entry.EventAt.IsZero() {
			report.FirstEventAt = entry.EventAt
		}
		if !entry.EventAt.IsZero() {
			report.LastEventAt = entry.EventAt
		}
		report.LastPhase = string(entry.Phase)
		report.LastStatus = strings.TrimSpace(entry.Status)
		report.ExecutionElapsedMS = entry.ExecutionElapsedMS
		report.RunElapsedMS = entry.RunElapsedMS
	}
	if err := scanner.Err(); err != nil {
		return ProgressReport{}, false, err
	}
	if report.EventCount == 0 {
		return ProgressReport{}, false, nil
	}
	return report, true, nil
}

func readProgressArtifactLenient(path string) (*ProgressReport, bool, error, error) {
	report, ok, err := readProgressArtifact(path)
	if err == nil {
		return &report, ok, nil, nil
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) || strings.HasPrefix(err.Error(), "json:") {
		return nil, false, err, nil
	}
	return nil, false, nil, err
}

func readRecipeArtifact(path string) (*recipeArtifact, bool, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, nil
	}

	var artifact recipeArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(artifact.Provider) == "" {
		return nil, false, fmt.Errorf("malformed recipe artifact: missing provider")
	}
	if strings.TrimSpace(artifact.Binary) == "" {
		return nil, false, fmt.Errorf("malformed recipe artifact: missing binary")
	}
	if strings.TrimSpace(artifact.ConfigPath) == "" || strings.TrimSpace(artifact.InstructionPath) == "" || strings.TrimSpace(artifact.SchemaPath) == "" {
		return nil, false, fmt.Errorf("malformed recipe artifact: missing path wiring")
	}
	return &artifact, true, nil
}

func readRecipeArtifactLenient(path string) (*recipeArtifact, bool, error, error) {
	artifact, ok, err := readRecipeArtifact(path)
	if err == nil {
		return artifact, ok, nil, nil
	}
	if strings.HasPrefix(err.Error(), "malformed recipe artifact:") {
		return nil, false, err, nil
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) || strings.HasPrefix(err.Error(), "json:") {
		return nil, false, err, nil
	}
	return nil, false, nil, err
}

func executionReportIfPresent(artifact *executionArtifact, ok bool) *ExecutionReport {
	if !ok || artifact == nil {
		return nil
	}
	return &ExecutionReport{
		DurationMS: artifact.DurationMS,
		TimedOut:   artifact.TimedOut,
		ExitCode:   artifact.ExitCode,
		ExitSignal: strings.TrimSpace(artifact.ExitSignal),
		Error:      strings.TrimSpace(artifact.Error),
	}
}

func progressLines(report *ProgressReport, ok bool) []string {
	if !ok || report == nil {
		return []string{"Progress trace:", indent("unavailable")}
	}

	lines := []string{"Progress trace:"}
	lines = append(lines,
		indent(fmt.Sprintf("events: %d", report.EventCount)),
		indent(fmt.Sprintf("pulses: %d", report.PulseCount)),
		indent(fmt.Sprintf("last phase: %s", emptyIfBlank(report.LastPhase, "unknown"))),
		indent(fmt.Sprintf("last status: %s", emptyIfBlank(report.LastStatus, "unknown"))),
	)
	if !report.FirstEventAt.IsZero() && !report.LastEventAt.IsZero() {
		lines = append(lines, indent(fmt.Sprintf("trace span: %s", report.LastEventAt.Sub(report.FirstEventAt).Round(100*time.Millisecond))))
	}
	if report.ExecutionElapsedMS > 0 {
		lines = append(lines, indent(fmt.Sprintf("last execution elapsed: %s (%d ms)", time.Duration(report.ExecutionElapsedMS)*time.Millisecond, report.ExecutionElapsedMS)))
	}
	if report.RunElapsedMS > 0 {
		lines = append(lines, indent(fmt.Sprintf("last run elapsed: %s (%d ms)", time.Duration(report.RunElapsedMS)*time.Millisecond, report.RunElapsedMS)))
	}
	return lines
}

func progressReportIfPresent(report *ProgressReport, ok bool) *ProgressReport {
	if !ok || report == nil {
		return nil
	}
	copy := *report
	return &copy
}

func recipeReportIfPresent(artifact *recipeArtifact, ok bool) *RecipeReport {
	if !ok || artifact == nil {
		return nil
	}
	return &RecipeReport{
		Provider:        artifact.Provider,
		Model:           artifact.Model,
		Binary:          artifact.Binary,
		CommitEnabled:   artifact.CommitEnabled,
		ConfigPath:      artifact.ConfigPath,
		InstructionPath: artifact.InstructionPath,
		SchemaPath:      artifact.SchemaPath,
		TimeoutSeconds:  artifact.TimeoutSeconds,
		Sandbox:         artifact.Sandbox,
		Approval:        artifact.Approval,
		PermissionMode:  artifact.PermissionMode,
		PromptStats:     artifact.PromptStats,
	}
}

func executionLines(artifact *executionArtifact, ok bool) []string {
	if !ok || artifact == nil {
		return []string{"Execution:", indent("unavailable")}
	}

	lines := []string{"Execution:"}
	lines = append(lines,
		indent(fmt.Sprintf("duration: %s (%d ms)", time.Duration(artifact.DurationMS)*time.Millisecond, artifact.DurationMS)),
		indent(fmt.Sprintf("timed out: %t", artifact.TimedOut)),
		indent(fmt.Sprintf("exit: %s", formatExecutionExit(artifact.ExitCode, artifact.ExitSignal))),
	)
	if strings.TrimSpace(artifact.Error) != "" {
		lines = append(lines, indent(fmt.Sprintf("error: %s", strings.TrimSpace(artifact.Error))))
	} else {
		lines = append(lines, indent("error: none"))
	}
	return lines
}

func recipeLines(artifact *recipeArtifact, ok bool) []string {
	if !ok || artifact == nil {
		return []string{"Recipe:", indent("unavailable")}
	}

	lines := []string{"Recipe:"}
	lines = append(lines,
		indent(fmt.Sprintf("provider: %s", emptyIfBlank(artifact.Provider, "unknown"))),
		indent(fmt.Sprintf("model: %s", emptyIfBlank(artifact.Model, "default"))),
		indent(fmt.Sprintf("binary: %s", emptyIfBlank(artifact.Binary, "unknown"))),
		indent(fmt.Sprintf("commit enabled: %t", artifact.CommitEnabled)),
		indent(fmt.Sprintf("timeout: %ds", artifact.TimeoutSeconds)),
	)
	if strings.TrimSpace(artifact.Sandbox) != "" {
		lines = append(lines, indent(fmt.Sprintf("sandbox: %s", artifact.Sandbox)))
	}
	if strings.TrimSpace(artifact.Approval) != "" {
		lines = append(lines, indent(fmt.Sprintf("approval: %s", artifact.Approval)))
	}
	if strings.TrimSpace(artifact.PermissionMode) != "" {
		lines = append(lines, indent(fmt.Sprintf("permission mode: %s", artifact.PermissionMode)))
	}
	lines = append(lines,
		indent(fmt.Sprintf("config: %s", artifact.ConfigPath)),
		indent(fmt.Sprintf("instruction: %s", artifact.InstructionPath)),
		indent(fmt.Sprintf("schema: %s", artifact.SchemaPath)),
		indent(formatPromptStatsSummary(artifact.PromptStats)),
	)
	return lines
}

func formatExecutionExit(code int, signal string) string {
	signal = strings.TrimSpace(signal)
	switch {
	case signal != "" && code != 0:
		return fmt.Sprintf("%d (%s)", code, signal)
	case signal != "":
		return signal
	case code != 0:
		return fmt.Sprintf("%d", code)
	default:
		return "0"
	}
}

func emptyIfBlank(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func configuredBinary(cfg config.Config, provider string) string {
	switch strings.TrimSpace(provider) {
	case agent.ProviderClaude:
		return strings.TrimSpace(cfg.Claude.Binary)
	default:
		return strings.TrimSpace(cfg.Codex.Binary)
	}
}

func formatPromptStatsSummary(stats prompt.BuildReport) string {
	parts := []string{
		fmt.Sprintf("prompt context: %d runes", stats.TotalRunes),
		fmt.Sprintf("summaries=%d", stats.RecentSummariesCount),
		fmt.Sprintf("instructions=%d", stats.PriorInstructionsCount),
		fmt.Sprintf("commits=%d", stats.RecentCommitsCount),
	}
	if stats.CurrentInstructionClipped || stats.RecoveryHintClipped || stats.GitStatusClipped {
		clipped := make([]string, 0, 3)
		if stats.CurrentInstructionClipped {
			clipped = append(clipped, "instruction")
		}
		if stats.RecoveryHintClipped {
			clipped = append(clipped, "fault-signal")
		}
		if stats.GitStatusClipped {
			clipped = append(clipped, fmt.Sprintf("git-status (+%d omitted lines)", stats.GitStatusOmittedLines))
		}
		parts = append(parts, "clipped="+strings.Join(clipped, ", "))
	}
	return strings.Join(parts, " | ")
}
