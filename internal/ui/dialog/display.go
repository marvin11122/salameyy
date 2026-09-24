package dialog

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/common"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

const (
	DisplaySettingsID     = "display"
	DisplayID             = DisplaySettingsID
	displayDialogMaxWidth = 62
)

type DisplayControl struct {
	ID    string
	Title string
}

var displayControlDefinitions = []DisplayControl{
	{ID: "physics_face", Title: "Physics face"},
	{ID: "animation_speed", Title: "Animation speed"},
	{ID: "show_metrics", Title: "Show metrics"},
	{ID: "transparency", Title: "Transparency"},
	{ID: "compact_mode", Title: "Compact mode"},
}

type DisplaySettingsState struct {
	PhysicsFace    bool
	AnimationSpeed string
	ShowMetrics    bool
	Transparent    bool
	CompactMode    bool
}

type DisplaySettings struct {
	st       *styles.Styles
	help     help.Model
	state    DisplaySettingsState
	controls []DisplayControl
	selected int
	err      string
	keyMap   struct {
		Up, Down, Left, Right, Select, Reset, Close key.Binding
	}
}

var _ Dialog = (*DisplaySettings)(nil)

func NewDisplaySettings(com *common.Common, current ...config.TUIOptions) *DisplaySettings {
	var options config.TUIOptions
	if len(current) > 0 {
		options = cloneTUIOptions(current[0])
	} else {
		options = currentDisplayOptions(com)
	}
	state := displayStateFromOptions(options)
	d := &DisplaySettings{
		st:       dialogStyles(com),
		state:    state,
		controls: append([]DisplayControl(nil), displayControlDefinitions...),
	}
	d.help = help.New()
	d.help.Styles = d.st.DialogHelpStyles()
	d.keyMap.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "previous setting"))
	d.keyMap.Down = key.NewBinding(key.WithKeys("down", "tab"), key.WithHelp("↓/tab", "next setting"))
	d.keyMap.Left = key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "previous value"))
	d.keyMap.Right = key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "next value"))
	d.keyMap.Select = key.NewBinding(key.WithKeys("enter", " ", "ctrl+y"), key.WithHelp("enter/space", "change value"))
	d.keyMap.Reset = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset settings"))
	d.keyMap.Close = CloseKey
	return d
}

func NewDisplaySettingsFromOptions(com *common.Common, options *config.TUIOptions) *DisplaySettings {
	if options == nil {
		return NewDisplaySettings(com)
	}
	return NewDisplaySettings(com, cloneTUIOptions(*options))
}

func NewDisplaySettingsDialog(com *common.Common, current ...config.TUIOptions) *DisplaySettings {
	return NewDisplaySettings(com, current...)
}

func (d *DisplaySettings) ID() string {
	return DisplaySettingsID
}

func (d *DisplaySettings) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(keyMsg, d.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, d.keyMap.Up):
		d.moveSelection(-1)
	case key.Matches(keyMsg, d.keyMap.Down):
		d.moveSelection(1)
	case key.Matches(keyMsg, d.keyMap.Left):
		return d.CycleControl(-1)
	case key.Matches(keyMsg, d.keyMap.Right):
		return d.CycleControl(1)
	case key.Matches(keyMsg, d.keyMap.Select):
		return d.CycleControl(1)
	case key.Matches(keyMsg, d.keyMap.Reset):
		return d.Reset()
	}
	return nil
}

func (d *DisplaySettings) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	width := max(0, min(displayDialogMaxWidth, area.Dx()))
	innerWidth := max(0, width-d.st.Dialog.View.GetHorizontalFrameSize())
	rc := NewRenderContext(d.st, width)
	rc.Title = "Display Settings"
	rc.Gap = 1
	rc.AddPart(d.renderContent(innerWidth))
	if d.err != "" {
		rc.AddPart(d.st.Dialog.TitleError.Render(ansi.Truncate(d.err, innerWidth, "…")))
	}
	rc.Help = d.help.View(d)
	DrawCenter(scr, area, rc.Render())
	return nil
}

func (d *DisplaySettings) ShortHelp() []key.Binding {
	return []key.Binding{d.keyMap.Up, d.keyMap.Right, d.keyMap.Select, d.keyMap.Reset, d.keyMap.Close}
}

