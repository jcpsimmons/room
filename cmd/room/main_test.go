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
			Phase:      app.RunProgressPhaseIterationFailure,
			Iteration:  4,
			Err:        errTestBoom,
			ExitCode:   137,
			ExitSignal: "killed",
		})
		want := []string{"Iteration 4 failed: boom [exit 137 via killed]"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("formatRunProgress() = %#v, want %#v", got, want)
		}
	})

	t.Run("execution pulse", func(t *testing.T) {
		got := formatRunProgress(app.RunProgressEvent{
			Phase:              app.RunProgressPhaseAgentExecutionPulse,
			Iteration:          3,
			Provider:           "codex",
			ExecutionElapsedMS: 1460,
		})
		want := []string{"Iteration 3 still running with Codex after 1.5s..."}
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

func TestToUIProgressEventCarriesDiagnostics(t *testing.T) {
	ev := toUIProgressEvent(app.RunProgressEvent{
		Phase:          app.RunProgressPhaseIterationFailure,
		Iteration:      5,
		Failures:       2,
		Err:            errTestBoom,
		StdoutFragment: "stdout fault",
		StderrFragment: "stderr fault",
		ExitCode:       70,
	})

	if ev.Kind != ui.ProgressFailure {
		t.Fatalf("kind = %q", ev.Kind)
	}
	if ev.Stdout != "stdout fault" {
		t.Fatalf("stdout = %q", ev.Stdout)
	}
	if ev.Stderr != "stderr fault" {
		t.Fatalf("stderr = %q", ev.Stderr)
	}
	if ev.Detail != "boom [exit 70]" {
		t.Fatalf("detail = %q", ev.Detail)
	}
}

func TestToUIProgressEventMapsExecutionPulse(t *testing.T) {
	ev := toUIProgressEvent(app.RunProgressEvent{
		Phase:              app.RunProgressPhaseAgentExecutionPulse,
		Iteration:          4,
		Provider:           "claude",
		ExecutionElapsedMS: 3200,
	})

	if ev.Kind != ui.ProgressMessageKind {
		t.Fatalf("kind = %q", ev.Kind)
	}
	if ev.Title != "CLAUDE carrier wave stable" {
		t.Fatalf("title = %q", ev.Title)
	}
	if ev.Detail != "step 4 in flight for 3.2s" {
		t.Fatalf("detail = %q", ev.Detail)
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
		Phase:           app.RunProgressPhaseIterationFailure,
		Iteration:       7,
		Status:          "failed",
		Err:             errors.New("malformed output"),
		EventAt:         fixedTime().Add(2500 * time.Millisecond),
		RunStartedAt:    fixedTime(),
		RunElapsedMS:    2500,
		PhaseStartedAt:  fixedTime().Add(500 * time.Millisecond),
		PhaseFinishedAt: fixedTime().Add(2500 * time.Millisecond),
		PhaseLatencyMS:  2000,
		StartedAt:       fixedTime().Add(500 * time.Millisecond),
		FinishedAt:      fixedTime().Add(2500 * time.Millisecond),
	}))
	stream.Write(makeRunJSONResultLine(app.RunReport{
		RepoRoot:            "/tmp/repo",
		Provider:            "codex",
		RequestedIterations: 3,
		CompletedIterations: 1,
		Failures:            1,
		LastStatus:          "failed",
		LastRunDir:          "/tmp/repo/.room/runs/0007",
		StartedAt:           fixedTime(),
		FinishedAt:          fixedTime().Add(2600 * time.Millisecond),
		DurationMS:          2600,
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
	if progress["schema_version"] != float64(runJSONSchemaVersion) {
		t.Fatalf("progress schema_version = %v", progress["schema_version"])
	}
	if progress["phase"] != string(app.RunProgressPhaseIterationFailure) {
		t.Fatalf("progress phase = %v", progress["phase"])
	}
	if progress["error"] != "malformed output" {
		t.Fatalf("progress error = %v", progress["error"])
	}
	if progress["run_elapsed_ms"] != float64(2500) {
		t.Fatalf("progress run_elapsed_ms = %v", progress["run_elapsed_ms"])
	}
	if progress["phase_latency_ms"] != float64(2000) {
		t.Fatalf("progress phase_latency_ms = %v", progress["phase_latency_ms"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["type"] != "result" {
		t.Fatalf("result type = %v", result["type"])
	}
	if result["schema_version"] != float64(runJSONSchemaVersion) {
		t.Fatalf("result schema_version = %v", result["schema_version"])
	}
	if got, ok := result["ok"].(bool); !ok || got {
		t.Fatalf("expected failure result, got ok=true")
	}
	if result["error"] != "malformed output" {
		t.Fatalf("result error = %v", result["error"])
	}
	resultPayload, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v", result["result"])
	}
	if resultPayload["duration_ms"] != float64(2600) {
		t.Fatalf("result duration_ms = %v", resultPayload["duration_ms"])
	}
}

func TestDoctorJSONEncoding(t *testing.T) {
	var buf bytes.Buffer
	report := app.DoctorReport{
		RepoRoot: "/tmp/repo",
		Checks: []app.DoctorCheck{
			{Name: "git", OK: true, Message: "git is available"},
			{Name: "state", OK: false, Message: "missing state.json"},
		},
		Lines: []string{"ROOM doctor"},
	}

	if err := writeDoctorJSON(&buf, report, nil); err != nil {
		t.Fatalf("writeDoctorJSON: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["schema_version"] != float64(doctorJSONSchemaVersion) {
		t.Fatalf("schema_version = %v", payload["schema_version"])
	}
	if payload["type"] != "result" {
		t.Fatalf("type = %v", payload["type"])
	}
	if got, ok := payload["ok"].(bool); !ok || !got {
		t.Fatalf("expected ok result, got %v", payload["ok"])
	}

	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", payload["result"])
	}
	if result["repo_root"] != report.RepoRoot {
		t.Fatalf("repo_root = %v", result["repo_root"])
	}
	checks, ok := result["checks"].([]any)
	if !ok || len(checks) != len(report.Checks) {
		t.Fatalf("checks = %#v", result["checks"])
	}
}

func TestStatusJSONEncoding(t *testing.T) {
	var buf bytes.Buffer
	report := app.StatusReport{
		RepoRoot: "/tmp/repo",
		Lines:    []string{"ROOM status"},
	}

	if err := writeStatusJSON(&buf, report, nil); err != nil {
		t.Fatalf("writeStatusJSON: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["schema_version"] != float64(doctorJSONSchemaVersion) {
		t.Fatalf("schema_version = %v", payload["schema_version"])
	}
	if payload["type"] != "result" {
		t.Fatalf("type = %v", payload["type"])
	}
	if got, ok := payload["ok"].(bool); !ok || !got {
		t.Fatalf("expected ok result, got %v", payload["ok"])
	}

	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", payload["result"])
	}
	if result["repo_root"] != report.RepoRoot {
		t.Fatalf("repo_root = %v", result["repo_root"])
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
}

var errTestBoom = testError("boom")

type testError string

func (e testError) Error() string { return string(e) }
