package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// queryCharLimit caps the raw query. YouTrack queries with several field
// clauses run long, so this is looser than the setup fields; it exists to stop
// a runaway paste, not to constrain a real query.
const queryCharLimit = 500

// queryPrompt is the one-line input for typing a YouTrack query by hand rather
// than picking a saved search. It is a peer of the list's own `/` filter, which
// only narrows what is already on screen.
type queryPrompt struct {
	input  textinput.Model
	active bool
}

func newQueryPrompt() queryPrompt {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = queryCharLimit
	in.Placeholder = "project: PAY #Unresolved for: me"
	return queryPrompt{input: in}
}

// open focuses the prompt, seeded with the query currently on screen so an
// existing one can be tweaked instead of retyped.
func (q *queryPrompt) open(seed string) tea.Cmd {
	q.active = true
	q.input.SetValue(seed)
	cmd := q.input.Focus()
	q.input.CursorEnd()
	return cmd
}

func (q *queryPrompt) close() {
	q.active = false
	q.input.Blur()
}

func (q queryPrompt) value() string { return strings.TrimSpace(q.input.Value()) }

func (q *queryPrompt) setWidth(w int) { q.input.SetWidth(max(10, w-12)) }

func (q *queryPrompt) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	q.input, cmd = q.input.Update(msg)
	return cmd
}

// lines is what the layout has to subtract from the body height.
func (q queryPrompt) lines() int {
	if q.active {
		return 1
	}
	return 0
}

func (q queryPrompt) view() string {
	return styQueryLabel.Render(" query ") + " " + q.input.View()
}
