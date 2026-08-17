package ui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// openedMsg reports the outcome of handing a URL to the desktop.
type openedMsg struct {
	url string
	err error
}

// openInBrowser runs the platform's URL handler. Spawning a process is I/O, so
// it lives in a tea.Cmd like every other side effect.
func openInBrowser(link string) tea.Cmd {
	return func() tea.Msg {
		return openedMsg{url: link, err: launch(link)}
	}
}

func launch(link string) error {
	// The URL is built from a validated provider base, but the handler is a
	// desktop-wide dispatcher: keep it to the two schemes we ever produce
	// rather than letting anything the API returns reach it.
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("refusing to open %q: not an http(s) URL", link)
	}

	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		name = "xdg-open"
	}
	// No shell involved, so nothing in the URL can be interpreted as a command.
	return exec.Command(name, append(args, link)...).Start()
}
