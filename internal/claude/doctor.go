package claude

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var requiredHelpSnippets = []string{
	"--print",
	"--output-format",
	"--json-schema",
	"--permission-mode",
	"--no-session-persistence",
}

func ValidateHelp(text string) error {
	var missing []string
	for _, snippet := range requiredHelpSnippets {
		if !strings.Contains(text, snippet) {
			missing = append(missing, snippet)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("Claude Code CLI is missing required flags: %s", strings.Join(missing, ", "))
	}
	return nil
}

func ValidateCLI(ctx context.Context, binary string) error {
	if strings.TrimSpace(binary) == "" {
		binary = "claude"
	}
	cmd := exec.CommandContext(ctx, binary, "--help")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to inspect Claude Code CLI: %w", err)
	}
	return ValidateHelp(stdout.String())
}
