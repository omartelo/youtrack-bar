package ui

import (
	"strings"
	"testing"

	"github.com/omartelo/youtrack-tui/internal/config"
)

// The badge is the whole feature on this side: the TUI reports a new release
// and names the command, it never installs anything itself.
func TestNewReleaseShowsInTheHeader(t *testing.T) {
	m := testModel(t, 50)
	m.w = 120

	if strings.Contains(m.header(), "youtrack-tui update") {
		t.Fatal("the header advertised an update before the check answered")
	}

	m.Update(updateMsg{tag: "v9.9.9"})
	head := m.header()
	if !strings.Contains(head, "v9.9.9") || !strings.Contains(head, "youtrack-tui update") {
		t.Errorf("header does not name the release or the command:\n%s", head)
	}
}

// The check runs once, when the program opens. Anything that polls would be
// spending someone's GitHub rate limit on news that changes a few times a year.
func TestUpdateCheckIsOnlyScheduledAtStartup(t *testing.T) {
	m := testModel(t, 50)
	m.Init()

	// Whatever else the model does with an answer, it must not ask again.
	if cmd := m.checkUpdateCmd(); cmd == nil {
		t.Fatal("the startup check was not scheduled")
	}
	_, cmd := m.Update(updateMsg{tag: "v9.9.9"})
	if cmd != nil {
		t.Error("the update answer scheduled more work")
	}
}

// A check nobody asked for must be able to fail without being seen: no dialog,
// no header, nothing. Same rule as a background watch poll.
func TestFailedUpdateCheckIsSilent(t *testing.T) {
	m := testModel(t, 50)
	m.w = 120

	// checkUpdate turns any failure into a nil message; feeding that back is
	// what the runtime does with it.
	m.Update(nil)
	if m.dlg != nil {
		t.Error("a failed update check raised a dialog")
	}
	if strings.Contains(m.header(), "youtrack-tui update") {
		t.Error("a failed update check reached the header")
	}
}

// `check_updates: false` has to actually stop the request.
func TestCheckUpdatesCanBeTurnedOff(t *testing.T) {
	m := testModel(t, 50)
	off := false
	m.cfg.CheckUpdates = &off

	if cmd := m.checkUpdateCmd(); cmd != nil {
		t.Error("the check ran with check_updates: false")
	}
	// Absent means on: the config is not a place to opt in from.
	m.cfg.CheckUpdates = nil
	if cmd := m.checkUpdateCmd(); cmd == nil {
		t.Error("the check did not run by default")
	}
	var nilCfg *config.Config
	if !nilCfg.ShouldCheckUpdates() {
		t.Error("a model with no config yet reported the check off")
	}
}
