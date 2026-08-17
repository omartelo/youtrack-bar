// Package ui holds the bubbletea program: three screens, one state machine.
package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/omartelo/youtrack-bar/internal/config"
	"github.com/omartelo/youtrack-bar/internal/youtrack"
)

type screen int

const (
	screenSetup screen = iota
	screenFilters
	screenIssues
	screenDetail
)

// builtinFilters are always offered alongside the user's saved searches. Their
// IDs are synthetic and prefixed so they can never collide with a YouTrack
// entity ID, and they are what `favorites` records.
var builtinFilters = []youtrack.SavedQuery{
	{ID: "builtin:my-open-issues", Name: "My open issues", Query: "for: me #Unresolved"},
	{ID: "builtin:all-unresolved", Name: "All unresolved", Query: "#Unresolved"},
}

// Model is the root program state.
type Model struct {
	cfg      *config.Config
	path     string // config file location, for the first-run save
	provider int
	client   *youtrack.Client

	screen  screen
	setup   setupForm
	prompt  queryPrompt
	filters list.Model
	issues  list.Model
	detail  viewport.Model
	spin    spinner.Model
	help    help.Model
	keys    keyMap

	// dlg is the modal error popup. Non-nil means it owns the keyboard.
	dlg *dialog

	// savePending means the config is written as soon as a request succeeds:
	// no point persisting a token that does not work. config.Save is what
	// keeps ${VAR} references intact.
	savePending bool

	// savedQueries is the filter list as YouTrack returned it, kept so
	// favouriting can re-sort without another round trip.
	savedQueries []youtrack.SavedQuery

	// allIssues accumulates across pages; it is also what the next $skip is
	// counted from. moreIssues is false once a short page comes back.
	allIssues  []youtrack.Issue
	moreIssues bool

	query    string
	current  *youtrack.Issue
	comments []youtrack.Comment

	// watch is the background poller for filters the user is monitoring, and
	// watchGen retires the tick chain when the provider changes or the watch
	// list is toggled — otherwise every toggle would leave another ticker
	// running alongside the first.
	watch    watcher
	watchGen int

	// gen invalidates in-flight responses after a provider switch or reload,
	// so a slow answer from the old instance never lands on the new one.
	gen     int
	loading bool
	w, h    int
}

// New builds the program. A nil cfg means there is no config file yet and the
// program opens on the setup screen instead of failing. provider selects a
// config entry by name; empty picks the first one.
func New(cfg *config.Config, provider, path string) (*Model, error) {
	m := &Model{
		path:    path,
		setup:   newSetupForm(path),
		prompt:  newQueryPrompt(),
		filters: newList("Filters"),
		issues:  newList("Issues"),
		detail:  viewport.New(),
		spin:    spinner.New(spinner.WithSpinner(spinner.Dot)),
		help:    help.New(),
		keys:    defaultKeys(),
	}
	m.issues.SetStatusBarItemName("issue", "issues")

	if cfg == nil {
		m.screen = screenSetup
		return m, nil
	}

	idx := 0
	if provider != "" {
		if idx = cfg.Find(provider); idx < 0 {
			return nil, fmt.Errorf("provider %q is not in the config", provider)
		}
	}
	m.cfg = cfg
	m.screen = screenFilters
	if err := m.setProvider(idx); err != nil {
		return nil, err
	}
	return m, nil
}

func newList(title string) list.Model {
	d := list.NewDefaultDelegate()
	l := list.New(nil, d, 0, 0)
	l.Title = title
	l.SetShowHelp(false)
	return l
}

// setProvider rebuilds the client for provider i. It can fail because the
// TLS settings are read here: a `ca_file` that is missing or not a PEM.
func (m *Model) setProvider(i int) error {
	p := m.cfg.Providers[i]
	c, err := youtrack.New(p.URL, p.Token, youtrack.TLS{CAFile: p.CAFile, Insecure: p.Insecure})
	if err != nil {
		return fmt.Errorf("provider %q: %w", p.Name, err)
	}
	m.provider, m.client = i, c
	// Watched filters, what has been seen and what is still marked new are all
	// per-instance, so switching starts over rather than carrying state across.
	m.watch = newWatcher(p.Watch)
	return nil
}

