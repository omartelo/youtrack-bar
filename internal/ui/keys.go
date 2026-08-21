package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Open     key.Binding
	Back     key.Binding
	Reload   key.Binding
	Provider key.Binding
	Filter   key.Binding
	Search   key.Binding
	Sort     key.Binding
	Favorite key.Binding
	Watch    key.Binding
	Mark     key.Binding
	Edit     key.Binding
	Browser  key.Binding
	Copy     key.Binding
	More     key.Binding
	Scroll   key.Binding

	// Detail screen only. CommentOrder shares its key with Sort — same handler,
	// but on an open issue `S` flips the comments, which "sort order" does not
	// describe.
	CommentOrder key.Binding
	Comments     key.Binding
	Top          key.Binding
	Bottom       key.Binding
	Help         key.Binding
	Quit         key.Binding

	// Setup screen only.
	Field  key.Binding
	Reveal key.Binding
	Save   key.Binding
	Abort  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Field:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		Reveal:   key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reveal token")),
		Save:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "verify and save")),
		Abort:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		Open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Reload:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Provider: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "next provider")),
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search list")),
		Search:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "query YouTrack")),
		Sort:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "sort order")),
		Favorite: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "pin/unpin")),
		Watch:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch/unwatch")),
		// What the mark means is the user's business — reviewed, read, come
		// back later — so the help says what it does, not what it is for.
		Mark: key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "mark/unmark")),
		// The one key that writes. It says "field" rather than "issue" because
		// that is the whole of it: no comment, no summary, no description.
		Edit:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit a field")),
		Browser: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in YouTrack")),
		// OSC 52 rather than a clipboard tool: it goes through the terminal,
		// which is the one thing that still works over SSH.
		Copy:         key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy URL")),
		CommentOrder: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "comment order")),
		Comments:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "jump to comments")),
		Top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		// Disabled until a full page comes back, which also hides it from help.
		More:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "load more"), key.WithDisabled()),
		Scroll: key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
		Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// screenKeys adapts keyMap to help.KeyMap, showing only what the current
// screen actually does.
type screenKeys struct {
	k keyMap
	s screen
}

func (s screenKeys) ShortHelp() []key.Binding {
	// Four or five keys, no more: the way in, the way out, the one thing this
	// screen is for, and `?` for everything else. A footer nobody can read at
	// a glance is a footer nobody reads.
	switch s.s {
	case screenSetup:
		return []key.Binding{s.k.Field, s.k.Save, s.k.Reveal, s.k.Abort}
	case screenEditField, screenEditValue:
		return []key.Binding{s.k.Open, s.k.Back, s.k.Help}
	case screenDetail:
		return []key.Binding{s.k.Scroll, s.k.Edit, s.k.Back, s.k.Help, s.k.Quit}
	case screenIssues:
		// More disables itself until a full page comes back, so it only takes
		// a slot here while there is another page to take.
		return []key.Binding{s.k.Open, s.k.More, s.k.Back, s.k.Help, s.k.Quit}
	default:
		return []key.Binding{s.k.Open, s.k.Filter, s.k.Help, s.k.Quit}
	}
}

func (s screenKeys) FullHelp() [][]key.Binding {
	// The popup is the reference now, so a screen only lists what it actually
	// answers to: `f` and `w` do nothing on the issue list, `o`, `y` and `x`
	// do nothing on the filters.
	switch s.s {
	case screenSetup:
		return [][]key.Binding{{s.k.Field, s.k.Save}, {s.k.Reveal, s.k.Abort}}
	case screenEditField, screenEditValue:
		return [][]key.Binding{{s.k.Open, s.k.Scroll}, {s.k.Filter, s.k.Back, s.k.Quit}}
	case screenDetail:
		return [][]key.Binding{
			{s.k.Scroll, s.k.Top, s.k.Bottom, s.k.Comments},
			{s.k.Browser, s.k.Copy, s.k.Mark, s.k.Edit, s.k.CommentOrder, s.k.Back, s.k.Reload},
			{s.k.Help, s.k.Quit},
		}
	case screenIssues:
		return [][]key.Binding{
			{s.k.Open, s.k.Scroll, s.k.Filter, s.k.More},
			{s.k.Browser, s.k.Copy, s.k.Mark, s.k.Search, s.k.Sort},
			{s.k.Back, s.k.Reload, s.k.Provider, s.k.Help, s.k.Quit},
		}
	default:
		return [][]key.Binding{
			{s.k.Open, s.k.Scroll, s.k.Filter},
			{s.k.Search, s.k.Sort, s.k.Favorite, s.k.Watch},
			{s.k.Reload, s.k.Provider, s.k.Help, s.k.Quit},
		}
	}
}
