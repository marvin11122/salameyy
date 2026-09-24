package dialog

import (
	"fmt"
	"maps"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/common"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

const (
	SamplingID             = "sampling"
	SamplingStudioID       = SamplingID
	samplingDialogMaxWidth = 74
)

type SamplingControl struct {
	ID    string
	Title string
}

var samplingControlDefinitions = []SamplingControl{
	{ID: "preset", Title: "Preset"},
	{ID: "verbosity", Title: "Verbosity"},
	{ID: "temperature", Title: "Temperature"},
	{ID: "top_p", Title: "Top-p"},
	{ID: "top_k", Title: "Top-k"},
	{ID: "max_tokens", Title: "Max tokens"},
	{ID: "repeat_penalty", Title: "Repeat penalty"},
	{ID: "min_p", Title: "Min-p"},
	{ID: "top_a", Title: "Top-a"},
	{ID: "seed", Title: "Seed"},
	{ID: "parallel_tools", Title: "Parallel tools"},
	{ID: "think", Title: "Thinking mode"},
}

type Sampling struct {
	st           *styles.Styles
	help         help.Model
	input        textinput.Model
	model        config.SelectedModel
	presetID     string
	labels       map[string]string
	summary      string
	presetIDs    []string
	presetTitles map[string]string
	controls     []SamplingControl
	selected     int
	editing      bool
	editControl  string
	lastMessage  string
	err          string
	keyMap       struct {
		Up, Down, Left, Right, Select, Edit, Commit, Reset, Close key.Binding
	}
}

var _ Dialog = (*Sampling)(nil)

func NewSampling(com *common.Common, current ...config.SelectedModel) *Sampling {
	st := dialogStyles(com)
	model := config.SelectedModel{}
	if len(current) > 0 {
		model = current[0]
	} else {
		model = currentLargeSelectedModel(com)
	}

	presetIDs := config.SamplingPresetIDs()
	presetTitles := make(map[string]string, len(presetIDs))
	for _, preset := range config.SamplingPresets() {
		presetTitles[preset.ID] = preset.Title
	}

	input := textinput.New()
	input.SetVirtualCursor(false)
	input.SetStyles(st.TextInput)
	input.Placeholder = "value"

	s := &Sampling{
		st:           st,
		input:        input,
		model:        cloneDialogModel(model),
		presetIDs:    append([]string(nil), presetIDs...),
		presetTitles: presetTitles,
		controls:     append([]SamplingControl(nil), samplingControlDefinitions...),
	}
	s.syncSnapshot()
	s.help = help.New()
	s.help.Styles = st.DialogHelpStyles()
	s.keyMap.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "previous control"))
	s.keyMap.Down = key.NewBinding(key.WithKeys("down", "tab"), key.WithHelp("↓/tab", "next control"))
	s.keyMap.Left = key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "previous value"))
	s.keyMap.Right = key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "next value"))
	s.keyMap.Select = key.NewBinding(key.WithKeys("enter", " ", "ctrl+y"), key.WithHelp("enter/space", "cycle value"))
	s.keyMap.Edit = key.NewBinding(key.WithKeys("e", "ctrl+e"), key.WithHelp("e", "edit value"))
	s.keyMap.Commit = key.NewBinding(key.WithKeys("enter", "ctrl+y"), key.WithHelp("enter", "apply value"))
	s.keyMap.Reset = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "manual preset"))
	s.keyMap.Close = CloseKey
	return s
}

func NewSamplingStudio(com *common.Common, current ...config.SelectedModel) *Sampling {
	return NewSampling(com, current...)
}

func NewSamplingWithModel(com *common.Common, model config.SelectedModel) *Sampling {
	return NewSampling(com, model)
}

func (s *Sampling) ID() string {
	return SamplingID
}

func (s *Sampling) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if s.editing {
		switch {
		case keyMsg.String() == "esc" || keyMsg.String() == "alt+esc":
			s.CancelEdit()
			return nil
		case key.Matches(keyMsg, s.keyMap.Commit):
			return s.commitEdit()
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(keyMsg)
			return ActionCmd{Cmd: cmd}
		}
	}

	switch {
	case key.Matches(keyMsg, s.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, s.keyMap.Up):
		s.moveSelection(-1)
	case key.Matches(keyMsg, s.keyMap.Down):
		s.moveSelection(1)
	case key.Matches(keyMsg, s.keyMap.Left):
		return s.CycleControl(-1)
	case key.Matches(keyMsg, s.keyMap.Right):
		return s.CycleControl(1)
	case key.Matches(keyMsg, s.keyMap.Select):
		return s.CycleControl(1)
	case key.Matches(keyMsg, s.keyMap.Edit):
		s.BeginEdit()
	case key.Matches(keyMsg, s.keyMap.Reset):
		return s.SetPreset("manual")
	}
	return nil
}

