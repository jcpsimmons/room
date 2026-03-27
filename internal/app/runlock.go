package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jcpsimmons/room/internal/fsutil"
)

type runLock struct {
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	RepoRoot    string    `json:"repo_root"`
	Provider    string    `json:"provider"`
	RoomVersion string    `json:"room_version"`
}

func runLockPath(roomDir string) string {
	return filepath.Join(roomDir, "run.lock.json")
}

func (s *Service) acquireRunLock(roomDir, repoRoot, provider string) (func() error, string, error) {
	path := runLockPath(roomDir)
	if err := fsutil.EnsureDir(roomDir); err != nil {
		return nil, "", err
	}

	existing, err := readRunLock(path)
	if err != nil {
		return nil, "", err
	}
	if existing.PID > 0 {
		alive, err := s.processAlive(existing.PID)
		if err != nil {
			return nil, "", err
		}
		if alive {
			return nil, "", fmt.Errorf("another ROOM run is already active (pid %d started %s)", existing.PID, existing.StartedAt.UTC().Format(time.RFC3339))
		}
	}

	lock := runLock{
		PID:         os.Getpid(),
		StartedAt:   s.now().UTC(),
		RepoRoot:    repoRoot,
		Provider:    provider,
		RoomVersion: s.version.Version,
	}
	if err := writeRunLock(path, lock); err != nil {
		return nil, "", err
	}

	lockNote := ""
	if existing.PID > 0 {
		lockNote = fmt.Sprintf("Reclaimed stale run lock from pid %d started %s.", existing.PID, existing.StartedAt.UTC().Format(time.RFC3339))
	}

	release := func() error {
		current, err := readRunLock(path)
		if err != nil {
			return nil
		}
		if current.PID != lock.PID {
			return nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return release, lockNote, nil
}

func readRunLock(path string) (runLock, error) {
	data, err := fsutil.ReadFileIfExists(path)
	if err != nil {
		return runLock{}, err
	}
	if len(data) == 0 {
		return runLock{}, nil
	}
	var lock runLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return runLock{}, err
	}
	return lock, nil
}

func writeRunLock(path string, lock runLock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(path, data, 0o644)
}

func runLockHint(roomDir string, alive func(int) (bool, error)) (string, error) {
	path := runLockPath(roomDir)
	lock, err := readRunLock(path)
	if err != nil {
		return "", err
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
		return fmt.Sprintf("Hint: active run lock held by pid %d since %s.", lock.PID, lock.StartedAt.UTC().Format(time.RFC3339)), nil
	}
	return fmt.Sprintf("Hint: stale run lock from pid %d since %s can be reclaimed on the next `room run`.", lock.PID, lock.StartedAt.UTC().Format(time.RFC3339)), nil
}
