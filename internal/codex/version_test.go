package codex

import "testing"

func TestParseVersionExtractsSemanticVersion(t *testing.T) {
	t.Parallel()

	version, err := ParseVersion("codex-cli 0.116.3")
	if err != nil {
		t.Fatalf("parse version: %v", err)
	}
	if version.String() != "0.116.3" {
		t.Fatalf("version = %s", version.String())
	}
}

func TestValidateVersionRejectsUnsupportedVersions(t *testing.T) {
	t.Parallel()

	if err := ValidateVersion("codex-cli 0.115.9"); err == nil {
		t.Fatalf("expected unsupported version error")
	}
	if err := ValidateVersion("codex"); err == nil {
		t.Fatalf("expected parse failure for malformed version text")
	}
	if err := ValidateVersion("codex-cli 0.116.0"); err != nil {
		t.Fatalf("expected minimum supported version to pass: %v", err)
	}
}
