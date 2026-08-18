package update

import (
	"path/filepath"
	"runtime"
	"testing"
)

// A binary Homebrew installed must never be replaced in place: the manager
// would still believe it has the old version, and its next upgrade would put
// that version back.
func TestHomebrewInstallIsDelegated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("paths are POSIX")
	}
	for _, dir := range []string{
		"/opt/homebrew/Caskroom/youtrack-tui/0.2.0",
		"/usr/local/Cellar/youtrack-tui/0.2.0/bin",
	} {
		m := DetectManager(filepath.Join(dir, "youtrack-tui"))
		if !m.Managed() || m.Name != "Homebrew" {
			t.Errorf("%s: manager = %+v, want Homebrew", dir, m)
		}
		// Homebrew owns its own prefix; asking for a password here would be
		// a prompt with nothing behind it.
		if m.Elevates != "" {
			t.Errorf("%s: elevates via %q, want nothing", dir, m.Elevates)
		}
	}
}

// A binary nobody manages is the one case that may replace itself.
func TestUnmanagedInstallSelfUpdates(t *testing.T) {
	m := DetectManager(filepath.Join(t.TempDir(), "youtrack-tui"))
	if m.Managed() {
		t.Errorf("a hand-installed binary reported manager %+v", m)
	}
}

// The AUR helper runs unprivileged and elevates for the install step alone.
// Routing that through pkexec is what keeps the password in a dialog that
// says what it is authorising, instead of a bare sudo prompt under our output.
func TestAURElevationGoesThroughPolkit(t *testing.T) {
	m := Manager{Name: "AUR", Cmd: []string{"/usr/bin/paru", "-S", aurPackage, "--sudo", "/usr/bin/pkexec", "--nosudoloop"}, Elevates: "polkit"}
	want := "paru -S youtrack-tui-bin --sudo /usr/bin/pkexec --nosudoloop"
	if got := m.String(); got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
	// Never as root: paru and yay both refuse to run that way.
	for _, arg := range m.Cmd {
		if arg == "sudo" || arg == "/usr/bin/sudo" {
			t.Error("the helper is being run under sudo")
		}
	}
}

// A recognised manager with no way to run its upgrade must say so rather than
// fall through to replacing the file it does not own.
func TestManagerWithoutCommandRefuses(t *testing.T) {
	m := Manager{Name: "AUR", Hint: "no AUR helper found"}
	if !m.Managed() {
		t.Fatal("a recognised manager reported itself unmanaged")
	}
	if err := m.Run(); err == nil {
		t.Error("running a manager with no command did not error")
	}
}
