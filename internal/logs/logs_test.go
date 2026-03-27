package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadRecentSummariesHandlesOversizedLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "summaries.log")

	oversized := strings.Repeat("x", 70_000)
	entries := []SummaryEntry{
		{Iteration: 1, Timestamp: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC), Status: "continue", Summary: "first"},
		{Iteration: 2, Timestamp: time.Date(2026, 3, 25, 12, 5, 0, 0, time.UTC), Status: "continue", Summary: oversized},
		{Iteration: 3, Timestamp: time.Date(2026, 3, 25, 12, 10, 0, 0, time.UTC), Status: "done", Summary: "third"},
	}
	var payload strings.Builder
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal summary: %v", err)
		}
		payload.Write(data)
		payload.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(payload.String()), 0o644); err != nil {
		t.Fatalf("write summaries: %v", err)
	}

	loaded, err := ReadRecentSummaries(path, 2)
	if err != nil {
		t.Fatalf("read recent summaries: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded summaries = %#v", loaded)
	}
	if loaded[0].Iteration != 2 || loaded[1].Iteration != 3 {
		t.Fatalf("loaded iterations = %#v", loaded)
	}
	if loaded[0].Summary != oversized {
		t.Fatalf("oversized summary truncated or changed")
	}
}

func TestReadRecentSummariesReportsMalformedEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "summaries.log")

	payload := strings.Join([]string{
		`{"iteration":1,"timestamp":"2026-03-25T12:00:00Z","status":"continue","summary":"first"}`,
		`{not-json`,
		`{"iteration":2,"timestamp":"2026-03-25T12:05:00Z","status":"done","summary":"second"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write summaries: %v", err)
	}

	loaded, malformed, err := ReadRecentSummariesDetailed(path, 10)
	if err != nil {
		t.Fatalf("read recent summaries detailed: %v", err)
	}
	if malformed != 1 {
		t.Fatalf("malformed entries = %d", malformed)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded summaries = %#v", loaded)
	}
	if loaded[0].Iteration != 1 || loaded[1].Iteration != 2 {
		t.Fatalf("loaded iterations = %#v", loaded)
	}
}

func TestReadSeenInstructionsHandlesOversizedLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "seen.log")

	oversized := strings.Repeat("y", 70_000)
	content := "  alpha  \n" + oversized + "\n\n beta \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write seen instructions: %v", err)
	}

	loaded, err := ReadSeenInstructions(path, 2)
	if err != nil {
		t.Fatalf("read seen instructions: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded instructions = %#v", loaded)
	}
	if loaded[0] != oversized || loaded[1] != "beta" {
		t.Fatalf("loaded instructions = %#v", loaded)
	}
}
