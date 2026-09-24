package physics

import (
	"github.com/charmbracelet/harmonica"
)

// Spring wraps harmonica.Spring for SALAMEY-compatible updates.
type Spring struct {
	spring harmonica.Spring
}

// NewSpring initializes a spring with crush-friendly defaults.
func NewSpring(fps int, tension, friction float64) *Spring {
	return &Spring{
		spring: harmonica.NewSpring(harmonica.FPS(fps), tension, friction),
	}
}

// Update applies spring physics and returns (position, velocity).
func (s *Spring) Update(pos, velocity, target float64) (float64, float64) {
	return s.spring.Update(pos, velocity, target)
}

// Projectile wraps harmonica.Projectile for SALAMEY-compatible updates.
type Projectile struct {
	projectile *harmonica.Projectile
}

// NewProjectile initializes a projectile.
func NewProjectile(fps int, pos, vel, acc harmonica.Vector) *Projectile {
	return &Projectile{
		projectile: harmonica.NewProjectile(
			harmonica.FPS(fps),
			harmonica.Point(pos),
			vel,
			acc,
		),
	}
}

// Update applies projectile physics.
func (p *Projectile) Update() harmonica.Point {
	return p.projectile.Update()
}
