package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type runLock struct {
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	RepoRoot    string    `json:"repo_root"`
	Provider    string    `json:"provider"`
	RoomVersion string    `json:"room_version"`
	Iteration   int       `json:"iteration,omitempty"`
	Phase       string    `json:"phase,omitempty"`
	RunDir      string    `json:"run_dir,omitempty"`
	PromptPath  string    `json:"prompt_path,omitempty"`
	HeartbeatAt time.Time `json:"heartbeat_at,omitempty"`
}

type activeRunLock struct {
	path string
	lock runLock
	now  Clock
}

type runLockUpdate struct {
	Iteration  int
	Phase      string
	RunDir     string
	PromptPath string
}

func runLockPath(roomDir string) string {
	return filepath.Join(roomDir, "run.lock.json")
}

func (s *Service) acquireRunLock(roomDir, repoRoot, provider string) (*activeRunLock, string, *bundleLockRecovery, error) {
	path := runLockPath(roomDir)
	if err := fsutil.EnsureDir(roomDir); err != nil {
		return nil, "", nil, err
	}

	lock := runLock{
		PID:         os.Getpid(),
		StartedAt:   s.now().UTC(),
		RepoRoot:    repoRoot,
		Provider:    provider,
		RoomVersion: s.version.Version,
	}

	lockNote := ""
	var recovery *bundleLockRecovery
	for attempt := 0; attempt < 4; attempt++ {
		note, nextRecovery, err := s.tryAcquireRunLock(path, lock)
		if err == nil {
			if note != "" {
				lockNote = note
			}
			if nextRecovery != nil {
				recovery = nextRecovery
			}
			return &activeRunLock{path: path, lock: lock, now: s.now}, lockNote, recovery, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", nil, err
		}
		if note != "" {
			lockNote = note
		}
		if nextRecovery != nil {
			recovery = nextRecovery
		}
	}

	return nil, "", nil, fmt.Errorf("run lock at %s kept changing while ROOM tried to acquire it; try again", path)
}

func (l *activeRunLock) Release() error {
	if l == nil {
		return nil
	}
	current, _, err := readRunLock(l.path)
	if err != nil {
		return nil
	}
	if !sameRunLock(current, l.lock) {
		return nil
	}
	if err := removeRunLock(l.path); err != nil {
		return err
	}
	return nil
}

func (l *activeRunLock) Update(update runLockUpdate) error {
	if l == nil {
		return nil
	}
	current, _, err := readRunLock(l.path)
	if err != nil {
		return err
	}
	if !sameRunLock(current, l.lock) {
		return nil
	}
	current.Iteration = update.Iteration
	current.Phase = strings.TrimSpace(update.Phase)
	current.RunDir = strings.TrimSpace(update.RunDir)
	current.PromptPath = strings.TrimSpace(update.PromptPath)
	if l.now != nil {
		current.HeartbeatAt = l.now().UTC()
	} else {
		current.HeartbeatAt = time.Now().UTC()
	}
	return writeRunLock(l.path, current)
}

func readRunLock(path string) (runLock, string, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return runLock{}, "", err
	}
	if len(data) == 0 {
		return runLock{}, "", nil
	}
	var lock runLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return runLock{}, fmt.Sprintf("Hint: unreadable run lock at %s; ROOM will replace it on the next `room run`.", path), nil
	}
	return lock, "", nil
}

func writeRunLock(path string, lock runLock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(path, data, 0o644)
}

func writeRunLockExclusive(path string, lock runLock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFileExclusive(path, data, 0o644)
}

func (s *Service) tryAcquireRunLock(path string, lock runLock) (string, *bundleLockRecovery, error) {
	if err := writeRunLockExclusive(path, lock); err == nil {
		return "", nil, nil
	} else if !errors.Is(err, os.ErrExist) {
		return "", nil, err
	}

	existing, hint, err := readRunLock(path)
	if err != nil {
		return "", nil, err
	}
	if hint != "" {
		if err := removeRunLock(path); err != nil {
			return "", nil, err
		}
		return hint, nil, os.ErrExist
	}
	if existing.PID == 0 {
		if err := removeRunLock(path); err != nil {
			return "", nil, err
		}
		return "Hint: empty run lock reclaimed; ROOM rewired the slot for a fresh run.", nil, os.ErrExist
	}

	alive, err := s.processAlive(existing.PID)
	if err != nil {
		return "", nil, err
	}
	if alive {
		return "", nil, fmt.Errorf("another ROOM run is already active (pid %d started %s)", existing.PID, existing.StartedAt.UTC().Format(time.RFC3339))
	}
	if err := removeRunLock(path); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("Reclaimed stale run lock from pid %d started %s.", existing.PID, existing.StartedAt.UTC().Format(time.RFC3339)), &bundleLockRecovery{
		PID:       existing.PID,
		StartedAt: existing.StartedAt.UTC(),
	}, os.ErrExist
}

func removeRunLock(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sameRunLock(a, b runLock) bool {
	return a.PID == b.PID && a.StartedAt.Equal(b.StartedAt) && a.RepoRoot == b.RepoRoot && a.Provider == b.Provider
}

func runLockHint(roomDir string, alive func(int) (bool, error)) (string, error) {
	path := runLockPath(roomDir)
	lock, hint, err := readRunLock(path)
	if err != nil {
		return "", err
	}
	if hint != "" {
		return hint, nil
	}
	if lock.PID == 0 {
		return "", nil
	}

	if alive == nil {
		alive = processAlive
	}
	isAlive, err := alive(lock.PID)
	if err != nil {
		return "", err
	}
	if isAlive {
		detail := formatRunLockDetail(lock)
		if detail == "" {
			return fmt.Sprintf("Hint: active run lock held by pid %d since %s.", lock.PID, lock.StartedAt.UTC().Format(time.RFC3339)), nil
		}
		return fmt.Sprintf("Hint: active run lock held by pid %d since %s; %s.", lock.PID, lock.StartedAt.UTC().Format(time.RFC3339), detail), nil
	}
	detail := formatRunLockDetail(lock)
	if detail == "" {
		return fmt.Sprintf("Hint: stale run lock from pid %d since %s can be reclaimed on the next `room run`.", lock.PID, lock.StartedAt.UTC().Format(time.RFC3339)), nil
	}
	return fmt.Sprintf("Hint: stale run lock from pid %d since %s can be reclaimed on the next `room run`; %s.", lock.PID, lock.StartedAt.UTC().Format(time.RFC3339), detail), nil
}

func formatRunLockDetail(lock runLock) string {
	parts := make([]string, 0, 4)
	if lock.Iteration > 0 {
		parts = append(parts, fmt.Sprintf("iteration %d", lock.Iteration))
	}
	if phase := strings.TrimSpace(lock.Phase); phase != "" {
		parts = append(parts, fmt.Sprintf("phase %s", phase))
	}
	if runDir := strings.TrimSpace(lock.RunDir); runDir != "" {
		parts = append(parts, fmt.Sprintf("run %s", runDir))
	}
	if !lock.HeartbeatAt.IsZero() {
		parts = append(parts, fmt.Sprintf("heartbeat %s", lock.HeartbeatAt.UTC().Format(time.RFC3339)))
	}
	return strings.Join(parts, ", ")
}
