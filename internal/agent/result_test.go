package agent

import (
	"errors"
	"os/exec"
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

func TestParseResultExtractsJSONObjectFromWrapperNoise(t *testing.T) {
	t.Parallel()

	raw := []byte("operator drift detected\n```json\n{\"summary\":\"ok\",\"next_instruction\":\"swing wider\",\"status\":\"continue\",\"commit_message\":\"swing wider\"}\n```\n")
	result, err := ParseResult(raw)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.NextInstruction != "swing wider" {
		t.Fatalf("next_instruction = %q", result.NextInstruction)
	}
}

func TestParseResultRejectsWrapperNoiseWithoutCompleteJSONObject(t *testing.T) {
	t.Parallel()

	raw := []byte("```json\n{\"summary\":\"ok\",\"next_instruction\":\"swing wider\"\n```")
	_, err := ParseResult(raw)
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if !errors.Is(err, errMalformedJSON) {
		t.Fatalf("expected malformed JSON sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "complete JSON object") {
		t.Fatalf("expected extraction failure in error, got %v", err)
	}
}

func TestCaptureExitMetadataReadsExitCode(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-c", "exit 7")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected shell exit")
	}

	code, signal := CaptureExitMetadata(err)
	if code != 7 || signal != "" {
		t.Fatalf("CaptureExitMetadata() = (%d, %q), want (7, \"\")", code, signal)
	}
}
