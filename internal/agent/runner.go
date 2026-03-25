package agent

import (
	"context"
	"time"
)

type Prompt struct {
	Body string
}

type Schema struct {
	Path string
}

type RunOptions struct {
	Binary         string
	WorkDir        string
	Model          string
	Sandbox        string
	Approval       string
	PermissionMode string
	Timeout        time.Duration
}

type Runner interface {
	Run(ctx context.Context, prompt Prompt, schema Schema, opts RunOptions, outputPath string) (Execution, error)
	Version(ctx context.Context, binary string) (string, error)
}
