package codex

import "testing"

func TestParseResultValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"summary":"added tests","next_instruction":"improve diagnostics","status":"continue","commit_message":"add tests"}`)
	result, err := ParseResult(raw)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Status != "continue" {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestParseResultRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	if _, err := ParseResult([]byte(`{`)); err == nil {
		t.Fatalf("expected malformed JSON error")
	}
	if _, err := ParseResult([]byte(`{"summary":"","next_instruction":"x","status":"continue","commit_message":"m"}`)); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestParseResultRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	if _, err := ParseResult([]byte(`{"summary":"added tests","next_instruction":"improve diagnostics","status":"continue","commit_message":"add tests","noise":"extra"}`)); err == nil {
		t.Fatalf("expected unknown field error")
	}
}
