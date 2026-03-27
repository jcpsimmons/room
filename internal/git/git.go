package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

type Client interface {
	IsRepo(ctx context.Context, dir string) (bool, error)
	Root(ctx context.Context, dir string) (string, error)
	StatusShort(ctx context.Context, dir string) (string, error)
	IsDirty(ctx context.Context, dir string) (bool, error)
	Diff(ctx context.Context, dir string) (string, error)
	DiffStats(ctx context.Context, dir string) (DiffStats, error)
	CommitAll(ctx context.Context, dir, message string) (string, error)
	RecentCommits(ctx context.Context, dir string, limit int) ([]Commit, error)
	RecentCommitsWithPrefix(ctx context.Context, dir string, limit int, prefix string) ([]Commit, error)
	Head(ctx context.Context, dir string) (string, error)
}

type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

type DiffStats struct {
	Files   int `json:"files"`
	Added   int `json:"added"`
	Deleted int `json:"deleted"`
}

type CLI struct{}

const roomIgnoreFileName = ".roomignore"

func NewClient() Client {
	return CLI{}
}

func (CLI) IsRepo(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(stdout.String()) == "true", nil
}

func (CLI) Root(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(out)), nil
}

func (CLI) StatusShort(ctx context.Context, dir string) (string, error) {
	entries, err := statusEntries(ctx, dir)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.raw)
	}
	return strings.Join(lines, "\n"), nil
}

func (c CLI) IsDirty(ctx context.Context, dir string) (bool, error) {
	out, err := c.StatusShort(ctx, dir)
	return strings.TrimSpace(out) != "", err
}

func (CLI) Diff(ctx context.Context, dir string) (string, error) {
	paths, err := changedPaths(ctx, dir, true)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", nil
	}
	return run(ctx, dir, append([]string{"diff", "--binary", "--"}, paths...)...)
}

func (CLI) DiffAndStats(ctx context.Context, dir string) (string, DiffStats, error) {
	paths, err := changedPaths(ctx, dir, true)
	if err != nil {
		return "", DiffStats{}, err
	}
	if len(paths) == 0 {
		return "", DiffStats{}, nil
	}
	diff, err := run(ctx, dir, append([]string{"diff", "--binary", "--"}, paths...)...)
	if err != nil {
		return "", DiffStats{}, err
	}
	out, err := run(ctx, dir, append([]string{"diff", "--numstat", "--"}, paths...)...)
	if err != nil {
		return "", DiffStats{}, err
	}
	return diff, parseDiffStats(out), nil
}

func (CLI) DiffStats(ctx context.Context, dir string) (DiffStats, error) {
	paths, err := changedPaths(ctx, dir, true)
	if err != nil {
		return DiffStats{}, err
	}
	if len(paths) == 0 {
		return DiffStats{}, nil
	}
	out, err := run(ctx, dir, append([]string{"diff", "--numstat", "--"}, paths...)...)
	if err != nil {
		return DiffStats{}, err
	}
	return parseDiffStats(out), nil
}

func (CLI) CommitAll(ctx context.Context, dir, message string) (string, error) {
	paths, err := changedPaths(ctx, dir, false)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", errors.New("no visible changes to commit")
	}
	if _, err := run(ctx, dir, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return "", err
	}
	if _, err := run(ctx, dir, "commit", "-m", message); err != nil {
		return "", err
	}
	return run(ctx, dir, "rev-parse", "HEAD")
}

func (CLI) RecentCommits(ctx context.Context, dir string, limit int) ([]Commit, error) {
	if limit <= 0 {
		return nil, nil
	}
	out, err := run(ctx, dir, "log", fmt.Sprintf("-%d", limit), "--pretty=format:%H%x1f%s")
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

func (CLI) RecentCommitsWithPrefix(ctx context.Context, dir string, limit int, prefix string) ([]Commit, error) {
	all, err := (CLI{}).RecentCommits(ctx, dir, limit*3)
	if err != nil {
		return nil, err
	}
	var filtered []Commit
	for _, commit := range all {
		if strings.HasPrefix(commit.Subject, prefix) {
			filtered = append(filtered, commit)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

func (CLI) Head(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func NormalizeCommitMessage(prefix, message string) string {
	prefix = strings.TrimSpace(prefix)
	message = strings.TrimSpace(message)
	if message == "" {
		message = "update repository"
	}
	if prefix == "" {
		return message
	}
	if strings.HasPrefix(strings.ToLower(message), strings.ToLower(prefix)) {
		return message
	}
	if strings.HasSuffix(prefix, " ") {
		return prefix + message
	}
	return prefix + " " + message
}

func parseCommits(raw string) []Commit {
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, Commit{Hash: strings.TrimSpace(parts[0]), Subject: strings.TrimSpace(parts[1])})
	}
	return commits
}

func parseDiffStats(raw string) DiffStats {
	var stats DiffStats
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		stats.Files++
		stats.Added += added
		stats.Deleted += deleted
	}
	return stats
}

type statusEntry struct {
	code    string
	raw     string
	paths   []string
	primary string
}

func statusEntries(ctx context.Context, dir string) ([]statusEntry, error) {
	raw, err := runRaw(ctx, dir, "status", "--short", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	matcher, err := compileIgnoreFile(filepath.Join(dir, roomIgnoreFileName))
	if err != nil {
		return nil, err
	}
	var entries []statusEntry
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := parseStatusEntry(line)
		if entry.primary == "" || ignoredEntry(entry, matcher) {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func changedPaths(ctx context.Context, dir string, trackedOnly bool) ([]string, error) {
	entries, err := statusEntries(ctx, dir)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var paths []string
	for _, entry := range entries {
		if trackedOnly && entry.code == "??" {
			continue
		}
		if _, ok := seen[entry.primary]; ok {
			continue
		}
		seen[entry.primary] = struct{}{}
		paths = append(paths, entry.primary)
	}
	return paths, nil
}

func parseStatusEntry(line string) statusEntry {
	if len(line) < 4 {
		return statusEntry{raw: strings.TrimSpace(line)}
	}
	pathText := strings.TrimSpace(line[3:])
	paths := []string{pathText}
	primary := pathText
	if before, after, ok := strings.Cut(pathText, " -> "); ok {
		paths = []string{strings.TrimSpace(before), strings.TrimSpace(after)}
		primary = strings.TrimSpace(after)
	}
	return statusEntry{
		code:    line[:2],
		raw:     line,
		paths:   paths,
		primary: primary,
	}
}

func ignoredEntry(entry statusEntry, matcher ignore.IgnoreParser) bool {
	for _, path := range entry.paths {
		if isIgnoredPath(path, matcher) {
			return true
		}
	}
	return false
}

func compileIgnoreFile(path string) (ignore.IgnoreParser, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return ignore.CompileIgnoreFile(path)
}

func isIgnoredPath(path string, matcher ignore.IgnoreParser) bool {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == ".room" || strings.HasPrefix(normalized, ".room/") {
		return true
	}
	return matcher != nil && matcher.MatchesPath(normalized)
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := runRaw(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runRaw(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
