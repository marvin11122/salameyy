package dialog

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/wiesnermarven-hue/salameyy/internal/commands"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/common"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/list"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

// CommandsID is the identifier for the commands dialog.
const CommandsID = "commands"

// DocsID is the identifier for the docs dialog.
const DocsID = "docs"

// CommandType represents the type of commands being displayed.
type CommandType uint

// String returns the string representation of the CommandType.
func (c CommandType) String() string { return []string{"System", "User", "MCP"}[c] }

const (
	sidebarCompactModeBreakpoint = 120
)

const (
	SystemCommands CommandType = iota
	UserCommands
	MCPPrompts
)

// Commands represents a dialog that shows available commands.
type Commands struct {
	com    *common.Common
	keyMap struct {
		Select,
		UpDown,
		Next,
		Previous,
		Tab,
		ShiftTab,
		Close key.Binding
	}

	sessionID  string
	hasSession bool
	hasTodos   bool
	hasQueue   bool
	selected   CommandType

	spinner spinner.Model
	loading bool

	help  help.Model
	input textinput.Model
	list  *list.FilterableList

	windowWidth int

	customCommands []commands.CustomCommand
	mcpPrompts     []commands.MCPPrompt
}

var _ Dialog = (*Commands)(nil)

// NewCommands creates a new commands dialog.
func NewCommands(com *common.Common, sessionID string, hasSession, hasTodos, hasQueue bool, customCommands []commands.CustomCommand, mcpPrompts []commands.MCPPrompt) (*Commands, error) {
	c := &Commands{
		com:            com,
		selected:       SystemCommands,
		sessionID:      sessionID,
		hasSession:     hasSession,
		hasTodos:       hasTodos,
		hasQueue:       hasQueue,
		customCommands: customCommands,
		mcpPrompts:     mcpPrompts,
	}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()

	c.help = help

	c.list = list.NewFilterableList()
	c.list.Focus()
	c.list.SetSelected(0)

	c.input = textinput.New()
	c.input.SetVirtualCursor(false)
	c.input.Placeholder = "Type to filter"
	c.input.SetStyles(com.Styles.TextInput)
	c.input.Focus()

	c.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "confirm"),
	)
	c.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	c.keyMap.Next = key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next item"),
	)
	c.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	c.keyMap.Tab = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch selection"),
	)
	c.keyMap.ShiftTab = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "switch selection prev"),
	)
	closeKey := CloseKey
	closeKey.SetHelp("esc", "cancel")
	c.keyMap.Close = closeKey

	// Set initial commands
	c.setCommandItems(c.selected)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = com.Styles.Dialog.Spinner
	c.spinner = s

	return c, nil
}

// ID implements Dialog.
func (c *Commands) ID() string {
	return CommandsID
}

// HandleMsg implements [Dialog].
func (c *Commands) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if c.loading {
			var cmd tea.Cmd
			c.spinner, cmd = c.spinner.Update(msg)
			return ActionCmd{Cmd: cmd}
		}
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, c.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, c.keyMap.Previous):
			c.list.Focus()
			if c.list.IsSelectedFirst() {
				c.list.SelectLast()
			} else {
				c.list.SelectPrev()
			}
			c.list.ScrollToSelected()
		case key.Matches(msg, c.keyMap.Next):
			c.list.Focus()
			if c.list.IsSelectedLast() {
				c.list.SelectFirst()
			} else {
				c.list.SelectNext()
			}
			c.list.ScrollToSelected()
		case key.Matches(msg, c.keyMap.Select):
			if selectedItem := c.list.SelectedItem(); selectedItem != nil {
				if item, ok := selectedItem.(*CommandItem); ok && item != nil {
					return item.Action()
				}
			}
		case key.Matches(msg, c.keyMap.Tab):
			if len(c.customCommands) > 0 || len(c.mcpPrompts) > 0 {
				c.selected = c.nextCommandType()
				c.setCommandItems(c.selected)
			}
		case key.Matches(msg, c.keyMap.ShiftTab):
			if len(c.customCommands) > 0 || len(c.mcpPrompts) > 0 {
				c.selected = c.previousCommandType()
				c.setCommandItems(c.selected)
			}
		default:
			var cmd tea.Cmd
			for _, item := range c.list.FilteredItems() {
				if item, ok := item.(*CommandItem); ok && item != nil {
					if msg.String() == item.Shortcut() {
						return item.Action()
					}
				}
			}
			c.input, cmd = c.input.Update(msg)
			value := c.input.Value()
			c.list.SetFilter(value)
			c.list.ScrollToTop()
			c.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns the cursor position relative to the dialog.
