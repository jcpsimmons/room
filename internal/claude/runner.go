package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/fsutil"
)

type CLI struct{}

func NewRunner() agent.Runner {
	return CLI{}
}

func (CLI) Version(ctx context.Context, binary string) (string, error) {
	if strings.TrimSpace(binary) == "" {
		binary = "claude"
	}
	cmd := exec.CommandContext(ctx, binary, "--version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to detect Claude Code version: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (CLI) Run(ctx context.Context, prompt agent.Prompt, schema agent.Schema, opts agent.RunOptions, outputPath string) (agent.Execution, error) {
	if strings.TrimSpace(schema.Path) == "" {
		return agent.Execution{}, errors.New("schema path is required")
	}
	if strings.TrimSpace(outputPath) == "" {
		return agent.Execution{}, errors.New("output path is required")
	}

	schemaBytes, err := os.ReadFile(schema.Path)
	if err != nil {
		return agent.Execution{}, err
	}
	command, err := BuildCommand(prompt, string(schemaBytes), opts)
	if err != nil {
		return agent.Execution{}, err
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	cmd.Dir = opts.WorkDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	execution := agent.Execution{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Command:    redactedCommand(command),
		DurationMS: duration.Milliseconds(),
		TimedOut:   errors.Is(runCtx.Err(), context.DeadlineExceeded),
	}

	if err != nil {
		if execution.TimedOut {
			return execution, fmt.Errorf("claude execution timed out after %s", opts.Timeout)
		}
		if parseErr := captureResult(stdout.Bytes(), outputPath, &execution); parseErr != nil {
			return execution, errors.Join(fmt.Errorf("claude execution failed: %w", err), parseErr)
		}
		return execution, fmt.Errorf("claude execution failed: %w", err)
	}
	if err := captureResult(stdout.Bytes(), outputPath, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func captureResult(stdout []byte, outputPath string, execution *agent.Execution) error {
	result, err := ParseOutput(stdout)
	if err != nil {
		if errors.Is(err, ErrMalformedOutputEnvelope) {
			return fmt.Errorf("claude wrapper drift detected: %w", err)
		}
		return err
	}
	execution.Result = result

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(outputPath, data, 0o644)
}

func redactedCommand(command []string) []string {
	if len(command) == 0 {
		return nil
	}
	out := append([]string(nil), command...)
	out[len(out)-1] = "<prompt>"
	return out
}
