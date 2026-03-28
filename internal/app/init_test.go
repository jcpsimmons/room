package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/prompt"
	"github.com/jcpsimmons/room/internal/state"
	"github.com/jcpsimmons/room/internal/version"
)

func TestInitUsesCustomPromptWhenInstructionDoesNotExist(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	customPrompt := "Build a release checklist generator for this repo."
	report, err := svc.Init(context.Background(), InitOptions{
		WorkingDir:    repoRoot,
		InitialPrompt: customPrompt,
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	gotInstruction, err := os.ReadFile(filepath.Join(repoRoot, ".room", "instruction.txt"))
	if err != nil {
		t.Fatalf("read instruction: %v", err)
	}
	if string(gotInstruction) != customPrompt+"\n" {
		t.Fatalf("instruction = %q, want %q", string(gotInstruction), customPrompt+"\n")
	}

	snapshot, err := state.Load(filepath.Join(repoRoot, ".room", "state.json"))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if snapshot.CurrentInstructionHash != state.InstructionHash(customPrompt) {
		t.Fatalf("instruction hash = %q, want %q", snapshot.CurrentInstructionHash, state.InstructionHash(customPrompt))
	}

	if !containsLine(report.Lines, "Seeded instruction.txt from the provided initial prompt.") {
		t.Fatalf("expected custom prompt note in report lines: %#v", report.Lines)
	}
}

func TestInitDoesNotOverwriteExistingInstructionWithCustomPrompt(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	if _, err := svc.Init(context.Background(), InitOptions{WorkingDir: repoRoot}); err != nil {
		t.Fatalf("initial init: %v", err)
	}

	customPrompt := "Act like an LLM pair programmer and build a docs site."
	report, err := svc.Init(context.Background(), InitOptions{
		WorkingDir:    repoRoot,
		InitialPrompt: customPrompt,
	})
	if err != nil {
		t.Fatalf("second init: %v", err)
	}

	gotInstruction, err := os.ReadFile(filepath.Join(repoRoot, ".room", "instruction.txt"))
	if err != nil {
		t.Fatalf("read instruction: %v", err)
	}
	if string(gotInstruction) != prompt.DefaultSeedInstruction()+"\n" {
		t.Fatalf("instruction = %q, want default seed", string(gotInstruction))
	}

	if !containsLine(report.Lines, "Instruction file already exists; custom prompt was not applied.") {
		t.Fatalf("expected preserve note in report lines: %#v", report.Lines)
	}
}

func TestInitPinsRoomStateIntoGitExclude(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Init(context.Background(), InitOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	excludePath := filepath.Join(repoRoot, ".git", "info", "exclude")
	excludeData, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(excludeData), ".room/") {
		t.Fatalf("exclude file missing .room/: %q", string(excludeData))
	}

	if status := runGit(t, repoRoot, "status", "--short", "--untracked-files=all"); status != "" {
		t.Fatalf("expected clean git status after init, got %q", status)
	}

	if !containsLine(report.Lines, "Added `.room/` to the git exclude file so plain `git status` stays quiet.") {
		t.Fatalf("expected exclude note in report lines: %#v", report.Lines)
	}
}

func TestInitPinsRoomStateIntoWorktreeExclude(t *testing.T) {
	repoRoot := t.TempDir()
	excludePath := writeLinkedWorktreeGitDir(t, repoRoot)

	svc := NewService(Dependencies{
		Git:     &fakeGit{root: repoRoot},
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Init(context.Background(), InitOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	excludeData, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(excludeData), ".room/") {
		t.Fatalf("exclude file missing .room/: %q", string(excludeData))
	}

	if !containsLine(report.Lines, "Added `.room/` to the git exclude file so plain `git status` stays quiet.") {
		t.Fatalf("expected exclude note in report lines: %#v", report.Lines)
	}
}

func TestInitRefreshesDriftedSchemaContract(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	roomDir := filepath.Join(repoRoot, ".room")
	if err := os.MkdirAll(roomDir, 0o755); err != nil {
		t.Fatalf("mkdir room: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "schema.json"), []byte("{\"type\":\"object\",\"title\":\"stale\"}\n"), 0o644); err != nil {
		t.Fatalf("write stale schema: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	if _, err := svc.Init(context.Background(), InitOptions{WorkingDir: repoRoot}); err != nil {
		t.Fatalf("init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(roomDir, "schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if string(data) != string(agent.DefaultSchema()) {
		t.Fatal("init did not refresh schema.json to the embedded contract")
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}
