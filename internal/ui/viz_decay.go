package ui

import (
	"math"
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jcpsimmons/room/internal/audio"
)

// decayParticle is a charged particle traveling through a magnetic field.
type decayParticle struct {
	x, y     float64
	vx, vy   float64
	energy   float64
	charge   float64 // +1 or -1, determines curvature direction
	age      float64
	trail    [][2]int // recent grid positions for ionization trail
	decayed  bool
}

// decayChamber simulates a cloud/bubble chamber.
type decayChamber struct {
	particles   []decayParticle
	gridW, gridH int
	epoch       float64 // cycle length in seconds (600-2700 = 10-45 min)
	elapsed     float64 // total elapsed time
	lastSpawn   float64 // time of last collision event
	spawnRate   float64 // current collision rate (modulated by epoch)
	rng         *rand.Rand
	decayFlash  float64 // brief brightness on decay events
}

func newDecayChamber(w, h int) decayChamber {
	r := rand.New(rand.NewSource(42))
	return decayChamber{
		gridW:     w,
		gridH:     h,
		epoch:     float64(600 + r.Intn(2100)), // 10-45 minutes
		spawnRate: 3.0,
		rng:       r,
	}
}

func (dc *decayChamber) step(dt float64) {
	dc.elapsed += dt
	dc.decayFlash *= 0.92 // decay flash fades

	// Collision rate oscillates over the epoch — goes from active to quiet.
	epochPhase := dc.elapsed * 2.0 * math.Pi / dc.epoch
	dc.spawnRate = 1.5 + 1.5*math.Sin(epochPhase)

	// Spawn collision events.
	if dc.elapsed-dc.lastSpawn > (4.0/math.Max(dc.spawnRate, 0.1)) && len(dc.particles) < 20 {
		dc.lastSpawn = dc.elapsed
		dc.spawnCollision()
	}

	// Update particles.
	alive := dc.particles[:0]
	var newParticles []decayParticle
	for i := range dc.particles {
		p := &dc.particles[i]
		p.age += dt

		// Magnetic field curves the trajectory: F = qv × B
		// In 2D with B pointing out of screen: ax = charge*vy/r, ay = -charge*vx/r
		curvature := p.charge * 0.3 / math.Max(p.energy, 0.5)
		speed := math.Sqrt(p.vx*p.vx + p.vy*p.vy)
		if speed > 0.01 {
			ax := curvature * p.vy * speed
			ay := -curvature * p.vx * speed
			p.vx += ax * dt
			p.vy += ay * dt
		}

		// Energy loss (synchrotron radiation).
		p.energy -= dt * 0.08 * speed

		// Move.
		p.x += p.vx * dt
		p.y += p.vy * dt

		// Record trail.
		gx := int(math.Round(p.x))
		gy := int(math.Round(p.y))
		if len(p.trail) == 0 || p.trail[len(p.trail)-1] != [2]int{gx, gy} {
			p.trail = append(p.trail, [2]int{gx, gy})
			if len(p.trail) > 16 {
				p.trail = p.trail[len(p.trail)-16:]
			}
		}

		// Decay: probability increases as energy drops.
		if p.energy < 1.0 && dc.rng.Float64() < dt*0.5*(1.0-p.energy) {
			// Spawn two daughter particles.
			angle := math.Atan2(p.vy, p.vx)
			spread := 0.4 + dc.rng.Float64()*0.5
			childEnergy := p.energy * 0.4
			childSpeed := speed * 0.7
			for _, sign := range []float64{-1, 1} {
				a := angle + sign*spread
				newParticles = append(newParticles, decayParticle{
					x: p.x, y: p.y,
					vx:     math.Cos(a) * childSpeed,
					vy:     math.Sin(a) * childSpeed,
					energy:  childEnergy + dc.rng.Float64()*0.3,
					charge:  sign,
				})
			}
			dc.decayFlash = 1.0
			p.decayed = true
		}

		// Cull if out of bounds or too old or no energy.
		inBounds := p.x >= -1 && p.x <= float64(dc.gridW) && p.y >= -1 && p.y <= float64(dc.gridH)
		if inBounds && p.energy > 0.05 && p.age < 12.0 && !p.decayed {
			alive = append(alive, *p)
		}
	}
	dc.particles = append(alive, newParticles...)
}

