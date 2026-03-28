package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/fsutil"
	toml "github.com/pelletier/go-toml/v2"
)

var envPlaceholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

const (
	RoomDirName            = ".room"
	DefaultConfigRelPath   = ".room/config.toml"
	DefaultStateRelPath    = ".room/state.json"
	DefaultSchemaRelPath   = ".room/schema.json"
	DefaultInstructionPath = ".room/instruction.txt"
	DefaultSummariesPath   = ".room/summaries.log"
	DefaultSeenPath        = ".room/seen_instructions.txt"
	DefaultRunsDir         = ".room/runs"
)

type Config struct {
	Agent  AgentConfig  `toml:"agent" json:"agent"`
	Run    RunConfig    `toml:"run" json:"run"`
	Codex  CodexConfig  `toml:"codex" json:"codex"`
	Claude ClaudeConfig `toml:"claude" json:"claude"`
	Prompt PromptConfig `toml:"prompt" json:"prompt"`
	Output OutputConfig `toml:"output" json:"output"`
}

type AgentConfig struct {
	Provider string `toml:"provider" json:"provider"`
}

type RunConfig struct {
	DefaultIterations int    `toml:"default_iterations" json:"default_iterations"`
	MaxFailures       int    `toml:"max_failures" json:"max_failures"`
	UntilDone         bool   `toml:"until_done" json:"until_done"`
	AllowDirty        bool   `toml:"allow_dirty" json:"allow_dirty"`
	Commit            bool   `toml:"commit" json:"commit"`
	CommitPrefix      string `toml:"commit_prefix" json:"commit_prefix"`
}

type CodexConfig struct {
	Binary         string `toml:"binary" json:"binary"`
	Model          string `toml:"model" json:"model"`
	Sandbox        string `toml:"sandbox" json:"sandbox"`
	Approval       string `toml:"approval" json:"approval"`
	TimeoutSeconds int    `toml:"timeout_seconds" json:"timeout_seconds"`
}

type ClaudeConfig struct {
	Binary         string `toml:"binary" json:"binary"`
	Model          string `toml:"model" json:"model"`
	PermissionMode string `toml:"permission_mode" json:"permission_mode"`
	TimeoutSeconds int    `toml:"timeout_seconds" json:"timeout_seconds"`
}

type PromptConfig struct {
	InstructionFile       string `toml:"instruction_file" json:"instruction_file"`
	MaxRecentSummaries    int    `toml:"max_recent_summaries" json:"max_recent_summaries"`
	MaxSeenInstructions   int    `toml:"max_seen_instructions" json:"max_seen_instructions"`
	ForcePivotOnDuplicate bool   `toml:"force_pivot_on_duplicate" json:"force_pivot_on_duplicate"`
}

type OutputConfig struct {
	Verbose bool `toml:"verbose" json:"verbose"`
	JSON    bool `toml:"json" json:"json"`
}

type Paths struct {
	RepoRoot             string `json:"repo_root"`
	RoomDir              string `json:"room_dir"`
	ConfigPath           string `json:"config_path"`
	StatePath            string `json:"state_path"`
	SchemaPath           string `json:"schema_path"`
	InstructionPath      string `json:"instruction_path"`
	SummariesPath        string `json:"summaries_path"`
	SeenInstructionsPath string `json:"seen_instructions_path"`
	RunsDir              string `json:"runs_dir"`
}

func Default() Config {
	return Config{
		Agent: AgentConfig{
			Provider: agent.ProviderCodex,
		},
		Run: RunConfig{
			DefaultIterations: 100,
			MaxFailures:       3,
			UntilDone:         false,
			AllowDirty:        false,
			Commit:            true,
			CommitPrefix:      "room:",
		},
		Codex: CodexConfig{
			Binary:         "codex",
			Model:          "",
			Sandbox:        "danger-full-access",
			Approval:       "never",
			TimeoutSeconds: 1800,
		},
		Claude: ClaudeConfig{
			Binary:         "claude",
			Model:          "",
			PermissionMode: "bypassPermissions",
			TimeoutSeconds: 1800,
		},
		Prompt: PromptConfig{
			InstructionFile:       DefaultInstructionPath,
			MaxRecentSummaries:    10,
			MaxSeenInstructions:   50,
			ForcePivotOnDuplicate: true,
		},
		Output: OutputConfig{
			Verbose: false,
			JSON:    false,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return Config{}, err
	}
	if len(data) == 0 {
		return cfg, nil
	}
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}
	cfg, err = expandEnvPlaceholders(cfg)
	if err != nil {
		return Config{}, err
	}
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(path, data, 0o644)
}

func ResolvePaths(repoRoot, configPath string, cfg Config) Paths {
	if configPath == "" {
		configPath = filepath.Join(repoRoot, DefaultConfigRelPath)
	}
	roomDir := filepath.Dir(configPath)
	instructionPath := resolve(repoRoot, cfg.Prompt.InstructionFile)
	if instructionPath == "" {
		instructionPath = filepath.Join(repoRoot, DefaultInstructionPath)
	}
	return Paths{
		RepoRoot:             repoRoot,
		RoomDir:              roomDir,
		ConfigPath:           configPath,
		StatePath:            filepath.Join(roomDir, filepath.Base(DefaultStateRelPath)),
		SchemaPath:           filepath.Join(roomDir, filepath.Base(DefaultSchemaRelPath)),
		InstructionPath:      instructionPath,
		SummariesPath:        filepath.Join(roomDir, filepath.Base(DefaultSummariesPath)),
		SeenInstructionsPath: filepath.Join(roomDir, filepath.Base(DefaultSeenPath)),
		RunsDir:              filepath.Join(roomDir, "runs"),
	}
}

