// Command youtrack-bar is a read-only terminal UI for browsing YouTrack issues.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/omartelo/youtrack-bar/internal/config"
	"github.com/omartelo/youtrack-bar/internal/ui"
)

func main() {
	path := flag.String("config", config.DefaultPath(), "path to config.yml")
	provider := flag.String("provider", "", "provider name (defaults to the first one)")
	flag.Parse()

	if err := run(*path, *provider); err != nil {
		fmt.Fprintln(os.Stderr, "youtrack-bar:", err)
		os.Exit(1)
	}
}

func run(path, provider string) error {
	// A missing config file is not an error: the TUI opens on its setup
	// screen and writes the file once the credentials check out. A file that
	// exists but is broken still fails loudly — overwriting someone's config
	// without asking is worse than an error message.
	cfg, err := config.Load(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	m, err := ui.New(cfg, provider, path)
	if err != nil {
		return err
	}
	// No mouse options on purpose: with mouse tracking on, the terminal stops
	// handling Ctrl+Click on the OSC 8 links this app emits for attachments.
	_, err = tea.NewProgram(m).Run()
	return err
}
