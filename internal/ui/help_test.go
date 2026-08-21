package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
)

func helpModel(s screen) *Model {
	m := &Model{keys: defaultKeys(), help: help.New(), screen: s}
	m.w, m.h = 100, 30
	return m
}

// The footer is a glance, not a manual: a handful of keys, and `?` for the
// rest. Anything dropped from it has to still be reachable in the popup, or
// trimming the footer is just hiding features.
func TestFooterIsShortAndThePopupIsNot(t *testing.T) {
	m := helpModel(screenDetail)

	foot := m.footer()
	if n := strings.Count(foot, "•") + 1; n > 5 {
		t.Errorf("footer shows %d keys, want at most 5: %s", n, foot)
	}
	for _, hidden := range []string{"copy URL", "mark/unmark", "jump to comments"} {
		if strings.Contains(foot, hidden) {
			t.Errorf("footer still carries %q: %s", hidden, foot)
		}
		if body := helpDialog(m.help, screenKeys{m.keys, m.screen}).body; !strings.Contains(body, hidden) {
			t.Errorf("popup does not list %q", hidden)
		}
	}
}

// `?` opens the modal and `?` closes it again, the way it used to untoggle the
// tall footer.
func TestHelpKeyTogglesThePopup(t *testing.T) {
	m := helpModel(screenIssues)

	press(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.dlg == nil || m.dlg.title != "Keys" {
		t.Fatalf("dialog = %+v, want the help popup", m.dlg)
	}

	press(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.dlg != nil {
		t.Fatalf("dialog = %+v, want it dismissed", m.dlg)
	}
}

// A screen must not advertise a key that does nothing on it: `f` and `w` are
// filters-only, `o`, `y` and `x` need an issue.
func TestHelpOnlyListsWhatTheScreenAnswers(t *testing.T) {
	for _, tc := range []struct {
		s      screen
		absent []string
	}{
		{screenFilters, []string{"copy URL", "mark/unmark", "open in YouTrack"}},
		{screenIssues, []string{"pin/unpin", "watch/unwatch"}},
		{screenDetail, []string{"pin/unpin", "watch/unwatch", "sort order"}},
	} {
		body := helpDialog(help.New(), screenKeys{defaultKeys(), tc.s}).body
		for _, k := range tc.absent {
			if strings.Contains(body, k) {
				t.Errorf("screen %d help offers %q, which does nothing there", tc.s, k)
			}
		}
	}
}
