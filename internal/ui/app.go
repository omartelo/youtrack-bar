// Package ui holds the bubbletea program: three screens, one state machine.
package ui

import (
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
	"github.com/charmbracelet/x/ansi"

	"github.com/omartelo/youtrack-tui/internal/config"
	"github.com/omartelo/youtrack-tui/internal/youtrack"
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

// markedFilterID is the filter that lists what `x` has ticked. It only exists
// while there is something to list, which is why it is not in builtinFilters.
const markedFilterID = "builtin:marked"

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

	// newVersion is the tag of a release newer than this binary, empty until
	// the startup check says otherwise. It only ever gets shown in the header:
	// the TUI is read-only about itself too, and `youtrack-tui update` is what
	// installs anything.
	newVersion string

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

	// cache holds each query's issues for issueCacheTTL, so that stepping out
	// of an issue and into the next one does not fetch the list again. Keyed
	// by the query as sent, ordering clause included; emptied by `r` and by a
	// provider switch, since the same query means something else elsewhere.
	cache map[string]cachedIssues

	// flash is a one-line confirmation in the header — what `y` copied, say.
	// flashGen retires the timer of a flash that was replaced by a newer one.
	flash    string
	flashGen int

	// commentsLine is where the comments start in the rendered issue, so `c`
	// can jump there. Recomputed by renderDetail, since the order flips.
	commentsLine int

	// sortBy indexes sortOrders: the `sort by:` clause pushed onto every issue
	// query. Session-only, like the query itself.
	sortBy int

	// query is what was sent; queryName is the saved search it came from, so
	// the header can say "TO DEPLOY" as well as the clause behind it. Empty
	// for a raw query typed at the `s` prompt — that one has no name.
	query     string
	queryName string

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

// cachedIssues is one query's result set and when it was fetched. The page
// count is not kept: a short page is what says "that was the last one", and
// the handler works that out again from the length.
type cachedIssues struct {
	issues []youtrack.Issue
	at     time.Time
}

// issueCacheTTL is deliberately short. The point is the back-and-forth between
// a list and the issues on it, not saving a request an hour from now — the
// list on screen must not be yesterday's board.
const issueCacheTTL = 30 * time.Second

// flashFor is how long a header confirmation stays up.
const flashFor = 2 * time.Second

// flashExpiredMsg clears a header confirmation, unless a newer one replaced it.
type flashExpiredMsg struct{ gen int }

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
	// A cached query belongs to the instance it was asked of: the same text
	// means different issues on the next one.
	clear(m.cache)
	// Watched filters, what has been seen and what is still marked new are all
	// per-instance, so switching starts over rather than carrying state across.
	m.watch = newWatcher(p.Watch)
	return nil
}

// flashCmd puts a confirmation in the header and takes it away again. Only
// for things that otherwise leave no trace: a clipboard write nobody sees is
// indistinguishable from a key that does nothing. Errors are still dialogs.
func (m *Model) flashCmd(text string) tea.Cmd {
	m.flashGen++
	m.flash = text
	gen := m.flashGen
	return tea.Tick(flashFor, func(time.Time) tea.Msg { return flashExpiredMsg{gen} })
}

// cacheKey is the query as it goes over the wire, ordering clause included:
// the same filter sorted two ways is two result sets.
func (m *Model) cacheKey(query string) string {
	return applySort(query, sortOrders[m.sortBy])
}

// cacheIssues remembers everything currently on the issue list, pages and all,
// so that going back to it is free for issueCacheTTL.
func (m *Model) cacheIssues() {
	if m.query == "" || len(m.allIssues) == 0 {
		return
	}
	if m.cache == nil {
		m.cache = make(map[string]cachedIssues)
	}
	// Cloned: appendNewIssues grows allIssues in place, which would otherwise
	// reach into the entry that was just stored.
	m.cache[m.cacheKey(m.query)] = cachedIssues{issues: slices.Clone(m.allIssues), at: time.Now()}
}

// commentsNewestFirst is the reading order of an issue's comments. It lives
// in the config so that `S` on the detail screen is remembered, and is read
// through here because there is no config at all on the setup screen.
func (m *Model) commentsNewestFirst() bool {
	return m.cfg != nil && m.cfg.CommentsNewestFirst
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
	return tea.Batch(m.spin.Tick, m.loadFilters(), m.checkUpdateCmd())
}

