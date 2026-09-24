package dialog

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/common"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

const (
	KeybindingsID             = "keybindings"
	KeyBindingsID             = KeybindingsID
	KeybindingID              = KeybindingsID
	keybindingsDialogMaxWidth = 68
)

type GlobalAction string

const (
	GlobalActionModels      GlobalAction = "models"
	GlobalActionSessions    GlobalAction = "sessions"
	GlobalActionCommands    GlobalAction = "commands"
	GlobalActionHelp        GlobalAction = "help"
	GlobalActionDisplay     GlobalAction = "display"
	GlobalActionSampling    GlobalAction = "sampling"
	GlobalActionKeybindings GlobalAction = "keybindings"
)

const (
	KeybindingModels      = GlobalActionModels
	KeybindingSessions    = GlobalActionSessions
	KeybindingCommands    = GlobalActionCommands
	KeybindingHelp        = GlobalActionHelp
	KeybindingDisplay     = GlobalActionDisplay
	KeybindingSampling    = GlobalActionSampling
	KeybindingKeybindings = GlobalActionKeybindings
)

var globalActionOrder = []GlobalAction{
	GlobalActionModels,
	GlobalActionSessions,
	GlobalActionCommands,
	GlobalActionHelp,
	GlobalActionDisplay,
	GlobalActionSampling,
	GlobalActionKeybindings,
}

var defaultGlobalKeybindings = map[GlobalAction]string{
	GlobalActionModels:      "ctrl+l",
	GlobalActionSessions:    "ctrl+s",
	GlobalActionCommands:    "ctrl+p",
	GlobalActionHelp:        "ctrl+g",
	GlobalActionDisplay:     "f2",
	GlobalActionSampling:    "f3",
	GlobalActionKeybindings: "f4",
}

type Keybindings struct {
	st       *styles.Styles
	help     help.Model
	bindings map[string]string
	selected int
	editing  bool
	err      string
	keyMap   struct {
		Up, Down, Edit, Reset, Close key.Binding
	}
}

var _ Dialog = (*Keybindings)(nil)

func NewKeybindings(com *common.Common, current ...map[string]string) *Keybindings {
	var configured map[string]string
	if len(current) > 0 {
		configured = cloneStringMap(current[0])
	} else {
		configured = currentKeybindings(com)
	}
	k := &Keybindings{
		st:       dialogStyles(com),
		bindings: make(map[string]string, len(globalActionOrder)),
	}
	k.help = help.New()
	k.help.Styles = k.st.DialogHelpStyles()
	k.keyMap.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "previous action"))
	k.keyMap.Down = key.NewBinding(key.WithKeys("down", "tab"), key.WithHelp("↓/tab", "next action"))
	k.keyMap.Edit = key.NewBinding(key.WithKeys("enter", "e", "ctrl+e"), key.WithHelp("enter/e", "edit key"))
	k.keyMap.Reset = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset key"))
	k.keyMap.Close = CloseKey
	k.load(configured)
	return k
}

func NewKeybindingDialog(com *common.Common, current ...map[string]string) *Keybindings {
	return NewKeybindings(com, current...)
}

func NewKeybindingsFromMap(com *common.Common, current map[string]string) *Keybindings {
	return NewKeybindings(com, cloneStringMap(current))
}

func (k *Keybindings) ID() string {
	return KeybindingsID
}

func (k *Keybindings) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if k.editing {
		if keyMsg.String() == "esc" || keyMsg.String() == "alt+esc" {
			k.editing = false
			k.err = ""
			return nil
		}
		return k.SetBinding(k.SelectedAction(), keyMsg.Keystroke())
	}

	switch {
	case key.Matches(keyMsg, k.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, k.keyMap.Up):
		k.moveSelection(-1)
	case key.Matches(keyMsg, k.keyMap.Down):
		k.moveSelection(1)
	case key.Matches(keyMsg, k.keyMap.Edit):
		k.editing = true
		k.err = ""
	case key.Matches(keyMsg, k.keyMap.Reset):
		return k.ResetSelected()
	}
	return nil
}

func (k *Keybindings) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	width := max(0, min(keybindingsDialogMaxWidth, area.Dx()))
	innerWidth := max(0, width-k.st.Dialog.View.GetHorizontalFrameSize())
	rc := NewRenderContext(k.st, width)
	rc.Title = "Keybindings"
	rc.Gap = 1
	rc.AddPart(k.renderContent(innerWidth))
	if k.err != "" {
		rc.AddPart(k.st.Dialog.TitleError.Render(ansi.Truncate(k.err, innerWidth, "…")))
	}
	if k.editing {
		rc.AddPart(k.st.Dialog.SecondaryText.Render("Press a key to bind, or esc to cancel"))
	}
	rc.Help = k.help.View(k)
	DrawCenter(scr, area, rc.Render())
	return nil
}

