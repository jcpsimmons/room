package logs

import (
	"encoding/json"
	"errors"
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
	FocusAreas   []string  `json:"focus_areas,omitempty"`
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
	if limit <= 0 {
		return nil, nil
	}
	lines, err := readRecentLogLines(path, limit)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry SummaryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func ReadRecentSummariesDetailed(path string, limit int) (entries []SummaryEntry, malformed int, err error) {
	if limit <= 0 {
		return nil, 0, nil
	}
	lines, err := readRecentLogLines(path, limit)
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
	lines, err := readRecentLogLines(path, limit)
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

func readRecentLogLines(path string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
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

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return nil, nil
	}

	const blockSize = 32 * 1024
	lines := make([]string, 0, limit)
	fragment := make([]byte, 0, blockSize)
	offset := info.Size()
	for offset > 0 && len(lines) < limit {
		size := int64(blockSize)
		if offset < size {
			size = offset
		}
		offset -= size

		buf := make([]byte, size)
		if _, err := f.ReadAt(buf, offset); err != nil {
			return nil, err
		}
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				if len(fragment) > 0 {
					lines = append(lines, reverseFragment(fragment))
					fragment = fragment[:0]
				}
				continue
			}
			fragment = append(fragment, buf[i])
		}
	}
	if len(fragment) > 0 && len(lines) < limit {
		lines = append(lines, reverseFragment(fragment))
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}

func reverseFragment(fragment []byte) string {
	buf := make([]byte, len(fragment))
	for i := range fragment {
		buf[i] = fragment[len(fragment)-1-i]
	}
	return string(buf)
}
