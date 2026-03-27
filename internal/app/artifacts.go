package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/fsutil"
)

type executionArtifact struct {
	Provider   string        `json:"provider"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Command    []string      `json:"command"`
	DurationMS int64         `json:"duration_ms"`
	TimedOut   bool          `json:"timed_out"`
	Error      string        `json:"error,omitempty"`
	Result     *agent.Result `json:"result,omitempty"`
}

type ExecutionReport struct {
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
	Error      string `json:"error,omitempty"`
}

func writeExecutionArtifact(path, provider string, execution agent.Execution, startedAt, finishedAt time.Time, runErr error) error {
	artifact := executionArtifact{
		Provider:   provider,
		StartedAt:  startedAt.UTC(),
		FinishedAt: finishedAt.UTC(),
		Command:    execution.Command,
		DurationMS: execution.DurationMS,
		TimedOut:   execution.TimedOut,
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

func executionReportIfPresent(artifact *executionArtifact, ok bool) *ExecutionReport {
	if !ok || artifact == nil {
		return nil
	}
	return &ExecutionReport{
		DurationMS: artifact.DurationMS,
		TimedOut:   artifact.TimedOut,
		Error:      strings.TrimSpace(artifact.Error),
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
	)
	if strings.TrimSpace(artifact.Error) != "" {
		lines = append(lines, indent(fmt.Sprintf("error: %s", strings.TrimSpace(artifact.Error))))
	} else {
		lines = append(lines, indent("error: none"))
	}
	return lines
}
