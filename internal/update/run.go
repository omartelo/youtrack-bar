package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// init fills Version in for a binary GoReleaser did not stamp. `go install
// github.com/omartelo/youtrack-tui@v0.3.0` records the module version in the
// build info, which is the only thing that knows it.
//
// Only a clean vX.Y.Z is taken. A local `go build` also lands a version here —
// a pseudo-version like v0.2.1-0.20260818201619-543769376f58+dirty, built from
// the commit it sits on — and that reads as a pre-release of a version that
// was never published, which would have the binary compare itself against a
// release it is actually ahead of.
func init() {
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if n, pre := parse(info.Main.Version); n != nil && pre == "" {
		Version = info.Main.Version
	}
}

// Run is `youtrack-tui update`: check, then either hand the work to the
// package manager that owns this binary or replace it in place.
func Run(ctx context.Context, out io.Writer) error {
	rel, err := Check(ctx)
	if err != nil {
		return err
	}
	switch {
	case !known(Version):
		// A binary built from source reports no version, so there is nothing
		// to compare — but the command was typed on purpose, so it installs
		// the release rather than refusing.
		fmt.Fprintf(out, "This build does not report a version; installing %s.\n%s\n\n", rel.Tag, rel.URL)
	case !rel.Newer(Version):
		fmt.Fprintf(out, "youtrack-tui %s is the latest release.\n", Version)
		return nil
	default:
		fmt.Fprintf(out, "youtrack-tui %s is available (you have %s).\n%s\n\n", rel.Tag, Version, rel.URL)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// A managed install is upgraded through its manager: replacing the file
	// would leave the manager holding a stale version, and its next upgrade
	// would put the old binary back.
	if mgr := DetectManager(exe); mgr.Managed() {
		if len(mgr.Cmd) == 0 {
			return fmt.Errorf("installed through %s: %s", mgr.Name, mgr.Hint)
		}
		fmt.Fprintf(out, "Installed through %s — running `%s`.\n", mgr.Name, mgr)
		switch mgr.Elevates {
		case "polkit":
			fmt.Fprintln(out, "It will ask for your password in a polkit dialog.")
		case "sudo":
			fmt.Fprintln(out, "It will ask for your sudo password below.")
		}
		return mgr.Run()
	}

	if err := Apply(ctx, rel); err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated to %s.\n", rel.Tag)
	return nil
}
