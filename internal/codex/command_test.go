package codex

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildCommandIncludesConfiguredFlags(t *testing.T) {
	t.Parallel()

	command, err := BuildCommand(Prompt{Body: "hello"}, Schema{Path: "/tmp/schema.json"}, "/tmp/result.json", RunOptions{
		Binary:   "codex",
		WorkDir:  "/tmp/repo",
		Model:    "gpt-5.4",
		Sandbox:  "danger-full-access",
		Approval: "never",
		Timeout:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	want := []string{"codex", "--ask-for-approval", "never", "exec", "--cd", "/tmp/repo", "--sandbox", "danger-full-access", "--model", "gpt-5.4", "--output-schema", "/tmp/schema.json", "--output-last-message", "/tmp/result.json", "--color", "never", "--ephemeral", "-"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command mismatch:\nwant %#v\ngot  %#v", want, command)
	}
}

func TestBuildCommandFallsBackToDefaultSandboxAndApproval(t *testing.T) {
	t.Parallel()

	command, err := BuildCommand(Prompt{Body: "hello"}, Schema{Path: "/tmp/schema.json"}, "/tmp/result.json", RunOptions{
		Binary:  "codex",
		WorkDir: "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	want := []string{"codex", "--ask-for-approval", "never", "exec", "--cd", "/tmp/repo", "--sandbox", "danger-full-access", "--output-schema", "/tmp/schema.json", "--output-last-message", "/tmp/result.json", "--color", "never", "--ephemeral", "-"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command mismatch:\nwant %#v\ngot  %#v", want, command)
	}
}
