package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
	"github.com/wiesnermarven-hue/salameyy/internal/charmdocs"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/common"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/list"
	"github.com/wiesnermarven-hue/salameyy/internal/ui/styles"
)

// Docs represents a dialog that shows available docs.
type Docs struct {
	com    *common.Common
	keyMap struct {
		Select,
		UpDown,
		Next,
		Previous,
		Close key.Binding
	}

	spinner spinner.Model
	loading bool

	help  help.Model
	input textinput.Model
	list  *list.FilterableList

	windowWidth int

	catalog charmdocs.Catalog
}

var _ Dialog = (*Docs)(nil)

// NewDocs creates a new docs dialog.
func NewDocs(com *common.Common) (*Docs, error) {
	d := &Docs{
		com: com,
	}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()

	d.help = help

	d.list = list.NewFilterableList()
	d.list.Focus()
	d.list.SetSelected(0)

	d.input = textinput.New()
	d.input.SetVirtualCursor(false)
	d.input.Placeholder = "Type to filter"
	d.input.SetStyles(com.Styles.TextInput)
	d.input.Focus()

	d.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "view doc"),
	)
	d.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	d.keyMap.Next = key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next item"),
	)
	d.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	closeKey := CloseKey
	closeKey.SetHelp("esc", "cancel")
	d.keyMap.Close = closeKey

	// Load docs catalog
	cwd := com.Config().WorkingDir()
	cat, err := charmdocs.Load(cwd)
	if err != nil {
		return nil, err
	}
	d.catalog = cat

	// Set initial docs items
	d.setDocsItems()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = com.Styles.Dialog.Spinner
	d.spinner = s

	return d, nil
}

// ID implements Dialog.
func (d *Docs) ID() string {
	return DocsID
}

// HandleMsg implements [Dialog].
func (d *Docs) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if d.loading {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(msg)
			return ActionCmd{Cmd: cmd}
		}
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, d.keyMap.Previous):
			d.list.Focus()
			if d.list.IsSelectedFirst() {
				d.list.SelectLast()
			} else {
				d.list.SelectPrev()
			}
			d.list.ScrollToSelected()
		case key.Matches(msg, d.keyMap.Next):
			d.list.Focus()
			if d.list.IsSelectedLast() {
				d.list.SelectFirst()
			} else {
				d.list.SelectNext()
			}
			d.list.ScrollToSelected()
		case key.Matches(msg, d.keyMap.Select):
			if selectedItem := d.list.SelectedItem(); selectedItem != nil {
				if item, ok := selectedItem.(*DocsItem); ok && item != nil {
					return item.Action()
				}
			}
		default:
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			value := d.input.Value()
			d.list.SetFilter(value)
			d.list.ScrollToTop()
			d.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns the cursor position relative to the dialog.
func (d *Docs) Cursor() *tea.Cursor {
	return InputCursor(d.com.Styles, d.input.Cursor())
}

// Draw implements [Dialog].
func (d *Docs) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(defaultDialogHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))

	innerWidth := width - d.com.Styles.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	d.input.SetWidth(max(0, innerWidth-t.Dialog.InputPrompt.GetHorizontalFrameSize()-1)) // (1) cursor padding

	d.list.SetSize(innerWidth, height-heightOffset)
	d.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)
	rc.Title = "Docs"
	inputView := t.Dialog.InputPrompt.Render(d.input.View())
	rc.AddPart(inputView)
	listView := t.Dialog.List.Height(d.list.Height()).Render(d.list.Render())
	rc.AddPart(listView)
	rc.Help = d.help.View(d)

	if d.loading {
		rc.Help = d.spinner.View() + " Loading..."
	}

	view := rc.Render()

	cur := d.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (d *Docs) ShortHelp() []key.Binding {
	return []key.Binding{
		d.keyMap.UpDown,
		d.keyMap.Select,
		d.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (d *Docs) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{d.keyMap.Select, d.keyMap.Next, d.keyMap.Previous},
		{d.keyMap.Close},
	}
}

// setDocsItems sets the docs items.
func (d *Docs) setDocsItems() {
	docsItems := []list.FilterableItem{}

	for _, slug := range d.catalog.Slugs() {
		doc, ok := d.catalog.Document(slug)
		if !ok {
			continue
		}
		action := ActionViewDoc{
			Slug: slug,
		}
		docsItems = append(docsItems, newDocsItem(d.com.Styles, doc, action))
	}

	d.list.SetItems(docsItems...)
	d.list.SetFilter("")
	d.list.ScrollToTop()
	d.list.SetSelected(0)
	d.input.SetValue("")
}

func newDocsItem(sty *styles.Styles, doc charmdocs.Document, action Action) *DocsItem {
	return &DocsItem{
		title:  doc.Title,
		path:   doc.Path,
		action: action,
		t:      sty,
	}
}

// DocsItem represents a docs item in the list.
type DocsItem struct {
	id      string
	title   string
	path    string
	action  Action
	t       *styles.Styles
	m       fuzzy.Match
	cache   map[int]string
	focused bool
}

var _ ListItem = &DocsItem{}

func (d *DocsItem) Filter() string {
	return d.title + " " + d.path
}

func (d *DocsItem) MatchSource() string {
	return d.title
}

func (d *DocsItem) ID() string {
	return d.id
}

func (d *DocsItem) SetFocused(focused bool) {
	d.focused = focused
	d.cache = nil
}

func (d *DocsItem) SetMatch(m fuzzy.Match) {
	d.m = m
	d.cache = nil
}

func (d *DocsItem) Action() Action {
	return d.action
}

func (d *DocsItem) Render(width int) string {
	if d.cache == nil {
		d.cache = make(map[int]string)
	}
	if cached, ok := d.cache[width]; ok {
		return cached
	}

	style := d.t.Dialog.NormalItem
	if d.focused {
		style = d.t.Dialog.SelectedItem
	}

	content := ansi.Truncate(d.title, width, "…")
	rendered := style.Render(content)
	d.cache[width] = rendered
	return rendered
}

// StartLoading implements [LoadingDialog].
func (d *Docs) StartLoading() tea.Cmd {
	if d.loading {
		return nil
	}
	d.loading = true
	return d.spinner.Tick
}

// StopLoading implements [LoadingDialog].
func (d *Docs) StopLoading() {
	d.loading = false
}
