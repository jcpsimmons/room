package claude

import (
	"reflect"
	"testing"

	"github.com/jcpsimmons/room/internal/agent"
)

func TestBuildCommandIncludesConfiguredFlags(t *testing.T) {
	t.Parallel()

	command, err := BuildCommand(agent.Prompt{Body: "hello"}, `{"type":"object"}`, agent.RunOptions{
		Binary:         "claude",
		WorkDir:        "/tmp/repo",
		Model:          "sonnet",
		PermissionMode: "bypassPermissions",
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	want := []string{"claude", "-p", "--permission-mode", "bypassPermissions", "--output-format", "json", "--json-schema", `{"type":"object"}`, "--no-session-persistence", "--disable-slash-commands", "--model", "sonnet", "hello"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command mismatch:\nwant %#v\ngot  %#v", want, command)
	}
}

func TestBuildCommandDefaultsPermissionMode(t *testing.T) {
	t.Parallel()

	command, err := BuildCommand(agent.Prompt{Body: "hello"}, `{"type":"object"}`, agent.RunOptions{
		Binary:  "claude",
		WorkDir: "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}

	want := []string{"claude", "-p", "--permission-mode", "bypassPermissions", "--output-format", "json", "--json-schema", `{"type":"object"}`, "--no-session-persistence", "--disable-slash-commands", "hello"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command mismatch:\nwant %#v\ngot  %#v", want, command)
	}
}
