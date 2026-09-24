package model

import (
	"charm.land/bubbles/v2/key"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/dialog"
)

type KeyMap struct {
	Editor struct {
		AddFile     key.Binding
		SendMessage key.Binding
		OpenEditor  key.Binding
		Newline     key.Binding
		AddImage    key.Binding
		PasteImage  key.Binding
		MentionFile key.Binding
		Commands    key.Binding

		// Attachments key maps
		AttachmentDeleteMode key.Binding
		Escape               key.Binding
		DeleteAllAttachments key.Binding

		// History navigation
		HistoryPrev key.Binding
		HistoryNext key.Binding
	}

	Chat struct {
		NewSession     key.Binding
		AddAttachment  key.Binding
		Cancel         key.Binding
		Tab            key.Binding
		Details        key.Binding
		TogglePills    key.Binding
		PillLeft       key.Binding
		PillRight      key.Binding
		Down           key.Binding
		Up             key.Binding
		UpDown         key.Binding
		DownOneItem    key.Binding
		UpOneItem      key.Binding
		UpDownOneItem  key.Binding
		PageDown       key.Binding
		PageUp         key.Binding
		HalfPageDown   key.Binding
		HalfPageUp     key.Binding
		Home           key.Binding
		End            key.Binding
		Copy           key.Binding
		ClearHighlight key.Binding
		Expand         key.Binding
	}

	Initialize struct {
		Yes,
		No,
		Enter,
		Switch key.Binding
	}

	// Global key maps
	Quit        key.Binding
	Help        key.Binding
	Commands    key.Binding
	Models      key.Binding
	Suspend     key.Binding
	Sessions    key.Binding
	Display     key.Binding
	Sampling    key.Binding
	Keybindings key.Binding
	Tab         key.Binding
}

