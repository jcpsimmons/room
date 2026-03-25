package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	return run(ctx, dir, "status", "--short")
}

func (c CLI) IsDirty(ctx context.Context, dir string) (bool, error) {
	out, err := c.StatusShort(ctx, dir)
	return strings.TrimSpace(out) != "", err
}

func (CLI) Diff(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "diff", "--binary")
}

func (CLI) DiffStats(ctx context.Context, dir string) (DiffStats, error) {
	out, err := run(ctx, dir, "diff", "--numstat")
	if err != nil {
		return DiffStats{}, err
	}
	var stats DiffStats
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
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
	return stats, nil
}

func (CLI) CommitAll(ctx context.Context, dir, message string) (string, error) {
	if _, err := run(ctx, dir, "add", "-A"); err != nil {
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

func run(ctx context.Context, dir string, args ...string) (string, error) {
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
	return strings.TrimSpace(stdout.String()), nil
}
