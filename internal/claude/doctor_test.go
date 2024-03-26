package claude

import "testing"

func TestValidateHelpRequiresROOMFlags(t *testing.T) {
	t.Parallel()

	help := `
--print
--output-format
--json-schema
--permission-mode
--no-session-persistence
`
	if err := ValidateHelp(help); err != nil {
		t.Fatalf("validate help: %v", err)
	}
}

func TestValidateHelpRejectsMissingFlags(t *testing.T) {
	t.Parallel()

	if err := ValidateHelp("--print"); err == nil {
		t.Fatalf("expected missing flag error")
	}
}
