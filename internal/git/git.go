package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode"

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
	diff, _, err := diffAndStats(ctx, dir)
	if err != nil {
		return "", err
	}
	return diff, nil
}

func (CLI) DiffAndStats(ctx context.Context, dir string) (string, DiffStats, error) {
	return diffAndStats(ctx, dir)
}

func (CLI) DiffStats(ctx context.Context, dir string) (DiffStats, error) {
	_, stats, err := diffAndStats(ctx, dir)
	if err != nil {
		return DiffStats{}, err
	}
	return stats, nil
}

func (CLI) ChangedPaths(ctx context.Context, dir string) ([]string, error) {
	return changedPaths(ctx, dir, true)
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
	raw, err := runRawBytes(ctx, dir, "status", "--short", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil, err
	}
	matcher, err := compileIgnoreFile(filepath.Join(dir, roomIgnoreFileName))
	if err != nil {
		return nil, err
	}
	var entries []statusEntry
	records := bytes.Split(raw, []byte{0})
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 {
			continue
		}
		entry, consumed := parseStatusEntry(record, records[i+1:])
		if consumed > 0 {
			i += consumed
		}
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

func diffAndStats(ctx context.Context, dir string) (string, DiffStats, error) {
	entries, err := statusEntries(ctx, dir)
	if err != nil {
		return "", DiffStats{}, err
	}
	tracked, untracked := diffTargets(entries)
	if len(tracked) == 0 && len(untracked) == 0 {
		return "", DiffStats{}, nil
	}

	var parts []string
	stats := DiffStats{}

	if len(tracked) > 0 {
		diff, err := run(ctx, dir, append([]string{"diff", "--binary", "--"}, tracked...)...)
		if err != nil {
			return "", DiffStats{}, err
		}
		if diff != "" {
			parts = append(parts, diff)
		}

		out, err := run(ctx, dir, append([]string{"diff", "--numstat", "--"}, tracked...)...)
		if err != nil {
			return "", DiffStats{}, err
		}
		stats = addDiffStats(stats, parseDiffStats(out))
	}

	for _, path := range untracked {
		diff, pathStats, err := untrackedDiffAndStats(ctx, dir, path)
		if err != nil {
			return "", DiffStats{}, err
		}
		if diff != "" {
			parts = append(parts, diff)
		}
		stats = addDiffStats(stats, pathStats)
	}

	return strings.Join(parts, "\n"), stats, nil
}

func diffTargets(entries []statusEntry) ([]string, []string) {
	trackedSeen := map[string]struct{}{}
	untrackedSeen := map[string]struct{}{}
	var tracked []string
	var untracked []string
	for _, entry := range entries {
		if entry.primary == "" {
			continue
		}
		if entry.code == "??" {
			if _, ok := untrackedSeen[entry.primary]; ok {
				continue
			}
			untrackedSeen[entry.primary] = struct{}{}
			untracked = append(untracked, entry.primary)
			continue
		}
		if _, ok := trackedSeen[entry.primary]; ok {
			continue
		}
		trackedSeen[entry.primary] = struct{}{}
		tracked = append(tracked, entry.primary)
	}
	return tracked, untracked
}

func untrackedDiffAndStats(ctx context.Context, dir, path string) (string, DiffStats, error) {
	diffOut, err := runRawAllowExitCodes(ctx, dir, []int{1}, "diff", "--binary", "--no-index", "--", os.DevNull, path)
	if err != nil {
		return "", DiffStats{}, err
	}
	statsOut, err := runRawAllowExitCodes(ctx, dir, []int{1}, "diff", "--numstat", "--no-index", "--", os.DevNull, path)
	if err != nil {
		return "", DiffStats{}, err
	}
	return strings.TrimSpace(diffOut), parseDiffStats(statsOut), nil
}

func addDiffStats(base, extra DiffStats) DiffStats {
	base.Files += extra.Files
	base.Added += extra.Added
	base.Deleted += extra.Deleted
	return base
}

func parseStatusEntry(record []byte, rest [][]byte) (statusEntry, int) {
	line := string(record)
	if len(line) < 4 {
		return statusEntry{raw: strings.TrimSpace(line)}, 0
	}
	code := line[:2]
	pathText := line[3:]
	paths := []string{pathText}
	primary := pathText
	raw := code + " " + formatStatusPath(pathText)
	if (code[0] == 'R' || code[0] == 'C') && len(rest) > 0 && len(rest[0]) > 0 {
		previous := string(rest[0])
		paths = []string{pathText, previous}
		primary = pathText
		raw = code + " " + formatStatusPath(previous) + " -> " + formatStatusPath(pathText)
		return statusEntry{
			code:    code,
			raw:     raw,
			paths:   paths,
			primary: primary,
		}, 1
	}
	return statusEntry{
		code:    code,
		raw:     raw,
		paths:   paths,
		primary: primary,
	}, 0
}

func formatStatusPath(path string) string {
	if strings.IndexFunc(path, unicode.IsSpace) >= 0 || strings.ContainsAny(path, "\"\\") {
		return strconv.Quote(path)
	}
	return path
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

func ValidateRoomIgnore(repoRoot string) error {
	path := filepath.Join(repoRoot, roomIgnoreFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if compiledPatternCount(ignore.CompileIgnoreLines(line)) == 0 {
			return fmt.Errorf("invalid .roomignore pattern on line %d: %q", i+1, trimmed)
		}
	}
	return nil
}

func compiledPatternCount(parser ignore.IgnoreParser) int {
	if parser == nil {
		return 0
	}
	value := reflect.ValueOf(parser)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() {
		return 0
	}
	field := value.FieldByName("patterns")
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return 0
	}
	return field.Len()
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
	out, err := runRawBytes(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runRawBytes(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return runRawBytesAllowExitCodes(ctx, dir, nil, args...)
}

func runRawAllowExitCodes(ctx context.Context, dir string, allowed []int, args ...string) (string, error) {
	out, err := runRawBytesAllowExitCodes(ctx, dir, allowed, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runRawBytesAllowExitCodes(ctx context.Context, dir string, allowed []int, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			for _, code := range allowed {
				if exitErr.ExitCode() == code {
					return stdout.Bytes(), nil
				}
			}
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
