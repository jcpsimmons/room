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

func TestCommitIdentityReportsAuthorIdentity(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	client := NewClient()

	identity, err := client.CommitIdentity(context.Background(), repo)
	if err != nil {
		t.Fatalf("commit identity: %v", err)
	}
	if identity != "Test User <test@example.com>" {
		t.Fatalf("identity = %q", identity)
	}
}

func TestCommitIdentityFailsWithoutConfiguredAuthor(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing-gitconfig"))
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_COMMITTER_EMAIL", "")
	runGit(t, repo, "init")
	client := NewClient()

	if _, err := client.CommitIdentity(context.Background(), repo); err == nil {
		t.Fatal("expected commit identity failure")
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

func TestMalformedRoomIgnoreFallsBackToBuiltInIgnore(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, ".roomignore"), "[\n")
	writeFile(t, filepath.Join(repo, "visible.txt"), "base\n")
	writeFile(t, filepath.Join(repo, ".room", "state.json"), "base\n")
	runGit(t, repo, "add", ".roomignore", "visible.txt")
	runGit(t, repo, "commit", "-m", "seed malformed ignore")

	writeFile(t, filepath.Join(repo, "visible.txt"), "base\nchange\n")
	writeFile(t, filepath.Join(repo, ".room", "state.json"), "updated\n")

	client := NewClient()
	ctx := context.Background()

	if err := ValidateRoomIgnore(repo); err == nil {
		t.Fatal("expected malformed .roomignore to fail validation")
	}

	status, err := client.StatusShort(ctx, repo)
	if err != nil {
		t.Fatalf("status short: %v", err)
	}
	if !strings.Contains(status, "visible.txt") {
		t.Fatalf("expected visible file in status, got %q", status)
	}
	if strings.Contains(status, ".room/state.json") {
		t.Fatalf("expected built-in .room ignore to survive malformed .roomignore, got %q", status)
	}

	if _, err := client.CommitAll(ctx, repo, "room: update visible file"); err != nil {
		t.Fatalf("commit all: %v", err)
	}

	paths := strings.Split(strings.TrimSpace(runGit(t, repo, "show", "--pretty=format:", "--name-only", "HEAD")), "\n")
	for _, path := range paths {
		if strings.HasPrefix(strings.TrimSpace(path), ".room/") {
			t.Fatalf("expected HEAD to exclude .room artifacts, got paths %v", paths)
		}
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

func TestSpacedPathsFlowThroughStatusDiffAndCommit(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, "space name.txt"), "base\n")
	runGit(t, repo, "add", "space name.txt")
	runGit(t, repo, "commit", "-m", "init")

	writeFile(t, filepath.Join(repo, "space name.txt"), "base\nchange\n")

	client := NewClient()
	ctx := context.Background()

	status, err := client.StatusShort(ctx, repo)
	if err != nil {
		t.Fatalf("status short: %v", err)
	}
	if !strings.Contains(status, "space name.txt") {
		t.Fatalf("expected spaced file in status, got %q", status)
	}

	diff, err := client.Diff(ctx, repo)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(diff, "space name.txt") {
		t.Fatalf("expected spaced file in diff, got %q", diff)
	}

	if _, err := client.CommitAll(ctx, repo, "room: commit spaced file"); err != nil {
		t.Fatalf("commit all: %v", err)
	}

	headPaths := runGit(t, repo, "show", "--pretty=format:", "--name-only", "HEAD")
	if !strings.Contains(headPaths, "space name.txt") {
		t.Fatalf("expected spaced file in HEAD, got %q", headPaths)
	}
}

func TestUntrackedFilesFlowThroughDiffAndStats(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, "tracked.txt"), "base\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "init")

	writeFile(t, filepath.Join(repo, "tracked.txt"), "base\nchange\n")
	writeFile(t, filepath.Join(repo, "fresh.txt"), "pulse\nwave\n")

	client := NewClient()
	ctx := context.Background()

	diff, err := client.Diff(ctx, repo)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	for _, want := range []string{"tracked.txt", "fresh.txt", "new file mode 100644"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("expected diff to contain %q, got %q", want, diff)
		}
	}

	stats, err := client.DiffStats(ctx, repo)
	if err != nil {
		t.Fatalf("diff stats: %v", err)
	}
	if stats.Files != 2 || stats.Added != 3 || stats.Deleted != 0 {
		t.Fatalf("unexpected diff stats %#v", stats)
	}
}

func TestRenameEntriesPreserveBothPathsForCommitAll(t *testing.T) {
	t.Parallel()

	repo := setupGitRepo(t)
	writeFile(t, filepath.Join(repo, "old space.txt"), "base\n")
	runGit(t, repo, "add", "old space.txt")
	runGit(t, repo, "commit", "-m", "init")

	runGit(t, repo, "mv", "old space.txt", "new space.txt")

	client := NewClient()
	ctx := context.Background()

	status, err := client.StatusShort(ctx, repo)
	if err != nil {
		t.Fatalf("status short: %v", err)
	}
	if !strings.Contains(status, "old space.txt") || !strings.Contains(status, "new space.txt") {
		t.Fatalf("expected rename paths in status, got %q", status)
	}

	if _, err := client.CommitAll(ctx, repo, "room: rename spaced file"); err != nil {
		t.Fatalf("commit all: %v", err)
	}

	nameStatus := runGit(t, repo, "show", "--pretty=format:", "--name-status", "HEAD")
	if !strings.Contains(nameStatus, "old space.txt") || !strings.Contains(nameStatus, "new space.txt") {
		t.Fatalf("expected rename paths in HEAD, got %q", nameStatus)
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
