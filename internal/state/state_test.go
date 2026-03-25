package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)

	snapshot := New("v0.1.0", now)
	snapshot.CurrentIteration = 7
	snapshot.TotalSuccessfulIterations = 5
	snapshot.TotalFailures = 1
	snapshot.LastStatus = "continue"
	snapshot.LastCommitHash = "abc123"
	snapshot.CurrentInstructionHash = InstructionHash("ship it")
	snapshot.LastCodexVersion = "codex-cli 0.116.0"
	snapshot.LastRunAt = now.Add(time.Minute)
	snapshot.LastSummary = "added tests"
	snapshot.LastNextInstruction = "improve observability"

	if err := Save(path, snapshot); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.CurrentIteration != snapshot.CurrentIteration {
		t.Fatalf("current iteration = %d", loaded.CurrentIteration)
	}
	if loaded.LastCommitHash != snapshot.LastCommitHash {
		t.Fatalf("last commit hash = %q", loaded.LastCommitHash)
	}
	if loaded.LastSummary != snapshot.LastSummary {
		t.Fatalf("last summary = %q", loaded.LastSummary)
	}
}

func TestInstructionHashStable(t *testing.T) {
	t.Parallel()

	a := InstructionHash("tighten tests")
	b := InstructionHash("tighten tests")
	c := InstructionHash("expand docs")

	if a != b {
		t.Fatalf("expected stable hash")
	}
	if a == c {
		t.Fatalf("expected different hash values")
	}
}
