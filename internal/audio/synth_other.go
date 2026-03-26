//go:build !darwin

package audio

// New creates a no-op Synth on non-macOS platforms.
func New() *Synth {
	return &Synth{}
}

func (s *Synth) Start() error { return nil }
func (s *Synth) Stop()        {}
func (s *Synth) Update(Params) {}
