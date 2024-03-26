package app

import (
	"encoding/json"
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
