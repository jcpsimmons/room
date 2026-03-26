package audio

// Synth is a minimal FM synthesizer driven by the UI animation state.
// On macOS it outputs through CoreAudio. On other platforms it is a no-op.
type Synth struct {
	Active bool
}

// Params maps animation state to sound.
type Params struct {
	Freq     float64 // base oscillator frequency in Hz
	Amp      float64 // amplitude 0..1
	ModFreq  float64 // FM modulator frequency in Hz
	ModDepth float64 // FM modulation index
	Detune   float64 // second oscillator detune in Hz
}
