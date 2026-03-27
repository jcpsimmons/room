package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
)

type ConfigOptions struct {
	WorkingDir string
	ConfigPath string
}

type ConfigReport struct {
	RepoRoot     string        `json:"repo_root"`
	ConfigPath   string        `json:"config_path"`
	RoomDir      string        `json:"room_dir"`
	ConfigExists bool          `json:"config_exists"`
	Config       config.Config `json:"config"`
	Paths        config.Paths  `json:"paths"`
	Lines        []string      `json:"lines"`
}

func (s *Service) Config(ctx context.Context, opts ConfigOptions) (ConfigReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return ConfigReport{}, err
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(repoRoot, config.DefaultConfigRelPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return ConfigReport{}, err
	}
	paths := config.ResolvePaths(repoRoot, configPath, cfg)

	lines := []string{
		"ROOM config",
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Config source: %s", paths.ConfigPath),
	}
	if fsutil.FileExists(paths.ConfigPath) {
		lines = append(lines, "Config status: present")
	} else {
		lines = append(lines, "Config status: missing; defaults are in effect")
	}
	lines = append(lines,
		fmt.Sprintf("Room dir: %s", paths.RoomDir),
		fmt.Sprintf("Provider: %s", agent.DisplayName(cfg.Agent.Provider)),
		fmt.Sprintf("Agent binary: %s", s.binaryForProvider(cfg)),
		fmt.Sprintf("Run defaults: iterations=%d max_failures=%d until_done=%t allow_dirty=%t commit=%t prefix=%q",
			cfg.Run.DefaultIterations,
			cfg.Run.MaxFailures,
			cfg.Run.UntilDone,
			cfg.Run.AllowDirty,
			cfg.Run.Commit,
			cfg.Run.CommitPrefix,
		),
		fmt.Sprintf("Prompt window: summaries=%d seen_instructions=%d force_pivot_on_duplicate=%t",
			cfg.Prompt.MaxRecentSummaries,
			cfg.Prompt.MaxSeenInstructions,
			cfg.Prompt.ForcePivotOnDuplicate,
		),
		fmt.Sprintf("Prompt file: %s", paths.InstructionPath),
		fmt.Sprintf("Schema file: %s", paths.SchemaPath),
		fmt.Sprintf("State file: %s", paths.StatePath),
		fmt.Sprintf("Summaries file: %s", paths.SummariesPath),
		fmt.Sprintf("Seen instructions file: %s", paths.SeenInstructionsPath),
		fmt.Sprintf("Runs dir: %s", paths.RunsDir),
	)
	if strings.TrimSpace(cfg.Codex.Model) != "" && agent.NormalizeProvider(cfg.Agent.Provider) == agent.ProviderCodex {
		lines = append(lines, fmt.Sprintf("Codex model: %s", cfg.Codex.Model))
	}
	if strings.TrimSpace(cfg.Claude.Model) != "" && agent.NormalizeProvider(cfg.Agent.Provider) == agent.ProviderClaude {
		lines = append(lines, fmt.Sprintf("Claude model: %s", cfg.Claude.Model))
	}

	return ConfigReport{
		RepoRoot:     repoRoot,
		ConfigPath:   paths.ConfigPath,
		RoomDir:      paths.RoomDir,
		ConfigExists: fsutil.FileExists(paths.ConfigPath),
		Config:       cfg,
		Paths:        paths,
		Lines:        lines,
	}, nil
}