func (c *Commands) Cursor() *tea.Cursor {
	return InputCursor(c.com.Styles, c.input.Cursor())
}

// commandsRadioView generates the command type selector radio buttons.
func commandsRadioView(sty *styles.Styles, selected CommandType, hasUserCmds bool, hasMCPPrompts bool) string {
	if !hasUserCmds && !hasMCPPrompts {
		return ""
	}

	selectedFn := func(t CommandType) string {
		if t == selected {
			return sty.RadioOn.Padding(0, 1).Render() + sty.HalfMuted.Render(t.String())
		}
		return sty.RadioOff.Padding(0, 1).Render() + sty.HalfMuted.Render(t.String())
	}

	parts := []string{
		selectedFn(SystemCommands),
	}

	if hasUserCmds {
		parts = append(parts, selectedFn(UserCommands))
	}
	if hasMCPPrompts {
		parts = append(parts, selectedFn(MCPPrompts))
	}

	return strings.Join(parts, " ")
}

// Draw implements [Dialog].
func (c *Commands) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := c.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(defaultDialogHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	if area.Dx() != c.windowWidth && c.selected == SystemCommands {
		c.windowWidth = area.Dx()
		// since some items in the list depend on width (e.g. toggle sidebar command),
		// we need to reset the command items when width changes
		c.setCommandItems(c.selected)
	}

	innerWidth := width - c.com.Styles.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	c.input.SetWidth(max(0, innerWidth-t.Dialog.InputPrompt.GetHorizontalFrameSize()-1)) // (1) cursor padding

	c.list.SetSize(innerWidth, height-heightOffset)
	c.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)
	rc.Title = "Commands"
	rc.TitleInfo = commandsRadioView(t, c.selected, len(c.customCommands) > 0, len(c.mcpPrompts) > 0)
	inputView := t.Dialog.InputPrompt.Render(c.input.View())
	rc.AddPart(inputView)
	listView := t.Dialog.List.Height(c.list.Height()).Render(c.list.Render())
	rc.AddPart(listView)
	rc.Help = c.help.View(c)

	if c.loading {
		rc.Help = c.spinner.View() + " Generating Prompt..."
	}

	view := rc.Render()

	cur := c.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (c *Commands) ShortHelp() []key.Binding {
	return []key.Binding{
		c.keyMap.Tab,
		c.keyMap.UpDown,
		c.keyMap.Select,
		c.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (c *Commands) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{c.keyMap.Select, c.keyMap.Next, c.keyMap.Previous, c.keyMap.Tab},
		{c.keyMap.Close},
	}
}

// nextCommandType returns the next command type in the cycle.
func (c *Commands) nextCommandType() CommandType {
	switch c.selected {
	case SystemCommands:
		if len(c.customCommands) > 0 {
			return UserCommands
		}
		if len(c.mcpPrompts) > 0 {
			return MCPPrompts
		}
		fallthrough
	case UserCommands:
		if len(c.mcpPrompts) > 0 {
			return MCPPrompts
		}
		fallthrough
	case MCPPrompts:
		return SystemCommands
	default:
		return SystemCommands
	}
}

// previousCommandType returns the previous command type in the cycle.
func (c *Commands) previousCommandType() CommandType {
	switch c.selected {
	case SystemCommands:
		if len(c.mcpPrompts) > 0 {
			return MCPPrompts
		}
		if len(c.customCommands) > 0 {
			return UserCommands
		}
		return SystemCommands
	case UserCommands:
		return SystemCommands
	case MCPPrompts:
		if len(c.customCommands) > 0 {
			return UserCommands
		}
		return SystemCommands
	default:
		return SystemCommands
	}
}

// setCommandItems sets the command items based on the specified command type.
func (c *Commands) setCommandItems(commandType CommandType) {
	c.selected = commandType

	commandItems := []list.FilterableItem{}
	switch c.selected {
	case SystemCommands:
		for _, cmd := range c.defaultCommands() {
			commandItems = append(commandItems, cmd)
		}
	case UserCommands:
		for _, cmd := range c.customCommands {
			action := ActionRunCustomCommand{
				Command:     cmd,
				Title:       cmd.Name,
				Description: cmd.Description,
				Content:     cmd.Content,
				Arguments:   cmd.Arguments,
			}
			commandItems = append(commandItems, newCustomCommandItem(c.com.Styles, cmd, action))
		}
	case MCPPrompts:
		for _, cmd := range c.mcpPrompts {
			action := ActionRunMCPPrompt{
				Title:       cmd.Title,
				Description: cmd.Description,
				PromptID:    cmd.PromptID,
				ClientID:    cmd.ClientID,
				Arguments:   cmd.Arguments,
			}
			commandItems = append(commandItems, newMCPCommandItem(c.com.Styles, cmd, action))
		}
	}

	c.list.SetItems(commandItems...)
	c.list.SetFilter("")
	c.list.ScrollToTop()
	c.list.SetSelected(0)
	c.input.SetValue("")
}

