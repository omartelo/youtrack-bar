package ui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/youtrack-bar/internal/config"
	"github.com/omartelo/youtrack-bar/internal/youtrack"
)

// notifyLines is how many issues a notification names before it starts
// counting. A desktop popup is not a list view.
const notifyLines = 3

// watcher tracks what each watched filter has already reported. It is memory
// only by design: a restart re-seeds and announces nothing, which beats being
// greeted by fifty notifications for issues opened while you were away.
type watcher struct {
	// watching is the set of saved-search IDs polled for the active provider,
	// seeded from the config and toggled with `w`. Never written back.
	watching map[string]bool
	// seen[filterID] is the issue IDs already accounted for. A filter absent
	// from the map has not been polled yet, which is what makes the first poll
	// silent.
	seen map[string]map[string]bool
	// fresh is the issue IDs to mark in the list until they are opened.
	fresh map[string]bool
	// failed is true when the last poll errored, so the header can say so
	// without a modal interrupting whatever is on screen.
	failed bool
}

func newWatcher(seed []string) watcher {
	w := watcher{
		watching: make(map[string]bool, len(seed)),
		seen:     map[string]map[string]bool{},
		fresh:    map[string]bool{},
	}
	for _, id := range seed {
		w.watching[id] = true
	}
	return w
}

// record folds a poll result in and returns the issues never seen before.
// The first poll for a filter only seeds, so nothing is announced.
func (w *watcher) record(filterID string, issues []youtrack.Issue) []youtrack.Issue {
	seen, known := w.seen[filterID]
	if !known {
		seen = make(map[string]bool, len(issues))
		w.seen[filterID] = seen
	}

	var out []youtrack.Issue
	for _, iss := range issues {
		if seen[iss.ID] {
			continue
		}
		seen[iss.ID] = true
		out = append(out, iss)
	}
	if !known {
		return nil
	}
	for _, iss := range out {
		w.fresh[iss.ID] = true
	}
	return out
}

// --- messages -------------------------------------------------------------

type watchTickMsg struct{ gen int }

type watchResultMsg struct {
	gen      int
	filterID string
	label    string
	issues   []youtrack.Issue
	err      error
}

type notifiedMsg struct{ err error }

// --- the notification itself ----------------------------------------------

// notifyNew puts one notification on screen per filter that gained issues.
func notifyNew(notifier, filter string, issues []youtrack.Issue) tea.Cmd {
	if len(issues) == 0 {
		return nil
	}
	title, body := fmt.Sprintf("youtrack-bar · %s", filter), notifyBody(issues)
	return func() tea.Msg {
		return notifiedMsg{err: notify(notifier, title, body)}
	}
}

// notifyBody names the first few issues and counts the rest.
func notifyBody(issues []youtrack.Issue) string {
	var b strings.Builder
	for i, iss := range issues {
		if i == notifyLines {
			fmt.Fprintf(&b, "\n…and %d more", len(issues)-notifyLines)
			break
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s  %s", iss.ID, iss.Summary)
	}
	return b.String()
}

// notify shells out to the desktop's notification command. The right one
// differs per machine, which is what earns it a config knob.
func notify(notifier, title, body string) error {
	name, args, err := notifyCommand(notifier, title, body)
	if err != nil || name == "" {
		return err
	}
	// No shell, so nothing in an issue summary can be interpreted as a command.
	return exec.Command(name, args...).Run()
}

func notifyCommand(notifier, title, body string) (string, []string, error) {
	switch notifier {
	case "", config.NotifierZenity:
		return "zenity", []string{"--notification", "--text=" + title + "\n" + body}, nil
	case config.NotifierNotifySend:
		return "notify-send", []string{"--app-name=youtrack-bar", title, body}, nil
	case config.NotifierNone:
		return "", nil, nil
	}
	return "", nil, fmt.Errorf("unknown notifier %q", notifier)
}