func (m *Model) insecure() bool {
	return m.cfg != nil && m.cfg.Providers[m.provider].Insecure
}

func (m *Model) providerName() string {
	if m.cfg == nil {
		return "setup"
	}
	return m.cfg.Providers[m.provider].Name
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	if m.screen == screenSetup {
		return tea.Batch(m.spin.Tick, m.setup.focusOn(fieldName))
	}
	return tea.Batch(m.spin.Tick, m.loadFilters())
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		m.renderDetail()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case errMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		// savePending survives on purpose: the retry offered by the dialog has
		// to still write the config once it works. Nothing is on disk yet.
		m.dlg = errorDialog(msg.err)
		return m, nil

	case filtersMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		if m.savePending {
			if err := m.saveConfig(); err != nil {
				// The session still works, it just will not be remembered.
				m.dlg = infoDialog("Config not saved", err.Error())
			}
			m.savePending = false
			m.screen = screenFilters
		}
		m.savedQueries = msg.queries
		m.migrateFavorites()
		// The watch list resolves IDs against these, so the poller can only
		// start once they have arrived.
		return m, tea.Batch(m.setFilterItems(), m.startWatch())

	case issuesMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		m.screen = screenIssues

		// A short page is how YouTrack says "that was the last one" — it has no
		// total count worth paying for.
		m.moreIssues = len(msg.issues) == m.cfg.PageSize
		m.keys.More.SetEnabled(m.moreIssues)

		at := m.issues.Index()
		if msg.appendTo {
			m.allIssues = append(m.allIssues, msg.issues...)
		} else {
			m.allIssues, at = msg.issues, 0
		}
		cmd := m.setIssueItems()
		m.issues.Select(at)
		return m, cmd

	case detailMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		m.screen = screenDetail
		m.current, m.comments = msg.issue, msg.comments
		m.renderDetail()
		m.detail.GotoTop()
		// Reading it is what makes it no longer new.
		delete(m.watch.fresh, msg.issue.ID)
		return m, m.refreshIssueMarks()

	case watchTickMsg:
		if msg.gen != m.watchGen {
			return m, nil
		}
		return m, tea.Batch(m.pollWatched(), m.tickWatch())

	case watchResultMsg:
		if msg.gen != m.watchGen {
			return m, nil
		}
		if msg.err != nil {
			// A background poll must not throw a modal over what the user is
			// doing; the header carries a badge until the next poll works.
			m.watch.failed = true
			return m, nil
		}
		m.watch.failed = false
		fresh := m.watch.record(msg.filterID, msg.issues)
		if len(fresh) == 0 {
			return m, nil
		}
		return m, tea.Batch(m.refreshIssueMarks(),
			notifyNew(m.cfg.Notifier, msg.label, fresh))

	case notifiedMsg:
		if msg.err != nil {
			m.dlg = infoDialog("Notification failed", msg.err.Error()+
				"\n\nSet `notifier` in the config to notify-send, or to none to stop trying.")
		}
		return m, nil

	case openedMsg:
		if msg.err != nil {
			// Over SSH or on a headless box there is no handler. Show the URL
			// so it can at least be copied out.
			m.dlg = infoDialog("Could not open a browser",
				msg.err.Error()+"\n\n"+msg.url)
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.onKey(msg)
	}

	// Everything else belongs to whichever sub-model is on screen. Routing only
	// a whitelist of message types silently breaks any bubble that answers its
	// own commands: bracketed paste arrives as tea.PasteMsg rather than key
	// presses, and the list runs its filter in a tea.Cmd and applies the result
	// when the FilterMatchesMsg comes back.
	return m, m.forward(msg)
}

