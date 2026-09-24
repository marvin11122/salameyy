package dialog

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

// CommandItemConfig controls how a command item is rendered and filtered.
type CommandItemConfig struct {
	Title      string
	Subtitle   string
	Metadata   string
	Info       string
	Shortcut   string
	Filter     string
	MatchValue string
	Action     Action
}

// CommandItem wraps a uicmd.Command to implement the ListItem interface.
type CommandItem struct {
	id         string
	title      string
	subtitle   string
	metadata   string
	info       string
	shortcut   string
	filter     string
	matchValue string
	action     Action
	t          *styles.Styles
	m          fuzzy.Match
	cache      map[int]string
	focused    bool
}

var _ ListItem = &CommandItem{}

// NewCommandItem creates a new CommandItem.
func NewCommandItem(t *styles.Styles, id string, cfg CommandItemConfig) *CommandItem {
	filter := strings.TrimSpace(cfg.Filter)
	if filter == "" {
		filter = cfg.Title
	}
	matchValue := strings.TrimSpace(cfg.MatchValue)
	if matchValue == "" {
		matchValue = cfg.Title
	}
	return &CommandItem{
		id:         id,
		t:          t,
		title:      cfg.Title,
		subtitle:   cfg.Subtitle,
		metadata:   cfg.Metadata,
		info:       cfg.Info,
		shortcut:   cfg.Shortcut,
		filter:     filter,
		matchValue: matchValue,
		action:     cfg.Action,
	}
}

// Filter implements ListItem.
func (c *CommandItem) Filter() string {
	return c.filter
}

// MatchSource returns the field that fuzzy matches should highlight.
func (c *CommandItem) MatchSource() string {
	return c.matchValue
}

// ID implements ListItem.
func (c *CommandItem) ID() string {
	return c.id
}

// SetFocused implements ListItem.
func (c *CommandItem) SetFocused(focused bool) {
	if c.focused != focused {
		c.cache = nil
	}
	c.focused = focused
}

// SetMatch implements ListItem.
func (c *CommandItem) SetMatch(m fuzzy.Match) {
	c.cache = nil
	c.m = m
}

// Action returns the action associated with the command item.
func (c *CommandItem) Action() Action {
	return c.action
}

// Shortcut returns the shortcut associated with the command item.
func (c *CommandItem) Shortcut() string {
	return c.shortcut
}

// Render implements ListItem.
func (c *CommandItem) Render(width int) string {
	if c.cache == nil {
		c.cache = make(map[int]string)
	}
	if cached, ok := c.cache[width]; ok {
		return cached
	}

	styles := ListItemStyles{
		ItemBlurred:     c.t.Dialog.NormalItem,
		ItemFocused:     c.t.Dialog.SelectedItem,
		InfoTextBlurred: c.t.Base,
		InfoTextFocused: c.t.Base,
	}
	style := styles.ItemBlurred
	metaStyle := c.t.HalfMuted
	if c.focused {
		style = styles.ItemFocused
		metaStyle = c.t.Base
	}

	lines := []string{
		renderCommandLine(styles, c.title, c.info, c.focused, width, &c.m),
	}

	details := joinCommandDetails(c.subtitle, c.metadata)
	if details != "" {
		lines = append(lines, style.Render(metaStyle.Render(ansi.Truncate(details, max(0, width), "…"))))
	}

	rendered := strings.Join(lines, "\n")
	c.cache[width] = rendered
	return rendered
}

func renderCommandLine(itemStyles ListItemStyles, title, info string, focused bool, width int, m *fuzzy.Match) string {
	style := itemStyles.ItemBlurred
	infoStyle := itemStyles.InfoTextBlurred
	if focused {
		style = itemStyles.ItemFocused
		infoStyle = itemStyles.InfoTextFocused
	}

	var infoText string
	var infoWidth int
	if info != "" {
		infoText = infoStyle.Render(" " + info + " ")
		infoWidth = ansi.StringWidth(infoText)
	}

	title = ansi.Truncate(title, max(0, width-infoWidth), "…")
	content := title
	if m != nil && len(m.MatchedIndexes) > 0 {
		var lastPos int
		parts := make([]string, 0)
		for _, rng := range matchedRanges(m.MatchedIndexes) {
			start, stop := bytePosToVisibleCharPos(title, rng)
			if start > lastPos {
				parts = append(parts, ansi.Cut(title, lastPos, start))
			}
			parts = append(parts,
				ansi.NewStyle().Underline(true).String(),
				ansi.Cut(title, start, stop+1),
				ansi.NewStyle().Underline(false).String(),
			)
			lastPos = stop + 1
		}
		if lastPos < ansi.StringWidth(title) {
			parts = append(parts, ansi.Cut(title, lastPos, ansi.StringWidth(title)))
		}
		content = strings.Join(parts, "")
	}

	gap := strings.Repeat(" ", max(0, width-ansi.StringWidth(title)-infoWidth))
	return style.Render(content + gap + infoText)
}

func joinCommandDetails(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "  ")
}