func (s *Sampling) Cursor() *tea.Cursor {
	if !s.editing {
		return nil
	}
	cur := InputCursor(s.st, s.input.Cursor())
	if cur != nil {
		cur.Y += 2 + s.selected
	}
	return cur
}

func (s *Sampling) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	width := max(0, min(samplingDialogMaxWidth, area.Dx()))
	innerWidth := max(0, width-s.st.Dialog.View.GetHorizontalFrameSize())
	s.input.SetWidth(max(0, innerWidth-s.st.Dialog.InputPrompt.GetHorizontalFrameSize()-1))

	rc := NewRenderContext(s.st, width)
	rc.Title = "Sampling Studio"
	rc.Gap = 1
	rc.AddPart(s.renderContent(innerWidth))
	if s.err != "" {
		rc.AddPart(s.st.Dialog.TitleError.Render(ansi.Truncate(s.err, innerWidth, "…")))
	}
	rc.Help = s.help.View(s)
	cur := s.Cursor()
	DrawCenterCursor(scr, area, rc.Render(), cur)
	return cur
}

func (s *Sampling) ShortHelp() []key.Binding {
	if s.editing {
		return []key.Binding{s.keyMap.Commit, s.keyMap.Close}
	}
	return []key.Binding{s.keyMap.Up, s.keyMap.Right, s.keyMap.Select, s.keyMap.Edit, s.keyMap.Close}
}

func (s *Sampling) FullHelp() [][]key.Binding {
	if s.editing {
		return [][]key.Binding{{s.keyMap.Commit, s.keyMap.Close}}
	}
	return [][]key.Binding{{s.keyMap.Up, s.keyMap.Down, s.keyMap.Left, s.keyMap.Right}, {s.keyMap.Select, s.keyMap.Edit, s.keyMap.Reset, s.keyMap.Close}}
}

func (s *Sampling) Model() config.SelectedModel {
	return cloneDialogModel(s.model)
}

func (s *Sampling) SetModel(model config.SelectedModel) {
	s.model = cloneDialogModel(model)
	s.syncSnapshot()
	s.err = ""
}

func (s *Sampling) SelectedControl() string {
	if s.selected < 0 || s.selected >= len(s.controls) {
		return ""
	}
	return s.controls[s.selected].ID
}

func (s *Sampling) SelectControl(controlID string) bool {
	for i, control := range s.controls {
		if control.ID == controlID {
			s.selected = i
			return true
		}
	}
	return false
}

func (s *Sampling) IsEditing() bool {
	return s.editing
}

func (s *Sampling) Error() string {
	return s.err
}

func (s *Sampling) LastMessage() string {
	return s.lastMessage
}

func (s *Sampling) syncSnapshot() {
	s.presetID = config.CurrentSamplingPreset(s.model)
	s.labels = config.SamplingControlLabels(s.model)
	s.summary = config.SamplingSummary(s.model)
}

func (s *Sampling) BeginEdit() bool {
	if s.editing || s.selected < 0 || s.selected >= len(s.controls) {
		return false
	}
	s.editControl = s.controls[s.selected].ID
	editValue := s.controlValue(s.editControl)
	if s.editControl == "preset" {
		editValue = s.presetID
	}
	s.input.SetValue(editValue)
	s.input.CursorEnd()
	s.input.Focus()
	s.editing = true
	s.err = ""
	return true
}

func (s *Sampling) CancelEdit() {
	s.editing = false
	s.editControl = ""
	s.input.Blur()
	s.input.Reset()
}

func (s *Sampling) SetControlValue(controlID, raw string) Action {
	updated, message, err := config.SetSamplingControlValue(s.model, controlID, raw)
	if err != nil {
		s.err = err.Error()
		return nil
	}
	s.model = cloneDialogModel(updated)
	s.syncSnapshot()
	s.lastMessage = message
	s.err = ""
	return ActionUpdateSampling{Model: s.Model()}
}

func (s *Sampling) SetPreset(presetID string) Action {
	if strings.EqualFold(strings.TrimSpace(presetID), "manual") {
		return s.SetControlValue("preset", "manual")
	}
	updated, message, err := config.ApplySamplingPreset(s.model, presetID)
	if err != nil {
		s.err = err.Error()
		return nil
	}
	s.model = cloneDialogModel(updated)
	s.syncSnapshot()
	s.lastMessage = message
	s.err = ""
	return ActionUpdateSampling{Model: s.Model()}
}

