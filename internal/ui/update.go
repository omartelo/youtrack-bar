// The update check: one request to GitHub at startup, and the header badge it
// feeds. It talks to a different host than everything in requests.go, and to
// no YouTrack instance at all, which is why it sits on its own.
package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/youtrack-tui/internal/update"
)

// updateMsg carries the release the check found. It is only ever sent when
// there is something newer to report: a failed check, a rate-limited one and
// an up-to-date binary are all the same non-event, and the user did not ask
// for any of them.
type updateMsg struct{ tag string }

// checkUpdate looks for a newer release once, in the background. Errors are
// dropped on purpose — a background check must not raise a modal (the same
// rule the watcher poll follows), and the header has nothing useful to say
// about a failed one.
func checkUpdate() tea.Cmd {
	return func() tea.Msg {
		rel, err := update.Check(context.Background())
		if err != nil || !rel.Newer(update.Version) {
			return nil
		}
		return updateMsg{tag: rel.Tag}
	}
}
