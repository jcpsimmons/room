package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/version"
)

func TestAcquireRunLockPreventsOverwriteWhenSlotAlreadyExists(t *testing.T) {
	t.Parallel()

	roomDir := t.TempDir()
	lockPath := runLockPath(roomDir)
	stale := runLock{
		PID:         4242,
		StartedAt:   time.Date(2026, 3, 25, 11, 0, 0, 0, time.UTC),
		RepoRoot:    "/tmp/stale",
		Provider:    "codex",
		RoomVersion: "dev",
	}
	if err := writeRunLock(lockPath, stale); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	svc := NewService(Dependencies{
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
		ProcessAlive: func(pid int) (bool, error) {
			if pid != stale.PID {
				t.Fatalf("unexpected pid probe: %d", pid)
			}
			return false, nil
		},
	})

	lock, note, recovery, err := svc.acquireRunLock(roomDir, "/tmp/repo", "codex")
	if err != nil {
		t.Fatalf("acquire run lock: %v", err)
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			t.Fatalf("release run lock: %v", releaseErr)
		}
	}()

	if note != "Reclaimed stale run lock from pid 4242 started 2026-03-25T11:00:00Z." {
		t.Fatalf("lock note = %q", note)
	}
	if recovery == nil || recovery.PID != stale.PID {
		t.Fatalf("recovery = %#v", recovery)
	}

	current, hint, readErr := readRunLock(lockPath)
	if readErr != nil {
		t.Fatalf("read lock: %v", readErr)
	}
	if hint != "" {
		t.Fatalf("unexpected lock hint: %q", hint)
	}
	if current.PID != os.Getpid() {
		t.Fatalf("expected current pid %d, got %d", os.Getpid(), current.PID)
	}
	if current.StartedAt.Equal(stale.StartedAt) {
		t.Fatalf("expected lock timestamp to be replaced")
	}
}

func TestAcquireRunLockReclaimsEmptyFile(t *testing.T) {
	t.Parallel()

	roomDir := t.TempDir()
	lockPath := runLockPath(roomDir)
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("write empty lock: %v", err)
	}

	svc := NewService(Dependencies{
		Now:     fixedClock(),
		Version: version.Info{Version: "dev"},
	})

	lock, note, recovery, err := svc.acquireRunLock(roomDir, "/tmp/repo", "codex")
	if err != nil {
		t.Fatalf("acquire run lock: %v", err)
	}
	if recovery != nil {
		t.Fatalf("expected no stale lock recovery, got %#v", recovery)
	}
	if note != "Hint: empty run lock reclaimed; ROOM rewired the slot for a fresh run." {
		t.Fatalf("lock note = %q", note)
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		t.Fatalf("release run lock: %v", releaseErr)
	}
	if _, statErr := os.Stat(lockPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected lock removal, stat err=%v", statErr)
	}
}

func TestAcquireRunLockUpdatesLiveProgress(t *testing.T) {
	t.Parallel()

	roomDir := t.TempDir()
	now := sequencedClock(
		time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 25, 12, 0, 3, 0, time.UTC),
	)
	svc := NewService(Dependencies{
		Now:     now,
		Version: version.Info{Version: "dev"},
	})

	lock, _, _, err := svc.acquireRunLock(roomDir, "/tmp/repo", "codex")
	if err != nil {
		t.Fatalf("acquire run lock: %v", err)
	}
	if err := lock.Update(runLockUpdate{
		Iteration:  7,
		Phase:      string(RunProgressPhaseAgentExecutionPulse),
		RunDir:     "/tmp/repo/.room/runs/0007",
		PromptPath: "/tmp/repo/.room/runs/0007/prompt.txt",
	}); err != nil {
		t.Fatalf("update run lock: %v", err)
	}

	current, hint, err := readRunLock(runLockPath(roomDir))
	if err != nil {
		t.Fatalf("read run lock: %v", err)
	}
	if hint != "" {
		t.Fatalf("unexpected hint: %q", hint)
	}
	if current.Iteration != 7 {
		t.Fatalf("iteration = %d", current.Iteration)
	}
	if current.Phase != string(RunProgressPhaseAgentExecutionPulse) {
		t.Fatalf("phase = %q", current.Phase)
	}
	if current.RunDir != "/tmp/repo/.room/runs/0007" {
		t.Fatalf("run dir = %q", current.RunDir)
	}
	if current.PromptPath != "/tmp/repo/.room/runs/0007/prompt.txt" {
		t.Fatalf("prompt path = %q", current.PromptPath)
	}
	if !current.HeartbeatAt.Equal(time.Date(2026, 3, 25, 12, 0, 3, 0, time.UTC)) {
		t.Fatalf("heartbeat = %s", current.HeartbeatAt)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release run lock: %v", err)
	}
}
