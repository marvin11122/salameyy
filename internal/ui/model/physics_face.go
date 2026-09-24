package model

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	uv "github.com/charmbracelet/ultraviolet"
	uvscreen "github.com/charmbracelet/ultraviolet/screen"

	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

type PhysicsFaceSpeed string

const (
	PhysicsFaceSpeedOff    PhysicsFaceSpeed = "off"
	PhysicsFaceSpeedSlow   PhysicsFaceSpeed = "slow"
	PhysicsFaceSpeedMedium PhysicsFaceSpeed = "medium"
	PhysicsFaceSpeedFast   PhysicsFaceSpeed = "fast"
)

const (
	physicsFaceBaseFPS       = 24
	physicsFaceMaxQueue      = 4
	physicsFaceMaxWidth      = 84
	physicsFaceMaxHeight     = 28
	physicsFaceCompactWidth  = 14
	physicsFaceCompactHeight = 8
	physicsFaceVisibleAt     = 0.03
	physicsFaceCompactRole   = -1
	physicsFacePhaseModulo   = uint64(1 << 20)
)

type PhysicsFaceMood struct {
	Busy  bool
	Queue int
}

type PhysicsFaceState struct {
	Enabled        bool
	Busy           bool
	Queue          int
	AnimationSpeed PhysicsFaceSpeed
}

type PhysicsFaceTickMsg struct {
	Owner      *PhysicsFace
	Generation uint64
}

type physicsFaceSpring struct {
	spring   harmonica.Spring
	position float64
	velocity float64
}

type physicsFacePalette struct {
	aura      uv.Style
	outline   uv.Style
	feature   uv.Style
	accent    uv.Style
	highlight uv.Style
}

type PhysicsFace struct {
	palette physicsFacePalette
	cell    uv.Cell

	enabled bool
	busy    bool
	queue   int
	speed   PhysicsFaceSpeed

	presence physicsFaceSpring
	gazeX    physicsFaceSpring
	gazeY    physicsFaceSpring
	microX   physicsFaceSpring
	microY   physicsFaceSpring
	blink    physicsFaceSpring
	mouth    physicsFaceSpring
	breath   physicsFaceSpring
	tilt     physicsFaceSpring
	halo     physicsFaceSpring
	glow     physicsFaceSpring

	frame       uint64
	nextBlink   uint64
	blinkHold   int
	generation  uint64
	tickPending bool
}

var _ uv.Drawable = (*PhysicsFace)(nil)

func NewPhysicsFace(st *styles.Styles) *PhysicsFace {
	f := &PhysicsFace{
		speed:     PhysicsFaceSpeedMedium,
		nextBlink: 72,
		palette:   physicsFacePaletteFrom(st),
		presence:  newPhysicsFaceSpring(3.0, 0.70),
		gazeX:     newPhysicsFaceSpring(5.4, 0.42),
		gazeY:     newPhysicsFaceSpring(5.4, 0.42),
		microX:    newPhysicsFaceSpring(8.0, 0.54),
		microY:    newPhysicsFaceSpring(8.0, 0.54),
		blink:     newPhysicsFaceSpring(10.0, 0.66),
		mouth:     newPhysicsFaceSpring(6.2, 0.48),
		breath:    newPhysicsFaceSpring(3.6, 0.55),
		tilt:      newPhysicsFaceSpring(4.8, 0.44),
		halo:      newPhysicsFaceSpring(3.2, 0.58),
		glow:      newPhysicsFaceSpring(4.2, 0.52),
	}
	return f
}

func newPhysicsFaceSpring(angularFrequency, dampingRatio float64) physicsFaceSpring {
	return physicsFaceSpring{
		spring: harmonica.NewSpring(harmonica.FPS(physicsFaceBaseFPS), angularFrequency, dampingRatio),
	}
}

func (s *physicsFaceSpring) update(target, lower, upper float64) {
	s.position, s.velocity = s.spring.Update(s.position, s.velocity, target)
	if math.IsNaN(s.position) || math.IsInf(s.position, 0) {
		s.position = target
		s.velocity = 0
	}
	if math.IsNaN(s.velocity) || math.IsInf(s.velocity, 0) {
		s.velocity = 0
	}
	s.position = physicsFaceClamp(s.position, lower, upper)
	s.velocity = physicsFaceClamp(s.velocity, -32, 32)
}

