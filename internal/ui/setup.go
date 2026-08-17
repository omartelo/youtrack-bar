package ui

import (
	"os"
	"strings"

	"github.com/omartelo/youtrack-bar/internal/config"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// fieldCharLimit caps each setup field. A permanent token is ~70 characters
// and a CA path well under that; the limit is only there so a runaway paste
// cannot fill the terminal.
const fieldCharLimit = 400

// Fields of the first-run form.
const (
	fieldName = iota
	fieldURL
	fieldToken
	fieldCA
	fieldCount
)

var (
	setupLabels = [fieldCount]string{
		"Provider name", "YouTrack URL", "Permanent token", "CA certificate (optional)",
	}
	setupHints = [fieldCount]string{
		"any label you like — it shows up in the header",
		"instance root, without /api",
		"Profile → Account Security → New permanent token · ${ENV_VAR} works too",
		"only needed when the certificate does not verify · ${ENV_VAR} works too",
	}
	setupPlaceholders = [fieldCount]string{
		"acme", "https://acme.youtrack.cloud", "perm:xxxx.yyyy.zzzz", "/etc/ssl/certs/corp-ca.pem",
	}

	// caEnvHints are the variables other tools already use for an extra CA
	// bundle. If one is set it is offered as a prefilled, editable suggestion —
	// reading it silently would hide a trust decision in the environment.
	caEnvHints = []string{"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE"}
)

func (m *Model) setupKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "tab", "down":
		return m, m.setup.focusOn(m.setup.focus + 1)
	case "shift+tab", "up":
		return m, m.setup.focusOn(m.setup.focus - 1)
	case "ctrl+r":
		m.setup.toggleReveal()
		return m, nil
	case "enter":
		if m.setup.focus != fieldCount-1 {
			return m, m.setup.focusOn(m.setup.focus + 1)
		}
		return m, m.submitSetup()
	}
	return m, m.setup.update(msg)
}

// submitSetup validates the typed provider and then proves it against the API.
// The file is written by the filtersMsg handler, not here.
func (m *Model) submitSetup() tea.Cmd {
	typed := config.Provider{
		Name:   m.setup.value(fieldName),
		URL:    m.setup.value(fieldURL),
		Token:  m.setup.value(fieldToken),
		CAFile: m.setup.value(fieldCA),
	}

	// Validate expands ${VARS} in place but stashes what was typed in
	// Provider.RawToken / RawCAFile, and config.Save writes those back — that
	// is what keeps a reference from being persisted as the secret it resolved
	// to. See the token invariant in CLAUDE.md.
	live := &config.Config{Providers: []config.Provider{typed}}
	if err := live.Validate(); err != nil {
		m.dlg = infoDialog("Check the form", err.Error())
		return nil
	}

	m.cfg = live
	m.savePending = true
	if err := m.setProvider(0); err != nil {
		m.dlg = errorDialog(err)
		return nil
	}
	return m.loadFilters()
}

// setupForm is the first-run configuration screen: three inputs, no wizard.
type setupForm struct {
	inputs [fieldCount]textinput.Model
	focus  int
	path   string // where the config will be written, shown to the user
}

func newSetupForm(path string) setupForm {
	f := setupForm{path: path}
	for i := range f.inputs {
		in := textinput.New()
		in.Prompt = ""
		in.CharLimit = fieldCharLimit
		in.Placeholder = setupPlaceholders[i]
		f.inputs[i] = in
	}
	// The token is masked by default: this screen gets screen-shared and
	// screenshotted. ctrl+r reveals it.
	f.inputs[fieldToken].EchoMode = textinput.EchoPassword

	for _, env := range caEnvHints {
		if os.Getenv(env) != "" {
			f.inputs[fieldCA].SetValue("${" + env + "}")
			break
		}
	}
	return f
}

func (f *setupForm) focusOn(i int) tea.Cmd {
	f.focus = ((i % fieldCount) + fieldCount) % fieldCount
	var cmd tea.Cmd
	for j := range f.inputs {
		if j == f.focus {
			cmd = f.inputs[j].Focus()
			continue
		}
		f.inputs[j].Blur()
	}
	return cmd
}

func (f *setupForm) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd
}

func (f *setupForm) setWidth(w int) {
	for i := range f.inputs {
		f.inputs[i].SetWidth(max(10, min(64, w-6)))
	}
}

func (f *setupForm) toggleReveal() {
	if f.inputs[fieldToken].EchoMode == textinput.EchoPassword {
		f.inputs[fieldToken].EchoMode = textinput.EchoNormal
		return
	}
	f.inputs[fieldToken].EchoMode = textinput.EchoPassword
}

func (f setupForm) value(i int) string { return strings.TrimSpace(f.inputs[i].Value()) }

func (f setupForm) view() string {
	var b strings.Builder
	b.WriteString("\n  " + styHead.Render("No config file yet — let's make one.") + "\n")
	b.WriteString("  " + styDim.Render("It will be written to "+f.path+" with 0600 permissions.") + "\n")

	for i := range f.inputs {
		marker, label := "  ", styLabel.Render(setupLabels[i])
		if i == f.focus {
			marker, label = stySection.Render("▌ "), stySection.Render(setupLabels[i])
		}
		b.WriteString("\n" + marker + label + "\n")
		b.WriteString("    " + f.inputs[i].View() + "\n")
		b.WriteString("    " + styDim.Render(setupHints[i]) + "\n")
	}
	return b.String()
}
