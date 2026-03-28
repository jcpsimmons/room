package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		Stdout:       "stdout pulse",
		Stderr:       "stderr shard",
	})

	view := model.View()

	if got := lipgloss.Width(view); got != 120 {
		t.Fatalf("view width = %d, want 120", got)
	}
	if got := lipgloss.Height(view); got != 32 {
		t.Fatalf("view height = %d, want 32", got)
	}
	if !strings.Contains(view, "PATCH") {
		t.Fatal("expected header in run view")
	}
	if !strings.Contains(view, "SEQUENCE MEMORY") {
		t.Fatal("expected events panel in run view")
	}
	if !strings.Contains(view, "DIAGNOSTICS") {
		t.Fatal("expected diagnostics panel in run view")
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

func TestRenderDiagnosticsPanelShowsFaultFragments(t *testing.T) {
	model := NewRunModel(1)
	model.stderr = "stderr shard"
	model.stdout = "stdout pulse"

	out := model.renderDiagnosticsPanel(48, 10)

	for _, want := range []string{"DIAGNOSTICS", "stderr shard", "stdout pulse"} {
		if !strings.Contains(out, want) {
			t.Fatalf("diagnostics panel missing %q:\n%s", want, out)
		}
	}
}

func TestRunModelTabSwitchesAuxPanel(t *testing.T) {
	model := NewRunModel(1)
	model.width = 120
	model.height = 32

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated := next.(RunModel)

	if updated.auxPanel != "flux" {
		t.Fatalf("auxPanel = %q, want flux", updated.auxPanel)
	}

	view := updated.View()
	if !strings.Contains(view, "FLUX") {
		t.Fatalf("expected flux panel in view after tab:\n%s", view)
	}
	if strings.Contains(view, "DIAGNOSTICS") {
		t.Fatalf("expected diagnostics panel to be hidden after tab:\n%s", view)
	}
}

func TestRunModelMuteToggleUpdatesManualAndStatus(t *testing.T) {
	model := NewRunModel(1, WithAudio())
	model.width = 120
	model.height = 32

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	updated := next.(RunModel)

	if !updated.audioMuted {
		t.Fatal("expected audio to mute")
	}

	if got := updated.manualText(); got != "q/esc/ctrl+c close gate  tab flips aux panel  m unmutes audio" {
		t.Fatalf("manualText() = %q", got)
	}

	status := updated.renderStatusPanel(60, 14)
	if !strings.Contains(status, "muted") {
		t.Fatalf("status panel missing muted state:\n%s", status)
	}
}
