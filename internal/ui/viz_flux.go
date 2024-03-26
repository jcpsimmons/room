package ui

import (
	"math"
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jcpsimmons/room/internal/audio"
)

// fieldCharge is a point source/sink in the electromagnetic field.
type fieldCharge struct {
	x, y       float64
	vx, vy     float64
	strength   float64 // positive = source, negative = sink
	fadeEpoch  float64 // seconds for appearance/disappearance cycle
	fadePhase  float64
}

// fluxTopology visualizes electromagnetic field lines from moving charges.
type fluxTopology struct {
	charges []fieldCharge
	gridW   int
	gridH   int
	elapsed float64
	epoch   float64 // master envelope (1800-6000s = 30-100 min)
	rng     *rand.Rand
}

func newFluxTopology(w, h int) fluxTopology {
	r := rand.New(rand.NewSource(271))
	charges := []fieldCharge{
		{
			x: float64(w) * 0.3, y: float64(h) * 0.4,
			vx: 0.03, vy: 0.01,
			strength: 1.0,
			fadeEpoch: float64(1800 + r.Intn(4200)),
			fadePhase: 0,
		},
		{
			x: float64(w) * 0.7, y: float64(h) * 0.6,
			vx: -0.02, vy: 0.015,
			strength: -0.8,
			fadeEpoch: float64(2400 + r.Intn(3600)),
			fadePhase: math.Pi * 0.7,
		},
		{
			x: float64(w) * 0.5, y: float64(h) * 0.3,
			vx: 0.01, vy: -0.02,
			strength: 0.6,
			fadeEpoch: float64(900 + r.Intn(5100)),
			fadePhase: math.Pi * 1.3,
		},
	}
	return fluxTopology{
		charges: charges,
		gridW:   w,
		gridH:   h,
		epoch:   float64(1800 + r.Intn(4200)),
		rng:     r,
	}
}

func (ft *fluxTopology) step(dt float64) {
	ft.elapsed += dt

	for i := range ft.charges {
		c := &ft.charges[i]

		// Brownian drift.
		c.vx += (ft.rng.Float64()*2 - 1) * 0.005 * dt
		c.vy += (ft.rng.Float64()*2 - 1) * 0.005 * dt

		// Damping.
		c.vx *= 0.999
		c.vy *= 0.999

		c.x += c.vx
		c.y += c.vy

		// Soft bounce off edges.
		margin := 1.0
		if c.x < margin {
			c.x = margin
			c.vx = math.Abs(c.vx)
		}
		if c.x > float64(ft.gridW)-margin-1 {
			c.x = float64(ft.gridW) - margin - 1
			c.vx = -math.Abs(c.vx)
		}
		if c.y < margin {
			c.y = margin
			c.vy = math.Abs(c.vy)
		}
		if c.y > float64(ft.gridH)-margin-1 {
			c.y = float64(ft.gridH) - margin - 1
			c.vy = -math.Abs(c.vy)
		}

		// Fade envelope.
		c.fadePhase += dt * 2 * math.Pi / c.fadeEpoch
	}
}

func (ft *fluxTopology) fieldAt(x, y float64) (float64, float64, float64) {
	// Returns (Ex, Ey, magnitude) at point (x,y).
	ex, ey := 0.0, 0.0
	for _, c := range ft.charges {
		fade := 0.5 + 0.5*math.Sin(c.fadePhase)
		eff := c.strength * fade

		dx := x - c.x
		dy := y - c.y
		r2 := dx*dx + dy*dy
		if r2 < 0.5 {
			r2 = 0.5
		}
		r := math.Sqrt(r2)
		// Coulomb-like: E ∝ q/r²
		ex += eff * dx / (r * r2)
		ey += eff * dy / (r * r2)
	}
	mag := math.Sqrt(ex*ex + ey*ey)
	return ex, ey, mag
}

func (ft *fluxTopology) render() string {
	// Direction arrows: 8 directions + center dot.
	arrows := []string{"→", "↗", "↑", "↖", "←", "↙", "↓", "↘"}

	var b strings.Builder
	for y := 0; y < ft.gridH; y++ {
		for x := 0; x < ft.gridW; x++ {
			fx := float64(x)
			fy := float64(y)

			// Check if a charge is near this cell.
			isCharge := false
			for _, c := range ft.charges {
				fade := 0.5 + 0.5*math.Sin(c.fadePhase)
				if fade < 0.1 {
					continue
				}
				if math.Abs(fx-c.x) < 0.6 && math.Abs(fy-c.y) < 0.6 {
					if c.strength > 0 {
						b.WriteString(accentBadge(accentGold).Render("◆"))
					} else {
						b.WriteString(accentBadge(accentViolet).Render("◇"))
					}
					isCharge = true
					break
				}
			}
			if isCharge {
				continue
			}

			ex, ey, mag := ft.fieldAt(fx, fy)

			if mag < 0.01 {
				b.WriteString(subtitleStyle().Render(" "))
				continue
			}

			// Quantize direction to 8 compass points.
			angle := math.Atan2(-ey, ex) // screen Y is inverted
			if angle < 0 {
				angle += 2 * math.Pi
			}
			di := int(math.Round(angle/(math.Pi/4))) % 8

			// Color by field strength.
			var color lipgloss.Color
			switch {
			case mag > 0.8:
				color = accentGold
			case mag > 0.3:
				color = accentOrange
			case mag > 0.1:
				color = accentCyan
			default:
				color = accentRed
			}

			b.WriteString(bulletStyle(color).Render(arrows[di]))
		}
		if y < ft.gridH-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (ft *fluxTopology) audioParams() audio.Params {
	// Master epoch envelope.
	masterEnv := 0.5 + 0.5*math.Sin(ft.elapsed*2*math.Pi/ft.epoch)

	// Average field magnitude across a few sample points.
	avgMag := 0.0
	samples := 0
	for y := 1; y < ft.gridH; y += 3 {
		for x := 1; x < ft.gridW; x += 4 {
			_, _, mag := ft.fieldAt(float64(x), float64(y))
			avgMag += mag
			samples++
		}
	}
	if samples > 0 {
		avgMag /= float64(samples)
	}

	// Filtered noise texture: high mod depth + mod freq for noisy timbre.
	// Resonant frequency follows field strength.
	freq := 80.0 + avgMag*200.0

	return audio.Params{
		Freq:     freq,
		Amp:      masterEnv * math.Min(avgMag*0.8, 0.2),
		ModFreq:  freq*3.0 + avgMag*100.0,
		ModDepth: 4.0 + avgMag*6.0, // heavy FM = noise-like
		Detune:   1.5 + avgMag*3.0,
	}
}
