package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/git"
	"github.com/jcpsimmons/room/internal/version"
)

func TestConfigReportsEffectiveConfig(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	writeRepoFile(t, filepath.Join(repoRoot, ".room", "config.toml"), `
[agent]
provider = "codex"

[run]
default_iterations = 12
max_failures = 4
until_done = true
allow_dirty = true
commit = false
commit_prefix = "loop:"

[codex]
binary = "codex-dev"
model = "gpt-5.4"

[prompt]
max_recent_summaries = 4
max_seen_instructions = 9
force_pivot_on_duplicate = false
`)

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Config(context.Background(), ConfigOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	if !strings.HasSuffix(report.ConfigPath, filepath.Join(".room", "config.toml")) {
		t.Fatalf("config path = %q", report.ConfigPath)
	}
	if !strings.HasSuffix(report.RoomDir, ".room") {
		t.Fatalf("room dir = %q", report.RoomDir)
	}
	if !report.ConfigExists {
		t.Fatal("expected config file to exist")
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"ROOM config",
		"Config status: present",
		"Provider: Codex",
		"Agent binary: codex-dev",
		"Run defaults: iterations=12 max_failures=4 until_done=true allow_dirty=true commit=false prefix=\"loop:\"",
		"Prompt window: summaries=4 seen_instructions=9 force_pivot_on_duplicate=false",
		".room/runs",
		"Codex model: gpt-5.4",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("config report missing %q in:\n%s", want, joined)
		}
	}
}

func TestConfigExplainsMissingFileDefaults(t *testing.T) {
	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Config(context.Background(), ConfigOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if report.ConfigExists {
		t.Fatal("expected missing config file")
	}

	joined := strings.Join(report.Lines, "\n")
	if !strings.Contains(joined, "Config status: missing; defaults are in effect") {
		t.Fatalf("expected missing-config note in:\n%s", joined)
	}
	if !strings.Contains(joined, "Provider: Codex") {
		t.Fatalf("expected default provider in:\n%s", joined)
	}
}

func TestConfigReportsExpandedEnvironmentValues(t *testing.T) {
	t.Setenv("ROOM_TEST_CODEX_BINARY", "codex-lab")
	t.Setenv("ROOM_TEST_CODEX_MODEL", "gpt-5.4")

	repoRoot := initGitRepo(t)
	writeRepoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")

	writeRepoFile(t, filepath.Join(repoRoot, ".room", "config.toml"), `
[codex]
binary = "${ROOM_TEST_CODEX_BINARY}"
model = "${ROOM_TEST_CODEX_MODEL}"
`)

	svc := NewService(Dependencies{
		Git:     git.NewClient(),
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	report, err := svc.Config(context.Background(), ConfigOptions{WorkingDir: repoRoot})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	joined := strings.Join(report.Lines, "\n")
	for _, want := range []string{"Agent binary: codex-lab", "Codex model: gpt-5.4"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("config report missing %q in:\n%s", want, joined)
		}
	}
}
