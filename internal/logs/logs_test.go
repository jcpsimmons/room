package logs

import (
	"encoding/json"
	"fmt"
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

func TestReadRecentSummariesDetailedOnlyScansTailWindow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "summaries.log")

	payload := strings.Join([]string{
		`{not-json`,
		`{"iteration":1,"timestamp":"2026-03-25T12:00:00Z","status":"continue","summary":"first"}`,
		`{"iteration":2,"timestamp":"2026-03-25T12:05:00Z","status":"continue","summary":"second"}`,
		`{"iteration":3,"timestamp":"2026-03-25T12:10:00Z","status":"done","summary":"third"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write summaries: %v", err)
	}

	loaded, malformed, err := ReadRecentSummariesDetailed(path, 2)
	if err != nil {
		t.Fatalf("read recent summaries detailed: %v", err)
	}
	if malformed != 0 {
		t.Fatalf("malformed entries = %d", malformed)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded summaries = %#v", loaded)
	}
	if loaded[0].Iteration != 2 || loaded[1].Iteration != 3 {
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

func TestAppendSeenInstructionPreservesMultilineInstructions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "seen.log")

	want := "Patch the relay\nListen for aliasing\nReturn with a different clock"
	if err := AppendSeenInstruction(path, want); err != nil {
		t.Fatalf("append seen instruction: %v", err)
	}

	loaded, err := ReadSeenInstructions(path, 1)
	if err != nil {
		t.Fatalf("read seen instructions: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded instructions = %#v", loaded)
	}
	if loaded[0] != want {
		t.Fatalf("loaded instruction = %q, want %q", loaded[0], want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if strings.Count(string(data), "\n") != 1 {
		t.Fatalf("expected one log line, got %q", string(data))
	}
}

func TestReadSeenInstructionsSupportsLegacyAndJSONEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "seen.log")

	payload := strings.Join([]string{
		"legacy plain line",
		`{"instruction":"json\nwrapped"}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write seen instructions: %v", err)
	}

	loaded, err := ReadSeenInstructions(path, 5)
	if err != nil {
		t.Fatalf("read seen instructions: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded instructions = %#v", loaded)
	}
	if loaded[0] != "legacy plain line" || loaded[1] != "json\nwrapped" {
		t.Fatalf("loaded instructions = %#v", loaded)
	}
}

func TestReadRecentLogsReturnTailEntriesFromLargeFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	summariesPath := filepath.Join(dir, "summaries.log")
	seenPath := filepath.Join(dir, "seen.log")

	var summaries strings.Builder
	var seen strings.Builder
	for i := 1; i <= 512; i++ {
		summary := SummaryEntry{
			Iteration: i,
			Timestamp: time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute),
			Status:    "continue",
			Summary:   strings.Repeat("x", 64_000),
		}
		if i >= 511 {
			summary.Summary = fmt.Sprintf("tail-%d", i)
		}
		data, err := json.Marshal(summary)
		if err != nil {
			t.Fatalf("marshal summary: %v", err)
		}
		summaries.Write(data)
		summaries.WriteByte('\n')

		instruction := strings.Repeat("y", 64_000)
		if i >= 511 {
			instruction = fmt.Sprintf("tail-%d", i)
		}
		seen.WriteString(instruction)
		seen.WriteByte('\n')
	}
	if err := os.WriteFile(summariesPath, []byte(summaries.String()), 0o644); err != nil {
		t.Fatalf("write summaries: %v", err)
	}
	if err := os.WriteFile(seenPath, []byte(seen.String()), 0o644); err != nil {
		t.Fatalf("write seen instructions: %v", err)
	}

	loadedSummaries, err := ReadRecentSummaries(summariesPath, 2)
	if err != nil {
		t.Fatalf("read recent summaries: %v", err)
	}
	if len(loadedSummaries) != 2 || loadedSummaries[0].Iteration != 511 || loadedSummaries[1].Iteration != 512 {
		t.Fatalf("loaded summaries = %#v", loadedSummaries)
	}
	if loadedSummaries[0].Summary != "tail-511" || loadedSummaries[1].Summary != "tail-512" {
		t.Fatalf("loaded summary tail = %#v", loadedSummaries)
	}

	loadedSeen, err := ReadSeenInstructions(seenPath, 2)
	if err != nil {
		t.Fatalf("read seen instructions: %v", err)
	}
	if len(loadedSeen) != 2 || loadedSeen[0] != "tail-511" || loadedSeen[1] != "tail-512" {
		t.Fatalf("loaded seen tail = %#v", loadedSeen)
	}
}
