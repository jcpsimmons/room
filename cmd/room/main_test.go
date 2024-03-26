package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jcpsimmons/room/internal/app"
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

var errTestBoom = testError("boom")

type testError string

func (e testError) Error() string { return string(e) }
