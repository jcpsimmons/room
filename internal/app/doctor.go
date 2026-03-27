package app

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/codex"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/state"
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

	provider := agent.NormalizeProvider(cfg.Agent.Provider)
	binary := s.binaryForProvider(cfg)
	displayName := agent.DisplayName(provider)

	runner, runnerErr := s.runnerForProvider(provider)
	if runnerErr != nil {
		checks = append(checks, DoctorCheck{Name: "provider", OK: false, Message: runnerErr.Error()})
	} else {
		if _, err := exec.LookPath(binary); err != nil {
			checks = append(checks, DoctorCheck{Name: "provider", OK: false, Message: fmt.Sprintf("%s binary not found: %s", displayName, binary)})
		} else {
			versionText, versionErr := runner.Version(ctx, binary)
			if versionErr != nil {
				checks = append(checks, DoctorCheck{Name: "provider", OK: false, Message: versionErr.Error()})
			} else {
				checks = append(checks, DoctorCheck{Name: "provider", OK: true, Message: fmt.Sprintf("%s available: %s", displayName, versionText)})
				switch provider {
				case agent.ProviderClaude:
					if err := claude.ValidateCLI(ctx, binary); err != nil {
						checks = append(checks, DoctorCheck{Name: "provider_capabilities", OK: false, Message: err.Error()})
					} else {
						checks = append(checks, DoctorCheck{Name: "provider_capabilities", OK: true, Message: "Claude Code CLI supports ROOM's required non-interactive flags"})
					}
				default:
					if err := codex.ValidateVersion(versionText); err != nil {
						checks = append(checks, DoctorCheck{Name: "provider_version", OK: false, Message: err.Error()})
					} else {
						checks = append(checks, DoctorCheck{Name: "provider_version", OK: true, Message: fmt.Sprintf("Codex version is supported (requires %s or newer)", codex.MinimumSupportedVersion())})
					}
				}
			}

			authArgs := []string{"login", "status"}
			authFailure := "login status failed; authenticate separately before running ROOM"
			if provider == agent.ProviderClaude {
				authArgs = []string{"auth", "status", "--text"}
				authFailure = "auth status failed; authenticate separately before running ROOM"
			}
			statusOut, statusErr := exec.CommandContext(ctx, binary, authArgs...).CombinedOutput()
			if statusErr != nil {
				checks = append(checks, DoctorCheck{Name: "auth", OK: false, Message: fmt.Sprintf("%s %s", displayName, authFailure)})
			} else {
				checks = append(checks, DoctorCheck{Name: "auth", OK: true, Message: strings.TrimSpace(string(statusOut))})
			}
		}
	}

	checks = append(checks, DoctorCheck{Name: "jq", OK: true, Message: "jq is not required for ROOM v1"})

	paths := config.ResolvePaths(repoRoot, configPath, cfg)
	if fsutil.DirExists(paths.RoomDir) {
		var problems []string
		if !fsutil.FileExists(paths.ConfigPath) {
			problems = append(problems, "missing config.toml")
		}
		if !fsutil.FileExists(paths.InstructionPath) {
			problems = append(problems, "missing instruction.txt")
		}
		if !fsutil.FileExists(paths.SchemaPath) {
			problems = append(problems, "missing schema.json")
		}
		if !fsutil.FileExists(paths.StatePath) {
			problems = append(problems, "missing state.json")
		} else if _, err := state.Load(paths.StatePath); err != nil {
			problems = append(problems, fmt.Sprintf("state load failed: %v", err))
		}
		if len(problems) == 0 {
			checks = append(checks, DoctorCheck{Name: "state", OK: true, Message: fmt.Sprintf("ROOM state directory is healthy: %s", paths.RoomDir)})
		} else {
			checks = append(checks, DoctorCheck{Name: "state", OK: false, Message: strings.Join(problems, "; ")})
		}
	} else {
		checks = append(checks, DoctorCheck{Name: "state", OK: true, Message: "ROOM is not initialized yet; `room init` will create state files"})
	}

	latestRunDir, bundleHint, err := newestBundleHint(paths.RunsDir)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "bundle", OK: false, Message: err.Error()})
	} else if bundleHint != "" {
		checks = append(checks, DoctorCheck{Name: "bundle", OK: false, Message: bundleHint})
	}
	lockHint, err := runLockHint(paths.RoomDir, s.processAlive)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "run_lock", OK: false, Message: err.Error()})
	} else if lockHint != "" {
		checks = append(checks, DoctorCheck{Name: "run_lock", OK: false, Message: lockHint})
	}
	if latestRunDir != "" && fsutil.FileExists(paths.StatePath) {
		snapshot, err := state.Load(paths.StatePath)
		if err != nil {
			checks = append(checks, DoctorCheck{Name: "run_directory", OK: false, Message: fmt.Sprintf("state load failed: %v", err)})
		} else if lastRunDir := strings.TrimSpace(snapshot.LastRunDirectory); lastRunDir != "" && filepath.Clean(lastRunDir) != filepath.Clean(latestRunDir) {
			checks = append(checks, DoctorCheck{
				Name:    "run_directory",
				OK:      false,
				Message: fmt.Sprintf("state points at %s but newest bundle is %s", lastRunDir, latestRunDir),
			})
		}
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
	checks = append(checks, DoctorCheck{Name: "expectation", OK: true, Message: "The selected agent CLI must be installed and authenticated separately; ROOM does not manage installation or login"})

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
