package fsutil

import (
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
