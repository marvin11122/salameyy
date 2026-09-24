package model

import (
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"

	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

func newTestPhysicsFace() *PhysicsFace {
	st := styles.DefaultStyles()
	face := NewPhysicsFace(&st)
	face.SetEnabled(true)
	return face
}

func advanceTestPhysicsFace(face *PhysicsFace, frames int) {
	face.SetAnimationSpeed(PhysicsFaceSpeedOff)
	for range frames {
		face.Update()
	}
}

func TestPhysicsFaceState(t *testing.T) {
	face := newTestPhysicsFace()
	face.SetEnabled(false)

	require.False(t, face.Enabled())
	require.Equal(t, PhysicsFaceSpeedMedium, face.AnimationSpeed())
	face.SetEnabled(true)
	face.SetMood(true, 99)
	face.SetQueue(-4)
	face.SetBusy(false)

	require.True(t, face.Enabled())
	require.Equal(t, PhysicsFaceMood{Busy: false, Queue: 0}, face.Mood())
	require.Equal(t, 0, face.Queue())

	face.SetState(PhysicsFaceState{
		Enabled:        true,
		Busy:           true,
		Queue:          12,
		AnimationSpeed: PhysicsFaceSpeedFast,
	})
	require.Equal(t, PhysicsFaceState{
		Enabled:        true,
		Busy:           true,
		Queue:          12,
		AnimationSpeed: PhysicsFaceSpeedFast,
	}, face.State())

	face.SetAnimationSpeed(PhysicsFaceSpeed("unknown"))
	require.Equal(t, PhysicsFaceSpeedMedium, face.AnimationSpeed())
}

func TestPhysicsFaceTickOwnership(t *testing.T) {
	face := newTestPhysicsFace()
	face.SetAnimationSpeed(PhysicsFaceSpeedFast)
	require.Equal(t, time.Second/36, face.TickInterval())

	first := face.TickCmd()
	require.NotNil(t, first)
	require.Nil(t, face.TickCmd())

	msg, ok := first().(PhysicsFaceTickMsg)
	require.True(t, ok)
	require.Same(t, face, msg.Owner)

	next := face.UpdateTick(msg)
	require.NotNil(t, next)
	require.Nil(t, face.UpdateTick(msg))

	face.Stop()
	require.Nil(t, face.UpdateTick(msg))
}

func TestPhysicsFaceDeterministicPhysics(t *testing.T) {
	first := newTestPhysicsFace()
	second := newTestPhysicsFace()
	first.SetMood(true, 2)
	second.SetMood(true, 2)
	advanceTestPhysicsFace(first, 180)
	advanceTestPhysicsFace(second, 180)

	require.Equal(t, first.State(), second.State())
	require.Equal(t, first.Presence(), second.Presence())
	require.Equal(t, first.gazeX.position, second.gazeX.position)
	require.Equal(t, first.mouth.position, second.mouth.position)

	firstScreen := uv.NewScreenBuffer(48, 22)
	secondScreen := uv.NewScreenBuffer(48, 22)
	area := uv.Rect(0, 0, 48, 22)
	first.Draw(firstScreen, area)
	second.Draw(secondScreen, area)
	require.Equal(t, firstScreen.Render(), secondScreen.Render())
}

func TestPhysicsFaceSmallAreaIsBoundedAndDeterministic(t *testing.T) {
	sizes := [][2]int{
		{1, 1},
		{2, 1},
		{2, 2},
		{3, 2},
		{4, 3},
		{5, 3},
		{8, 4},
		{11, 7},
	}

	for _, size := range sizes {
		first := newTestPhysicsFace()
		second := newTestPhysicsFace()
		advanceTestPhysicsFace(first, 100)
		advanceTestPhysicsFace(second, 100)

		firstScreen := uv.NewScreenBuffer(size[0]+4, size[1]+4)
		secondScreen := uv.NewScreenBuffer(size[0]+4, size[1]+4)
		area := uv.Rect(2, 2, size[0], size[1])
		first.Draw(firstScreen, area)
		second.Draw(secondScreen, area)
		require.Equal(t, firstScreen.Render(), secondScreen.Render())

		found := false
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				if cell := firstScreen.CellAt(x, y); cell != nil && cell.Content != " " {
					found = true
				}
			}
		}
		require.True(t, found, "size %dx%d", size[0], size[1])
	}
}

func TestPhysicsFaceDrawClipsAndClearsArea(t *testing.T) {
	face := newTestPhysicsFace()
	advanceTestPhysicsFace(face, 100)
	screen := uv.NewScreenBuffer(20, 12)
	marker := uv.Cell{Content: "X", Width: 1}
	for y := 0; y < 12; y++ {
		for x := 0; x < 20; x++ {
			screen.SetCell(x, y, &marker)
		}
	}

	face.Draw(screen, uv.Rect(-8, -7, 13, 11))
	require.Equal(t, "X", screen.CellAt(19, 0).Content)

	face.SetEnabled(false)
	face.Draw(screen, uv.Rect(0, 0, 5, 5))
	require.Equal(t, " ", screen.CellAt(0, 0).Content)
	require.Equal(t, "X", screen.CellAt(19, 0).Content)
}

func TestPhysicsFaceUsesSemanticStyles(t *testing.T) {
	st := styles.DefaultStyles()
	face := NewPhysicsFace(&st)
	face.SetEnabled(true)
	face.SetAnimationSpeed(PhysicsFaceSpeedOff)
	advanceTestPhysicsFace(face, 100)
	screen := uv.NewScreenBuffer(40, 20)
	face.Draw(screen, screen.Bounds())

	found := false
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			cell := screen.CellAt(x, y)
			if cell != nil && cell.Content != " " && !cell.Style.IsZero() {
				found = true
			}
		}
	}
	require.True(t, found)
}

func TestPhysicsFaceNilSafety(t *testing.T) {
	var face *PhysicsFace
	require.NotPanics(t, func() {
		face.SetEnabled(true)
		face.SetMood(true, 2)
		face.SetAnimationSpeed(PhysicsFaceSpeedFast)
		face.Stop()
		face.Update()
		face.Draw(nil, uv.Rect(0, 0, 1, 1))
	})
	require.False(t, face.Enabled())
	require.Equal(t, PhysicsFaceSpeedMedium, face.AnimationSpeed())
}
