package model

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

func TestExperienceEffectTickOwnership(t *testing.T) {
	st := styles.DefaultStyles()
	effect := NewExperienceEffect(&st)
	command := effect.Trigger(ExperienceEffectSuccess)
	require.NotNil(t, command)
	require.Nil(t, effect.TickCmd())

	msg, ok := command().(ExperienceEffectTickMsg)
	require.True(t, ok)
	require.Same(t, effect, msg.Owner)
	require.NotNil(t, effect.UpdateTick(msg))
	require.Nil(t, effect.UpdateTick(msg))
}

func TestExperienceEffectDrawsAndStops(t *testing.T) {
	st := styles.DefaultStyles()
	effect := NewExperienceEffect(&st)
	effect.Trigger(ExperienceEffectError)
	screen := uv.NewScreenBuffer(20, 2)
	effect.Draw(screen, screen.Bounds())
	found := false
	for y := 0; y < 2; y++ {
		for x := 0; x < 20; x++ {
			if cell := screen.CellAt(x, y); cell != nil && cell.Content == "×" {
				found = true
			}
		}
	}
	require.True(t, found)
	effect.Stop()
	screen = uv.NewScreenBuffer(20, 2)
	effect.Draw(screen, screen.Bounds())
	require.Equal(t, " ", screen.CellAt(18, 0).Content)
}
