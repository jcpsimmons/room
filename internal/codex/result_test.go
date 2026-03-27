package codex

import (
	"errors"
	"strings"
	"testing"
)

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

func TestParseResultIgnoresWrapperNoise(t *testing.T) {
	t.Parallel()

	raw := []byte("codex wrapper booting\n\n{\"summary\":\"added tests\",\"next_instruction\":\"improve diagnostics\",\"status\":\"continue\",\"commit_message\":\"add tests\"}\nnoise after json")
	result, err := ParseResult(raw)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.CommitMessage != "add tests" {
		t.Fatalf("commit message = %q", result.CommitMessage)
	}
}

func TestParseResultIncludesPreviewWhenNoObjectIsPresent(t *testing.T) {
	t.Parallel()

	_, err := ParseResult([]byte("codex wrapper booting without a payload"))
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if !errors.Is(err, errMalformedJSON) {
		t.Fatalf("expected malformed JSON sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "codex wrapper booting") {
		t.Fatalf("expected input preview in error, got %v", err)
	}
}