// checkUpdateCmd is the startup update check, unless the config turned it off.
func (m *Model) checkUpdateCmd() tea.Cmd {
	if !m.cfg.ShouldCheckUpdates() {
		return nil
	}
	return checkUpdate()
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
		return m, tea.Batch(m.refreshMarkedFilter(), m.startWatch())

	case updateMsg:
		m.newVersion = msg.tag
		return m, nil

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
			m.allIssues = appendNewIssues(m.allIssues, msg.issues)
		} else {
			m.allIssues, at = msg.issues, 0
		}
		m.cacheIssues()
		cmd := m.setIssueItems()
		m.issues.Select(at)
		return m, cmd

	case flashExpiredMsg:
		if msg.gen == m.flashGen {
			m.flash = ""
		}
		return m, nil

	case detailMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		m.screen = screenDetail
		if m.commentsNewestFirst() {
			slices.Reverse(msg.comments)
		}
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
		// `r` means "ask again", so it has to outrank the cache.
		clear(m.cache)
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

	case key.Matches(msg, m.keys.Sort):
		if m.screen == screenDetail {
			// Comments are ordered here, not by the instance: they all came
			// down with the issue, so reversing the slice is the whole job.
			// The config holds the choice, so the flip outlives the session
			// the same way `f` and `w` do.
			m.cfg.CommentsNewestFirst = !m.cfg.CommentsNewestFirst
			slices.Reverse(m.comments)
			if err := m.saveConfig(); err != nil {
				m.dlg = infoDialog("Config not saved", err.Error())
			}
			m.renderDetail()
			m.detail.GotoTop()
			return m, nil
		}
		if m.screen != screenFilters && m.screen != screenIssues {
			return m, nil
		}
		m.sortBy = (m.sortBy + 1) % len(sortOrders)
		if m.screen != screenIssues || m.query == "" {
			return m, nil
		}
		// Ordering is the instance's job: what is on screen is one window onto
		// the result set, so the query is run again from the first page.
		return m, m.loadIssues(m.query)

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

	case key.Matches(msg, m.keys.Mark):
		return m, m.toggleMark()

	case key.Matches(msg, m.keys.Browser):
		if id := m.selectedIssueID(); id != "" {
			return m, openInBrowser(m.client.IssueURL(id))
		}
		return m, nil

	case key.Matches(msg, m.keys.Copy):
		id := m.selectedIssueID()
		if id == "" {
			return m, nil
		}
		// OSC 52 hands the URL to the terminal rather than to a clipboard
		// tool, which is what makes it work over SSH — where `o` has no
		// browser to open and answers with a dialog holding the URL instead.
		return m, tea.Batch(tea.SetClipboard(m.client.IssueURL(id)),
			m.flashCmd(id+" URL copied"))

	case m.screen == screenDetail && key.Matches(msg, m.keys.Comments):
		m.detail.SetYOffset(m.commentsLine)
		return m, nil

	case m.screen == screenDetail && key.Matches(msg, m.keys.Top):
		m.detail.GotoTop()
		return m, nil

	case m.screen == screenDetail && key.Matches(msg, m.keys.Bottom):
		m.detail.GotoBottom()
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
				m.queryName = it.Name
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
		m.queryName = ""
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

// appendNewIssues adds a page, skipping anything already on the list. Paging is
// by offset, so an issue added to the filter between two requests shifts the
// window and makes the next page overlap the previous one.
func appendNewIssues(have, page []youtrack.Issue) []youtrack.Issue {
	seen := make(map[string]bool, len(have))
	for _, iss := range have {
		seen[iss.ID] = true
	}
	for _, iss := range page {
		if !seen[iss.ID] {
			have = append(have, iss)
		}
	}
	return have
}

// setIssueItems rebuilds the issue list from every page fetched so far.
func (m *Model) setIssueItems() tea.Cmd {
	p := m.cfg.Providers[m.provider]
	marked := make(map[string]bool, len(p.Marked))
	for _, id := range p.Marked {
		marked[id] = true
	}
	items := make([]list.Item, 0, len(m.allIssues))
	for _, iss := range m.allIssues {
		items = append(items, issueItem{
			issue:  iss,
			fields: p.ListFields,
			isNew:  m.watch.isFresh(iss.ID),
			marked: marked[iss.ID],
		})
	}
	return m.issues.SetItems(items)
}

// isMarked reports whether an issue carries the user's tick.
func (m *Model) isMarked(id string) bool {
	return m.cfg != nil && slices.Contains(m.cfg.Providers[m.provider].Marked, id)
}

// toggleMark ticks the selected issue off, or takes the tick back. It records
// IDs in the config next to `favorites` and `watch`, so a list worked through
// over a day survives closing the program — which is the only reason to tick
// anything off in the first place.
//
// The app assigns no meaning to it: reviewed, read, answered, deal with it
// tomorrow. Whoever presses `x` knows what they meant.
func (m *Model) toggleMark() tea.Cmd {
	id := m.selectedIssueID()
	if id == "" {
		return nil
	}
	p := &m.cfg.Providers[m.provider]
	if i := slices.Index(p.Marked, id); i >= 0 {
		p.Marked = slices.Delete(p.Marked, i, i+1)
	} else {
		p.Marked = append(p.Marked, id)
	}
	if err := m.saveConfig(); err != nil {
		m.dlg = infoDialog("Config not saved", err.Error())
	}
	// The list picks up the glyph; the Marked filter picks up the ID. On the
	// detail screen the header is what says it, since neither is on show.
	return tea.Batch(m.refreshIssueMarks(), m.refreshMarkedFilter())
}