func (k *Keybindings) ShortHelp() []key.Binding {
	if k.editing {
		return []key.Binding{k.keyMap.Close}
	}
	return []key.Binding{k.keyMap.Up, k.keyMap.Down, k.keyMap.Edit, k.keyMap.Reset, k.keyMap.Close}
}

func (k *Keybindings) FullHelp() [][]key.Binding {
	if k.editing {
		return [][]key.Binding{{k.keyMap.Close}}
	}
	return [][]key.Binding{{k.keyMap.Up, k.keyMap.Down}, {k.keyMap.Edit, k.keyMap.Reset, k.keyMap.Close}}
}

func (k *Keybindings) SelectedAction() string {
	if k.selected < 0 || k.selected >= len(globalActionOrder) {
		return ""
	}
	return string(globalActionOrder[k.selected])
}

func (k *Keybindings) Binding(action string) string {
	return k.bindings[strings.ToLower(strings.TrimSpace(action))]
}

func (k *Keybindings) GetBinding(action GlobalAction) string {
	return k.bindings[string(action)]
}

func (k *Keybindings) Bindings() map[string]string {
	return cloneStringMap(k.bindings)
}

func (k *Keybindings) IsEditing() bool {
	return k.editing
}

func (k *Keybindings) Error() string {
	return k.err
}

func (k *Keybindings) SelectAction(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	for i, candidate := range globalActionOrder {
		if string(candidate) == action {
			k.selected = i
			return true
		}
	}
	return false
}

func (k *Keybindings) SetBinding(action, raw string) Action {
	action = strings.ToLower(strings.TrimSpace(action))
	if !knownGlobalAction(action) {
		k.err = fmt.Sprintf("unknown global action %q", action)
		return nil
	}
	normalized, err := NormalizeKeyString(raw)
	if err != nil {
		k.err = err.Error()
		return nil
	}
	for existingAction, existingKey := range k.bindings {
		if existingAction != action && existingKey == normalized {
			k.err = fmt.Sprintf("key %q is already assigned to %s", normalized, existingAction)
			return nil
		}
	}
	k.bindings[action] = normalized
	k.err = ""
	k.editing = false
	return ActionUpdateKeybinding{
		Action:   action,
		Key:      normalized,
		Bindings: k.Bindings(),
	}
}

func (k *Keybindings) ResetSelected() Action {
	action := k.SelectedAction()
	if action == "" {
		return nil
	}
	return k.SetBinding(action, defaultGlobalKeybindings[GlobalAction(action)])
}

func (k *Keybindings) moveSelection(delta int) {
	if len(globalActionOrder) == 0 {
		return
	}
	k.selected = (k.selected + delta) % len(globalActionOrder)
	if k.selected < 0 {
		k.selected += len(globalActionOrder)
	}
}

func (k *Keybindings) load(configured map[string]string) {
	used := make(map[string]string, len(globalActionOrder))
	for _, action := range globalActionOrder {
		name := string(action)
		value := defaultGlobalKeybindings[action]
		if raw, ok := configured[name]; ok {
			if normalized, err := NormalizeKeyString(raw); err == nil {
				if _, exists := used[normalized]; !exists {
					value = normalized
				}
			}
		}
		if owner, exists := used[value]; exists && owner != name {
			value = availableFallbackKey(used)
		}
		k.bindings[name] = value
		used[value] = name
	}
}

