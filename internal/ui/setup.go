package ui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

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
		in.CharLimit = 400
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