func DefaultKeyMap() KeyMap {
	km := KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "more"),
		),
		Commands: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "commands"),
		),
		Models: key.NewBinding(
			key.WithKeys("ctrl+m", "ctrl+l"),
			key.WithHelp("ctrl+l", "models"),
		),
		Suspend: key.NewBinding(
			key.WithKeys("ctrl+z"),
			key.WithHelp("ctrl+z", "suspend"),
		),
		Sessions: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "sessions"),
		),
		Display: key.NewBinding(
			key.WithKeys("f2", "ctrl+shift+d"),
			key.WithHelp("f2", "display"),
		),
		Sampling: key.NewBinding(
			key.WithKeys("f3", "ctrl+shift+s"),
			key.WithHelp("f3", "sampling"),
		),
		Keybindings: key.NewBinding(
			key.WithKeys("f4", "ctrl+shift+k"),
			key.WithHelp("f4", "keybindings"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "change focus"),
		),
	}

	km.Editor.AddFile = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "add file"),
	)
	km.Editor.SendMessage = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	)
	km.Editor.OpenEditor = key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "open editor"),
	)
	km.Editor.Newline = key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+j"),
		// "ctrl+j" is a common keybinding for newline in many editors. If
		// the terminal supports "shift+enter", we substitute the help tex
		// to reflect that.
		key.WithHelp("ctrl+j", "newline"),
	)
	km.Editor.AddImage = key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "add image"),
	)
	km.Editor.PasteImage = key.NewBinding(
		key.WithKeys("ctrl+v"),
		key.WithHelp("ctrl+v", "paste image from clipboard"),
	)
	km.Editor.MentionFile = key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "mention file"),
	)
	km.Editor.Commands = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "commands"),
	)
	km.Editor.AttachmentDeleteMode = key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r+{i}", "delete attachment at index i"),
	)
	km.Editor.Escape = key.NewBinding(
		key.WithKeys("esc", "alt+esc"),
		key.WithHelp("esc", "cancel delete mode"),
	)
	km.Editor.DeleteAllAttachments = key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("ctrl+r+r", "delete all attachments"),
	)
	km.Editor.HistoryPrev = key.NewBinding(
		key.WithKeys("up"),
	)
	km.Editor.HistoryNext = key.NewBinding(
		key.WithKeys("down"),
	)

	km.Chat.NewSession = key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "new session"),
	)
	km.Chat.AddAttachment = key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "add attachment"),
	)
	km.Chat.Cancel = key.NewBinding(
		key.WithKeys("esc", "alt+esc"),
		key.WithHelp("esc", "cancel"),
	)
	km.Chat.Tab = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "change focus"),
	)
	km.Chat.Details = key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "toggle details"),
	)
	km.Chat.TogglePills = key.NewBinding(
		key.WithKeys("ctrl+t", "ctrl+space"),
		key.WithHelp("ctrl+t", "toggle tasks"),
	)
	km.Chat.PillLeft = key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←/→", "switch section"),
	)
	km.Chat.PillRight = key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("←/→", "switch section"),
	)

	km.Chat.Down = key.NewBinding(
		key.WithKeys("down", "ctrl+j", "j"),
		key.WithHelp("↓", "down"),
	)
	km.Chat.Up = key.NewBinding(
		key.WithKeys("up", "ctrl+k", "k"),
		key.WithHelp("↑", "up"),
	)
	km.Chat.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑↓", "scroll"),
	)
	km.Chat.UpOneItem = key.NewBinding(
		key.WithKeys("shift+up", "K"),
		key.WithHelp("shift+↑", "up one item"),
	)
	km.Chat.DownOneItem = key.NewBinding(
		key.WithKeys("shift+down", "J"),
		key.WithHelp("shift+↓", "down one item"),
	)
	km.Chat.UpDownOneItem = key.NewBinding(
		key.WithKeys("shift+up", "shift+down"),
		key.WithHelp("shift+↑↓", "scroll one item"),
	)
	km.Chat.HalfPageDown = key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "half page down"),
	)
	km.Chat.PageDown = key.NewBinding(
		key.WithKeys("pgdown", " ", "f"),
		key.WithHelp("f/pgdn", "page down"),
	)
	km.Chat.PageUp = key.NewBinding(
		key.WithKeys("pgup", "b"),
		key.WithHelp("b/pgup", "page up"),
	)
	km.Chat.HalfPageUp = key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "half page up"),
	)
	km.Chat.Home = key.NewBinding(
		key.WithKeys("g", "home"),
		key.WithHelp("g", "home"),
	)
	km.Chat.End = key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G", "end"),
	)
	km.Chat.Copy = key.NewBinding(
		key.WithKeys("c", "y", "C", "Y"),
		key.WithHelp("c/y", "copy"),
	)
	km.Chat.ClearHighlight = key.NewBinding(
		key.WithKeys("esc", "alt+esc"),
		key.WithHelp("esc", "clear selection"),
	)
	km.Chat.Expand = key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "expand/collapse"),
	)
	km.Initialize.Yes = key.NewBinding(
		key.WithKeys("y", "Y"),
		key.WithHelp("y", "yes"),
	)
	km.Initialize.No = key.NewBinding(
		key.WithKeys("n", "N", "esc", "alt+esc"),
		key.WithHelp("n", "no"),
	)
	km.Initialize.Switch = key.NewBinding(
		key.WithKeys("left", "right", "tab"),
		key.WithHelp("tab", "switch"),
	)
	km.Initialize.Enter = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	)

	return km
}

func (km *KeyMap) ApplyBindings(bindings map[string]string) {
	for action, raw := range bindings {
		normalized, err := dialog.NormalizeKeyString(raw)
		if err != nil {
			continue
		}
		var target *key.Binding
		switch action {
		case string(dialog.GlobalActionModels):
			target = &km.Models
		case string(dialog.GlobalActionSessions):
			target = &km.Sessions
		case string(dialog.GlobalActionCommands):
			target = &km.Commands
		case string(dialog.GlobalActionHelp):
			target = &km.Help
		case string(dialog.GlobalActionDisplay):
			target = &km.Display
		case string(dialog.GlobalActionSampling):
			target = &km.Sampling
		case string(dialog.GlobalActionKeybindings):
			target = &km.Keybindings
		}
		if target != nil {
			help := target.Help()
			target.SetKeys(normalized)
			if help.Desc != "" {
				target.SetHelp(normalized, help.Desc)
			}
		}
	}
}
