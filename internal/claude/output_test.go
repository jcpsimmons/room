package claude

import (
	"errors"
	"strings"
	"testing"
)

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

func TestParseOutputRejectsEnvelopeDrift(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"is_error":false,"structured_output":{"summary":"added tests","next_instruction":"improve diagnostics","status":"continue","commit_message":"add tests"},"noise":"extra"}`)
	_, err := ParseOutput(raw)
	if err == nil {
		t.Fatalf("expected envelope error")
	}
	if !errors.Is(err, ErrMalformedOutputEnvelope) {
		t.Fatalf("expected malformed envelope sentinel, got %v", err)
	}
}

func TestParseOutputIgnoresWrapperNoiseAroundEnvelope(t *testing.T) {
	t.Parallel()

	raw := []byte("claude wrapper booting\n\n{\"is_error\":false,\"structured_output\":{\"summary\":\"added tests\",\"next_instruction\":\"improve diagnostics\",\"status\":\"continue\",\"commit_message\":\"add tests\"}}\nnoise after json")
	result, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if result.CommitMessage != "add tests" {
		t.Fatalf("commit message = %q", result.CommitMessage)
	}
}

func TestParseOutputIgnoresLeadingAndTrailingTextAroundErrorEnvelope(t *testing.T) {
	t.Parallel()

	raw := []byte("note: provider emitted a warning\n{\"is_error\":true,\"result\":\"Not logged in\"}\nextra lines")
	if _, err := ParseOutput(raw); err == nil {
		t.Fatalf("expected claude error")
	}
}

func TestParseOutputIncludesInputPreviewOnMalformedJSON(t *testing.T) {
	t.Parallel()

	raw := []byte("{\n  \"is_error\": false,\n  \"structured_output\": {\"summary\": \"added tests\"}\n")
	_, err := ParseOutput(raw)
	if err == nil {
		t.Fatal("expected malformed envelope error")
	}
	if !errors.Is(err, ErrMalformedOutputEnvelope) {
		t.Fatalf("expected malformed envelope sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "is_error") || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("expected input preview in error, got %v", err)
	}
}
