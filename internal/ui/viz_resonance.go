package ui

import (
	"math"
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jcpsimmons/room/internal/audio"
)

// waveSource is a single oscillator contributing to the interference field.
type waveSource struct {
	freq      float64 // Hz (display frequency, not audio)
	baseFreq  float64 // center frequency for brownian drift
	phase     float64
	amplitude float64
	fadeEpoch float64 // seconds for this source's fade cycle
	fadePhase float64
	drift     float64 // brownian velocity for frequency
}

// resonanceField shows standing wave interference from multiple sources.
type resonanceField struct {
	sources []waveSource
	gridW   int
	gridH   int
	elapsed float64
	epoch   float64 // master epoch for overall amplitude (1200-6000s = 20-100 min)
	rng     *rand.Rand
}

func newResonanceField(w, h int) resonanceField {
	r := rand.New(rand.NewSource(137))
	// Create 4 wave sources with near-harmonic frequencies.
	baseFreqs := []float64{1.0, 1.5, 2.0, 3.0}
	sources := make([]waveSource, len(baseFreqs))
	for i, bf := range baseFreqs {
		sources[i] = waveSource{
			freq:      bf,
			baseFreq:  bf,
			phase:     r.Float64() * 2 * math.Pi,
			amplitude: 0.6 + r.Float64()*0.4,
			fadeEpoch: float64(1200 + r.Intn(2400)), // 20-60 minutes each
			fadePhase: r.Float64() * 2 * math.Pi,    // stagger the fades
		}
	}
	return resonanceField{
		sources: sources,
		gridW:   w,
		gridH:   h,
		epoch:   float64(1200 + r.Intn(4800)), // 20-100 min master
		rng:     r,
	}
}

func (rf *resonanceField) step(dt float64) {
	rf.elapsed += dt

	for i := range rf.sources {
		s := &rf.sources[i]

		// Advance phase.
		s.phase += dt * s.freq * 2 * math.Pi

		// Advance fade envelope.
		s.fadePhase += dt * 2 * math.Pi / s.fadeEpoch

		// Brownian frequency drift.
		s.drift += (rf.rng.Float64()*2 - 1) * 0.002 * dt
		s.drift *= 0.998 // mean-revert slightly
		s.freq = s.baseFreq + s.drift
		if s.freq < s.baseFreq*0.8 {
			s.freq = s.baseFreq * 0.8
			s.drift *= -0.5
		}
		if s.freq > s.baseFreq*1.2 {
			s.freq = s.baseFreq * 1.2
			s.drift *= -0.5
		}

		// Fade envelope.
		s.amplitude = 0.3 + 0.7*(0.5+0.5*math.Sin(s.fadePhase))
	}
}

func (rf *resonanceField) render() string {
	bars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	colors := []lipgloss.Color{accentRed, accentOrange, accentGold, accentCyan}

	var b strings.Builder
	for y := 0; y < rf.gridH; y++ {
		for x := 0; x < rf.gridW; x++ {
			// Sample the interference pattern at this grid point.
			// X maps to spatial position, Y adds a phase offset for 2D pattern.
			sx := float64(x) / float64(rf.gridW) * 4 * math.Pi
			sy := float64(y) / float64(rf.gridH) * 2 * math.Pi

			val := 0.0
			for _, s := range rf.sources {
				val += s.amplitude * math.Sin(s.phase+sx*s.freq+sy*s.freq*0.3)
			}
			// Normalize: max possible is sum of amplitudes.
			maxVal := 0.0
			for _, s := range rf.sources {
				maxVal += s.amplitude
			}
			if maxVal > 0 {
				val = val / maxVal // -1..1
			}
			val = (val + 1) * 0.5 // 0..1

			bi := int(val * float64(len(bars)-1))
			if bi < 0 {
				bi = 0
			}
			if bi >= len(bars) {
				bi = len(bars) - 1
			}

			ci := int(val * float64(len(colors)-1))
			if ci < 0 {
				ci = 0
			}
			if ci >= len(colors) {
				ci = len(colors) - 1
			}

			b.WriteString(bulletStyle(colors[ci]).Render(bars[bi]))
		}
		if y < rf.gridH-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (rf *resonanceField) audioParams() audio.Params {
	// Master epoch envelope.
	masterEnv := 0.5 + 0.5*math.Sin(rf.elapsed*2*math.Pi/rf.epoch)

	// Average source amplitude and frequency.
	avgFreq := 0.0
	avgAmp := 0.0
	for _, s := range rf.sources {
		avgFreq += s.freq
		avgAmp += s.amplitude
	}
	avgFreq /= float64(len(rf.sources))
	avgAmp /= float64(len(rf.sources))

	// Map to audio: pure-ish tones (low mod depth).
	// Base frequency in mid range for clarity.
	freq := 220.0 * avgFreq // ~220-660 Hz

	// Beating comes from the slight frequency differences between sources.
	// Use detune to create audible beating.
	freqSpread := 0.0
	for _, s := range rf.sources {
		freqSpread += math.Abs(s.freq - avgFreq)
	}
	detune := freqSpread * 2.0

	return audio.Params{
		Freq:     freq,
		Amp:      avgAmp * masterEnv * 0.15, // quiet, crystalline
		ModFreq:  freq * 2.0,
		ModDepth: 0.3, // low mod for clean tone
		Detune:   detune,
	}
}
