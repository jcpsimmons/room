package agent

import (
	"fmt"
	"strings"
)

const (
	ProviderCodex  = "codex"
	ProviderClaude = "claude"
)

func NormalizeProvider(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return ProviderCodex
	}
	return normalized
}

func ValidateProvider(value string) error {
	switch NormalizeProvider(value) {
	case ProviderCodex, ProviderClaude:
		return nil
	default:
		return fmt.Errorf("unsupported provider %q; expected %q or %q", strings.TrimSpace(value), ProviderCodex, ProviderClaude)
	}
}

func DisplayName(value string) string {
	switch NormalizeProvider(value) {
	case ProviderClaude:
		return "Claude Code"
	default:
		return "Codex"
	}
}