func (d *DisplaySettings) FullHelp() [][]key.Binding {
	return [][]key.Binding{{d.keyMap.Up, d.keyMap.Down, d.keyMap.Left, d.keyMap.Right}, {d.keyMap.Select, d.keyMap.Reset, d.keyMap.Close}}
}

func (d *DisplaySettings) State() DisplaySettingsState {
	return d.state
}

func (a ActionUpdateDisplaySettings) TUIOptions() config.TUIOptions {
	return DisplaySettingsState{
		PhysicsFace:    a.PhysicsFace,
		AnimationSpeed: a.AnimationSpeed,
		ShowMetrics:    a.ShowMetrics,
		Transparent:    a.Transparent,
		CompactMode:    a.CompactMode,
	}.TUIOptions()
}

func (d *DisplaySettings) Options() config.TUIOptions {
	return d.state.TUIOptions()
}

func (s DisplaySettingsState) TUIOptions() config.TUIOptions {
	return config.TUIOptions{
		PhysicsFace:    cloneDialogBool(&s.PhysicsFace),
		AnimationSpeed: s.AnimationSpeed,
		ShowMetrics:    cloneDialogBool(&s.ShowMetrics),
		Transparent:    cloneDialogBool(&s.Transparent),
		CompactMode:    s.CompactMode,
	}
}

func (d *DisplaySettings) SetState(state DisplaySettingsState) {
	d.state = normalizeDisplayState(state)
	d.err = ""
}

func (d *DisplaySettings) SetControl(controlID string, value string) Action {
	controlID = strings.TrimSpace(controlID)
	switch controlID {
	case "physics_face":
		return d.setBool(controlID, value, func(v bool) { d.state.PhysicsFace = v })
	case "show_metrics":
		return d.setBool(controlID, value, func(v bool) { d.state.ShowMetrics = v })
	case "transparency", "transparent":
		return d.setBool(controlID, value, func(v bool) { d.state.Transparent = v })
	case "compact_mode":
		return d.setBool(controlID, value, func(v bool) { d.state.CompactMode = v })
	case "animation_speed":
		speed := strings.ToLower(strings.TrimSpace(value))
		if !validAnimationSpeed(speed) {
			d.err = fmt.Sprintf("invalid animation speed %q", value)
			return nil
		}
		d.state.AnimationSpeed = speed
		d.err = ""
		return d.action()
	default:
		d.err = fmt.Sprintf("unknown display setting %q", controlID)
		return nil
	}
}

func (d *DisplaySettings) CycleControl(direction int) Action {
	if direction == 0 {
		return nil
	}
	controlID := d.SelectedControl()
	switch controlID {
	case "physics_face":
		return d.SetControl(controlID, displayBoolLabel(!d.state.PhysicsFace))
	case "show_metrics":
		return d.SetControl(controlID, displayBoolLabel(!d.state.ShowMetrics))
	case "transparency":
		return d.SetControl(controlID, displayBoolLabel(!d.state.Transparent))
	case "compact_mode":
		return d.SetControl(controlID, displayBoolLabel(!d.state.CompactMode))
	case "animation_speed":
		values := []string{"off", "slow", "medium", "fast"}
		index := 0
		for i, value := range values {
			if value == d.state.AnimationSpeed {
				index = i
				break
			}
		}
		if direction < 0 {
			index--
		} else {
			index++
		}
		index = (index + len(values)) % len(values)
		return d.SetControl(controlID, values[index])
	default:
		return nil
	}
}

func (d *DisplaySettings) Reset() Action {
	d.state = DisplaySettingsState{
		PhysicsFace:    true,
		AnimationSpeed: "medium",
		ShowMetrics:    true,
		Transparent:    false,
		CompactMode:    false,
	}
	d.err = ""
	return d.action()
}

func (d *DisplaySettings) SelectedControl() string {
	if d.selected < 0 || d.selected >= len(d.controls) {
		return ""
	}
	return d.controls[d.selected].ID
}

func (d *DisplaySettings) SelectControl(controlID string) bool {
	for i, control := range d.controls {
		if control.ID == controlID {
			d.selected = i
			return true
		}
	}
	return false
}

func (d *DisplaySettings) Error() string {
	return d.err
}