func physicsFacePaletteFrom(st *styles.Styles) physicsFacePalette {
	if st == nil {
		return physicsFacePalette{}
	}

	base := st.Base
	if base.GetForeground() == nil && st.FgBase != nil {
		base = base.Foreground(st.FgBase)
	}
	outline := st.HalfMuted
	if outline.GetForeground() == nil {
		outline = base
	}
	aura := st.Subtle
	if aura.GetForeground() == nil {
		aura = outline
	}
	accentColor := st.Primary
	if st.Experience.Face.GetForeground() != nil {
		accentColor = st.Experience.Face.GetForeground()
	}
	if accentColor == nil {
		accentColor = st.Secondary
	}
	highlightColor := st.White
	if highlightColor == nil {
		highlightColor = accentColor
	}

	return physicsFacePalette{
		aura:      physicsFaceUVStyle(aura),
		outline:   physicsFaceUVStyle(outline),
		feature:   physicsFaceUVStyle(base),
		accent:    physicsFaceUVStyle(base.Foreground(accentColor)),
		highlight: physicsFaceUVStyle(base.Foreground(highlightColor).Bold(true)),
	}
}

func physicsFaceUVStyle(style lipgloss.Style) uv.Style {
	result := uv.Style{
		Fg:             style.GetForeground(),
		Bg:             style.GetBackground(),
		UnderlineColor: style.GetUnderlineColor(),
		Underline:      uv.Underline(style.GetUnderlineStyle()),
	}
	if style.GetBold() {
		result.Attrs |= uv.AttrBold
	}
	if style.GetFaint() {
		result.Attrs |= uv.AttrFaint
	}
	if style.GetItalic() {
		result.Attrs |= uv.AttrItalic
	}
	if style.GetBlink() {
		result.Attrs |= uv.AttrBlink
	}
	if style.GetReverse() {
		result.Attrs |= uv.AttrReverse
	}
	if style.GetStrikethrough() {
		result.Attrs |= uv.AttrStrikethrough
	}
	return result
}

func (f *PhysicsFace) Enabled() bool {
	return f != nil && f.enabled
}

func (f *PhysicsFace) SetEnabled(enabled bool) {
	if f == nil || f.enabled == enabled {
		return
	}
	f.enabled = enabled
	if !enabled {
		f.invalidateTick()
	}
}

func (f *PhysicsFace) Busy() bool {
	return f != nil && f.busy
}

func (f *PhysicsFace) SetBusy(busy bool) {
	if f != nil {
		f.busy = busy
	}
}

func (f *PhysicsFace) Queue() int {
	if f == nil {
		return 0
	}
	return f.queue
}

func (f *PhysicsFace) SetQueue(queue int) {
	if f != nil {
		if queue < 0 {
			queue = 0
		}
		f.queue = queue
	}
}

func (f *PhysicsFace) Mood() PhysicsFaceMood {
	if f == nil {
		return PhysicsFaceMood{}
	}
	return PhysicsFaceMood{Busy: f.busy, Queue: f.queue}
}

func (f *PhysicsFace) SetMood(busy bool, queue int) {
	if f == nil {
		return
	}
	f.busy = busy
	if queue < 0 {
		queue = 0
	}
	f.queue = queue
}

func (f *PhysicsFace) AnimationSpeed() PhysicsFaceSpeed {
	if f == nil {
		return PhysicsFaceSpeedMedium
	}
	return physicsFaceNormalizeSpeed(f.speed)
}

func (f *PhysicsFace) SetAnimationSpeed(speed PhysicsFaceSpeed) {
	if f == nil {
		return
	}
	speed = physicsFaceNormalizeSpeed(speed)
	if f.speed == speed {
		return
	}
	f.speed = speed
	f.invalidateTick()
}

func (f *PhysicsFace) State() PhysicsFaceState {
	if f == nil {
		return PhysicsFaceState{AnimationSpeed: PhysicsFaceSpeedMedium}
	}
	return PhysicsFaceState{
		Enabled:        f.enabled,
		Busy:           f.busy,
		Queue:          f.queue,
		AnimationSpeed: physicsFaceNormalizeSpeed(f.speed),
	}
}

