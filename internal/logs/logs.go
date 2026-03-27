package logs

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type SummaryEntry struct {
	Iteration    int       `json:"iteration"`
	Timestamp    time.Time `json:"timestamp"`
	Status       string    `json:"status"`
	Summary      string    `json:"summary"`
	CommitHash   string    `json:"commit_hash"`
	ChangedFiles int       `json:"changed_files"`
	LinesAdded   int       `json:"lines_added"`
	LinesDeleted int       `json:"lines_deleted"`
}

func AppendSummary(path string, entry SummaryEntry) (err error) {
	if err := fsutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	entry.Timestamp = entry.Timestamp.UTC()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func ReadRecentSummaries(path string, limit int) (entries []SummaryEntry, err error) {
	entries, _, err = ReadRecentSummariesDetailed(path, limit)
	return entries, err
}

func ReadRecentSummariesDetailed(path string, limit int) (entries []SummaryEntry, malformed int, err error) {
	if limit <= 0 {
		return nil, 0, nil
	}
	lines, err := readLogLines(path)
	if err != nil {
		return nil, 0, err
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry SummaryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			malformed++
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) <= limit {
		return entries, malformed, nil
	}
	return entries[len(entries)-limit:], malformed, nil
}

func AppendSeenInstruction(path, instruction string) (err error) {
	if err := fsutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	trimmed := strings.TrimSpace(instruction)
	if trimmed == "" {
		return nil
	}
	_, err = f.WriteString(trimmed + "\n")
	return err
}

func ReadSeenInstructions(path string, limit int) (values []string, err error) {
	if limit <= 0 {
		return nil, nil
	}
	lines, err := readLogLines(path)
	if err != nil {
		return nil, err
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			values = append(values, line)
		}
	}
	if len(values) <= limit {
		return values, nil
	}
	return values[len(values)-limit:], nil
}

func readLogLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	reader := bufio.NewReader(f)
	var lines []string
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
	}
	return lines, nil
}
