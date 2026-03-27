package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeCommitMessage(t *testing.T) {
	t.Parallel()

	got := NormalizeCommitMessage("room:", "tighten config errors")
	if got != "room: tighten config errors" {
		t.Fatalf("normalized = %q", got)
	}

	already := NormalizeCommitMessage("room:", "room: tighten config errors")
	if already != "room: tighten config errors" {
		t.Fatalf("already prefixed = %q", already)
	}
}

func TestRoomDirectoryIsIgnoredByStatusAndDiff(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, ".room", "state.json"), "initial\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "seed room state")

	writeFile(t, filepath.Join(repo, ".room", "state.json"), "updated\n")

	client := NewClient()
	ctx := context.Background()

	dirty, err := client.IsDirty(ctx, repo)
	if err != nil {
		t.Fatalf("is dirty: %v", err)
	}
	if dirty {
		t.Fatalf("expected .room changes to be ignored")
	}

	status, err := client.StatusShort(ctx, repo)
	if err != nil {
		t.Fatalf("status short: %v", err)
	}
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected empty status, got %q", status)
	}

	diff, err := client.Diff(ctx, repo)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("expected empty diff, got %q", diff)
	}

	stats, err := client.DiffStats(ctx, repo)
	if err != nil {
		t.Fatalf("diff stats: %v", err)
	}
	if stats != (DiffStats{}) {
		t.Fatalf("expected empty diff stats, got %#v", stats)
	}
}

func TestCommitAllSkipsRoomDirectory(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, "tracked.txt"), "base\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "init")

	writeFile(t, filepath.Join(repo, ".room", "state.json"), "artifact\n")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "base\nchange\n")

	client := NewClient()
	ctx := context.Background()
	if _, err := client.CommitAll(ctx, repo, "room: update tracked file"); err != nil {
		t.Fatalf("commit all: %v", err)
	}

	paths := strings.Split(strings.TrimSpace(runGit(t, repo, "show", "--pretty=format:", "--name-only", "HEAD")), "\n")
	for _, path := range paths {
		if strings.HasPrefix(strings.TrimSpace(path), ".room/") {
			t.Fatalf("expected HEAD to exclude .room, got paths %v", paths)
		}
	}

	dirty, err := client.IsDirty(ctx, repo)
	if err != nil {
		t.Fatalf("is dirty: %v", err)
	}
	if dirty {
		t.Fatalf("expected ROOM client to ignore remaining .room artifacts")
	}
}

func TestRoomIgnoreHidesFilesFromStatusAndDiff(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, "visible.txt"), "base\n")
	writeFile(t, filepath.Join(repo, "ignored.log"), "base\n")
	writeFile(t, filepath.Join(repo, ".roomignore"), "*.log\nscratch/\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "seed ignore rules")

	writeFile(t, filepath.Join(repo, "visible.txt"), "base\nchange\n")
	writeFile(t, filepath.Join(repo, "ignored.log"), "base\nchange\n")
	writeFile(t, filepath.Join(repo, "scratch", "noise.txt"), "ignore me\n")

	client := NewClient()
	ctx := context.Background()

	status, err := client.StatusShort(ctx, repo)
	if err != nil {
		t.Fatalf("status short: %v", err)
	}
	if strings.Contains(status, "ignored.log") || strings.Contains(status, "scratch/noise.txt") {
		t.Fatalf("expected ignored files to be hidden, got %q", status)
	}
	if !strings.Contains(status, "visible.txt") {
		t.Fatalf("expected visible file in status, got %q", status)
	}

	diff, err := client.Diff(ctx, repo)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if strings.Contains(diff, "ignored.log") {
		t.Fatalf("expected ignored file to be absent from diff, got %q", diff)
	}
	if !strings.Contains(diff, "visible.txt") {
		t.Fatalf("expected visible file in diff, got %q", diff)
	}

	stats, err := client.DiffStats(ctx, repo)
	if err != nil {
		t.Fatalf("diff stats: %v", err)
	}
	if stats.Files != 1 {
		t.Fatalf("expected one visible file in diff stats, got %#v", stats)
	}
}

func TestCommitAllSkipsRoomIgnoreMatches(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, "visible.txt"), "base\n")
	writeFile(t, filepath.Join(repo, "ignored.log"), "base\n")
	writeFile(t, filepath.Join(repo, ".roomignore"), "*.log\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "seed ignore rules")

	writeFile(t, filepath.Join(repo, "visible.txt"), "base\nchange\n")
	writeFile(t, filepath.Join(repo, "ignored.log"), "base\nchange\n")

	client := NewClient()
	ctx := context.Background()
	if _, err := client.CommitAll(ctx, repo, "room: commit visible only"); err != nil {
		t.Fatalf("commit all: %v", err)
	}

	paths := strings.Split(strings.TrimSpace(runGit(t, repo, "show", "--pretty=format:", "--name-only", "HEAD")), "\n")
	for _, path := range paths {
		if strings.TrimSpace(path) == "ignored.log" {
			t.Fatalf("expected HEAD to exclude ignored.log, got paths %v", paths)
		}
	}

	status, err := client.StatusShort(ctx, repo)
	if err != nil {
		t.Fatalf("status short: %v", err)
	}
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected ignored remainder to stay hidden, got %q", status)
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")
	return repo
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
