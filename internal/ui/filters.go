// The filters list: the saved searches YouTrack returned, the two built-ins
// that are always offered, the synthetic Marked one, and the pin `f` leaves on
// a row. Everything here matches by ID, so a rename in YouTrack keeps the pin.
package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/youtrack-tui/internal/youtrack"
)

// builtinFilters are always offered alongside the user's saved searches. Their
// IDs are synthetic and prefixed so they can never collide with a YouTrack
// entity ID, and they are what `favorites` records.
var builtinFilters = []youtrack.SavedQuery{
	{ID: "builtin:my-open-issues", Name: "My open issues", Query: "for: me #Unresolved"},
	{ID: "builtin:all-unresolved", Name: "All unresolved", Query: "#Unresolved"},
}

// markedFilterID is the filter that lists what `x` has ticked. It only exists
// while there is something to list, which is why it is not in builtinFilters.
const markedFilterID = "builtin:marked"

// setFilterItems rebuilds the filters list, favourites first. Both lists are
// sorted stably: favourites keep the order they were pinned in, everything
// else keeps the order YouTrack returned.
func (m *Model) setFilterItems() tea.Cmd {
	fav := m.cfg.Providers[m.provider].Favorites
	rank := func(id string) int {
		if i := slices.Index(fav, id); i >= 0 {
			return i
		}
		return len(fav)
	}

	sorted := slices.Clone(m.savedQueries)
	slices.SortStableFunc(sorted, func(a, b youtrack.SavedQuery) int {
		return rank(a.ID) - rank(b.ID)
	})

	items := make([]list.Item, 0, len(sorted))
	for _, q := range sorted {
		items = append(items, filterItem{
			SavedQuery: q,
			fav:        rank(q.ID) < len(fav),
			watched:    m.watch.watching[q.ID],
		})
	}
	return m.filters.SetItems(items)
}

// migrateFavorites rewrites favourites recorded as names — the original format
// — into IDs, so that renaming a saved search in YouTrack stops losing its pin.
// Memory only: the next `f` persists the result, and an entry that matches
// nothing is left alone rather than dropped.
func (m *Model) migrateFavorites() {
	p := &m.cfg.Providers[m.provider]
	ids := make(map[string]bool, len(m.savedQueries))
	byName := make(map[string]string, len(m.savedQueries))
	for _, q := range m.savedQueries {
		ids[q.ID] = true
		byName[q.Name] = q.ID
	}
	for i, f := range p.Favorites {
		if ids[f] {
			continue
		}
		if id, ok := byName[f]; ok {
			p.Favorites[i] = id
		}
	}
}

// refreshMarkedFilter keeps the synthetic "Marked" filter in step with what is
// ticked, and drops it once nothing is. Without a way back to them, marks
// accumulate in a config nobody can review: the filters they were found under
// change, and an ID alone says nothing about where it came from.
//
// ponytail: the whole list travels as one `issue id:` query, so a few hundred
// marks make a URL long enough for a server to refuse. Upgrade: page the IDs,
// or keep only the newest N.
func (m *Model) refreshMarkedFilter() tea.Cmd {
	m.savedQueries = slices.DeleteFunc(m.savedQueries, func(q youtrack.SavedQuery) bool {
		return q.ID == markedFilterID
	})
	if ids := m.cfg.Providers[m.provider].Marked; len(ids) > 0 {
		at := min(len(builtinFilters), len(m.savedQueries))
		m.savedQueries = slices.Insert(m.savedQueries, at, youtrack.SavedQuery{
			ID:    markedFilterID,
			Name:  "Marked",
			Query: "issue id: " + strings.Join(ids, ", "),
		})
	}
	return m.setFilterItems()
}

// toggleFavorite pins or unpins the selected filter and persists it. Matching
// is by ID, so renaming a saved search in YouTrack keeps its pin and two
// searches sharing a name stay independent.
func (m *Model) toggleFavorite() tea.Cmd {
	it, ok := m.filters.SelectedItem().(filterItem)
	if !ok {
		return nil
	}
	p := &m.cfg.Providers[m.provider]
	if i := slices.Index(p.Favorites, it.ID); i >= 0 {
		p.Favorites = slices.Delete(p.Favorites, i, i+1)
	} else {
		p.Favorites = append(p.Favorites, it.ID)
	}
	if err := m.saveConfig(); err != nil {
		m.dlg = infoDialog("Config not saved", err.Error())
	}

	cmd := m.setFilterItems()
	// Follow the item to its new position instead of resetting to the top:
	// pinning moves it, the cursor should go with it.
	//
	// Only while unfiltered. Items() is the unfiltered order but Select indexes
	// the visible one, so following a filtered list lands the cursor on some
	// other row — and the next `f` then pins the wrong filter. With a filter
	// applied the visible order is the fuzzy ranking, which pinning does not
	// touch, so the cursor is already where it belongs.
	if !m.filters.IsFiltered() {
		for i, li := range m.filters.Items() {
			if f, ok := li.(filterItem); ok && f.ID == it.ID {
				m.filters.Select(i)
				break
			}
		}
	}
	return cmd
}
