//go:build !windows

package fsutil

import (
	"os"
	"path/filepath"
)

func syncParentDir(path string) error {
	dir := filepath.Dir(path)
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return f.Sync()
}