func (dc *decayChamber) spawnCollision() {
	// Collision at a random interior point.
	cx := float64(dc.gridW/4) + dc.rng.Float64()*float64(dc.gridW/2)
	cy := float64(dc.gridH/4) + dc.rng.Float64()*float64(dc.gridH/2)

	n := 3 + dc.rng.Intn(4) // 3-6 particles per collision
	for i := 0; i < n; i++ {
		angle := dc.rng.Float64() * 2 * math.Pi
		energy := 2.0 + dc.rng.Float64()*3.0
		speed := 1.5 + dc.rng.Float64()*2.5
		charge := 1.0
		if dc.rng.Float64() < 0.5 {
			charge = -1.0
		}
		dc.particles = append(dc.particles, decayParticle{
			x: cx, y: cy,
			vx:     math.Cos(angle) * speed,
			vy:     math.Sin(angle) * speed,
			energy:  energy,
			charge:  charge,
		})
	}
	dc.decayFlash = 0.6
}

func (dc *decayChamber) render() string {
	// Build grid.
	type cell struct {
		glyph string
		color lipgloss.Color
	}
	grid := make([][]cell, dc.gridH)
	for y := range grid {
		grid[y] = make([]cell, dc.gridW)
	}

	// Draw trails first (dimmer).
	trailGlyphs := []string{"·", "∘", ".", "·"}
	trailColors := []lipgloss.Color{accentRed, accentOrange, accentGold}
	for _, p := range dc.particles {
		for i, pt := range p.trail {
			x, y := pt[0], pt[1]
			if x < 0 || x >= dc.gridW || y < 0 || y >= dc.gridH {
				continue
			}
			if grid[y][x].glyph != "" {
				continue
			}
			ci := i * len(trailColors) / (len(p.trail) + 1)
			if ci >= len(trailColors) {
				ci = len(trailColors) - 1
			}
			gi := i * len(trailGlyphs) / (len(p.trail) + 1)
			if gi >= len(trailGlyphs) {
				gi = len(trailGlyphs) - 1
			}
			grid[y][x] = cell{trailGlyphs[gi], trailColors[ci]}
		}
	}

	// Draw particle heads.
	for _, p := range dc.particles {
		x := clampInt(int(math.Round(p.x)), 0, dc.gridW-1)
		y := clampInt(int(math.Round(p.y)), 0, dc.gridH-1)
		var glyph string
		var color lipgloss.Color
		switch {
		case p.energy > 3.0:
			glyph = "◉"
			color = accentGold
		case p.energy > 1.5:
			glyph = "⊕"
			color = accentOrange
		case p.energy > 0.5:
			glyph = "○"
			color = accentCyan
		default:
			glyph = "∘"
			color = accentRed
		}
		grid[y][x] = cell{glyph, color}
	}

	// Render.
	var b strings.Builder
	for y := 0; y < dc.gridH; y++ {
		for x := 0; x < dc.gridW; x++ {
			c := grid[y][x]
			if c.glyph == "" {
				b.WriteString(subtitleStyle().Render(" "))
			} else {
				b.WriteString(bulletStyle(c.color).Render(c.glyph))
			}
		}
		if y < dc.gridH-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (dc *decayChamber) audioParams() audio.Params {
	// Activity level drives the drone.
	activity := float64(len(dc.particles)) / 20.0
	if activity > 1 {
		activity = 1
	}

	// Epoch envelope: slow fade over the cycle.
	epochPhase := dc.elapsed * 2.0 * math.Pi / dc.epoch
	envelope := 0.5 + 0.5*math.Sin(epochPhase)

	amp := activity * envelope * 0.25

	// Decay flash adds a brief high-frequency ping.
	freq := 36.0 + activity*24.0 // sub-bass range 36-60 Hz
	if dc.decayFlash > 0.3 {
		freq = 800 + dc.decayFlash*1200 // ping up to 2000 Hz
		amp += dc.decayFlash * 0.15
	}

	return audio.Params{
		Freq:     freq,
		Amp:      amp,
		ModFreq:  freq * 2.0,
		ModDepth: activity * 3.0, // gritty modulation when active
		Detune:   0.3 + activity*0.5,
	}
}
