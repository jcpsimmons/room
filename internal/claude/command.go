package claude

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
)

func BuildCommand(prompt agent.Prompt, schemaJSON string, opts agent.RunOptions) ([]string, error) {
	if strings.TrimSpace(opts.Binary) == "" {
		return nil, fmt.Errorf("claude binary is required")
	}
	if strings.TrimSpace(opts.WorkDir) == "" {
		return nil, fmt.Errorf("workdir is required")
	}
	if strings.TrimSpace(schemaJSON) == "" {
		return nil, fmt.Errorf("schema JSON is required")
	}
	if strings.TrimSpace(prompt.Body) == "" {
		return nil, fmt.Errorf("prompt body is required")
	}

	permissionMode := strings.TrimSpace(opts.PermissionMode)
	if permissionMode == "" {
		permissionMode = "bypassPermissions"
	}

	args := []string{
		"-p",
		"--permission-mode", permissionMode,
		"--output-format", "json",
		"--json-schema", schemaJSON,
		"--no-session-persistence",
		"--disable-slash-commands",
	}
	if strings.TrimSpace(opts.Model) != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, prompt.Body)
	return append([]string{opts.Binary}, args...), nil
}
