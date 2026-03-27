package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestParseResultIncludesInputPreviewOnMalformedJSON(t *testing.T) {
	t.Parallel()

	raw := []byte("{\n  \"summary\": \"ok\",\n  \"next_instruction\": \"keep going\",\n  \"status\": \"continue\",\n  \"commit_message\": \"done\"\n")
	_, err := ParseResult(raw)
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if !errors.Is(err, errMalformedJSON) {
		t.Fatalf("expected malformed JSON sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "summary") || !strings.Contains(err.Error(), "commit_message") {
		t.Fatalf("expected input preview in error, got %v", err)
	}
}
