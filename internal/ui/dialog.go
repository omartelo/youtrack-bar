package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/youtrack-tui/internal/youtrack"
)

// dialog is the modal shown over the current screen. A nil *dialog means none
// is up. It owns the keyboard while it is open.
type dialog struct {
	title string
	body  string
	hint  string
	// wide lifts the prose width cap. The help popup is a table of key and
	// description columns, and a table re-wrapped to 68 columns stops being
	// one; a paragraph of error text at terminal width stops being readable.
	wide bool
	// offerTrust marks the one dialog with an action attached: a certificate
	// the machine does not trust, which the user can choose to accept.
	offerTrust bool
}

func infoDialog(title, body string) *dialog {
	return &dialog{title: title, body: body, hint: "esc  dismiss"}
}

// helpDialog is what `?` opens: every binding the current screen answers to,
// in the modal rather than crammed into the footer. The footer names the four
// keys worth having on screen permanently; this is the rest of them.
func helpDialog(h help.Model, k help.KeyMap) *dialog {
	return &dialog{
		title: "Keys",
		body:  h.FullHelpView(k.FullHelp()),
		hint:  "?  or  esc   close",
		wide:  true,
	}
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
	inner := max(24, min(68, termWidth-10))
	if d.wide {
		// Wide enough for the columns as rendered, never wider than the term.
		inner = max(24, min(lipgloss.Width(d.body), termWidth-10))
	}
	body := styDialogBody.Width(inner).Render(d.body)
	return titledBox(d.title, body+"\n\n"+styDialogHint.Render(d.hint), inner)
}

// titledBox draws a rounded border with the title cut into the top edge, the
// way lazydocker does it — ported from omartelo/lazyovpn.
//
// Nothing is painted behind the content: each line is padded to innerW, which
// is what covers the screen underneath. A background style would have to be
// re-asserted on every line, because any styled run nested inside it emits an
// SGR reset that clears the background for the rest of that row.
func titledBox(title, content string, innerW int) string {
	span := innerW + 2 // between the corners: one space of padding each side

	label := " " + title + " "
	fill := span - 1 - lipgloss.Width(label) // -1 for the leading dash
	top := styDialogBorder.Render("╭" + strings.Repeat("─", span) + "╮")
	if fill >= 0 {
		top = styDialogBorder.Render("╭─") + styDialogTitle.Render(label) +
			styDialogBorder.Render(strings.Repeat("─", fill)+"╮")
	}

	side := styDialogBorder.Render("│")
	cell := lipgloss.NewStyle().Width(innerW)

	var b strings.Builder
	b.WriteString(top + "\n")
	for _, ln := range strings.Split(content, "\n") {
		b.WriteString(side + " " + cell.Render(ln) + " " + side + "\n")
	}
	b.WriteString(styDialogBorder.Render("╰" + strings.Repeat("─", span) + "╯"))
	return b.String()
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
