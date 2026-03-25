package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestRunModelViewFillsViewport(t *testing.T) {
	model := NewRunModel(100)
	model.width = 120
	model.height = 32
	model.started = time.Now().Add(-5 * time.Second)
	model = model.consume(ProgressEvent{
		Kind:         ProgressStep,
		Title:        "codex is cooking",
		Detail:       "iteration 4 prompt locked in",
		Iteration:    4,
		Total:        100,
		Completed:    3,
		Failures:     0,
		Percent:      0.03,
		HasIteration: true,
		HasTotal:     true,
		HasCompleted: true,
		HasFailures:  true,
		HasPercent:   true,
	})

	view := model.View()

	if got := lipgloss.Width(view); got != 120 {
		t.Fatalf("view width = %d, want 120", got)
	}
	if got := lipgloss.Height(view); got != 32 {
		t.Fatalf("view height = %d, want 32", got)
	}
	if !strings.Contains(view, "ROOM LIVE") {
		t.Fatal("expected header in run view")
	}
	if !strings.Contains(view, "EVENTS") {
		t.Fatal("expected events panel in run view")
	}
}

func TestRenderPanelUsesRequestedSize(t *testing.T) {
	out := renderPanel("TEST", "line one\nline two", accentCyan, 48, 8)

	if got := lipgloss.Width(out); got != 48 {
		t.Fatalf("panel width = %d, want 48", got)
	}
	if got := lipgloss.Height(out); got != 8 {
		t.Fatalf("panel height = %d, want 8", got)
	}
}