func (f *PhysicsFace) SetState(state PhysicsFaceState) {
	if f == nil {
		return
	}
	f.SetMood(state.Busy, state.Queue)
	f.SetAnimationSpeed(state.AnimationSpeed)
	f.SetEnabled(state.Enabled)
}

func physicsFaceNormalizeSpeed(speed PhysicsFaceSpeed) PhysicsFaceSpeed {
	switch speed {
	case PhysicsFaceSpeedOff, PhysicsFaceSpeedSlow, PhysicsFaceSpeedMedium, PhysicsFaceSpeedFast:
		return speed
	default:
		return PhysicsFaceSpeedMedium
	}
}

func physicsFaceClampInt(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func physicsFaceClamp(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func (f *PhysicsFace) Presence() float64 {
	if f == nil {
		return 0
	}
	return physicsFaceClamp(f.presence.position, 0, 1)
}

func (f *PhysicsFace) TickInterval() time.Duration {
	if f == nil {
		return 0
	}
	switch f.speed {
	case PhysicsFaceSpeedOff:
		return 0
	case PhysicsFaceSpeedSlow:
		return time.Second / 12
	case PhysicsFaceSpeedFast:
		return time.Second / 36
	default:
		return time.Second / physicsFaceBaseFPS
	}
}

func (f *PhysicsFace) TickCmd() tea.Cmd {
	if f == nil || !f.enabled || f.tickPending {
		return nil
	}
	interval := f.TickInterval()
	if interval <= 0 {
		return nil
	}
	f.tickPending = true
	generation := f.generation
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return PhysicsFaceTickMsg{Owner: f, Generation: generation}
	})
}

func (f *PhysicsFace) Tick() tea.Cmd {
	return f.TickCmd()
}

func (f *PhysicsFace) Update() tea.Cmd {
	if f == nil {
		return nil
	}
	f.step()
	return f.TickCmd()
}

func (f *PhysicsFace) Prime() {
	if f != nil {
		f.step()
	}
}

func (f *PhysicsFace) UpdateTick(msg PhysicsFaceTickMsg) tea.Cmd {
	if f == nil || msg.Owner != f || msg.Generation != f.generation || !f.tickPending {
		return nil
	}
	f.tickPending = false
	f.generation++
	f.step()
	return f.TickCmd()
}

func (f *PhysicsFace) HandleTick(msg PhysicsFaceTickMsg) tea.Cmd {
	return f.UpdateTick(msg)
}

func (f *PhysicsFace) Stop() {
	if f == nil {
		return
	}
	f.invalidateTick()
}

func (f *PhysicsFace) invalidateTick() {
	f.tickPending = false
	f.generation++
}

func (f *PhysicsFace) step() {
	f.frame++
	presenceTarget := 0.0
	if f.enabled {
		presenceTarget = 1
	}
	f.presence.update(presenceTarget, 0, 1)

	energy := 0.0
	if f.busy {
		energy = 1
	}
	queue := min(f.queue, physicsFaceMaxQueue)
	energy += float64(queue) * 0.18
	phase := float64(f.frame%physicsFacePhaseModulo) / physicsFaceBaseFPS

	gazeXTarget := math.Sin(phase*0.41)*1.4 + math.Sin(phase*2.9)*0.18 + energy*math.Sin(phase*1.7)*0.65
	gazeYTarget := math.Cos(phase*0.27)*0.72 + math.Cos(phase*2.3)*0.12 + energy*math.Cos(phase*1.2)*0.24
	microXTarget := math.Sin(phase*3.7)*0.24 + math.Cos(phase*5.2)*0.10
	microYTarget := math.Cos(phase*4.3)*0.16 + math.Sin(phase*6.1)*0.06
	mouthTarget := 0.15 + (math.Sin(phase*0.33)+1)*0.08 + energy*0.16 + float64(queue)*0.04
	breathTarget := math.Sin(phase * 0.45)
	tiltTarget := math.Sin(phase*0.19)*0.8 + energy*math.Sin(phase*0.72)*0.3
	haloTarget := 0.38 + (math.Sin(phase*0.18)+1)*0.16 + energy*0.14
	glowTarget := 0.20 + (math.Cos(phase*0.52)+1)*0.10 + energy*0.18

	blinkTarget := 0.0
	if f.blinkHold > 0 {
		blinkTarget = 1
		f.blinkHold--
	} else if f.frame >= f.nextBlink {
		f.blinkHold = 2
		blinkTarget = 1
		f.nextBlink = f.frame + 70 + uint64(math.Abs(math.Sin(phase*0.61))*55)
	}

	f.gazeX.update(gazeXTarget, -4, 4)
	f.gazeY.update(gazeYTarget, -4, 4)
	f.microX.update(microXTarget, -2, 2)
	f.microY.update(microYTarget, -2, 2)
	f.mouth.update(mouthTarget, -1, 1)
	f.breath.update(breathTarget, -2, 2)
	f.tilt.update(tiltTarget, -3, 3)
	f.halo.update(haloTarget, -1, 2)
	f.glow.update(glowTarget, -1, 2)
	f.blink.update(blinkTarget, 0, 1)
}

