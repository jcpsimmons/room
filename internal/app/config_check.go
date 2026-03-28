package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
)

type ConfigCheckOptions struct {
	WorkingDir string
	ConfigPath string
}

type ConfigCheckReport struct {
	RepoRoot     string                `json:"repo_root"`
	ConfigPath   string                `json:"config_path"`
	ConfigExists bool                  `json:"config_exists"`
	Config       config.Config         `json:"config"`
	Paths        config.Paths          `json:"paths"`
	EnvRefs      []config.EnvReference `json:"env_references,omitempty"`
	SchemaHint   string                `json:"schema_hint,omitempty"`
	Lines        []string              `json:"lines"`
}

func (s *Service) ConfigCheck(ctx context.Context, opts ConfigCheckOptions) (ConfigCheckReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return ConfigCheckReport{}, err
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(repoRoot, config.DefaultConfigRelPath)
	}
	envRefs, envErr := config.ReadEnvReferences(configPath)
	if envErr != nil {
		return ConfigCheckReport{}, envErr
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		lines := []string{
			"ROOM config check",
			fmt.Sprintf("Repo: %s", repoRoot),
			fmt.Sprintf("Config source: %s", configPath),
			fmt.Sprintf("Config status: %s", configStatusLabel(configPath)),
			fmt.Sprintf("Config parse failed: %v", err),
		}
		if envLine := formatEnvReferences(envRefs); envLine != "" {
			lines = append(lines, fmt.Sprintf("Env patch points: %s", envLine))
		}
		return ConfigCheckReport{
			RepoRoot:     repoRoot,
			ConfigPath:   configPath,
			ConfigExists: fsutil.FileExists(configPath),
			EnvRefs:      envRefs,
			Lines:        lines,
		}, err
	}

	paths := config.ResolvePaths(repoRoot, configPath, cfg)
	if err := config.ValidatePaths(paths); err != nil {
		lines := []string{
			"ROOM config check",
			fmt.Sprintf("Repo: %s", repoRoot),
			fmt.Sprintf("Config source: %s", paths.ConfigPath),
			fmt.Sprintf("Config status: %s", configStatusLabel(paths.ConfigPath)),
		}
		if envLine := formatEnvReferences(envRefs); envLine != "" {
			lines = append(lines, fmt.Sprintf("Env patch points: %s", envLine))
		}
		lines = append(lines, fmt.Sprintf("Config wiring failed: %v", err))
		return ConfigCheckReport{
			RepoRoot:     repoRoot,
			ConfigPath:   paths.ConfigPath,
			ConfigExists: fsutil.FileExists(paths.ConfigPath),
			Config:       cfg,
			Paths:        paths,
			EnvRefs:      envRefs,
			Lines:        lines,
		}, err
	}
	lines := []string{
		"ROOM config check",
		fmt.Sprintf("Repo: %s", repoRoot),
		fmt.Sprintf("Config source: %s", paths.ConfigPath),
		fmt.Sprintf("Config status: %s", configStatusLabel(paths.ConfigPath)),
		"Config parses cleanly.",
		fmt.Sprintf("Provider: %s", agent.DisplayName(cfg.Agent.Provider)),
		fmt.Sprintf("Runs dir: %s", paths.RunsDir),
	}
	if envLine := formatEnvReferences(envRefs); envLine != "" {
		lines = append(lines, fmt.Sprintf("Env patch points: %s", envLine))
	}
	schemaSignal, err := inspectSchemaContract(paths.SchemaPath)
	if err != nil {
		return ConfigCheckReport{}, err
	}
	schemaHint := schemaContractHint(schemaSignal, paths.SchemaPath)
	if schemaHint != "" {
		lines = append(lines, fmt.Sprintf("Schema contract: %s", schemaHint))
	}

	return ConfigCheckReport{
		RepoRoot:     repoRoot,
		ConfigPath:   paths.ConfigPath,
		ConfigExists: fsutil.FileExists(paths.ConfigPath),
		Config:       cfg,
		Paths:        paths,
		EnvRefs:      envRefs,
		SchemaHint:   schemaHint,
		Lines:        lines,
	}, nil
}

func configStatusLabel(path string) string {
	if fsutil.FileExists(path) {
		return "present"
	}
	return "missing; defaults are in effect"
}
