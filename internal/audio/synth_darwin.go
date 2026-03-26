//go:build darwin

package audio

/*
#cgo LDFLAGS: -framework AudioToolbox -framework CoreFoundation
#include <AudioToolbox/AudioToolbox.h>
#include <math.h>
#include <string.h>

#define SAMPLE_RATE 44100.0
#define BUF_FRAMES  512
#define NUM_BUFS    3

// Synth state — written from Go, read from the audio callback.
// Fine to race slightly; we want smooth sweeps, not sample-accurate jumps.
static volatile double g_freq      = 220.0;
static volatile double g_amp       = 0.0;
static volatile double g_mod_freq  = 110.0;
static volatile double g_mod_depth = 0.0;
static volatile double g_detune    = 0.7;

static double g_phase1 = 0.0;
static double g_phase2 = 0.0;
static double g_mod_phase = 0.0;

// Smooth the amplitude to avoid clicks.
static double g_smooth_amp = 0.0;

void synthSetParams(double freq, double amp, double modFreq, double modDepth, double detune) {
    g_freq      = freq;
    g_amp       = amp;
    g_mod_freq  = modFreq;
    g_mod_depth = modDepth;
    g_detune    = detune;
}

static void callback(void *unused, AudioQueueRef q, AudioQueueBufferRef buf) {
    float *out = (float *)buf->mAudioData;
    int frames = BUF_FRAMES;

    double freq  = g_freq;
    double amp   = g_amp;
    double mf    = g_mod_freq;
    double md    = g_mod_depth;
    double det   = g_detune;
    double dt    = 1.0 / SAMPLE_RATE;
    double twopi = 2.0 * M_PI;

    for (int i = 0; i < frames; i++) {
        // Exponential amplitude smoothing (~5ms time constant).
        g_smooth_amp += (amp - g_smooth_amp) * 0.02;

        // FM modulator.
        double mod = sin(g_mod_phase) * md;
        g_mod_phase += twopi * mf * dt;

        // Two detuned oscillators.
        double osc1 = sin(g_phase1 + mod);
        double osc2 = sin(g_phase2 + mod * 0.7);
        g_phase1 += twopi * freq * dt;
        g_phase2 += twopi * (freq + det) * dt;

        // Keep phases bounded.
        if (g_phase1 > twopi * 100.0) g_phase1 -= twopi * 100.0;
        if (g_phase2 > twopi * 100.0) g_phase2 -= twopi * 100.0;
        if (g_mod_phase > twopi * 100.0) g_mod_phase -= twopi * 100.0;

        // Mix and soft-clip.
        double mix = (osc1 + osc2 * 0.6) * 0.5 * g_smooth_amp;
        if (mix > 1.0) mix = 1.0;
        if (mix < -1.0) mix = -1.0;
        out[i] = (float)mix;
    }

    buf->mAudioDataByteSize = (UInt32)(frames * sizeof(float));
    AudioQueueEnqueueBuffer(q, buf, 0, NULL);
}

static AudioQueueRef g_queue = NULL;

int synthStart(void) {
    AudioStreamBasicDescription fmt;
    memset(&fmt, 0, sizeof(fmt));
    fmt.mSampleRate       = SAMPLE_RATE;
    fmt.mFormatID         = kAudioFormatLinearPCM;
    fmt.mFormatFlags      = kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked;
    fmt.mBytesPerPacket   = sizeof(float);
    fmt.mFramesPerPacket  = 1;
    fmt.mBytesPerFrame    = sizeof(float);
    fmt.mChannelsPerFrame = 1;
    fmt.mBitsPerChannel   = 32;

    OSStatus err = AudioQueueNewOutput(&fmt, callback, NULL, NULL, NULL, 0, &g_queue);
    if (err != noErr) return (int)err;

    // Set volume low — this is texture, not a lead.
    AudioQueueSetParameter(g_queue, kAudioQueueParam_Volume, 0.35);

    for (int i = 0; i < NUM_BUFS; i++) {
        AudioQueueBufferRef buf;
        AudioQueueAllocateBuffer(g_queue, BUF_FRAMES * sizeof(float), &buf);
        buf->mAudioDataByteSize = BUF_FRAMES * sizeof(float);
        memset(buf->mAudioData, 0, buf->mAudioDataByteSize);
        AudioQueueEnqueueBuffer(g_queue, buf, 0, NULL);
    }

    return (int)AudioQueueStart(g_queue, NULL);
}

void synthStop(void) {
    if (g_queue) {
        AudioQueueStop(g_queue, true);
        AudioQueueDispose(g_queue, true);
        g_queue = NULL;
    }
}
*/
import "C"

import "fmt"

// New creates a Synth. Call Start() to begin audio output.
func New() *Synth {
	return &Synth{}
}

// Start opens the system audio output and begins synthesis.
func (s *Synth) Start() error {
	if s.Active {
		return nil
	}
	rc := C.synthStart()
	if rc != 0 {
		return fmt.Errorf("CoreAudio error %d", int(rc))
	}
	s.Active = true
	return nil
}

// Stop shuts down audio output.
func (s *Synth) Stop() {
	if !s.Active {
		return
	}
	C.synthStop()
	s.Active = false
}

// Update sends new parameters to the audio thread.
func (s *Synth) Update(p Params) {
	if !s.Active {
		return
	}
	C.synthSetParams(
		C.double(p.Freq),
		C.double(p.Amp),
		C.double(p.ModFreq),
		C.double(p.ModDepth),
		C.double(p.Detune),
	)
}