func (f *PhysicsFace) DrawStatus(scr uv.Screen, area uv.Rectangle) {
	if f == nil || scr == nil || !f.enabled {
		return
	}
	area = area.Intersect(scr.Bounds())
	if area.Dy() <= 0 || area.Dx() < 3 {
		return
	}
	glyph := "·ᴗ·"
	if f.busy || f.queue > 0 {
		glyph = "•‿•"
	}
	runes := []rune(glyph)
	start := area.Min.X + max(0, (area.Dx()-len(runes))/2)
	y := area.Min.Y + (area.Dy()-1)/2
	style := f.palette.accent
	for i, r := range runes {
		x := start + i
		if x >= area.Max.X {
			break
		}
		f.cell = uv.Cell{Content: string(r), Style: style, Width: 1}
		scr.SetCell(x, y, &f.cell)
	}
}

func (f *PhysicsFace) Draw(scr uv.Screen, area uv.Rectangle) {
	if f == nil || scr == nil {
		return
	}
	area = area.Intersect(scr.Bounds())
	if area.Empty() {
		return
	}

	width := min(area.Dx(), physicsFaceMaxWidth)
	height := min(area.Dy(), physicsFaceMaxHeight)
	if width <= 0 || height <= 0 {
		return
	}
	rect := uv.Rect(
		area.Min.X+(area.Dx()-width)/2,
		area.Min.Y+(area.Dy()-height)/2,
		width,
		height,
	)
	uvscreen.ClearArea(scr, rect)
	if !f.enabled {
		return
	}
	compact := width < physicsFaceCompactWidth || height < physicsFaceCompactHeight
	if !compact && f.presence.position < physicsFaceVisibleAt {
		return
	}

	if compact {
		f.drawCompact(scr, rect)
		return
	}
	f.drawFull(scr, rect)
}

func (f *PhysicsFace) drawCompact(scr uv.Screen, area uv.Rectangle) {
	width := area.Dx()
	height := area.Dy()
	if width <= 0 || height <= 0 {
		return
	}
	if width == 1 {
		f.put(scr, area, 0, (height-1)/2, "•", f.palette.feature, physicsFaceCompactRole)
		return
	}

	leftEye := 0
	rightEye := width - 1
	mouthStart := 0
	mouthEnd := width - 1
	if width >= 5 {
		leftEye = 1
		rightEye = width - 2
		mouthStart = 1
		mouthEnd = width - 2
	}
	eyeY := (height - 1) / 2
	f.put(scr, area, leftEye, eyeY, "•", f.palette.feature, physicsFaceCompactRole)
	f.put(scr, area, rightEye, eyeY, "•", f.palette.feature, physicsFaceCompactRole)
	if height < 2 {
		return
	}
	mouthY := eyeY + 1
	for x := mouthStart + 1; x < mouthEnd; x++ {
		f.put(scr, area, x, mouthY, "─", f.palette.accent, physicsFaceCompactRole)
	}
	f.put(scr, area, mouthStart, mouthY, "╰", f.palette.accent, physicsFaceCompactRole)
	f.put(scr, area, mouthEnd, mouthY, "╯", f.palette.accent, physicsFaceCompactRole)
}

