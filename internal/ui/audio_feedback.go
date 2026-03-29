package ui

import (
	"math"

	"github.com/jcpsimmons/room/internal/audio"
)

const (
	outcomeAmpBlend         = 0.5  // cue signal mix ratio before per-field saturation
	outcomeAmpBlendCap      = 0.10 // max additive cue strength, independent of ambient level
	outcomeFreqBlendCap     = 0.25 // clamp how much the cue can pull frequency
	outcomeModFreqBlendCap  = 0.22
	outcomeModDepthBlendCap = 0.30
	outcomeDetuneBlendCap   = 0.40
)

// audioCue carries a short-lived tone that rides on top of the ambient voices.
// It lets outcome events punch through with a distinct timbre before fading
// back into the flux bed.
type audioCue struct {
	recipe audio.Params
	energy float64
}

func (c *audioCue) trigger(ev ProgressEvent) {
	recipe, ok := outcomeToneRecipe(ev)
	if !ok {
		return
	}
	c.recipe = recipe
	c.energy = 1
}

func (c *audioCue) step(dt float64) {
	if c.energy <= 0 {
		return
	}
	c.energy *= math.Exp(-dt * 3.5)
	if c.energy < 0.001 {
		c.energy = 0
	}
}

func (c audioCue) audioParams(fallback audio.Params) audio.Params {
	if c.energy <= 0 {
		return fallback
	}

	out := c.recipe
	blend := clamp01(c.energy)
	outBlend := blend * outcomeAmpBlend

	// Keep the cue as an articulate micro-spike, not a full voice takeover.
	boost := math.Min(out.Amp*outBlend, outcomeAmpBlendCap)
	outParam := clamp01(fallback.Amp + boost*(1-fallback.Amp))

	return audio.Params{
		Freq:     fallbackBlend(fallback.Freq, out.Freq, outBlend, outcomeFreqBlendCap),
		Amp:      outParam,
		ModFreq:  fallbackBlend(fallback.ModFreq, out.ModFreq, outBlend, outcomeModFreqBlendCap),
		ModDepth: clampPositive(fallbackBlend(fallback.ModDepth, out.ModDepth, outBlend, outcomeModDepthBlendCap)),
		Detune:   fallbackBlend(fallback.Detune, out.Detune, outBlend, outcomeDetuneBlendCap),
	}
}

func fallbackBlend(base, cue float64, blend, cap float64) float64 {
	blend = clamp01(blend * cap)
	return base + (cue-base)*blend
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampPositive(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

func outcomeToneRecipe(ev ProgressEvent) (audio.Params, bool) {
	switch {
	case progressPhase(ev) == "iteration_success":
		return audio.Params{
			Freq:     659.25,
			Amp:      0.26,
			ModFreq:  987.77,
			ModDepth: 0.42,
			Detune:   7.0,
		}, true
	case ev.Kind == ProgressFailure:
		return audio.Params{
			Freq:     82.41,
			Amp:      0.24,
			ModFreq:  164.82,
			ModDepth: 5.5,
			Detune:   0.12,
		}, true
	case ev.Kind == ProgressPivot:
		return audio.Params{
			Freq:     196.0,
			Amp:      0.18,
			ModFreq:  294.0,
			ModDepth: 1.35,
			Detune:   2.2,
		}, true
	case ev.Kind == ProgressDone:
		return audio.Params{
			Freq:     784.0,
			Amp:      0.2,
			ModFreq:  1176.0,
			ModDepth: 0.32,
			Detune:   4.5,
		}, true
	case ev.Kind == ProgressComplete:
		return audio.Params{
			Freq:     261.63,
			Amp:      0.16,
			ModFreq:  523.25,
			ModDepth: 0.9,
			Detune:   1.0,
		}, true
	default:
		return audio.Params{}, false
	}
}

func progressPhase(ev ProgressEvent) string {
	if ev.Meta == nil {
		return ""
	}
	phase, _ := ev.Meta["phase"].(string)
	return phase
}
