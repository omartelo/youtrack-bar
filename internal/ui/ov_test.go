package ui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// A ragged box means text was wrapped twice or a line was not padded, which
// also leaves the background torn on those rows.
func TestDialogBoxIsRectangular(t *testing.T) {
	for _, d := range []*dialog{
		errorDialog(errors.New("short")),
		errorDialog(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")),
		infoDialog("Config not saved", "permission denied"),
	} {
		lines := strings.Split(d.view(100), "\n")
		want := lipgloss.Width(lines[0])
		for i, l := range lines {
			if got := lipgloss.Width(l); got != want {
				t.Errorf("%q line %d width = %d, want %d", d.title, i, got, want)
			}
		}
	}
}

// The dialog has to draw over the screen without displacing it. Canvas.Compose
// on a parent layer silently ignores child layers, which renders either the
// base alone or the box alone — both look like "the dialog is broken".
func TestOverlayDrawsOverBase(t *testing.T) {
	base := strings.TrimRight(strings.Repeat("abcdefghij\n", 5), "\n")
	out := overlay(base, "XXXX\nXXXX", 10, 5)

	want := []string{
		"abcdefghij",
		"abcXXXXhij",
		"abcXXXXhij",
		"abcdefghij",
		"abcdefghij",
	}
	got := strings.Split(out, "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}
