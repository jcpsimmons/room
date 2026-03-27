package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type Snapshot struct {
	CurrentIteration          int       `json:"current_iteration"`
	TotalSuccessfulIterations int       `json:"total_successful_iterations"`
	TotalFailures             int       `json:"total_failures"`
	LastStatus                string    `json:"last_status"`
	LastFailure               string    `json:"last_failure,omitempty"`
	LastFailureRunDirectory   string    `json:"last_failure_run_directory,omitempty"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
	LastRunAt                 time.Time `json:"last_run_at"`
	LastCommitHash            string    `json:"last_commit_hash"`
	CurrentInstructionHash    string    `json:"current_instruction_hash"`
	RoomVersion               string    `json:"room_version"`
	LastProvider              string    `json:"last_provider"`
	LastProviderVersion       string    `json:"last_provider_version"`
	ConsecutiveNoChange       int       `json:"consecutive_no_change"`
	ConsecutiveTinyDiffs      int       `json:"consecutive_tiny_diffs"`
	LastSummary               string    `json:"last_summary"`
	LastNextInstruction       string    `json:"last_next_instruction"`
	LastRunDirectory          string    `json:"last_run_directory"`
}

func New(version string, now time.Time) Snapshot {
	return Snapshot{
		LastStatus:  "idle",
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
		RoomVersion: version,
	}
}

func Load(path string) (Snapshot, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) == 0 {
		return Snapshot{}, errors.New("ROOM state not found; run `room init` first")
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func Save(path string, snapshot Snapshot) error {
	return SaveAt(path, snapshot, time.Now())
}

func SaveAt(path string, snapshot Snapshot, now time.Time) error {
	now = now.UTC()
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = now
	} else {
		snapshot.CreatedAt = snapshot.CreatedAt.UTC()
	}
	snapshot.UpdatedAt = now
	if !snapshot.LastRunAt.IsZero() {
		snapshot.LastRunAt = snapshot.LastRunAt.UTC()
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(path, data, 0o644)
}

func InstructionHash(instruction string) string {
	sum := sha256.Sum256([]byte(instruction))
	return hex.EncodeToString(sum[:])
}
