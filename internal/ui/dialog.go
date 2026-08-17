package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/omartelo/youtrack-bar/internal/youtrack"
)

// dialog is the modal shown over the current screen. A nil *dialog means none
// is up. It owns the keyboard while it is open.
type dialog struct {
	title string
	body  string
	hint  string
	// offerTrust marks the one dialog with an action attached: a certificate
	// the machine does not trust, which the user can choose to accept.
	offerTrust bool
}

func infoDialog(title, body string) *dialog {
	return &dialog{title: title, body: body, hint: "esc  dismiss"}
}

func errorDialog(err error) *dialog {
	if youtrack.IsCertError(err) {
		return &dialog{
			title: "Certificate not trusted",
			body: err.Error() + "\n\n" +
				"No CA on this machine signs this certificate — normal for an internal instance.\n\n" +
				"Clean fix: add `ca_file: /path/to/ca.pem` to this provider in the config.\n\n" +
				"Skipping verification instead sends your token to whoever answers on that " +
				"address. It applies to this provider only and stays marked in the header.",
			hint:       "i  skip verification and retry  ·  esc  dismiss",
			offerTrust: true,
		}
	}
	return &dialog{title: "Request failed", body: err.Error(), hint: "esc  dismiss"}
}

func (d *dialog) view(termWidth int) string {
	// Each block is wrapped and padded to the same width before the box wraps
	// it: letting the border style re-wrap already-rendered text shreds it.
	w := max(24, min(68, termWidth-10))
	blank := styDialogBody.Width(w).Render("")
	return styDialogBox.Render(strings.Join([]string{
		styDialogTitle.Width(w).Render(d.title),
		blank,
		styDialogBody.Width(w).Render(d.body),
		blank,
		styDialogHint.Width(w).Render(d.hint),
	}, "\n"))
}

// overlay draws the dialog centred on top of the rendered screen, so the
// layout underneath is preserved rather than displaced.
//
// It must go through a Compositor: Canvas.Compose calls Layer.Draw, which
// paints that layer's own content and ignores its children entirely. Only the
// compositor flattens the tree and honours X/Y/Z.
func overlay(base, box string, w, h int) string {
	x := max(0, (w-lipgloss.Width(box))/2)
	y := max(0, (h-lipgloss.Height(box))/2)
	return lipgloss.NewCanvas(w, h).Compose(
		lipgloss.NewCompositor(
			lipgloss.NewLayer(base),
			lipgloss.NewLayer(box).X(x).Y(y).Z(1),
		),
	).Render()
}