func (f *PhysicsFace) drawFull(scr uv.Screen, area uv.Rectangle) {
	width := area.Dx()
	height := area.Dy()
	cx := width/2 + int(math.Round(f.gazeX.position*0.10))
	cy := height/2 + int(math.Round(f.breath.position*0.85))
	rx := float64(width) * 0.29
	ry := float64(height) * 0.35
	rotation := f.tilt.position * 0.06
	sinRotation := math.Sin(rotation)
	cosRotation := math.Cos(rotation)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx := float64(x-cx) / rx
			dy := float64(y-cy) / ry
			rotatedX := dx*cosRotation - dy*sinRotation
			rotatedY := dx*sinRotation + dy*cosRotation
			distance := rotatedX*rotatedX + rotatedY*rotatedY
			switch {
			case distance > 1.10 && distance < 1.40+f.halo.position*0.08:
				f.put(scr, area, x, y, physicsFaceGlyph(x+y+int(f.frame%4)), f.palette.aura, 0)
			case distance > 0.88 && distance < 1.08:
				f.put(scr, area, x, y, physicsFaceGlyph(x+y/2+int(f.frame%4)), f.palette.outline, 1)
			case distance > 0.72 && distance < 0.88 && y > cy:
				f.put(scr, area, x, y, physicsFaceGlyph(x*3+y+int(f.frame%3)), f.palette.aura, 0)
			}
		}
	}

	f.drawOrbit(scr, area, cx, cy, rx, ry)

	eyeGap := max(3, width/6)
	eyeY := physicsFaceClampInt(cy-max(1, height/8), 1, height-2)
	pupilX := physicsFaceClampInt(int(math.Round(f.gazeX.position*0.38+f.microX.position)), -1, 1)
	pupilY := physicsFaceClampInt(int(math.Round(f.gazeY.position*0.28+f.microY.position)), -1, 1)
	tilt := physicsFaceClampInt(int(math.Round(f.tilt.position*0.35)), -2, 2)
	leftEye := cx - eyeGap
	rightEye := cx + eyeGap
	f.drawBrow(scr, area, leftEye, eyeY-2, -1, tilt)
	f.drawBrow(scr, area, rightEye, eyeY-2, 1, tilt)
	f.drawEye(scr, area, leftEye, eyeY, pupilX, pupilY, tilt)
	f.drawEye(scr, area, rightEye, eyeY, pupilX, pupilY, tilt)

	noseX := physicsFaceClampInt(cx+tilt/2, 1, width-2)
	f.put(scr, area, noseX, cy, "│", f.palette.feature, 2)
	if cy+1 < height-1 {
		f.put(scr, area, noseX, cy+1, "╵", f.palette.feature, 2)
	}
	f.drawMouth(scr, area, cx, cy, tilt)

	for dx := -3; dx <= 3; dx++ {
		y := cy + 2 + physicsFaceAbs(dx)/3
		f.put(scr, area, cx+dx, y, physicsFaceGlyph(dx+int(f.frame%2)), f.palette.aura, 0)
	}
}

func (f *PhysicsFace) drawOrbit(scr uv.Screen, area uv.Rectangle, cx, cy int, rx, ry float64) {
	count := min(26, max(12, area.Dx()/3))
	phase := float64(f.frame%physicsFacePhaseModulo) / 9
	for i := 0; i < count; i++ {
		angle := phase*0.16 + float64(i)*(math.Pi*2/float64(count))
		radiusX := rx + 3.5 + f.halo.position*1.8 + math.Sin(phase*0.23+float64(i))*1.2
		radiusY := ry + 2.0 + f.halo.position*1.1
		x := cx + int(math.Cos(angle)*radiusX)
		y := cy + int(math.Sin(angle)*radiusY)
		f.put(scr, area, x, y, physicsFaceGlyph(i+int(f.frame%4)), f.palette.aura, 0)
	}
}

func (f *PhysicsFace) drawBrow(scr uv.Screen, area uv.Rectangle, cx, cy, direction, tilt int) {
	for i := -3; i <= 3; i++ {
		y := cy + (i*direction+tilt)/3
		f.put(scr, area, cx+i, y, "─", f.palette.outline, 1)
	}
}

