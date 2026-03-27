package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/prompt"
	"github.com/jcpsimmons/room/internal/state"
)

type InitOptions struct {
	WorkingDir    string
	InitialPrompt string
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
	seedInstruction := prompt.DefaultSeedInstruction()
	customPrompt := strings.TrimSpace(opts.InitialPrompt)
	if customPrompt != "" {
		seedInstruction = customPrompt
	}

	wroteInstruction := false
	if !fsutil.FileExists(paths.InstructionPath) {
		if err := fsutil.AtomicWriteFile(paths.InstructionPath, []byte(seedInstruction+"\n"), 0o644); err != nil {
			return InitReport{}, err
		}
		wroteInstruction = true
	}
	if !fsutil.FileExists(paths.SchemaPath) {
		if err := agent.WriteSchema(paths.SchemaPath); err != nil {
			return InitReport{}, err
		}
	}
	if !fsutil.FileExists(paths.StatePath) {
		currentInstruction, err := requireInstructionSignal(paths.InstructionPath)
		if err != nil {
			return InitReport{}, err
		}
		snapshot := state.New(s.version.Version, s.now())
		snapshot.CurrentInstructionHash = state.InstructionHash(currentInstruction)
		if err := s.saveState(paths.StatePath, snapshot); err != nil {
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
		"  room config-check",
		"  room inspect",
		"  room run --iterations 5",
	}
	if customPrompt != "" {
		if wroteInstruction {
			lines = append(lines, "Seeded instruction.txt from the provided initial prompt.")
		} else {
			lines = append(lines, "Instruction file already exists; custom prompt was not applied.")
		}
	}
	if missingIgnore(repoRoot) {
		if wrote, err := ensureRoomIgnored(repoRoot); err == nil && wrote {
			lines = append(lines, "Added `.room/` to the git exclude file so plain `git status` stays quiet.")
		} else {
			lines = append(lines, "ROOM ignores `.room/` in its own dirty checks, diffs, and commits.")
			lines = append(lines, "Recommendation: add `.room/` to the git exclude file or `.gitignore` if you also want plain `git status` to stay clean.")
		}
	}

	return InitReport{
		RepoRoot: repoRoot,
		RoomDir:  paths.RoomDir,
		Lines:    lines,
	}, nil
}

func missingIgnore(repoRoot string) bool {
	return !roomIgnoreConfigured(repoRoot)
}

func roomIgnoreConfigured(repoRoot string) bool {
	paths := []string{filepath.Join(repoRoot, ".gitignore")}
	if excludePath, err := gitInfoExcludePath(repoRoot); err == nil {
		paths = append(paths, excludePath)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == ".room/" {
				return true
			}
		}
	}
	return false
}

func ensureRoomIgnored(repoRoot string) (bool, error) {
	if roomIgnoreConfigured(repoRoot) {
		return false, nil
	}

	excludePath, err := gitInfoExcludePath(repoRoot)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed != "" {
		trimmed += "\n"
	}
	trimmed += ".room/\n"

	if err := fsutil.AtomicWriteFile(excludePath, []byte(trimmed), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
