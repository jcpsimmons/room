package git

import "testing"

func TestNormalizeCommitMessage(t *testing.T) {
	t.Parallel()

	got := NormalizeCommitMessage("room:", "tighten config errors")
	if got != "room: tighten config errors" {
		t.Fatalf("normalized = %q", got)
	}

	already := NormalizeCommitMessage("room:", "room: tighten config errors")
	if already != "room: tighten config errors" {
		t.Fatalf("already prefixed = %q", already)
	}
}
