package config

import (
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/fsutil"
	toml "github.com/pelletier/go-toml/v2"
)

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
	Run    RunConfig    `toml:"run" json:"run"`
	Codex  CodexConfig  `toml:"codex" json:"codex"`
	Prompt PromptConfig `toml:"prompt" json:"prompt"`
	Output OutputConfig `toml:"output" json:"output"`
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
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg.Normalize(), nil
}

func Save(path string, cfg Config) error {
	data, err := toml.Marshal(cfg.Normalize())
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

func resolve(base, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}