func availableFallbackKey(used map[string]string) string {
	for _, base := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"} {
		candidate := "ctrl+alt+" + base
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
	for i := 0; i < 10; i++ {
		candidate := fmt.Sprintf("ctrl+alt+%d", i)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
	return "ctrl+alt+space"
}

func (k *Keybindings) renderContent(width int) string {
	if width <= 0 {
		return ""
	}
	lines := make([]string, 0, len(globalActionOrder))
	for i, action := range globalActionOrder {
		marker := "  "
		if i == k.selected {
			marker = k.st.Dialog.PrimaryText.Render("▸ ")
		}
		value := k.bindings[string(action)]
		if k.editing && i == k.selected {
			value = "press a key"
		}
		text := fmt.Sprintf("%s%s  %s", marker, keybindingTitle(action), value)
		text = ansi.Truncate(text, width, "…")
		if i == k.selected {
			lines = append(lines, k.st.Dialog.SelectedItem.Render(text))
		} else {
			lines = append(lines, k.st.Dialog.NormalItem.Render(text))
		}
	}
	return strings.Join(lines, "\n")
}

func keybindingTitle(action GlobalAction) string {
	switch action {
	case GlobalActionModels:
		return "Models"
	case GlobalActionSessions:
		return "Sessions"
	case GlobalActionCommands:
		return "Commands"
	case GlobalActionHelp:
		return "Help"
	case GlobalActionDisplay:
		return "Display"
	case GlobalActionSampling:
		return "Sampling"
	case GlobalActionKeybindings:
		return "Keybindings"
	default:
		return string(action)
	}
}

func knownGlobalAction(action string) bool {
	for _, candidate := range globalActionOrder {
		if string(candidate) == action {
			return true
		}
	}
	return false
}

func DefaultKeybindings() map[string]string {
	out := make(map[string]string, len(defaultGlobalKeybindings))
	for action, value := range defaultGlobalKeybindings {
		out[string(action)] = value
	}
	return out
}

func GlobalActions() []GlobalAction {
	return append([]GlobalAction(nil), globalActionOrder...)
}

func NormalizeKeyString(raw string) (string, error) {
	if raw == " " {
		return "space", nil
	}
	if raw == "+" {
		return "+", nil
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("key cannot be empty")
	}
	if len(value) > 32 || strings.ContainsAny(value, "\r\n\t") || !utf8.ValidString(value) {
		return "", fmt.Errorf("invalid key %q", raw)
	}
	parts := strings.Split(strings.ToLower(value), "+")
	if len(parts) < 1 || len(parts) > 7 {
		return "", fmt.Errorf("invalid key %q", raw)
	}
	modifierNames := map[string]string{
		"ctrl":    "ctrl",
		"control": "ctrl",
		"alt":     "alt",
		"option":  "alt",
		"shift":   "shift",
		"meta":    "meta",
		"cmd":     "super",
		"command": "super",
		"super":   "super",
		"hyper":   "hyper",
	}
	modifiers := make([]string, 0, len(parts)-1)
	seen := make(map[string]bool, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		canonical, ok := modifierNames[part]
		if !ok || seen[canonical] {
			return "", fmt.Errorf("invalid key %q", raw)
		}
		seen[canonical] = true
		modifiers = append(modifiers, canonical)
	}
	base := parts[len(parts)-1]
	if base == "" {
		return "", fmt.Errorf("invalid key %q", raw)
	}
	if canonical, ok := canonicalKeyName(base); ok {
		base = canonical
	} else {
		if utf8.RuneCountInString(base) != 1 || !unicode.IsPrint([]rune(base)[0]) || unicode.IsSpace([]rune(base)[0]) {
			return "", fmt.Errorf("invalid key %q", raw)
		}
	}
	order := map[string]int{"ctrl": 0, "alt": 1, "shift": 2, "meta": 3, "hyper": 4, "super": 5}
	for i := range modifiers {
		for j := i + 1; j < len(modifiers); j++ {
			if order[modifiers[j]] < order[modifiers[i]] {
				modifiers[i], modifiers[j] = modifiers[j], modifiers[i]
			}
		}
	}
	normalized := append(modifiers, base)
	value = strings.Join(normalized, "+")
	if value == "esc" || value == "alt+esc" || value == "ctrl+c" || value == "ctrl+z" {
		return "", fmt.Errorf("key %q is reserved", raw)
	}
	return value, nil
}

func canonicalKeyName(name string) (string, bool) {
	switch name {
	case "return":
		return "enter", true
	case "escape":
		return "esc", true
	case "pgup":
		return "pageup", true
	case "pgdown":
		return "pagedown", true
	case "del":
		return "delete", true
	case "ins":
		return "insert", true
	case "plus":
		return "+", true
	case "spacebar":
		return "space", true
	case "up", "down", "left", "right", "begin", "find", "select", "enter", "esc", "tab", "space", "backspace", "delete", "insert", "home", "end", "pageup", "pagedown", "equal", "mul", "comma", "minus", "period", "div", "capslock", "scrolllock", "numlock", "printscreen", "pause", "menu", "mediaplay", "mediapause", "mediaplaypause", "mediareverse", "mediastop", "mediafastforward", "mediarewind", "volup", "voldown", "volmute":
		return name, true
	default:
		if len(name) > 1 && name[0] == 'f' {
			value := 0
			for _, char := range name[1:] {
				if char < '0' || char > '9' {
					return "", false
				}
				value = value*10 + int(char-'0')
			}
			if value >= 1 && value <= 63 {
				return name, true
			}
		}
		return "", false
	}
}

func ValidateKeybinding(raw string) error {
	_, err := NormalizeKeyString(raw)
	return err
}

func ParseKeybinding(raw string) (string, error) {
	return NormalizeKeyString(raw)
}

func ValidateKeybindings(bindings map[string]string) error {
	normalized := make(map[string]string, len(bindings))
	for action, raw := range bindings {
		value, err := NormalizeKeyString(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", action, err)
		}
		if previous, exists := normalized[value]; exists {
			return fmt.Errorf("%s conflicts with %s", action, previous)
		}
		normalized[value] = action
	}
	return nil
}

func currentKeybindings(com *common.Common) map[string]string {
	return cloneStringMap(currentDisplayOptions(com).Keybindings)
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