func (c Config) Normalize() Config {
	cfg := c
	def := Default()

	cfg.Agent.Provider = agent.NormalizeProvider(cfg.Agent.Provider)
	if cfg.Run.DefaultIterations <= 0 {
		cfg.Run.DefaultIterations = def.Run.DefaultIterations
	}
	if cfg.Run.MaxFailures <= 0 {
		cfg.Run.MaxFailures = def.Run.MaxFailures
	}
	if strings.TrimSpace(cfg.Run.CommitPrefix) == "" {
		cfg.Run.CommitPrefix = def.Run.CommitPrefix
	}
	if strings.TrimSpace(cfg.Codex.Binary) == "" {
		cfg.Codex.Binary = def.Codex.Binary
	}
	if strings.TrimSpace(cfg.Codex.Sandbox) == "" {
		cfg.Codex.Sandbox = def.Codex.Sandbox
	}
	if strings.TrimSpace(cfg.Codex.Approval) == "" {
		cfg.Codex.Approval = def.Codex.Approval
	}
	if cfg.Codex.TimeoutSeconds <= 0 {
		cfg.Codex.TimeoutSeconds = def.Codex.TimeoutSeconds
	}
	if strings.TrimSpace(cfg.Claude.Binary) == "" {
		cfg.Claude.Binary = def.Claude.Binary
	}
	if strings.TrimSpace(cfg.Claude.PermissionMode) == "" {
		cfg.Claude.PermissionMode = def.Claude.PermissionMode
	}
	if cfg.Claude.TimeoutSeconds <= 0 {
		cfg.Claude.TimeoutSeconds = def.Claude.TimeoutSeconds
	}
	if strings.TrimSpace(cfg.Prompt.InstructionFile) == "" {
		cfg.Prompt.InstructionFile = def.Prompt.InstructionFile
	}
	if cfg.Prompt.MaxRecentSummaries <= 0 {
		cfg.Prompt.MaxRecentSummaries = def.Prompt.MaxRecentSummaries
	}
	if cfg.Prompt.MaxSeenInstructions <= 0 {
		cfg.Prompt.MaxSeenInstructions = def.Prompt.MaxSeenInstructions
	}

	return cfg
}

func (c Config) Validate() error {
	if err := agent.ValidateProvider(c.Agent.Provider); err != nil {
		return err
	}
	if agent.NormalizeProvider(c.Agent.Provider) == agent.ProviderClaude && strings.TrimSpace(c.Claude.PermissionMode) != "bypassPermissions" {
		return fmt.Errorf("unsupported claude permission_mode %q; ROOM currently requires %q", strings.TrimSpace(c.Claude.PermissionMode), "bypassPermissions")
	}
	return nil
}

func ValidatePaths(paths Paths) error {
	var issues []string
	instructionPath := filepath.Clean(paths.InstructionPath)
	configPath := filepath.Clean(paths.ConfigPath)
	runsDir := filepath.Clean(paths.RunsDir)

	if isSubpath(instructionPath, runsDir) {
		issues = append(issues, fmt.Sprintf("prompt.instruction_file resolves inside the runs archive: %s", instructionPath))
	}
	if isSubpath(configPath, runsDir) {
		issues = append(issues, fmt.Sprintf("config file resolves inside the runs archive: %s", configPath))
	}

	for _, collision := range []struct {
		label string
		path  string
	}{
		{label: "config file", path: configPath},
		{label: "state file", path: paths.StatePath},
		{label: "schema file", path: paths.SchemaPath},
		{label: "summaries log", path: paths.SummariesPath},
		{label: "seen instructions log", path: paths.SeenInstructionsPath},
	} {
		if samePath(instructionPath, collision.path) {
			issues = append(issues, fmt.Sprintf("prompt.instruction_file collides with the ROOM %s: %s", collision.label, instructionPath))
		}
	}

	if len(issues) == 0 {
		return nil
	}
	return errors.New(strings.Join(issues, "; "))
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func isSubpath(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolve(base, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func expandEnvPlaceholders(cfg Config) (Config, error) {
	var missing []string
	expand := func(value string) string {
		return envPlaceholderRe.ReplaceAllStringFunc(value, func(match string) string {
			pieces := envPlaceholderRe.FindStringSubmatch(match)
			if len(pieces) < 2 {
				return match
			}
			if resolved, ok := os.LookupEnv(pieces[1]); ok {
				return resolved
			}
			if len(pieces) >= 3 && pieces[2] != "" {
				return pieces[2]
			}
			missing = append(missing, pieces[1])
			return match
		})
	}

	cfg.Agent.Provider = expand(cfg.Agent.Provider)
	cfg.Run.CommitPrefix = expand(cfg.Run.CommitPrefix)
	cfg.Codex.Binary = expand(cfg.Codex.Binary)
	cfg.Codex.Model = expand(cfg.Codex.Model)
	cfg.Codex.Sandbox = expand(cfg.Codex.Sandbox)
	cfg.Codex.Approval = expand(cfg.Codex.Approval)
	cfg.Claude.Binary = expand(cfg.Claude.Binary)
	cfg.Claude.Model = expand(cfg.Claude.Model)
	cfg.Claude.PermissionMode = expand(cfg.Claude.PermissionMode)
	cfg.Prompt.InstructionFile = expand(cfg.Prompt.InstructionFile)

	if len(missing) > 0 {
		slices.Sort(missing)
		missing = slices.Compact(missing)
		return Config{}, fmt.Errorf("config references missing environment variable(s): %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}
