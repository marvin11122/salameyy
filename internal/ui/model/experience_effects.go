package model

import (
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/util"
)

type ExperienceEffectKind uint8

const (
	ExperienceEffectInfo ExperienceEffectKind = iota
	ExperienceEffectSuccess
	ExperienceEffectWarning
	ExperienceEffectError
)

const experienceEffectFrames = 8

type ExperienceEffectTickMsg struct {
	Owner      *ExperienceEffect
	Generation uint64
}

type ExperienceEffect struct {
	styles     *styles.Styles
	kind       ExperienceEffectKind
	frame      int
	active     bool
	pending    bool
	generation uint64
	cell       uv.Cell
}

func NewExperienceEffect(st *styles.Styles) *ExperienceEffect {
	if st == nil {
		defaultStyles := styles.DefaultStyles()
		st = &defaultStyles
	}
	return &ExperienceEffect{styles: st}
}

func (e *ExperienceEffect) Trigger(kind ExperienceEffectKind) tea.Cmd {
	if e == nil {
		return nil
	}
	e.kind = kind
	e.frame = 0
	e.active = true
	e.invalidate()
	return e.TickCmd()
}

func (e *ExperienceEffect) TickCmd() tea.Cmd {
	if e == nil || !e.active || e.pending {
		return nil
	}
	e.pending = true
	generation := e.generation
	return tea.Tick(70*time.Millisecond, func(time.Time) tea.Msg {
		return ExperienceEffectTickMsg{Owner: e, Generation: generation}
	})
}

func (e *ExperienceEffect) UpdateTick(msg ExperienceEffectTickMsg) tea.Cmd {
	if e == nil || msg.Owner != e || msg.Generation != e.generation || !e.pending {
		return nil
	}
	e.pending = false
	e.generation++
	e.frame++
	if e.frame >= experienceEffectFrames {
		e.active = false
		return nil
	}
	return e.TickCmd()
}

func (e *ExperienceEffect) Stop() {
	if e != nil {
		e.active = false
		e.invalidate()
	}
}

func (e *ExperienceEffect) invalidate() {
	e.pending = false
	e.generation++
}

func (e *ExperienceEffect) Draw(scr uv.Screen, area uv.Rectangle) {
	if e == nil || scr == nil || !e.active || area.Dx() < 1 || area.Dy() < 1 {
		return
	}
	glyph := "✦"
	style := e.styles.Experience.Accent
	switch e.kind {
	case ExperienceEffectSuccess:
		glyph = "✓"
		style = e.styles.Experience.Positive
	case ExperienceEffectWarning:
		glyph = "!"
		style = e.styles.Experience.Warning
	case ExperienceEffectError:
		glyph = "×"
		style = e.styles.Experience.Metric
	}
	if e.frame%4 >= 2 {
		glyph = "·"
	}
	x := area.Max.X - 2
	y := area.Min.Y + (area.Dy()-1)/2
	if x < area.Min.X {
		x = area.Min.X
	}
	e.cell = uv.Cell{Content: glyph, Style: physicsFaceUVStyle(style), Width: 1}
	scr.SetCell(x, y, &e.cell)
}

func experienceEffectKind(infoType util.InfoType) ExperienceEffectKind {
	switch infoType {
	case util.InfoTypeSuccess:
		return ExperienceEffectSuccess
	case util.InfoTypeWarn:
		return ExperienceEffectWarning
	case util.InfoTypeError:
		return ExperienceEffectError
	default:
		return ExperienceEffectInfo
	}
}
