package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileOverwritesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.json")

	if err := AtomicWriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if err := AtomicWriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "second" {
		t.Fatalf("written file = %q", data)
	}
}

func TestAtomicWriteFileExclusiveRefusesOverwrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "run.lock.json")

	if err := AtomicWriteFileExclusive(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	err := AtomicWriteFileExclusive(path, []byte("second"), 0o600)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read written file: %v", readErr)
	}
	if string(data) != "first" {
		t.Fatalf("written file = %q", data)
	}
}