func (m *Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.dlg != nil {
		return m.dialogKey(msg)
	}

	// The setup form owns every key while it is up — "q" has to type a q, not
	// quit the program.
	if m.screen == screenSetup {
		return m.setupKey(msg)
	}
	if m.prompt.active {
		return m.promptKey(msg)
	}

	// While the list's own filter input is open every key belongs to it.
	if (m.screen == screenFilters && m.filters.SettingFilter()) ||
		(m.screen == screenIssues && m.issues.SettingFilter()) {
		return m, m.forward(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case key.Matches(msg, m.keys.Provider):
		if len(m.cfg.Providers) < 2 {
			return m, nil
		}
		if err := m.setProvider((m.provider + 1) % len(m.cfg.Providers)); err != nil {
			m.dlg = errorDialog(err)
			return m, nil
		}
		m.screen = screenFilters
		m.filters.SetItems(nil)
		m.issues.SetItems(nil)
		m.current, m.comments = nil, nil
		return m, m.loadFilters()

	case key.Matches(msg, m.keys.Reload):
		return m, m.reload()

	case key.Matches(msg, m.keys.More):
		if m.screen != screenIssues {
			return m, nil
		}
		return m, m.loadMoreIssues()

	case key.Matches(msg, m.keys.Search):
		if m.screen != screenFilters && m.screen != screenIssues {
			return m, nil
		}
		cmd := m.prompt.open(m.query)
		m.layout()
		return m, cmd

	case key.Matches(msg, m.keys.Favorite):
		if m.screen != screenFilters {
			return m, nil
		}
		return m, m.toggleFavorite()

	case key.Matches(msg, m.keys.Watch):
		if m.screen != screenFilters {
			return m, nil
		}
		return m, m.toggleWatch()

	case key.Matches(msg, m.keys.Browser):
		if id := m.selectedIssueID(); id != "" {
			return m, openInBrowser(m.client.IssueURL(id))
		}
		return m, nil

	case key.Matches(msg, m.keys.Back):
		// An applied filter owns esc: clearing it has to come before leaving
		// the screen, or there is no way back to the full list.
		if (m.screen == screenIssues && m.issues.IsFiltered()) ||
			(m.screen == screenFilters && m.filters.IsFiltered()) {
			return m, m.forward(msg)
		}
		switch m.screen {
		case screenDetail:
			m.screen = screenIssues
		case screenIssues:
			m.screen = screenFilters
		}
		return m, nil

	case key.Matches(msg, m.keys.Open):
		switch m.screen {
		case screenFilters:
			if it, ok := m.filters.SelectedItem().(filterItem); ok {
				return m, m.loadIssues(it.Query)
			}
		case screenIssues:
			if it, ok := m.issues.SelectedItem().(issueItem); ok {
				return m, m.loadDetail(it.issue.ID)
			}
		}
		return m, nil
	}

	return m, m.forward(msg)
}

// promptKey handles the raw-query input. Like the setup form it owns every key
// while it is open, otherwise typing a query containing "for" would fire the
// favourite, open-in-browser and reload commands.
func (m *Model) promptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.prompt.close()
		m.layout()
		return m, nil
	case "enter":
		q := m.prompt.value()
		m.prompt.close()
		m.layout()
		if q == "" {
			return m, nil
		}
		return m, m.loadIssues(q)
	}
	return m, m.prompt.update(msg)
}

// dialogKey handles the modal. It swallows everything else so a stray key
// never leaks into the screen behind it.
func (m *Model) dialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "i":
		if !m.dlg.offerTrust {
			return m, nil
		}
		m.dlg = nil
		return m, m.trustAnyway()
	case "esc", "enter", "q":
		m.dlg = nil
		return m, nil
	}
	return m, nil
}

// trustAnyway is the user accepting an untrusted certificate for this provider
// only. It is written to the config alongside everything else, so the choice
// stays visible instead of living in an environment variable.
func (m *Model) trustAnyway() tea.Cmd {
	p := &m.cfg.Providers[m.provider]
	p.CAFile, p.RawCAFile = "", ""
	p.Insecure = true
	if err := m.setProvider(m.provider); err != nil {
		m.dlg = errorDialog(err)
		return nil
	}
	// Persist the downgrade so the next run does not ask again.
	m.savePending = true
	return m.reload()
}