func (f *PhysicsFace) drawEye(scr uv.Screen, area uv.Rectangle, cx, cy, pupilX, pupilY, tilt int) {
	if f.blink.position > 0.72 {
		for dx := -3; dx <= 3; dx++ {
			f.put(scr, area, cx+dx, cy+tilt/5, "─", f.palette.outline, 1)
		}
		return
	}

	f.put(scr, area, cx-3, cy-1, "╭", f.palette.outline, 1)
	f.put(scr, area, cx+3, cy-1, "╮", f.palette.outline, 1)
	f.put(scr, area, cx-3, cy+1, "╰", f.palette.outline, 1)
	f.put(scr, area, cx+3, cy+1, "╯", f.palette.outline, 1)
	for dx := -2; dx <= 2; dx++ {
		f.put(scr, area, cx+dx, cy-1+tilt/7, "─", f.palette.outline, 1)
		f.put(scr, area, cx+dx, cy+1-tilt/7, "─", f.palette.outline, 1)
	}
	f.put(scr, area, cx-3, cy, "│", f.palette.outline, 1)
	f.put(scr, area, cx+3, cy, "│", f.palette.outline, 1)
	f.put(scr, area, cx+pupilX, cy+pupilY, "●", f.palette.highlight, 3)
	f.put(scr, area, cx+pupilX+1, cy+pupilY, "•", f.palette.accent, 2)
}

func (f *PhysicsFace) drawMouth(scr uv.Screen, area uv.Rectangle, cx, cy, tilt int) {
	width := area.Dx()
	height := area.Dy()
	mouthWidth := min(11, max(5, width/4))
	start := cx - mouthWidth/2
	end := start + mouthWidth - 1
	mouthY := physicsFaceClampInt(cy+4, 0, height-1)
	mood := physicsFaceClamp(f.mouth.position, 0, 1)
	if mood > 0.52 && mouthY+2 < height-1 {
		for x := start + 1; x < end; x++ {
			f.put(scr, area, x, mouthY, "─", f.palette.accent, 2)
			f.put(scr, area, x, mouthY+2, "─", f.palette.accent, 2)
		}
		f.put(scr, area, start, mouthY, "╭", f.palette.accent, 2)
		f.put(scr, area, end, mouthY, "╮", f.palette.accent, 2)
		f.put(scr, area, start, mouthY+2, "╰", f.palette.accent, 2)
		f.put(scr, area, end, mouthY+2, "╯", f.palette.accent, 2)
		f.put(scr, area, start, mouthY+1, "│", f.palette.accent, 2)
		f.put(scr, area, end, mouthY+1, "│", f.palette.accent, 2)
		return
	}
	for x := start + 1; x < end; x++ {
		f.put(scr, area, x, mouthY, "─", f.palette.accent, 2)
	}
	f.put(scr, area, start, mouthY+tilt/8, "╰", f.palette.accent, 2)
	f.put(scr, area, end, mouthY-tilt/8, "╯", f.palette.accent, 2)
}

func (f *PhysicsFace) put(scr uv.Screen, area uv.Rectangle, x, y int, content string, style uv.Style, role int) {
	if x < 0 || y < 0 || x >= area.Dx() || y >= area.Dy() {
		return
	}
	if !physicsFaceVisible(x, y, role, f.presence.position) {
		return
	}
	f.cell = uv.Cell{Content: content, Style: style, Width: 1}
	scr.SetCell(area.Min.X+x, area.Min.Y+y, &f.cell)
}

func physicsFaceVisible(x, y, role int, presence float64) bool {
	if role == physicsFaceCompactRole {
		return true
	}
	presence = physicsFaceClamp(presence, 0, 1)
	if presence >= 0.999 {
		return true
	}
	keep := presence + float64(role)*0.13
	if keep >= 1 {
		return true
	}
	value := uint32(x+17)*0x9e3779b1 ^ uint32(y+31)*0x85ebca6b ^ uint32(role+7)*0xc2b2ae35
	return float64(value%256)/255 < keep
}

func physicsFaceGlyph(value int) string {
	switch value & 3 {
	case 0:
		return "·"
	case 1:
		return "∘"
	case 2:
		return "∙"
	default:
		return "•"
	}
}

func physicsFaceAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
