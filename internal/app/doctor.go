package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/codex"
	"github.com/jcpsimmons/room/internal/config"
	"github.com/jcpsimmons/room/internal/fsutil"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/logs"
	"github.com/jcpsimmons/room/internal/state"
)

var lookPath = exec.LookPath
var execCommandContext = exec.CommandContext

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
	RepoRoot          string        `json:"repo_root"`
	Checks            []DoctorCheck `json:"checks"`
	Lines             []string      `json:"lines"`
	PromptHistoryHint string        `json:"prompt_history_hint,omitempty"`
}

type providerDiagnostics struct {
	Checks          []DoctorCheck
	AuthStatus      string
	AuthDriftInline string
}

func (s *Service) Doctor(ctx context.Context, opts DoctorOptions) (DoctorReport, error) {
	var checks []DoctorCheck
	promptHistoryHint := ""

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

	checks = append(checks, s.providerDiagnostics(ctx, cfg).Checks...)

	checks = append(checks, DoctorCheck{Name: "jq", OK: true, Message: "jq is not required for ROOM v1"})

	paths := config.ResolvePaths(repoRoot, configPath, cfg)
	if ignoreOK, err := gitInfoExcludeProtectsRoom(repoRoot); err != nil {
		checks = append(checks, DoctorCheck{Name: "git_info_exclude", OK: false, Message: err.Error()})
	} else if ignoreOK {
		checks = append(checks, DoctorCheck{Name: "git_info_exclude", OK: true, Message: ".git/info/exclude already protects .room/"})
	} else {
		checks = append(checks, DoctorCheck{Name: "git_info_exclude", OK: false, Message: ".git/info/exclude does not mention .room/; run `room init` or add it manually to keep plain `git status` clean"})
	}
	roomIgnorePath := filepath.Join(repoRoot, ".roomignore")
	if fsutil.FileExists(roomIgnorePath) {
		if err := git.ValidateRoomIgnore(repoRoot); err != nil {
			checks = append(checks, DoctorCheck{
				Name:    "room_ignore",
				OK:      false,
				Message: fmt.Sprintf("malformed .roomignore; ROOM will skip custom ignore rules until fixed: %v", err),
			})
		} else {
			checks = append(checks, DoctorCheck{Name: "room_ignore", OK: true, Message: ".roomignore parses cleanly"})
		}
	}
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

	recentSummaries, malformedSummaries, err := logs.ReadRecentSummariesDetailed(paths.SummariesPath, cfg.Prompt.MaxRecentSummaries)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "history", OK: false, Message: fmt.Sprintf("summary history read failed: %v", err)})
	} else if malformedSummaries > 0 {
		checks = append(checks, DoctorCheck{
			Name:    "history",
			OK:      false,
			Message: fmt.Sprintf("summary history log has %d malformed entrie(s); ROOM will ignore them but prompt context is thinner", malformedSummaries),
		})
	} else if len(recentSummaries) == 0 {
		checks = append(checks, DoctorCheck{Name: "history", OK: true, Message: "summary history log is empty"})
	} else {
		checks = append(checks, DoctorCheck{Name: "history", OK: true, Message: fmt.Sprintf("summary history log parsed %d entrie(s)", len(recentSummaries))})
	}
	priorInstructions, err := logs.ReadSeenInstructions(paths.SeenInstructionsPath, cfg.Prompt.MaxSeenInstructions)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "prompt_history", OK: false, Message: fmt.Sprintf("seen instruction history read failed: %v", err)})
	} else {
		currentInstruction := ""
		if instructionBody, readErr := fsutil.ReadFileIfExists(paths.InstructionPath); readErr != nil {
			checks = append(checks, DoctorCheck{Name: "prompt_history", OK: false, Message: fmt.Sprintf("current instruction read failed: %v", readErr)})
		} else {
			currentInstruction = strings.TrimSpace(string(instructionBody))
		}
		promptHistoryHint, _ = promptHistorySignal(currentInstruction, priorInstructions, recentSummaries)
		if promptHistoryHint != "" {
			checks = append(checks, DoctorCheck{Name: "prompt_history", OK: false, Message: promptHistoryHint})
		}
	}

	assessment, err := assessNewestBundle(paths.RunsDir)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "bundle", OK: false, Message: err.Error()})
	} else if assessment.Hint != "" {
		checks = append(checks, DoctorCheck{Name: "bundle", OK: false, Message: assessment.Hint})
	}
	lockHint, err := runLockHint(paths.RoomDir, s.processAlive)
	if err != nil {
		checks = append(checks, DoctorCheck{Name: "run_lock", OK: false, Message: err.Error()})
	} else if lockHint != "" {
		checks = append(checks, DoctorCheck{Name: "run_lock", OK: false, Message: lockHint})
	}
	if assessment.RunDir != "" && fsutil.FileExists(paths.StatePath) {
		snapshot, err := state.Load(paths.StatePath)
		if err != nil {
			checks = append(checks, DoctorCheck{Name: "run_directory", OK: false, Message: fmt.Sprintf("state load failed: %v", err)})
		} else if lastRunDir := strings.TrimSpace(snapshot.LastRunDirectory); lastRunDir != "" && filepath.Clean(lastRunDir) != filepath.Clean(assessment.RunDir) {
			checks = append(checks, DoctorCheck{
				Name:    "run_directory",
				OK:      false,
				Message: fmt.Sprintf("state points at %s but newest bundle is %s", lastRunDir, assessment.RunDir),
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
		RepoRoot:          repoRoot,
		Checks:            checks,
		PromptHistoryHint: promptHistoryHint,
		Lines:             lines,
	}, nil
}

func gitInfoExcludeProtectsRoom(repoRoot string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".git", "info", "exclude"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == ".room/" {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) providerDiagnostics(ctx context.Context, cfg config.Config) providerDiagnostics {
	provider := agent.NormalizeProvider(cfg.Agent.Provider)
	binary := s.binaryForProvider(cfg)
	displayName := agent.DisplayName(provider)

	diag := providerDiagnostics{
		Checks: []DoctorCheck{
			{Name: "provider_binary", OK: true, Message: fmt.Sprintf("configured %s binary: %s", strings.ToLower(displayName), binary)},
		},
	}

	runner, runnerErr := s.runnerForProvider(provider)
	if runnerErr != nil {
		diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider", OK: false, Message: runnerErr.Error()})
		return diag
	}

	resolvedBinary, err := lookPath(binary)
	if err != nil {
		diag.Checks = append(diag.Checks,
			DoctorCheck{Name: "provider_path", OK: false, Message: fmt.Sprintf("PATH search for %s failed: %v", binary, err)},
			DoctorCheck{Name: "provider", OK: false, Message: fmt.Sprintf("%s binary not found on PATH: %s", displayName, binary)},
		)
		return diag
	}

	diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider_path", OK: true, Message: fmt.Sprintf("PATH search resolved %s to %s", binary, resolvedBinary)})
	versionText, versionErr := runner.Version(ctx, resolvedBinary)
	if versionErr != nil {
		diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider", OK: false, Message: versionErr.Error()})
		return diag
	}

	diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider", OK: true, Message: fmt.Sprintf("%s available: %s", displayName, versionText)})
	switch provider {
	case agent.ProviderClaude:
		if err := claude.ValidateCLI(ctx, binary); err != nil {
			diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider_capabilities", OK: false, Message: err.Error()})
		} else {
			diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider_capabilities", OK: true, Message: "Claude Code CLI supports ROOM's required non-interactive flags"})
		}
	default:
		if err := codex.ValidateVersion(versionText); err != nil {
			diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider_version", OK: false, Message: err.Error()})
		} else {
			diag.Checks = append(diag.Checks, DoctorCheck{Name: "provider_version", OK: true, Message: fmt.Sprintf("Codex version is supported (requires %s or newer)", codex.MinimumSupportedVersion())})
		}
	}

	authArgs := []string{"login", "status"}
	authFailure := "login status failed; authenticate separately before running ROOM"
	if provider == agent.ProviderClaude {
		authArgs = []string{"auth", "status", "--text"}
		authFailure = "auth status failed; authenticate separately before running ROOM"
	}

	statusOut, statusErr := execCommandContext(ctx, resolvedBinary, authArgs...).CombinedOutput()
	if statusErr != nil {
		diag.AuthStatus = fmt.Sprintf("%s %s", displayName, authFailure)
		diag.AuthDriftInline = fmt.Sprintf("%s auth drift: %s", displayName, authFailure)
		diag.Checks = append(diag.Checks, DoctorCheck{Name: "auth", OK: false, Message: diag.AuthStatus})
		return diag
	}

	diag.AuthStatus = strings.TrimSpace(string(statusOut))
	diag.Checks = append(diag.Checks, DoctorCheck{Name: "auth", OK: true, Message: diag.AuthStatus})
	return diag
}
