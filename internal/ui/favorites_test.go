package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/omartelo/youtrack-bar/internal/config"
	"github.com/omartelo/youtrack-bar/internal/youtrack"
)

func filterNames(m *Model) []string {
	out := make([]string, 0, len(m.filters.Items()))
	for _, li := range m.filters.Items() {
		out = append(out, li.(filterItem).Name)
	}
	return out
}

func selectFilter(t *testing.T, m *Model, name string) {
	t.Helper()
	for i, li := range m.filters.Items() {
		if f, ok := li.(filterItem); ok && f.Name == name {
			m.filters.Select(i)
			return
		}
	}
	t.Fatalf("filter %q is not in the list: %v", name, filterNames(m))
}

func wantOrder(t *testing.T, m *Model, want ...string) {
	t.Helper()
	if got := filterNames(m); !strings.EqualFold(strings.Join(got, "|"), strings.Join(want, "|")) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestFavoritesSortToTopAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg := &config.Config{Providers: []config.Provider{{
		Name: "acme", URL: "https://acme.youtrack.cloud", Token: "perm:tok",
	}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := New(cfg, "", path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.savedQueries = []youtrack.SavedQuery{
		{Name: "My open issues", Query: "for: me #Unresolved"},
		{Name: "All unresolved", Query: "#Unresolved"},
		{Name: "Sprint 42", Query: "Sprint: {Sprint 42}"},
		{Name: "Waiting on me", Query: "State: {Waiting for review}"},
	}
	m.setFilterItems()

	selectFilter(t, m, "Sprint 42")
	m.toggleFavorite()
	if m.dlg != nil {
		t.Fatalf("unexpected dialog: %s", m.dlg.body)
	}
	wantOrder(t, m, "Sprint 42", "My open issues", "All unresolved", "Waiting on me")
	if !m.filters.Items()[0].(filterItem).fav {
		t.Error("pinned item is not marked as a favourite")
	}
	if got := m.filters.Index(); got != 0 {
		t.Errorf("cursor = %d, want it to follow the pinned item to 0", got)
	}

	// A second pin lands below the first: pin order, then API order.
	selectFilter(t, m, "Waiting on me")
	m.toggleFavorite()
	wantOrder(t, m, "Sprint 42", "Waiting on me", "My open issues", "All unresolved")

	raw := readConfig(t, path)
	if !strings.Contains(raw, "- Sprint 42") || !strings.Contains(raw, "- Waiting on me") {
		t.Errorf("favourites not persisted:\n%s", raw)
	}

	// Unpinning sends it back to where YouTrack had it.
	selectFilter(t, m, "Sprint 42")
	m.toggleFavorite()
	wantOrder(t, m, "Waiting on me", "My open issues", "All unresolved", "Sprint 42")
	if strings.Contains(readConfig(t, path), "- Sprint 42") {
		t.Error("unpinned filter is still in the config")
	}
}
