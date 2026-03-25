package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errMalformedJSON = errors.New("malformed ROOM JSON result")

type Result struct {
	Summary         string `json:"summary"`
	NextInstruction string `json:"next_instruction"`
	Status          string `json:"status"`
	CommitMessage   string `json:"commit_message"`
}

type Execution struct {
	Result     Result   `json:"result"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	Command    []string `json:"command"`
	DurationMS int64    `json:"duration_ms"`
	TimedOut   bool     `json:"timed_out"`
}

func ParseResult(raw []byte) (Result, error) {
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return Result{}, fmt.Errorf("%w: %v", errMalformedJSON, err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (r Result) Validate() error {
	if strings.TrimSpace(r.Summary) == "" {
		return errors.New("summary is required")
	}
	if strings.TrimSpace(r.NextInstruction) == "" {
		return errors.New("next_instruction is required")
	}
	switch strings.TrimSpace(r.Status) {
	case "continue", "pivot", "done":
	default:
		return fmt.Errorf("status must be one of continue, pivot, done")
	}
	if strings.TrimSpace(r.CommitMessage) == "" {
		return errors.New("commit_message is required")
	}
	return nil
}
