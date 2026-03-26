//go:build !darwin || !cgo

package audio

// New creates a no-op Synth when platform audio is unavailable.
func New() *Synth {
	return &Synth{}
}

func (s *Synth) Start() error            { return nil }
func (s *Synth) Stop()                   {}
func (s *Synth) Update(Params)           {}
func (s *Synth) UpdateVoice(int, Params) {}