// setFilterItems rebuilds the filters list, favourites first. Both lists are
// sorted stably: favourites keep the order they were pinned in, everything
// else keeps the order YouTrack returned.
func (m *Model) setFilterItems() tea.Cmd {
	fav := m.cfg.Providers[m.provider].Favorites
	rank := func(id string) int {
		if i := slices.Index(fav, id); i >= 0 {
			return i
		}
		return len(fav)
	}

	sorted := slices.Clone(m.savedQueries)
	slices.SortStableFunc(sorted, func(a, b youtrack.SavedQuery) int {
		return rank(a.ID) - rank(b.ID)
	})

	items := make([]list.Item, 0, len(sorted))
	for _, q := range sorted {
		items = append(items, filterItem{
			SavedQuery: q,
			fav:        rank(q.ID) < len(fav),
			watched:    m.watch.watching[q.ID],
		})
	}
	return m.filters.SetItems(items)
}

// migrateFavorites rewrites favourites recorded as names — the original format
// — into IDs, so that renaming a saved search in YouTrack stops losing its pin.
// Memory only: the next `f` persists the result, and an entry that matches
// nothing is left alone rather than dropped.
func (m *Model) migrateFavorites() {
	p := &m.cfg.Providers[m.provider]
	ids := make(map[string]bool, len(m.savedQueries))
	byName := make(map[string]string, len(m.savedQueries))
	for _, q := range m.savedQueries {
		ids[q.ID] = true
		byName[q.Name] = q.ID
	}
	for i, f := range p.Favorites {
		if ids[f] {
			continue
		}
		if id, ok := byName[f]; ok {
			p.Favorites[i] = id
		}
	}
}

// setIssueItems rebuilds the issue list from every page fetched so far.
func (m *Model) setIssueItems() tea.Cmd {
	fields := m.cfg.Providers[m.provider].ListFields
	items := make([]list.Item, 0, len(m.allIssues))
	for _, iss := range m.allIssues {
		items = append(items, issueItem{issue: iss, fields: fields, isNew: m.watch.fresh[iss.ID]})
	}
	return m.issues.SetItems(items)
}

// refreshIssueMarks redraws the list so a newly arrived issue picks up its
// marker, without disturbing where the user is.
func (m *Model) refreshIssueMarks() tea.Cmd {
	if len(m.allIssues) == 0 {
		return nil
	}
	at := m.issues.Index()
	cmd := m.setIssueItems()
	m.issues.Select(at)
	return cmd
}

// --- background watching ---------------------------------------------------

// startWatch retires any running tick chain and begins a new one. The first
// poll only seeds, so nothing is announced for issues that were already there.
func (m *Model) startWatch() tea.Cmd {
	m.watchGen++
	if len(m.watch.watching) == 0 {
		return nil
	}
	return tea.Batch(m.pollWatched(), m.tickWatch())
}

func (m *Model) tickWatch() tea.Cmd {
	gen := m.watchGen
	return tea.Tick(m.cfg.WatchEvery, func(time.Time) tea.Msg {
		return watchTickMsg{gen: gen}
	})
}

