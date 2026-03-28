package app

import (
	"fmt"
	"strings"

	"github.com/jcpsimmons/room/internal/config"
)

func formatEnvReferences(refs []config.EnvReference) string {
	if len(refs) == 0 {
		return ""
	}

	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		label := ref.Source
		if ref.Source == "default" && ref.DefaultEmpty {
			label = "default(empty)"
		}
		parts = append(parts, fmt.Sprintf("%s=%s", ref.Name, label))
	}
	return strings.Join(parts, ", ")
}
