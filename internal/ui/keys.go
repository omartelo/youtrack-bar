package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Open     key.Binding
	Back     key.Binding
	Reload   key.Binding
	Provider key.Binding
	Filter   key.Binding
	Search   key.Binding
	Favorite key.Binding
	Watch    key.Binding
	Browser  key.Binding
	More     key.Binding
	Scroll   key.Binding
	Help     key.Binding
	Quit     key.Binding

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
		Favorite: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "pin/unpin")),
		Watch:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch/unwatch")),
		Browser:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in YouTrack")),
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
	switch s.s {
	case screenSetup:
		return []key.Binding{s.k.Field, s.k.Save, s.k.Reveal, s.k.Abort}
	case screenDetail:
		return []key.Binding{s.k.Scroll, s.k.Browser, s.k.Back, s.k.Reload, s.k.Help, s.k.Quit}
	case screenIssues:
		return []key.Binding{s.k.Open, s.k.Browser, s.k.More, s.k.Search, s.k.Filter, s.k.Back, s.k.Reload, s.k.Help, s.k.Quit}
	default:
		return []key.Binding{s.k.Open, s.k.Favorite, s.k.Watch, s.k.Search, s.k.Filter, s.k.Reload, s.k.Help, s.k.Quit}
	}
}

func (s screenKeys) FullHelp() [][]key.Binding {
	if s.s == screenSetup {
		return [][]key.Binding{{s.k.Field, s.k.Save}, {s.k.Reveal, s.k.Abort}}
	}
	return [][]key.Binding{
		{s.k.Open, s.k.Browser, s.k.Back, s.k.Scroll},
		{s.k.Search, s.k.Filter, s.k.Favorite, s.k.Watch, s.k.More, s.k.Reload, s.k.Provider},
		{s.k.Help, s.k.Quit},
	}
}