func (c *Commands) shortcut(action, fallback string) string {
	if c.com == nil || c.com.Config() == nil || c.com.Config().Options == nil || c.com.Config().Options.TUI == nil {
		return fallback
	}
	if value := c.com.Config().Options.TUI.Keybindings[action]; value != "" {
		if normalized, err := NormalizeKeyString(value); err == nil {
			return normalized
		}
	}
	return fallback
}

// defaultCommands returns the list of default system commands.
func (c *Commands) defaultCommands() []*CommandItem {
	commands := []*CommandItem{
		newSystemCommandItem(c.com.Styles, "new_session", "New Session", "ctrl+n", ActionNewSession{}),
		newSystemCommandItem(c.com.Styles, "switch_session", "Sessions", c.shortcut("sessions", "ctrl+s"), ActionOpenDialog{SessionsID}),
		newSystemCommandItem(c.com.Styles, "switch_model", "Switch Model", c.shortcut("models", "ctrl+l"), ActionOpenDialog{ModelsID}),
		newSystemCommandItem(c.com.Styles, "display_settings", "Display Settings", c.shortcut("display", "f2"), ActionOpenDialog{DisplaySettingsID}),
		newSystemCommandItem(c.com.Styles, "sampling_studio", "Sampling Studio", c.shortcut("sampling", "f3"), ActionOpenDialog{SamplingID}),
		newSystemCommandItem(c.com.Styles, "keybindings", "Keybindings", c.shortcut("keybindings", "f4"), ActionOpenDialog{KeybindingsID}),
	}

	// Only show compact command if there's an active session
	if c.hasSession {
		commands = append(commands, newSystemCommandItem(c.com.Styles, "summarize", "Summarize Session", "", ActionSummarize{SessionID: c.sessionID}))
	}

	// Add reasoning toggle for models that support it
	cfg := c.com.Config()
	if agentCfg, ok := cfg.Agents[config.AgentCoder]; ok {
		providerCfg := cfg.GetProviderForModel(agentCfg.Model)
		model := cfg.GetModelByType(agentCfg.Model)
		if providerCfg != nil && model != nil && model.CanReason {
			selectedModel := cfg.Models[agentCfg.Model]

			// Anthropic models: thinking toggle
			if model.CanReason && len(model.ReasoningLevels) == 0 {
				status := "Enable"
				if selectedModel.Think {
					status = "Disable"
				}
				commands = append(commands, newSystemCommandItem(c.com.Styles, "toggle_thinking", status+" Thinking Mode", "", ActionToggleThinking{}))
			}

			// OpenAI models: reasoning effort dialog
			if len(model.ReasoningLevels) > 0 {
				commands = append(commands, newSystemCommandItem(c.com.Styles, "select_reasoning_effort", "Select Reasoning Effort", "", ActionOpenDialog{
					DialogID: ReasoningID,
				}))
			}
		}
	}
	// Only show toggle compact mode command if window width is larger than compact breakpoint (120)
	if c.windowWidth >= sidebarCompactModeBreakpoint && c.hasSession {
		commands = append(commands, newSystemCommandItem(c.com.Styles, "toggle_sidebar", "Toggle Sidebar", "", ActionToggleCompactMode{}))
	}
	if c.hasSession {
		cfg := c.com.Config()
		agentCfg := cfg.Agents[config.AgentCoder]
		model := cfg.GetModelByType(agentCfg.Model)
		if model != nil && model.SupportsImages {
			commands = append(commands, newSystemCommandItem(c.com.Styles, "file_picker", "Open File Picker", "ctrl+f", ActionOpenDialog{
				// TODO: Pass in the file picker dialog id
			}))
		}
	}

	// Add external editor command if $EDITOR is available
	// TODO: Use [tea.EnvMsg] to get environment variable instead of os.Getenv
	if os.Getenv("EDITOR") != "" {
		commands = append(commands, newSystemCommandItem(c.com.Styles, "open_external_editor", "Open External Editor", "ctrl+o", ActionExternalEditor{}))
	}

	if c.hasTodos || c.hasQueue {
		var label string
		switch {
		case c.hasTodos && c.hasQueue:
			label = "Toggle To-Dos/Queue"
		case c.hasQueue:
			label = "Toggle Queue"
		default:
			label = "Toggle To-Dos"
		}
		commands = append(commands, newSystemCommandItem(c.com.Styles, "toggle_pills", label, "ctrl+t", ActionTogglePills{}))
	}

	commands = append(commands,
		newSystemCommandItem(c.com.Styles, "docs", "Browse Docs", "", ActionOpenDialog{DocsID}),
		newSystemCommandItem(c.com.Styles, "toggle_yolo", "Toggle Yolo Mode", "", ActionToggleYoloMode{}),
		newSystemCommandItem(c.com.Styles, "toggle_help", "Toggle Help", c.shortcut("help", "ctrl+g"), ActionToggleHelp{}),
		newSystemCommandItem(c.com.Styles, "init", "Initialize Project", "", ActionInitializeProject{}),
		newSystemCommandItem(c.com.Styles, "quit", "Quit", "ctrl+c", tea.QuitMsg{}),
	)

	return commands
}

