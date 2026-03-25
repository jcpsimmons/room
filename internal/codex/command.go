package codex

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
)

type Prompt = agent.Prompt
type RunOptions = agent.RunOptions

func BuildCommand(prompt Prompt, schema Schema, outputPath string, opts RunOptions) ([]string, error) {
	_ = prompt
	if strings.TrimSpace(opts.Binary) == "" {
		return nil, fmt.Errorf("codex binary is required")
	}
	if strings.TrimSpace(opts.WorkDir) == "" {
		return nil, fmt.Errorf("workdir is required")
	}
	if strings.TrimSpace(schema.Path) == "" {
		return nil, fmt.Errorf("schema path is required")
	}
	if strings.TrimSpace(outputPath) == "" {
		return nil, fmt.Errorf("output path is required")
	}

	sandbox := strings.TrimSpace(opts.Sandbox)
	if sandbox == "" {
		sandbox = "danger-full-access"
	}
	approval := strings.TrimSpace(opts.Approval)
	if approval == "" {
		approval = "never"
	}

	args := []string{"--ask-for-approval", approval, "exec", "--cd", opts.WorkDir, "--sandbox", sandbox}
	if strings.TrimSpace(opts.Model) != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, "--output-schema", schema.Path, "--output-last-message", outputPath, "--color", "never", "--ephemeral", "-")
	return append([]string{opts.Binary}, args...), nil
}
