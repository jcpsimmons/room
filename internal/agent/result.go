package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
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
	ExitCode   int      `json:"exit_code,omitempty"`
	ExitSignal string   `json:"exit_signal,omitempty"`
}

func CaptureExitMetadata(err error) (int, string) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, ""
	}
	if exitErr.ProcessState == nil {
		return 0, ""
	}

	exitCode := exitErr.ProcessState.ExitCode()
	waitStatus, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		return exitCode, ""
	}
	if waitStatus.Signaled() {
		return exitCode, waitStatus.Signal().String()
	}
	return exitCode, ""
}

func ParseResult(raw []byte) (Result, error) {
	payload := bytes.TrimSpace(raw)
	if len(payload) > 0 && payload[0] != '{' {
		extracted, err := extractJSONObject(payload)
		if err != nil {
			return Result{}, malformedResultError(raw, err)
		}
		payload = extracted
	}

	var result Result
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Result{}, malformedResultError(raw, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return Result{}, malformedResultError(raw, errors.New("unexpected trailing data"))
		}
		return Result{}, malformedResultError(raw, err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func malformedResultError(raw []byte, err error) error {
	return fmt.Errorf("%w: %v (input %q)", errMalformedJSON, err, previewJSON(raw))
}

func previewJSON(raw []byte) string {
	const limit = 160
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	buf := make([]byte, 0, len(trimmed))
	for _, b := range trimmed {
		switch b {
		case '\n', '\r', '\t':
			b = ' '
		}
		if b < 0x20 {
			b = ' '
		}
		buf = append(buf, b)
		if len(buf) >= limit {
			break
		}
	}
	if len(trimmed) > len(buf) {
		return string(buf) + "..."
	}
	return string(buf)
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
