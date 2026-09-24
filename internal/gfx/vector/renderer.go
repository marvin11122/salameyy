package vector

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
)

// Point represents a 2D point.
type Point struct {
	X, Y float64
}

// Path defines a series of connected Points.
type Path []Point

// Stroke renders a Path to ANSI escape sequences.
func Stroke(path Path, clr color.RGBA) string {
	var buf bytes.Buffer
	for i := 1; i < len(path); i++ {
		from := path[i-1]
		to := path[i]
		// Line to ANSI escape: \x1b[<y>;<x>H\x1b[38;2;<r>;<g>;<b>m█
		fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[38;2;%d;%d;%dm█",
			int(from.Y), int(from.X), clr.R, clr.G, clr.B)
		// Interpolate intermediate points if step > 1.
		steps := int(math.Max(math.Abs(to.X-from.X), math.Abs(to.Y-from.Y)))
		for j := 1; j <= steps; j++ {
			t := float64(j) / float64(steps)
			x := from.X + t*(to.X-from.X)
			y := from.Y + t*(to.Y-from.Y)
			fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[38;2;%d;%d;%dm█",
				int(y), int(x), clr.R, clr.G, clr.B)
		}
	}
	return buf.String()
}

// ClearScreen returns ANSI escape to clear the screen.
func ClearScreen() string {
	return "\x1b[2J"
}

// ResetCursor returns ANSI escape to reset cursor position.
func ResetCursor() string {
	return "\x1b[H"
}
