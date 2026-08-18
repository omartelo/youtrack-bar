package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Manager is the package manager that owns this binary, when there is one.
// Replacing a file Homebrew or pacman installed would leave the manager
// convinced it still has the old version and would be undone by the next
// upgrade, so those installs are updated by running the manager instead.
type Manager struct {
	// Name is what to call it on screen: "Homebrew", "AUR", or empty for a
	// binary nobody manages.
	Name string
	// Cmd is the upgrade command, ready to run. Empty when the manager was
	// recognised but its command is not available — an AUR package on a
	// machine with no AUR helper, say — in which case Hint says what to do.
	Cmd []string
	// Hint is the instruction to print when Cmd is empty.
	Hint string
	// Elevates names what will ask for a password, when something will:
	// "polkit" for a pkexec dialog, "sudo" for a terminal prompt, empty when
	// the upgrade needs no privileges at all. Printed before the command runs
	// — a password prompt nobody announced looks like a phishing attempt.
	Elevates string
}

// Managed reports whether the binary belongs to a package manager, in which
// case self-replacing it is the wrong move.
func (m Manager) Managed() bool { return m.Name != "" }

// aurPackage is the name GoReleaser publishes to the AUR.
const aurPackage = repo + "-bin"

// DetectManager works out how this binary was installed, from where it lives.
//
// ponytail: path and pacman-ownership only — enough for the two managers we
// publish to. Scoop, winget, nix and a distro package built by someone else
// all read as unmanaged and get the self-update path. Upgrade: ask each
// manager whether it owns the path, which costs a process spawn per manager.
func DetectManager(exe string) Manager {
	// A managed install is usually reached through a symlink in a bin
	// directory; the real file is what identifies the owner.
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}

	if strings.Contains(exe, string(filepath.Separator)+"Cellar"+string(filepath.Separator)) ||
		strings.Contains(exe, string(filepath.Separator)+"Caskroom"+string(filepath.Separator)) {
		// Homebrew owns its own prefix, so nothing here is elevated.
		m := Manager{Name: "Homebrew"}
		if brew, err := exec.LookPath("brew"); err == nil {
			// The cask, not a formula: the Linux formula is gone, and on macOS
			// this is what the tap publishes.
			m.Cmd = []string{brew, "upgrade", "--cask", owner + "/tap/" + repo}
		} else {
			m.Hint = "brew upgrade --cask " + owner + "/tap/" + repo
		}
		return m
	}

	if pkg := pacmanOwner(exe); pkg != "" {
		m := Manager{Name: "AUR"}
		// pacman cannot reach the AUR, so an AUR install is upgraded by
		// whichever helper the user already has. The helper runs unprivileged
		// — paru and yay both refuse to run as root — and elevates on its own
		// for the install step.
		for _, helper := range []string{"paru", "yay"} {
			bin, err := exec.LookPath(helper)
			if err != nil {
				continue
			}
			m.Cmd = []string{bin, "-S", pkg}
			// Point the helper at polkit instead of sudo, so the password is
			// asked for in a dialog that says what is being authorised rather
			// than as a bare prompt underneath our own output. --nosudoloop
			// goes with it: the background `sudo -v` refresh has no pkexec
			// equivalent.
			if pkexec, err := exec.LookPath("pkexec"); err == nil {
				m.Cmd = append(m.Cmd, "--sudo", pkexec, "--nosudoloop")
				m.Elevates = "polkit"
			} else {
				m.Elevates = "sudo"
			}
			return m
		}
		m.Hint = "no AUR helper found; update with `paru -S " + pkg + "` or rebuild the package by hand"
		return m
	}

	return Manager{}
}

// pacmanOwner returns the package owning path, or empty when pacman is absent,
// or the file belongs to no package.
func pacmanOwner(path string) string {
	if runtime.GOOS != "linux" {
		return ""
	}
	pacman, err := exec.LookPath("pacman")
	if err != nil {
		return ""
	}
	// -Qoq prints the package name alone; it exits non-zero when nothing owns
	// the file, which is the common case for a hand-installed binary.
	out, err := exec.Command(pacman, "-Qoq", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Run executes the manager's upgrade command with the terminal attached, so
// its password prompt and progress reach the user.
func (m Manager) Run() error {
	if len(m.Cmd) == 0 {
		return fmt.Errorf("%s", m.Hint)
	}
	cmd := exec.Command(m.Cmd[0], m.Cmd[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// String is the command as it would be typed, for printing before it runs.
func (m Manager) String() string {
	if len(m.Cmd) == 0 {
		return m.Hint
	}
	// The looked-up path is noise on screen; the command name is what the
	// user would type.
	parts := append([]string{filepath.Base(m.Cmd[0])}, m.Cmd[1:]...)
	return strings.Join(parts, " ")
}
