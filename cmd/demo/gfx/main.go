package main

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/charmbracelet/harmonica"
	"github.com/wiesnermarven-hue/salameyy/internal/gfx/physics"
	"github.com/wiesnermarven-hue/salameyy/internal/gfx/vector"
)

func main() {
	// Spring animation
	spring := physics.NewSpring(60, 6.0, 0.2)
	pos := 0.0
	vel := 0.0
	target := 100.0

	// Projectile animation
	proj := physics.NewProjectile(
		60,
		harmonica.Vector{X: 1.0, Y: 1.0, Z: 0.0},
		harmonica.Vector{X: 2.0, Y: 20.0, Z: 0.0},
		harmonica.Vector{X: 0.0, Y: -9.81, Z: 0.0},
	)

	// ANSI rendering loop
	ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
	defer ticker.Stop()

	for range ticker.C {
		// Physics updates
		pos, vel = spring.Update(pos, vel, target)
		projPos := proj.Update()

		// Clear screen and reset cursor
		fmt.Print(vector.ClearScreen(), vector.ResetCursor())

		// Draw spring-driven circle
		path := make(vector.Path, 16)
		for i := 0; i < 16; i++ {
			t := float64(i) * math.Pi / 8.0
			x := pos + 5*math.Cos(t)
			y := 10 + 5*math.Sin(t)
			path[i] = vector.Point{X: x, Y: y}
		}
		fmt.Print(vector.Stroke(path, color.RGBA{R: 0, G: 255, B: 0, A: 255}))

		// Draw projectile
		fmt.Printf("\x1b[%d;%dH\x1b[38;2;%d;%d;%dm○",
			int(projPos.Y), int(projPos.X), 255, 0, 0)
	}
}
