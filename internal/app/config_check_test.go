package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/version"
)

func TestConfigCheckReportsCleanConfig(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	writeRepoFile(t, filepath.Join(repoRoot, ".room", "config.toml"), `
[agent]
provider = "claude"

[claude]
permission_mode = "bypassPermissions"

[run]
default_iterations = 7
`)

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.ConfigCheck(context.Background(), ConfigCheckOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("config check: %v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"ROOM config check",
		"Config status: present",
		"Config parses cleanly.",
		"Provider: Claude",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("config check missing %q in:\n%s", want, joined)
		}
	}
}

func TestConfigCheckSurfacesInvalidOverrides(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	writeRepoFile(t, filepath.Join(repoRoot, ".room", "config.toml"), `
[agent]
provider = "codex"

[run]
default_iterations = 12
unexpected_toggle = true
`)

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.ConfigCheck(context.Background(), ConfigCheckOptions{WorkingDir: repoRoot})
	if err == nil {
		t.Fatal("expected config check failure")
	}

	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Config parse failed:") {
		t.Fatalf("expected parse failure note in:\n%s", joined)
	}
	if !strings.Contains(joined, "strict mode") {
		t.Fatalf("expected strict parser note in:\n%s", joined)
	}
}

func TestConfigCheckRejectsInstructionPathCollisions(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	writeRepoFile(t, filepath.Join(repoRoot, ".room", "config.toml"), `
[prompt]
instruction_file = ".room/state.json"
`)

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.ConfigCheck(context.Background(), ConfigCheckOptions{WorkingDir: repoRoot})
	if err == nil {
		t.Fatal("expected config wiring failure")
	}

	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Config wiring failed:") {
		t.Fatalf("expected wiring failure note in:\n%s", joined)
	}
	if !strings.Contains(joined, "collides with the ROOM state file") {
		t.Fatalf("expected collision detail in:\n%s", joined)
	}
}

func TestConfigCheckSurfacesSchemaContractDrift(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	roomDir := filepath.Join(repoRoot, ".room")
	if err := os.MkdirAll(roomDir, 0o755); err != nil {
		t.Fatalf("mkdir room: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "schema.json"), []byte("{\"type\":\"object\",\"title\":\"stale\"}\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.ConfigCheck(context.Background(), ConfigCheckOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("config check: %v", err)
	}

	if !strings.Contains(report.SchemaHint, "drifted from this ROOM build") {
		t.Fatalf("schema hint = %q", report.SchemaHint)
	}
	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Schema contract:") || !strings.Contains(joined, "drifted from this ROOM build") {
		t.Fatalf("expected schema drift note in:\n%s", joined)
	}
	data, err := os.ReadFile(filepath.Join(roomDir, "schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if string(data) == string(agent.DefaultSchema()) {
		t.Fatal("config check should not rewrite schema.json")
	}
}