// refreshMarkedFilter keeps the synthetic "Marked" filter in step with what is
// ticked, and drops it once nothing is. Without a way back to them, marks
// accumulate in a config nobody can review: the filters they were found under
// change, and an ID alone says nothing about where it came from.
//
// ponytail: the whole list travels as one `issue id:` query, so a few hundred
// marks make a URL long enough for a server to refuse. Upgrade: page the IDs,
// or keep only the newest N.
func (m *Model) refreshMarkedFilter() tea.Cmd {
	m.savedQueries = slices.DeleteFunc(m.savedQueries, func(q youtrack.SavedQuery) bool {
		return q.ID == markedFilterID
	})
	if ids := m.cfg.Providers[m.provider].Marked; len(ids) > 0 {
		at := min(len(builtinFilters), len(m.savedQueries))
		m.savedQueries = slices.Insert(m.savedQueries, at, youtrack.SavedQuery{
			ID:    markedFilterID,
			Name:  "Marked",
			Query: "issue id: " + strings.Join(ids, ", "),
		})
	}
	return m.setFilterItems()
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
	//
	// Only while unfiltered. Items() is the unfiltered order but Select indexes
	// the visible one, so following a filtered list lands the cursor on some
	// other row — and the next `f` then pins the wrong filter. With a filter
	// applied the visible order is the fuzzy ranking, which pinning does not
	// touch, so the cursor is already where it belongs.
	if !m.filters.IsFiltered() {
		for i, li := range m.filters.Items() {
			if f, ok := li.(filterItem); ok && f.ID == it.ID {
				m.filters.Select(i)
				break
			}
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

// chromeLines is what the layout reserves outside the body: two header lines
// (title row and rule) plus one footer line.
const chromeLines = 3

func (m *Model) layout() {
	body := max(1, m.h-chromeLines-m.prompt.lines())
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
	body := renderIssue(m.client, m.current, m.comments, m.w)
	m.detail.SetContent(body)
	m.commentsLine = commentsLineOf(body)
}

// commentsLineOf finds the comments heading in a rendered issue, which is
// where `c` scrolls to. Zero — the top — when there is none to find.
//
// ponytail: counts lines of the rendered document, so it assumes the viewport
// shows them one for one. Everything here is already wrapped to the pane
// width, so it does; a soft-wrapping viewport would need its own line count.
func commentsLineOf(body string) int {
	for i, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "Comments (") {
			return i
		}
	}
	return 0
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
	left := styTitle.Render(" youtrack-tui ") + styProvider.Render(" "+m.providerName()+" ")
	if m.insecure() {
		// A downgraded connection stays on screen for as long as it is on.
		left += styInsecure.Render(" !insecure ")
	}
	if n := len(m.watch.watching); n > 0 {
		// The glyph alone does not say what it counts, and this is the only
		// place the ◉ in the filters gutter gets explained.
		style, label := styWatch, fmt.Sprintf("◉ watching %d", n)
		if m.watch.failed {
			// A background poll cannot raise a modal, so it says so here.
			style, label = styWatchFail, fmt.Sprintf("◉ watching %d (failed)", n)
		}
		left += " " + style.Render(label)
	}

	if clause := sortOrders[m.sortBy]; clause != "" {
		// Exactly what was appended to the query, not a prettier name for it:
		// the same string works when typed into the `s` prompt.
		left += " " + styDim.Render("sort by: "+clause)
	}

	if m.screen == screenDetail && m.current != nil && m.isMarked(m.current.ID) {
		// The list glyph is two screens away; without this `x` on an open
		// issue looks like a key that does nothing.
		left += " " + styMark.Render("✓ marked")
	}

	if m.screen == screenDetail && m.commentsNewestFirst() {
		// Written to the config, so it stays on until it is turned off —
		// worth a word on screen, like the sort clause above.
		left += " " + styDim.Render("comments: newest first")
	}

	if m.flash != "" {
		left += " " + styWatch.Render(m.flash)
	}

	if m.newVersion != "" {
		// Ambient, like the watch counter: it says a newer release exists and
		// names the command that installs it, and nothing about it interrupts.
		left += " " + styUpdate.Render("↑ "+m.newVersion+" — run `youtrack-tui update`")
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
		if m.queryName != "" {
			right = styValue.Render(m.queryName) + "  " + right
		}
	default:
		right = styDim.Render("pick a filter")
	}

	// The name pushed an already long clause further right; without this the
	// row overflows and lipgloss wraps it under the title.
	if avail := m.w - lipgloss.Width(left) - 1; avail > 0 && lipgloss.Width(right) > avail {
		right = ansi.Truncate(right, avail, "…")
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
