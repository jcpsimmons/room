package codex

import (
	_ "embed"

	"github.com/jcpsimmons/room/internal/fsutil"
)

//go:embed schema.json
var defaultSchema []byte

func DefaultSchema() []byte {
	out := make([]byte, len(defaultSchema))
	copy(out, defaultSchema)
	return out
}

func WriteSchema(path string) error {
	return fsutil.AtomicWriteFile(path, DefaultSchema(), 0o644)
}