// pollWatched fetches every watched filter once. Watched IDs that no longer
// resolve to a saved search are skipped rather than reported.
func (m *Model) pollWatched() tea.Cmd {
	c, gen, top := m.client, m.watchGen, m.cfg.PageSize
	var cmds []tea.Cmd
	for _, q := range m.savedQueries {
		if !m.watch.watching[q.ID] {
			continue
		}
		id, query, label := q.ID, q.Query, q.Name
		cmds = append(cmds, func() tea.Msg {
			issues, err := c.Issues(context.Background(), query, 0, top)
			return watchResultMsg{gen: gen, filterID: id, label: label, issues: issues, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// toggleWatch starts or stops monitoring the selected filter for this session.
// Nothing is written to the config: turning a poller on and off should not
// rewrite a file.
func (m *Model) toggleWatch() tea.Cmd {
	it, ok := m.filters.SelectedItem().(filterItem)
	if !ok {
		return nil
	}
	if m.watch.watching[it.ID] {
		delete(m.watch.watching, it.ID)
		delete(m.watch.seen, it.ID)
	} else {
		m.watch.watching[it.ID] = true
	}
	return tea.Batch(m.setFilterItems(), m.startWatch())
}

// toggleFavorite pins or unpins the selected filter and persists it. Matching
// is by ID, so renaming a saved search in YouTrack keeps its pin and two
// searches sharing a name stay independent.
func (m *Model) toggleFavorite() tea.Cmd {
	it, ok := m.filters.SelectedItem().(filterItem)
	if !ok {
		return nil
	}
	p := &m.cfg.Providers[m.provider]
	if i := slices.Index(p.Favorites, it.ID); i >= 0 {
		p.Favorites = slices.Delete(p.Favorites, i, i+1)
	} else {
		p.Favorites = append(p.Favorites, it.ID)
	}
	if err := m.saveConfig(); err != nil {
		m.dlg = infoDialog("Config not saved", err.Error())
	}

	cmd := m.setFilterItems()
	// Follow the item to its new position instead of resetting to the top:
	// pinning moves it, the cursor should go with it.
	for i, li := range m.filters.Items() {
		if f, ok := li.(filterItem); ok && f.ID == it.ID {
			m.filters.Select(i)
			break
		}
	}
	return cmd
}

// selectedIssueID is the issue the user is looking at, from either the list or
// the detail view. Empty on the screens that have none.
func (m *Model) selectedIssueID() string {
	switch m.screen {
	case screenIssues:
		if it, ok := m.issues.SelectedItem().(issueItem); ok {
			return it.issue.ID
		}
	case screenDetail:
		if m.current != nil {
			return m.current.ID
		}
	}
	return ""
}

// reload re-runs whatever the current screen is showing.
func (m *Model) reload() tea.Cmd {
	switch m.screen {
	case screenSetup, screenFilters:
		return m.loadFilters()
	case screenIssues:
		return m.loadIssues(m.query)
	case screenDetail:
		if m.current != nil {
			return m.loadDetail(m.current.ID)
		}
	}
	return nil
}

func (m *Model) saveConfig() error {
	if err := config.Save(m.path, m.cfg); err != nil {
		return fmt.Errorf("could not write %s: %w", m.path, err)
	}
	return nil
}

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

	// Validate normalizes in place and expands ${VARS} in the token, so it
	// runs on its own copy: what gets persisted must stay the reference the
	// user typed, never the resolved secret.
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

// forward hands the message to whichever sub-model owns the current screen.
func (m *Model) forward(msg tea.Msg) tea.Cmd {
	// The prompt floats above the screens, so it takes precedence — this is
	// also what lets a query be pasted in.
	if m.prompt.active {
		return m.prompt.update(msg)
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenSetup:
		cmd = m.setup.update(msg)
	case screenFilters:
		m.filters, cmd = m.filters.Update(msg)
	case screenIssues:
		m.issues, cmd = m.issues.Update(msg)
	case screenDetail:
		m.detail, cmd = m.detail.Update(msg)
	}
	return cmd
}

func (m *Model) layout() {
	body := max(1, m.h-3-m.prompt.lines())
	m.setup.setWidth(m.w)
	m.prompt.setWidth(m.w)
	m.filters.SetSize(m.w, body)
	m.issues.SetSize(m.w, body)
	m.detail.SetWidth(m.w)
	m.detail.SetHeight(body)
}

func (m *Model) renderDetail() {
	if m.current == nil || m.w == 0 {
		return
	}
	m.detail.SetContent(renderIssue(m.client, m.current, m.comments, m.w))
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("")
	}

	var body string
	switch m.screen {
	case screenSetup:
		body = m.setup.view()
	case screenFilters:
		body = m.filters.View()
	case screenIssues:
		body = m.issues.View()
	case screenDetail:
		body = m.detail.View()
	}

	rows := []string{m.header()}
	if m.prompt.active {
		rows = append(rows, m.prompt.view())
	}
	rows = append(rows, body, m.footer())

	screen := lipgloss.JoinVertical(lipgloss.Left, rows...)
	if m.dlg != nil {
		screen = overlay(screen, m.dlg.view(m.w), m.w, m.h)
	}

	v := tea.NewView(screen)
	// Alt screen only. Mouse tracking stays off on purpose: it would steal
	// Ctrl+Click from the terminal and break every OSC 8 link we emit.
	v.AltScreen = true
	return v
}

func (m *Model) header() string {
	left := styTitle.Render(" youtrack-bar ") + styProvider.Render(" "+m.providerName()+" ")
	if m.insecure() {
		// A downgraded connection stays on screen for as long as it is on.
		left += styInsecure.Render(" !insecure ")
	}
	if n := len(m.watch.watching); n > 0 {
		style, label := styWatch, fmt.Sprintf(" watching %d ", n)
		if m.watch.failed {
			// A background poll cannot raise a modal, so it says so here.
			style, label = styWatchFail, fmt.Sprintf(" watch failed (%d) ", n)
		}
		left += style.Render(label)
	}

	var right string
	switch {
	case m.loading:
		right = m.spin.View() + " loading…"
	case m.screen == screenSetup:
		right = styDim.Render("first run")
	case m.screen == screenDetail && m.current != nil:
		right = styDim.Render(m.current.ID)
	case m.query != "":
		right = styDim.Render(m.query)
	default:
		right = styDim.Render("pick a filter")
	}

	gap := max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right + "\n" +
		styRule.Render(strings.Repeat("─", m.w))
}

func (m *Model) footer() string {
	if m.dlg != nil {
		// The dialog carries its own keys; repeating them down here just
		// leaves the hint peeking out beside the box.
		return ""
	}
	if m.prompt.active {
		return styDim.Render("enter  run the query  ·  esc  cancel")
	}
	return m.help.View(screenKeys{m.keys, m.screen})
}

// --- messages -------------------------------------------------------------

type errMsg struct {
	gen int
	err error
}

type filtersMsg struct {
	gen     int
	queries []youtrack.SavedQuery
}

type issuesMsg struct {
	gen      int
	issues   []youtrack.Issue
	appendTo bool // a further page, not a fresh query
}

type detailMsg struct {
	gen      int
	issue    *youtrack.Issue
	comments []youtrack.Comment
}

// begin marks a new request generation and returns the values a command needs
// to be self-contained (invariant: no I/O reads Model state).
func (m *Model) begin() (*youtrack.Client, int) {
	m.gen++
	m.loading, m.dlg = true, nil
	return m.client, m.gen
}

func (m *Model) loadFilters() tea.Cmd {
	c, gen := m.begin()
	return func() tea.Msg {
		saved, err := c.SavedQueries(context.Background())
		if err != nil {
			return errMsg{gen, err}
		}
		return filtersMsg{gen, append(append([]youtrack.SavedQuery(nil), builtinFilters...), saved...)}
	}
}

func (m *Model) loadIssues(query string) tea.Cmd {
	m.query = query
	return m.fetchIssues(query, 0, false)
}

// loadMoreIssues fetches the page after everything already on screen.
func (m *Model) loadMoreIssues() tea.Cmd {
	if !m.moreIssues || m.query == "" {
		return nil
	}
	return m.fetchIssues(m.query, len(m.allIssues), true)
}

func (m *Model) fetchIssues(query string, skip int, appendTo bool) tea.Cmd {
	c, gen := m.begin()
	top := m.cfg.PageSize
	return func() tea.Msg {
		issues, err := c.Issues(context.Background(), query, skip, top)
		if err != nil {
			return errMsg{gen, err}
		}
		return issuesMsg{gen, issues, appendTo}
	}
}

func (m *Model) loadDetail(id string) tea.Cmd {
	c, gen := m.begin()
	return func() tea.Msg {
		ctx := context.Background()
		issue, err := c.Issue(ctx, id)
		if err != nil {
			return errMsg{gen, err}
		}
		comments, err := c.Comments(ctx, id)
		if err != nil {
			return errMsg{gen, err}
		}
		return detailMsg{gen, issue, comments}
	}
}
