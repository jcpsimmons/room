package app

import (
	"context"
	"time"

	"github.com/jcpsimmons/room/internal/codex"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/state"
	"github.com/jcpsimmons/room/internal/version"
)

type Clock func() time.Time

type Dependencies struct {
	Git     git.Client
	Runner  codex.Runner
	Now     Clock
	Version version.Info
}

type Service struct {
	git     git.Client
	runner  codex.Runner
	now     Clock
	version version.Info
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		git:     deps.Git,
		runner:  deps.Runner,
		now:     now,
		version: deps.Version,
	}
}

func (s *Service) codexVersion(ctx context.Context, binary string) (string, error) {
	versionText, err := s.runner.Version(ctx, binary)
	if err != nil {
		return "", err
	}
	if err := codex.ValidateVersion(versionText); err != nil {
		return "", err
	}
	return versionText, nil
}

func (s *Service) saveState(path string, snapshot state.Snapshot) error {
	return state.SaveAt(path, snapshot, s.now())
}
