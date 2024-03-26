package claude

import "testing"

func TestParseOutputUsesStructuredOutput(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"is_error":false,"structured_output":{"summary":"added tests","next_instruction":"improve diagnostics","status":"continue","commit_message":"add tests"}}`)
	result, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if result.Status != "continue" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestParseOutputReturnsClaudeErrors(t *testing.T) {
	t.Parallel()

	if _, err := ParseOutput([]byte(`{"is_error":true,"result":"Not logged in"}`)); err == nil {
		t.Fatalf("expected claude error")
	}
}

func TestParseOutputRejectsMissingStructuredOutput(t *testing.T) {
	t.Parallel()

	if _, err := ParseOutput([]byte(`{"is_error":false,"result":""}`)); err == nil {
		t.Fatalf("expected structured output error")
	}
}
