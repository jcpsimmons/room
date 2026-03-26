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
#define NUM_VOICES  4

// Per-voice synth state — written from Go, read from the audio callback.
static volatile double g_freq[NUM_VOICES]      = {220.0, 55.0, 330.0, 110.0};
static volatile double g_amp[NUM_VOICES]       = {0.0, 0.0, 0.0, 0.0};
static volatile double g_mod_freq[NUM_VOICES]  = {110.0, 27.5, 165.0, 440.0};
static volatile double g_mod_depth[NUM_VOICES] = {0.0, 0.0, 0.0, 0.0};
static volatile double g_detune[NUM_VOICES]    = {0.7, 0.3, 0.1, 1.5};

static double g_phase1[NUM_VOICES]     = {0};
static double g_phase2[NUM_VOICES]     = {0};
static double g_mod_phase[NUM_VOICES]  = {0};
static double g_smooth_amp[NUM_VOICES] = {0};

void synthSetVoiceParams(int voice, double freq, double amp, double modFreq, double modDepth, double detune) {
    if (voice < 0 || voice >= NUM_VOICES) return;
    g_freq[voice]      = freq;
    g_amp[voice]       = amp;
    g_mod_freq[voice]  = modFreq;
    g_mod_depth[voice] = modDepth;
    g_detune[voice]    = detune;
}

// Legacy single-voice setter (voice 0).
void synthSetParams(double freq, double amp, double modFreq, double modDepth, double detune) {
    synthSetVoiceParams(0, freq, amp, modFreq, modDepth, detune);
}

static void callback(void *unused, AudioQueueRef q, AudioQueueBufferRef buf) {
    float *out = (float *)buf->mAudioData;
    int frames = BUF_FRAMES;
    double dt    = 1.0 / SAMPLE_RATE;
    double twopi = 2.0 * M_PI;

    for (int i = 0; i < frames; i++) {
        double mix = 0.0;

        for (int v = 0; v < NUM_VOICES; v++) {
            double freq  = g_freq[v];
            double amp   = g_amp[v];
            double mf    = g_mod_freq[v];
            double md    = g_mod_depth[v];
            double det   = g_detune[v];

            // Exponential amplitude smoothing (~5ms time constant).
            g_smooth_amp[v] += (amp - g_smooth_amp[v]) * 0.02;

            if (g_smooth_amp[v] < 0.0001) continue; // skip silent voices

            // FM modulator.
            double mod = sin(g_mod_phase[v]) * md;
            g_mod_phase[v] += twopi * mf * dt;

            // Two detuned oscillators.
            double osc1 = sin(g_phase1[v] + mod);
            double osc2 = sin(g_phase2[v] + mod * 0.7);
            g_phase1[v] += twopi * freq * dt;
            g_phase2[v] += twopi * (freq + det) * dt;

            // Keep phases bounded.
            if (g_phase1[v] > twopi * 100.0) g_phase1[v] -= twopi * 100.0;
            if (g_phase2[v] > twopi * 100.0) g_phase2[v] -= twopi * 100.0;
            if (g_mod_phase[v] > twopi * 100.0) g_mod_phase[v] -= twopi * 100.0;

            mix += (osc1 + osc2 * 0.6) * 0.5 * g_smooth_amp[v];
        }

        // Soft-clip the mix.
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

// Update sends new parameters to voice 0 (legacy).
func (s *Synth) Update(p Params) {
	s.UpdateVoice(0, p)
}

// UpdateVoice sends new parameters to a specific voice (0..NumVoices-1).
func (s *Synth) UpdateVoice(voice int, p Params) {
	if !s.Active || voice < 0 || voice >= NumVoices {
		return
	}
	C.synthSetVoiceParams(
		C.int(voice),
		C.double(p.Freq),
		C.double(p.Amp),
		C.double(p.ModFreq),
		C.double(p.ModDepth),
		C.double(p.Detune),
	)
}