func (s *Sampling) CycleControl(direction int) Action {
	controlID := s.SelectedControl()
	if controlID == "" {
		return nil
	}
	if direction == 0 {
		return nil
	}
	if controlID == "preset" {
		return s.cyclePreset(direction)
	}
	steps := samplingCycleLength(controlID)
	if steps < 2 {
		return nil
	}
	if direction < 0 {
		steps--
	} else {
		steps = 1
	}
	updated := s.model
	message := ""
	for range steps {
		var err error
		updated, message, err = config.ApplySamplingControl(updated, controlID)
		if err != nil {
			s.err = err.Error()
			return nil
		}
	}
	s.model = cloneDialogModel(updated)
	s.syncSnapshot()
	s.lastMessage = message
	s.err = ""
	return ActionUpdateSampling{Model: s.Model()}
}

func (s *Sampling) commitEdit() Action {
	controlID := s.editControl
	action := s.SetControlValue(controlID, s.input.Value())
	if action != nil {
		s.CancelEdit()
	}
	return action
}

func (s *Sampling) cyclePreset(direction int) Action {
	if len(s.presetIDs) == 0 {
		return nil
	}
	current := s.presetID
	index := -1
	for i, id := range s.presetIDs {
		if id == current {
			index = i
			break
		}
	}
	if index < 0 {
		index = 0
	}
	next := (index + direction) % len(s.presetIDs)
	if next < 0 {
		next += len(s.presetIDs)
	}
	return s.SetPreset(s.presetIDs[next])
}

func (s *Sampling) moveSelection(delta int) {
	if len(s.controls) == 0 {
		return
	}
	s.selected = (s.selected + delta) % len(s.controls)
	if s.selected < 0 {
		s.selected += len(s.controls)
	}
}

func (s *Sampling) controlValue(controlID string) string {
	if controlID == "preset" {
		id := s.presetID
		if id == "manual" {
			return "manual"
		}
		if title := s.presetTitles[id]; title != "" {
			return title + " (" + id + ")"
		}
		return id
	}
	return s.labels[controlID]
}

func (s *Sampling) renderContent(width int) string {
	if width <= 0 {
		return ""
	}
	summary := s.st.Dialog.SecondaryText.Render(ansi.Truncate(s.summary, width, "…"))
	lines := []string{summary, ""}
	for i, control := range s.controls {
		marker := "  "
		if i == s.selected {
			marker = s.st.Dialog.PrimaryText.Render("▸ ")
		}
		text := fmt.Sprintf("%s%s  %s", marker, control.Title, s.controlValue(control.ID))
		text = ansi.Truncate(text, width, "…")
		if i == s.selected {
			lines = append(lines, s.st.Dialog.SelectedItem.Render(text))
		} else {
			lines = append(lines, s.st.Dialog.NormalItem.Render(text))
		}
	}
	if s.editing {
		input := s.st.Dialog.InputPrompt.Render(s.input.View())
		lines = append(lines, input)
	}
	return strings.Join(lines, "\n")
}

func samplingCycleLength(controlID string) int {
	switch controlID {
	case "verbosity":
		return 3
	case "temperature", "top_p", "top_k", "max_tokens":
		return 6
	case "repeat_penalty", "min_p", "top_a", "seed":
		return 5
	case "parallel_tools":
		return 3
	case "think":
		return 2
	default:
		return 0
	}
}

func dialogStyles(com *common.Common) *styles.Styles {
	if com != nil && com.Styles != nil {
		return com.Styles
	}
	st := styles.DefaultStyles()
	return &st
}

func currentLargeSelectedModel(com *common.Common) config.SelectedModel {
	if com == nil || com.App == nil {
		return config.SelectedModel{}
	}
	cfg := com.App.Config()
	if cfg == nil {
		return config.SelectedModel{}
	}
	return cloneDialogModel(cfg.Models[config.SelectedModelTypeLarge])
}

func cloneDialogModel(model config.SelectedModel) config.SelectedModel {
	out := model
	out.ProviderOptions = maps.Clone(model.ProviderOptions)
	out.Temperature = cloneDialogFloat64(model.Temperature)
	out.TopP = cloneDialogFloat64(model.TopP)
	out.TopK = cloneDialogInt64(model.TopK)
	out.FrequencyPenalty = cloneDialogFloat64(model.FrequencyPenalty)
	out.PresencePenalty = cloneDialogFloat64(model.PresencePenalty)
	if model.Sampling != nil {
		sampling := *model.Sampling
		sampling.Seed = cloneDialogInt64(model.Sampling.Seed)
		sampling.MinP = cloneDialogFloat64(model.Sampling.MinP)
		sampling.TopA = cloneDialogFloat64(model.Sampling.TopA)
		sampling.RepeatPenalty = cloneDialogFloat64(model.Sampling.RepeatPenalty)
		sampling.ParallelToolCalls = cloneDialogBool(model.Sampling.ParallelToolCalls)
		out.Sampling = &sampling
	}
	return out
}

func cloneDialogFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDialogInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDialogBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func SamplingControls() []SamplingControl {
	return append([]SamplingControl(nil), samplingControlDefinitions...)
}
