// The write half of the program: `e` on an open issue picks one field and one
// value for it. Two list screens rather than a cursor inside the detail
// viewport, which has none — reusing list.Model is what keeps this small.
package ui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/youtrack-tui/internal/youtrack"
)

// editFieldItem is one writable field, showing what it reads now. The index is
// carried so choosing a row does not have to find it again by name.
type editFieldItem struct {
	i int
	f youtrack.Editable
}

func (e editFieldItem) Title() string       { return e.f.Name }
func (e editFieldItem) Description() string { return "    " + e.f.Value }
func (e editFieldItem) FilterValue() string { return e.f.Name }

// editValueItem is one value the field's bundle offers. The gutter column is
// reserved on every row, the way the other lists do it, so marking the current
// value never shifts the names sideways.
type editValueItem struct {
	opt     youtrack.Option
	current bool
}

func (e editValueItem) Title() string {
	tick := " "
	if e.current {
		tick = styMark.Render("✓")
	}
	return tick + " " + e.opt.Label
}

func (e editValueItem) Description() string { return "" }
func (e editValueItem) FilterValue() string { return e.opt.Label }

var (
	_ list.DefaultItem = editFieldItem{}
	_ list.DefaultItem = editValueItem{}
)

// showEditFields opens the field picker on whatever the instance said is
// writable for this issue.
func (m *Model) showEditFields() tea.Cmd {
	m.edit.SetDelegate(list.NewDefaultDelegate())
	m.edit.Title = "Edit field"
	m.edit.ResetFilter()

	items := make([]list.Item, 0, len(m.editFields))
	for i, f := range m.editFields {
		items = append(items, editFieldItem{i, f})
	}
	m.screen = screenEditField
	cmd := m.edit.SetItems(items)
	m.edit.Select(0)
	return cmd
}

// showEditValues opens the value picker for the field at index i.
func (m *Model) showEditValues(i int) tea.Cmd {
	f := m.editFields[i]

	// No description line: a value is one word, and a blank second row under
	// every option is half the list saying nothing.
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	m.edit.SetDelegate(d)
	m.edit.Title = f.Name
	m.edit.ResetFilter()

	items := make([]list.Item, 0, len(f.Options))
	at := 0
	for j, o := range f.Options {
		if o.Label == f.Value {
			at = j
		}
		items = append(items, editValueItem{opt: o, current: o.Label == f.Value})
	}
	m.editField = i
	m.screen = screenEditValue
	cmd := m.edit.SetItems(items)
	// Start on what it reads now: the move being made is usually one step
	// away from it.
	m.edit.Select(at)
	return cmd
}

// chooseEditValue writes the highlighted value, unless it is the one already
// there — a POST that changes nothing still runs the instance's workflow.
func (m *Model) chooseEditValue() tea.Cmd {
	it, ok := m.edit.SelectedItem().(editValueItem)
	if !ok || m.current == nil {
		return nil
	}
	f := m.editFields[m.editField]
	if it.opt.Label == f.Value {
		m.screen = screenDetail
		return nil
	}
	return m.applyEdit(m.current.ID, f, it.opt.Value)
}

// editing reports whether one of the two picker screens is up.
func (m *Model) editing() bool {
	return m.screen == screenEditField || m.screen == screenEditValue
}
