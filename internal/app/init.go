package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/codex"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/prompt"
	"github.com/jcpsimmons/room/internal/state"
)

type InitOptions struct {
	WorkingDir string
}

type InitReport struct {
	RepoRoot string   `json:"repo_root"`
	RoomDir  string   `json:"room_dir"`
	Lines    []string `json:"lines"`
}

func (s *Service) Init(ctx context.Context, opts InitOptions) (InitReport, error) {
	repoRoot, err := s.requireRepo(ctx, opts.WorkingDir)
	if err != nil {
		return InitReport{}, err
	}

	cfg := config.Default()
	paths := config.ResolvePaths(repoRoot, filepath.Join(repoRoot, config.DefaultConfigRelPath), cfg)
	if err := fsutil.EnsureDir(paths.RoomDir); err != nil {
		return InitReport{}, err
	}
	if err := fsutil.EnsureDir(paths.RunsDir); err != nil {
		return InitReport{}, err
	}

	if !fsutil.FileExists(paths.ConfigPath) {
		if err := config.Save(paths.ConfigPath, cfg); err != nil {
			return InitReport{}, err
		}
	}
	if !fsutil.FileExists(paths.InstructionPath) {
		if err := fsutil.AtomicWriteFile(paths.InstructionPath, []byte(prompt.DefaultSeedInstruction()+"\n"), 0o644); err != nil {
			return InitReport{}, err
		}
	}
	if !fsutil.FileExists(paths.SchemaPath) {
		if err := codex.WriteSchema(paths.SchemaPath); err != nil {
			return InitReport{}, err
		}
	}
	if !fsutil.FileExists(paths.StatePath) {
		snapshot := state.New(s.version.Version, s.now())
		snapshot.CurrentInstructionHash = state.InstructionHash(prompt.DefaultSeedInstruction())
		if err := state.Save(paths.StatePath, snapshot); err != nil {
			return InitReport{}, err
		}
	}
	for _, path := range []string{paths.SummariesPath, paths.SeenInstructionsPath} {
		if !fsutil.FileExists(path) {
			if err := fsutil.AtomicWriteFile(path, nil, 0o644); err != nil {
				return InitReport{}, err
			}
		}
	}

	lines := []string{
		fmt.Sprintf("Initialized ROOM in %s", repoRoot),
		fmt.Sprintf("State directory: %s", paths.RoomDir),
		"Next steps:",
		"  room doctor",
		"  room inspect",
		"  room run --iterations 5",
	}
	if missingIgnore(repoRoot) {
		lines = append(lines, "Recommendation: add `.room/` to `.gitignore` or `.git/info/exclude` if you do not want local ROOM state committed.")
	}

	return InitReport{
		RepoRoot: repoRoot,
		RoomDir:  paths.RoomDir,
		Lines:    lines,
	}, nil
}

func missingIgnore(repoRoot string) bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		return true
	}
	text := string(data)
	return !strings.Contains(text, ".room/")
}