func (d *DisplaySettings) moveSelection(delta int) {
	if len(d.controls) == 0 {
		return
	}
	d.selected = (d.selected + delta) % len(d.controls)
	if d.selected < 0 {
		d.selected += len(d.controls)
	}
}

func (d *DisplaySettings) setBool(controlID, value string, set func(bool)) Action {
	var parsed bool
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "yes", "y":
		parsed = true
	case "off", "false", "0", "no", "n":
		parsed = false
	default:
		d.err = fmt.Sprintf("invalid value %q for %s", value, controlID)
		return nil
	}
	set(parsed)
	d.err = ""
	return d.action()
}

func (d *DisplaySettings) action() Action {
	return ActionUpdateDisplaySettings{
		PhysicsFace:    d.state.PhysicsFace,
		AnimationSpeed: d.state.AnimationSpeed,
		ShowMetrics:    d.state.ShowMetrics,
		Transparent:    d.state.Transparent,
		CompactMode:    d.state.CompactMode,
	}
}

func (d *DisplaySettings) renderContent(width int) string {
	if width <= 0 {
		return ""
	}
	lines := make([]string, 0, len(d.controls))
	for i, control := range d.controls {
		marker := "  "
		if i == d.selected {
			marker = d.st.Dialog.PrimaryText.Render("▸ ")
		}
		text := fmt.Sprintf("%s%s  %s", marker, control.Title, d.controlValue(control.ID))
		text = ansi.Truncate(text, width, "…")
		if i == d.selected {
			lines = append(lines, d.st.Dialog.SelectedItem.Render(text))
		} else {
			lines = append(lines, d.st.Dialog.NormalItem.Render(text))
		}
	}
	return strings.Join(lines, "\n")
}

func (d *DisplaySettings) controlValue(controlID string) string {
	switch controlID {
	case "physics_face":
		return displayBoolLabel(d.state.PhysicsFace)
	case "animation_speed":
		return d.state.AnimationSpeed
	case "show_metrics":
		return displayBoolLabel(d.state.ShowMetrics)
	case "transparency":
		return displayBoolLabel(d.state.Transparent)
	case "compact_mode":
		return displayBoolLabel(d.state.CompactMode)
	default:
		return ""
	}
}

func displayStateFromOptions(options config.TUIOptions) DisplaySettingsState {
	state := DisplaySettingsState{
		PhysicsFace:    true,
		AnimationSpeed: "medium",
		ShowMetrics:    true,
	}
	if options.PhysicsFace != nil {
		state.PhysicsFace = *options.PhysicsFace
	}
	if validAnimationSpeed(options.AnimationSpeed) {
		state.AnimationSpeed = options.AnimationSpeed
	}
	if options.ShowMetrics != nil {
		state.ShowMetrics = *options.ShowMetrics
	}
	if options.Transparent != nil {
		state.Transparent = *options.Transparent
	}
	state.CompactMode = options.CompactMode
	return state
}

func normalizeDisplayState(state DisplaySettingsState) DisplaySettingsState {
	if !validAnimationSpeed(state.AnimationSpeed) {
		state.AnimationSpeed = "medium"
	}
	return state
}

func validAnimationSpeed(speed string) bool {
	switch speed {
	case "off", "slow", "medium", "fast":
		return true
	default:
		return false
	}
}

func displayBoolLabel(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func currentDisplayOptions(com *common.Common) config.TUIOptions {
	if com == nil || com.App == nil {
		return config.TUIOptions{}
	}
	cfg := com.App.Config()
	if cfg == nil || cfg.Options == nil || cfg.Options.TUI == nil {
		return config.TUIOptions{}
	}
	return cloneTUIOptions(*cfg.Options.TUI)
}

func cloneTUIOptions(options config.TUIOptions) config.TUIOptions {
	out := options
	out.Transparent = cloneDialogBool(options.Transparent)
	out.PhysicsFace = cloneDialogBool(options.PhysicsFace)
	out.ShowMetrics = cloneDialogBool(options.ShowMetrics)
	if options.Keybindings != nil {
		out.Keybindings = make(map[string]string, len(options.Keybindings))
		for key, value := range options.Keybindings {
			out.Keybindings[key] = value
		}
	}
	return out
}

func DisplayControls() []DisplayControl {
	return append([]DisplayControl(nil), displayControlDefinitions...)
}