func newSystemCommandItem(sty *styles.Styles, id, title, shortcut string, action Action) *CommandItem {
	filter := title
	if shortcut != "" {
		filter += " " + shortcut
	}
	return NewCommandItem(sty, id, CommandItemConfig{
		Title:      title,
		Info:       shortcut,
		Shortcut:   shortcut,
		Filter:     filter,
		MatchValue: title,
		Action:     action,
	})
}

func newCustomCommandItem(sty *styles.Styles, cmd commands.CustomCommand, action Action) *CommandItem {
	return NewCommandItem(sty, "custom_"+cmd.ID, CommandItemConfig{
		Title:      cmd.Name,
		Subtitle:   cmd.Description,
		Metadata:   customCommandMetadata(cmd),
		Info:       cmd.Scope,
		Filter:     customCommandFilter(cmd),
		MatchValue: cmd.Name,
		Action:     action,
	})
}

func newMCPCommandItem(sty *styles.Styles, cmd commands.MCPPrompt, action Action) *CommandItem {
	title := strings.TrimSpace(cmd.Title)
	if title == "" {
		title = cmd.PromptID
	}
	return NewCommandItem(sty, "mcp_"+cmd.ID, CommandItemConfig{
		Title:      title,
		Subtitle:   cmd.Description,
		Metadata:   "prompt: " + cmd.PromptID,
		Info:       cmd.ClientID,
		Filter:     strings.Join(compactStrings(title, cmd.PromptID, cmd.Description, cmd.ClientID), " "),
		MatchValue: title,
		Action:     action,
	})
}

func customCommandMetadata(cmd commands.CustomCommand) string {
	parts := compactStrings()
	if len(cmd.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(cmd.Tags, ", "))
	}
	if len(cmd.Aliases) > 0 {
		parts = append(parts, "aliases: "+strings.Join(cmd.Aliases, ", "))
	}
	if cmd.HasPipeline() {
		parts = append(parts, fmt.Sprintf("pipeline: %d steps", cmd.StepCount()))
	}
	return strings.Join(parts, " • ")
}

func customCommandFilter(cmd commands.CustomCommand) string {
	parts := compactStrings(
		cmd.Name,
		cmd.Description,
		"scope "+cmd.Scope,
		"tags "+strings.Join(cmd.Tags, " "),
		"aliases "+strings.Join(cmd.Aliases, " "),
	)
	if cmd.HasPipeline() {
		parts = append(parts, fmt.Sprintf("pipeline %d steps", cmd.StepCount()))
	}
	for _, step := range cmd.Steps {
		parts = append(parts, compactStrings(step.Title, step.Description, step.Prompt, step.Run)...)
	}
	return strings.Join(parts, " ")
}

func compactStrings(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

// SetCustomCommands sets the custom commands and refreshes the view if user commands are currently displayed.
func (c *Commands) SetCustomCommands(customCommands []commands.CustomCommand) {
	c.customCommands = customCommands
	if c.selected == UserCommands {
		c.setCommandItems(c.selected)
	}
}

// SetMCPPrompts sets the MCP prompts and refreshes the view if MCP prompts are currently displayed.
func (c *Commands) SetMCPPrompts(mcpPrompts []commands.MCPPrompt) {
	c.mcpPrompts = mcpPrompts
	if c.selected == MCPPrompts {
		c.setCommandItems(c.selected)
	}
}

// StartLoading implements [LoadingDialog].
func (a *Commands) StartLoading() tea.Cmd {
	if a.loading {
		return nil
	}
	a.loading = true
	return a.spinner.Tick
}

// StopLoading implements [LoadingDialog].
func (a *Commands) StopLoading() {
	a.loading = false
}
