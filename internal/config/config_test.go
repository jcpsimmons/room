package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[agent]
provider = "codex"

[run]
default_iterations = 12
commit_prefix = "loop:"

[codex]
binary = "codex-dev"
timeout_seconds = 45

[prompt]
max_recent_summaries = 4
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Run.DefaultIterations != 12 {
		t.Fatalf("default iterations = %d", cfg.Run.DefaultIterations)
	}
	if cfg.Run.MaxFailures != Default().Run.MaxFailures {
		t.Fatalf("expected default max failures, got %d", cfg.Run.MaxFailures)
	}
	if cfg.Run.CommitPrefix != "loop:" {
		t.Fatalf("commit prefix = %q", cfg.Run.CommitPrefix)
	}
	if cfg.Agent.Provider != "codex" {
		t.Fatalf("provider = %q", cfg.Agent.Provider)
	}
	if cfg.Codex.Binary != "codex-dev" {
		t.Fatalf("binary = %q", cfg.Codex.Binary)
	}
	if cfg.Codex.TimeoutSeconds != 45 {
		t.Fatalf("timeout = %d", cfg.Codex.TimeoutSeconds)
	}
	if cfg.Prompt.MaxRecentSummaries != 4 {
		t.Fatalf("max recent summaries = %d", cfg.Prompt.MaxRecentSummaries)
	}
	if cfg.Prompt.MaxSeenInstructions != Default().Prompt.MaxSeenInstructions {
		t.Fatalf("expected default max seen instructions, got %d", cfg.Prompt.MaxSeenInstructions)
	}
}

func TestResolvePathsHonorsRepoRoot(t *testing.T) {
	t.Parallel()

	repoRoot := "/tmp/repo"
	cfg := Default()
	paths := ResolvePaths(repoRoot, "", cfg)

	if paths.ConfigPath != filepath.Join(repoRoot, DefaultConfigRelPath) {
		t.Fatalf("config path = %q", paths.ConfigPath)
	}
	if paths.InstructionPath != filepath.Join(repoRoot, DefaultInstructionPath) {
		t.Fatalf("instruction path = %q", paths.InstructionPath)
	}
	if paths.RunsDir != filepath.Join(repoRoot, DefaultRunsDir) {
		t.Fatalf("runs dir = %q", paths.RunsDir)
	}
}

func TestLoadDefaultsClaudeConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[agent]
provider = "claude"
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Agent.Provider != "claude" {
		t.Fatalf("provider = %q", cfg.Agent.Provider)
	}
	if cfg.Claude.Binary != "claude" {
		t.Fatalf("claude binary = %q", cfg.Claude.Binary)
	}
	if cfg.Claude.PermissionMode != "bypassPermissions" {
		t.Fatalf("permission mode = %q", cfg.Claude.PermissionMode)
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[agent]
provider = "other"
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected provider validation error")
	}
}

func TestLoadRejectsUnsupportedClaudePermissionMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[agent]
provider = "claude"

[claude]
permission_mode = "acceptEdits"
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatalf("expected claude permission mode validation error")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[agent]
provider = "codex"

[run]
default_iterations = 12
unexpected_toggle = true
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected unknown key validation error")
	}
}

func TestValidatePathsRejectsInstructionFileCollisions(t *testing.T) {
	t.Parallel()

	paths := ResolvePaths("/tmp/repo", "", Default())
	paths.InstructionPath = paths.StatePath

	err := ValidatePaths(paths)
	if err == nil {
		t.Fatal("expected path validation error")
	}
	if !strings.Contains(err.Error(), "collides with the ROOM state file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePathsRejectsRunsArchiveTargets(t *testing.T) {
	t.Parallel()

	paths := ResolvePaths("/tmp/repo", "", Default())
	paths.InstructionPath = filepath.Join(paths.RunsDir, "seed.txt")

	err := ValidatePaths(paths)
	if err == nil {
		t.Fatal("expected path validation error")
	}
	if !strings.Contains(err.Error(), "resolves inside the runs archive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadExpandsEnvironmentPlaceholders(t *testing.T) {
	t.Setenv("ROOM_TEST_PROVIDER", "claude")
	t.Setenv("ROOM_TEST_MODEL", "sonnet")
	t.Setenv("ROOM_TEST_PROMPT_FILE", ".room/live-instruction.txt")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[agent]
provider = "${ROOM_TEST_PROVIDER}"

[claude]
model = "${ROOM_TEST_MODEL}"
binary = "${ROOM_TEST_BINARY:-claude-dev}"

[prompt]
instruction_file = "${ROOM_TEST_PROMPT_FILE}"
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Agent.Provider != "claude" {
		t.Fatalf("provider = %q", cfg.Agent.Provider)
	}
	if cfg.Claude.Model != "sonnet" {
		t.Fatalf("claude model = %q", cfg.Claude.Model)
	}
	if cfg.Claude.Binary != "claude-dev" {
		t.Fatalf("claude binary = %q", cfg.Claude.Binary)
	}
	if cfg.Prompt.InstructionFile != ".room/live-instruction.txt" {
		t.Fatalf("instruction file = %q", cfg.Prompt.InstructionFile)
	}
}

func TestLoadRejectsMissingEnvironmentPlaceholders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	input := []byte(`
[codex]
binary = "${ROOM_TEST_MISSING_BINARY}"
model = "${ROOM_TEST_MISSING_BINARY}"
`)
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected missing environment variable error")
	}
	if !strings.Contains(err.Error(), "config references missing environment variable(s): ROOM_TEST_MISSING_BINARY") {
		t.Fatalf("unexpected error: %v", err)
	}
}
