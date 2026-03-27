package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/codex"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/state"
	"github.com/jcpsimmons/room/internal/version"
)

type Clock func() time.Time

type Dependencies struct {
	Git          git.Client
	Providers    map[string]agent.Runner
	Now          Clock
	Version      version.Info
	ProcessAlive func(int) (bool, error)
}

type Service struct {
	git          git.Client
	providers    map[string]agent.Runner
	now          Clock
	version      version.Info
	processAlive func(int) (bool, error)
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	providers := deps.Providers
	if providers == nil {
		providers = map[string]agent.Runner{
			agent.ProviderCodex:  codex.NewRunner(),
			agent.ProviderClaude: claude.NewRunner(),
		}
	}
	return &Service{
		git:          deps.Git,
		providers:    providers,
		now:          now,
		version:      deps.Version,
		processAlive: processAliveOrDefault(deps.ProcessAlive),
	}
}

func (s *Service) runnerForProvider(provider string) (agent.Runner, error) {
	provider = agent.NormalizeProvider(provider)
	runner, ok := s.providers[provider]
	if !ok {
		return nil, fmt.Errorf("provider %q is not configured", provider)
	}
	return runner, nil
}

func (s *Service) agentVersion(ctx context.Context, cfg config.Config) (string, string, error) {
	provider := agent.NormalizeProvider(cfg.Agent.Provider)
	runner, err := s.runnerForProvider(provider)
	if err != nil {
		return "", "", err
	}

	binary := s.binaryForProvider(cfg)
	versionText, err := runner.Version(ctx, binary)
	if err != nil {
		return "", "", err
	}

	switch provider {
	case agent.ProviderCodex:
		if err := codex.ValidateVersion(versionText); err != nil {
			return "", "", err
		}
	case agent.ProviderClaude:
	default:
		return "", "", fmt.Errorf("unsupported provider %q", provider)
	}

	return provider, versionText, nil
}

func (s *Service) binaryForProvider(cfg config.Config) string {
	def := config.Default()
	switch agent.NormalizeProvider(cfg.Agent.Provider) {
	case agent.ProviderClaude:
		if cfg.Claude.Binary == "" {
			return def.Claude.Binary
		}
		return cfg.Claude.Binary
	default:
		if cfg.Codex.Binary == "" {
			return def.Codex.Binary
		}
		return cfg.Codex.Binary
	}
}

func (s *Service) runOptionsForProvider(cfg config.Config, repoRoot string) agent.RunOptions {
	switch agent.NormalizeProvider(cfg.Agent.Provider) {
	case agent.ProviderClaude:
		return agent.RunOptions{
			Binary:         cfg.Claude.Binary,
			WorkDir:        repoRoot,
			Model:          cfg.Claude.Model,
			PermissionMode: cfg.Claude.PermissionMode,
			Timeout:        time.Duration(cfg.Claude.TimeoutSeconds) * time.Second,
		}
	default:
		return agent.RunOptions{
			Binary:   cfg.Codex.Binary,
			WorkDir:  repoRoot,
			Model:    cfg.Codex.Model,
			Sandbox:  cfg.Codex.Sandbox,
			Approval: cfg.Codex.Approval,
			Timeout:  time.Duration(cfg.Codex.TimeoutSeconds) * time.Second,
		}
	}
}

func (s *Service) saveState(path string, snapshot state.Snapshot) error {
	return state.SaveAt(path, snapshot, s.now())
}

func processAliveOrDefault(fn func(int) (bool, error)) func(int) (bool, error) {
	if fn != nil {
		return fn
	}
	return processAlive
}
