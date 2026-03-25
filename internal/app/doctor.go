package app

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
)

type DoctorOptions struct {
	WorkingDir string
	ConfigPath string
}

type DoctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type DoctorReport struct {
	RepoRoot string        `json:"repo_root"`
	Checks   []DoctorCheck `json:"checks"`
	Lines    []string      `json:"lines"`
}

func (s *Service) Doctor(ctx context.Context, opts DoctorOptions) (DoctorReport, error) {
	var checks []DoctorCheck

	if _, err := exec.LookPath("git"); err != nil {
		checks = append(checks, DoctorCheck{Name: "git", OK: false, Message: "git is not installed"})
	} else {
		checks = append(checks, DoctorCheck{Name: "git", OK: true, Message: "git is available"})
	}

	repoOK, err := s.git.IsRepo(ctx, opts.WorkingDir)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "repo", OK: false, Message: err.Error()})
	} else if !repoOK {
		checks = append(checks, DoctorCheck{Name: "repo", OK: false, Message: "current directory is not a git repository"})
	} else {
		checks = append(checks, DoctorCheck{Name: "repo", OK: true, Message: "current directory is a git repository"})
	}

	repoRoot := opts.WorkingDir
	if repoOK {
		repoRoot, _ = s.git.Root(ctx, opts.WorkingDir)
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(repoRoot, config.DefaultConfigRelPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "config", OK: false, Message: fmt.Sprintf("config parse failed: %v", err)})
	} else if fsutil.FileExists(configPath) {
		checks = append(checks, DoctorCheck{Name: "config", OK: true, Message: fmt.Sprintf("config parses: %s", configPath)})
	} else {
		checks = append(checks, DoctorCheck{Name: "config", OK: true, Message: "config not initialized yet; `room init` will create one"})
		cfg = config.Default()
	}

	codexBinary := cfg.Codex.Binary
	if strings.TrimSpace(codexBinary) == "" {
		codexBinary = "codex"
	}
	if _, err := exec.LookPath(codexBinary); err != nil {
		checks = append(checks, DoctorCheck{Name: "codex", OK: false, Message: fmt.Sprintf("Codex binary not found: %s", codexBinary)})
	} else {
		versionText, versionErr := s.runner.Version(ctx, codexBinary)
		if versionErr != nil {
			checks = append(checks, DoctorCheck{Name: "codex", OK: false, Message: versionErr.Error()})
		} else {
			checks = append(checks, DoctorCheck{Name: "codex", OK: true, Message: fmt.Sprintf("Codex available: %s", versionText)})
		}
		statusOut, statusErr := exec.CommandContext(ctx, codexBinary, "login", "status").CombinedOutput()
		if statusErr != nil {
			checks = append(checks, DoctorCheck{Name: "auth", OK: false, Message: "Codex login status failed; authenticate separately before running ROOM"})
		} else {
			checks = append(checks, DoctorCheck{Name: "auth", OK: true, Message: strings.TrimSpace(string(statusOut))})
		}
	}

	checks = append(checks, DoctorCheck{Name: "jq", OK: true, Message: "jq is not required for ROOM v1"})

	paths := config.ResolvePaths(repoRoot, configPath, cfg)
	if fsutil.DirExists(paths.RoomDir) {
		checks = append(checks, DoctorCheck{Name: "state", OK: true, Message: fmt.Sprintf("ROOM state directory exists: %s", paths.RoomDir)})
	} else {
		checks = append(checks, DoctorCheck{Name: "state", OK: true, Message: "ROOM is not initialized yet; `room init` will create state files"})
	}
	writeTarget := filepath.Join(repoRoot, ".room-doctor-write-test")
	if fsutil.DirExists(paths.RoomDir) {
		writeTarget = filepath.Join(paths.RoomDir, ".doctor-write-test")
	}
	if err := fsutil.TouchWritable(writeTarget); err != nil {
		checks = append(checks, DoctorCheck{Name: "write", OK: false, Message: fmt.Sprintf("write test failed: %v", err)})
	} else {
		checks = append(checks, DoctorCheck{Name: "write", OK: true, Message: "ROOM can write to disk"})
	}
	checks = append(checks, DoctorCheck{Name: "expectation", OK: true, Message: "Codex must be installed and authenticated separately; ROOM does not manage installation or login"})

	lines := []string{"ROOM doctor"}
	for _, check := range checks {
		prefix := "ok"
		if !check.OK {
			prefix = "fail"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", prefix, check.Name, check.Message))
	}

	return DoctorReport{
		RepoRoot: repoRoot,
		Checks:   checks,
		Lines:    lines,
	}, nil
}
