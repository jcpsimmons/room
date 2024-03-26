package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
)

type Runner = agent.Runner

type CLI struct{}

func NewRunner() Runner {
	return CLI{}
}

func (CLI) Version(ctx context.Context, binary string) (string, error) {
	if strings.TrimSpace(binary) == "" {
		binary = "codex"
	}
	cmd := exec.CommandContext(ctx, binary, "--version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to detect Codex version: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (CLI) Run(ctx context.Context, prompt Prompt, schema Schema, opts RunOptions, outputPath string) (Execution, error) {
	command, err := BuildCommand(prompt, schema, outputPath, opts)
	if err != nil {
		return Execution{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	cmd.Stdin = strings.NewReader(prompt.Body)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	outBytes, readErr := os.ReadFile(outputPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return Execution{}, readErr
	}

	execution := Execution{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Command:    command,
		DurationMS: duration.Milliseconds(),
		TimedOut:   errors.Is(runCtx.Err(), context.DeadlineExceeded),
	}
	if len(outBytes) > 0 {
		result, parseErr := ParseResult(outBytes)
		if parseErr != nil {
			return execution, parseErr
		}
		execution.Result = result
	}
	if err != nil {
		if execution.TimedOut {
			return execution, fmt.Errorf("codex execution timed out after %s", opts.Timeout)
		}
		return execution, fmt.Errorf("codex execution failed: %w", err)
	}
	if len(outBytes) == 0 {
		return execution, errors.New("codex did not write a final message")
	}
	return execution, nil
}
