package ui

import "testing"

func TestOutcomeToneRecipeUsesLevelUpCueForIterationSuccess(t *testing.T) {
	ev := ProgressEvent{
		Kind: ProgressComplete,
		Meta: map[string]any{"phase": "iteration_success"},
	}

	got, ok := outcomeToneRecipe(ev)
	if !ok {
		t.Fatal("expected cue recipe for iteration success")
	}

	want := struct {
		freq, amp, modFreq, modDepth, detune float64
	}{
		freq: 659.25, amp: 0.26, modFreq: 987.77, modDepth: 0.42, detune: 7.0,
	}

	if got.Freq != want.freq || got.Amp != want.amp || got.ModFreq != want.modFreq || got.ModDepth != want.modDepth || got.Detune != want.detune {
		t.Fatalf("recipe = %#v, want %#v", got, want)
	}
}

func TestOutcomeToneRecipeKeepsGenericCompleteCueOutsideIterationSuccess(t *testing.T) {
	ev := ProgressEvent{Kind: ProgressComplete}

	got, ok := outcomeToneRecipe(ev)
	if !ok {
		t.Fatal("expected cue recipe for generic completion")
	}
	if got.Freq != 261.63 {
		t.Fatalf("freq = %v, want 261.63", got.Freq)
	}
}
