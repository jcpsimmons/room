package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jcpsimmons/room/internal/app"
	"github.com/jcpsimmons/room/internal/claude"
	"github.com/jcpsimmons/room/internal/ui"
)

func TestFormatRunProgress(t *testing.T) {
	t.Run("run start", func(t *testing.T) {
		got := formatRunProgress(app.RunProgressEvent{
			Phase:               app.RunProgressPhaseRunStart,
			RepoRoot:            "/tmp/repo",
			Provider:            "codex",
			RequestedIterations: 3,
			CommitEnabled:       true,
		})
		want := []string{
			"ROOM run in /tmp/repo",
			"Provider: Codex",
			"Iterations requested: 3",
			"Commit mode: true",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("formatRunProgress() = %#v, want %#v", got, want)
		}
	})

	t.Run("dry run success", func(t *testing.T) {
		got := formatRunProgress(app.RunProgressEvent{
			Phase:      app.RunProgressPhaseIterationSuccess,
			Iteration:  2,
			DryRun:     true,
			PromptPath: "/tmp/repo/.room/runs/0002/prompt.txt",
		})
		want := []string{"Dry run prepared prompt for iteration 2 at /tmp/repo/.room/runs/0002/prompt.txt"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("formatRunProgress() = %#v, want %#v", got, want)
		}
	})

	t.Run("iteration failure", func(t *testing.T) {
		got := formatRunProgress(app.RunProgressEvent{
			Phase:     app.RunProgressPhaseIterationFailure,
			Iteration: 4,
			Err:       errTestBoom,
		})
		want := []string{"Iteration 4 failed: boom"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("formatRunProgress() = %#v, want %#v", got, want)
		}
	})

	t.Run("wrapper drift failure", func(t *testing.T) {
		got := formatRunProgress(app.RunProgressEvent{
			Phase:     app.RunProgressPhaseIterationFailure,
			Iteration: 7,
			Err:       errors.Join(errors.New("claude wrapper drift detected"), claude.ErrMalformedOutputEnvelope),
		})
		want := []string{"Iteration 7 failed: claude wrapper drift detected\nmalformed claude output envelope"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("formatRunProgress() = %#v, want %#v", got, want)
		}
	})
}

func TestShouldUseRunUI(t *testing.T) {
	t.Setenv("ROOM_TUI", "")
	if !shouldUseRunUI() {
		t.Fatal("expected ROOM_TUI to default to enabled")
	}

	for _, value := range []string{"0", "false", "no", "off"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ROOM_TUI", value)
			if shouldUseRunUI() {
				t.Fatalf("expected ROOM_TUI=%q to disable the TUI", value)
			}
		})
	}

	for _, value := range []string{"1", "true", "yes", "on"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ROOM_TUI", value)
			if !shouldUseRunUI() {
				t.Fatalf("expected ROOM_TUI=%q to enable the TUI", value)
			}
		})
	}
}

func TestRunUIOptions(t *testing.T) {
	if got := runUIOptions(true); got != nil {
		t.Fatalf("runUIOptions(true) = %#v, want nil", got)
	}

	opts := runUIOptions(false)
	if len(opts) != 1 {
		t.Fatalf("len(runUIOptions(false)) = %d, want 1", len(opts))
	}

	model := ui.NewRunModel(1, opts...)
	if reflect.ValueOf(model).FieldByName("synth").IsNil() {
		t.Fatal("expected audio synth when sound is enabled")
	}
}

func TestResolveInitPrompt(t *testing.T) {
	got, err := resolveInitPrompt("Build a release bot", nil)
	if err != nil {
		t.Fatalf("resolveInitPrompt direct: %v", err)
	}
	if got != "Build a release bot" {
		t.Fatalf("prompt = %q", got)
	}

	got, err = resolveInitPrompt("-", strings.NewReader("  Ship an API client.  \n"))
	if err != nil {
		t.Fatalf("resolveInitPrompt stdin: %v", err)
	}
	if got != "Ship an API client." {
		t.Fatalf("stdin prompt = %q", got)
	}

	if _, err := resolveInitPrompt("-", strings.NewReader(" \n\t")); err == nil {
		t.Fatal("expected empty stdin prompt to fail")
	}
}

func TestRunJSONStreamEncoding(t *testing.T) {
	var buf bytes.Buffer
	stream := newJSONLineWriter(&buf)

	stream.Write(makeRunJSONProgressLine(app.RunProgressEvent{
		Phase:      app.RunProgressPhaseIterationFailure,
		Iteration:  7,
		Status:     "failed",
		Err:        errors.New("malformed output"),
		StartedAt:  fixedTime(),
		FinishedAt: fixedTime().Add(2 * time.Second),
	}))
	stream.Write(makeRunJSONResultLine(app.RunReport{
		RepoRoot:            "/tmp/repo",
		Provider:            "codex",
		RequestedIterations: 3,
		CompletedIterations: 1,
		Failures:            1,
		LastStatus:          "failed",
		LastRunDir:          "/tmp/repo/.room/runs/0007",
		Lines:               []string{"Iteration 7 failed: malformed output"},
	}, errors.New("malformed output")))

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}

	var progress map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &progress); err != nil {
		t.Fatalf("decode progress: %v", err)
	}
	if progress["type"] != "progress" {
		t.Fatalf("progress type = %v", progress["type"])
	}
	if progress["phase"] != string(app.RunProgressPhaseIterationFailure) {
		t.Fatalf("progress phase = %v", progress["phase"])
	}
	if progress["error"] != "malformed output" {
		t.Fatalf("progress error = %v", progress["error"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["type"] != "result" {
		t.Fatalf("result type = %v", result["type"])
	}
	if got, ok := result["ok"].(bool); !ok || got {
		t.Fatalf("expected failure result, got ok=true")
	}
	if result["error"] != "malformed output" {
		t.Fatalf("result error = %v", result["error"])
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
}

var errTestBoom = testError("boom")

type testError string

func (e testError) Error() string { return string(e) }
