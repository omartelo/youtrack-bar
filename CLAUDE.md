# youtrack-bar

## Objective

A **read-only** TUI for browsing YouTrack issues without leaving the terminal.

Flow: pick a saved filter → list issues → open one → read its body, comments
and fields. Attachments and issue links are emitted as OSC 8 hyperlinks, so
Ctrl+Click hands them to the browser. Nothing is ever written back to
YouTrack.

Multiple providers (YouTrack instances) are declared in a single `config.yml`.
With no config file the program opens on a setup screen and writes one — it
never exits on the user's first run.

## Layout

```
main.go                      flags, config load, program boot
internal/config/config.go    config.yml parsing, validation and saving
internal/youtrack/client.go  read-only REST client
internal/youtrack/types.go   API types + dynamic custom-field rendering
internal/ui/app.go           root bubbletea model (4 screens, one state machine)
internal/ui/setup.go         first-run configuration form
internal/ui/query.go         raw-query prompt (`s`)
internal/ui/dialog.go        modal error popup and the layer overlay
internal/ui/browser.go       handing a URL to the desktop (`o`)
internal/ui/notify.go        background watcher and the notification command
internal/ui/requests.go      every call to a YouTrack instance and its messages
internal/ui/render.go        styles, list items, issue detail composition
internal/ui/keys.go          key bindings and per-screen help
docs/tui-mockup.md           reference layout for every screen
```

One state machine, one screen at a time: `screenSetup` (first run only) →
`screenFilters` → `screenIssues` → `screenDetail`. No side-by-side panes, no
focus management.

## Hard invariants

These are not style preferences. Breaking any one of them breaks the product.

1. **Read-only means read-only.** The HTTP client issues `GET` only. No
   `POST`, `PUT`, `PATCH` or `DELETE` ever lands in `internal/youtrack`. If
   someone wants "just a quick comment", that is a different project.

2. **Mouse tracking stays off.** Never enable mouse reporting on the
   `tea.Program` or the `tea.View`. With mouse tracking on, the terminal stops
   handling Ctrl+Click on OSC 8 hyperlinks — the only way this app lets you
   download an attachment. Losing wheel scroll is the price; the keyboard
   covers everything.

3. **Tokens are never literals in the repo.** `token` in the YAML goes through
   `os.ExpandEnv`. No default, no built-in fallback, no logging of the value.
   Network and HTTP errors must never echo the `Authorization` header.

   `Config.Validate` expands `${VAR}` **in place** but stashes the original in
   `Provider.RawToken` / `RawCAFile`, and `config.Save` writes those back.
   Without that, any save — the first-run one, or the rewrite that accepting a
   certificate triggers — would replace `${YOUTRACK_TOKEN}` with the secret it
   resolved to and leave it on disk. Code that clears `CAFile` must clear
   `RawCAFile` too, or Save resurrects it. See
   `TestSaveAfterLoadKeepsReferences`. `Save` writes 0600 inside a 0700
   directory.

4. **A missing config opens setup; a broken config still fails.** `main.run`
   only swallows `fs.ErrNotExist`. A file that exists but does not parse, or
   whose `${VAR}` is unset, exits with the reason — silently overwriting
   somebody's config is worse than an error message.

5. **The config file is written only after the credentials work.**
   `submitSetup` parks the config in `Model.pendingSave` and fires a real API
   call; the `filtersMsg` handler is what writes the file. A token that gets
   rejected leaves nothing on disk.

6. **Issue fields are dynamic.** The field list comes from whatever
   `customFields` the API returns, in the order it returns them. There is no
   struct with hardcoded `State`, `Priority` or `Assignee` — every YouTrack
   instance names its fields differently. Rendering is generic over `$type`
   plus the JSON shape.

7. **Every network call is a `tea.Cmd`.** No I/O inside `Update` or `View`,
   and commands never read `Model` state — `begin()` hands them everything
   they need. `View` is pure: same state, same output.

8. **Any open text input owns every key while it is up.** `onKey` routes to
   `setupKey` and `promptKey` before any global binding, otherwise `q`, `p`,
   `/`, `f`, `o` and `s` would trigger commands instead of typing characters —
   `for: me` alone hits four of them. `forward` gives the prompt the same
   precedence so a query can be pasted.

9. **`Update` forwards every message it does not handle to the active
   sub-model.** Never `return m, nil` as the fallthrough. The bubbles answer
   their own commands, and a whitelist of message types silently swallows
   those answers: bracketed paste arrives as `tea.PasteMsg` rather than key
   presses, and the list runs its filter in a `tea.Cmd` and only applies the
   result when the `FilterMatchesMsg` comes back. Both failures look like "the
   feature does nothing" with no error anywhere. See
   `TestPasteReachesTheSetupForm` and `TestIssueListFilterApplies`.

   Consequence for tests: calling `Update` alone proves nothing about anything
   command-driven. `press()` in `filter_test.go` runs the returned commands and
   feeds their messages back, which is the only way that class of bug shows up.

10. **TLS trust is per-provider, explicit and visible.** Never a global flag,
    never an environment variable read behind the user's back, never a silent
    fallback: trusting one internal host must not weaken the others. `ca_file`
    is the answer to reach for first, `insecure_skip_verify` is the last
    resort, and the two are mutually exclusive. Both live in the config next to
    the provider they affect; the dialog that offers the downgrade says what it
    costs, and the header carries `!insecure` for as long as it is on.

    The setup form prefills the CA field from `NODE_EXTRA_CA_CERTS` /
    `SSL_CERT_FILE` when set. That is a *suggestion on screen*, editable and
    persisted as the `${VAR}` reference — not the same thing as reading the
    variable at connect time. Do not make it the latter.

11. **Watching is session state and is never written back.** `watch:` in the
    config seeds it; `w` toggles it for the session. Turning a background
    poller on and off must not rewrite a file, and `config.Save` — which other
    features do call — has to carry `Provider.Watch` through untouched. See
    `TestWatchToggleDoesNotTouchTheConfig`.

12. **The first poll of a filter only seeds.** `watcher.record` returns nothing
    the first time it sees a filter ID. Without that, launching the program
    announces every issue that already matched, which is noise, not news. The
    same rule is why `w` on a filter drops its `seen` entry: re-watching starts
    from silence.

13. **A background poll never raises a modal.** It sets `watch.failed` and the
    header says so. An error from something the user did not ask for right now
    must not cover what they are reading.

14. **Every watch toggle bumps `watchGen`.** Ticks and results carry the
    generation they were scheduled under and are dropped if it has moved on.
    Without it each toggle leaves another `tea.Tick` chain polling the
    instance forever. See `TestWatchTogglesRetireTheOldTicker`.

15. **Errors go to the dialog, never to the header.** A wrapped TLS error is
    three lines long; the header is one. `errMsg` builds a `*dialog`, which
    owns the keyboard until dismissed.

16. **`overlay` must go through `lipgloss.NewCompositor`.** `Canvas.Compose`
    on a parent layer calls `Layer.Draw`, which paints that layer's own content
    and ignores its children — you get the base alone or the box alone, never
    both. See `TestOverlayDrawsOverBase`.

17. **Inside the dialog, every line carries its own background.** A styled
    string nested in a background style emits an SGR reset that kills the
    background for the rest of that row, so a title or hint rendered by an
    inner style leaves a black band across the box. Style each line directly
    with foreground *and* `dialogBG`, never `outer.Render(inner.Render(…))`.
    Same reason each block is wrapped to its final width before the border
    style sees it. See `TestDialogBoxIsRectangular`.

18. **`internal/ui` does not speak HTTP and `internal/youtrack` does not speak
    lipgloss.** The client returns data; the UI decides color and layout.

## Known ceilings

Deliberate simplifications. Each one has its upgrade path written down. None
of them needs "fixing" before somebody actually complains.

- **Setup only covers the first run.** There is no UI to add a second
  provider, edit an existing one or set `list_fields` — that is hand-editing
  the YAML. Submitting the form writes the whole file, so exposing it on an
  existing config would clobber the other providers. *Upgrade:* make
  `submitSetup` append to `m.cfg.Providers` instead of replacing them, then
  reach the screen from a key on the filters list.

- **Paging is forward-only and has no total.** `m` appends the next page;
  there is no way back and no "showing 50 of 812", because a count is a second
  request. A short page is the only end-of-list signal, which also means a
  filter whose size is an exact multiple of `page_size` offers one empty
  fetch. *Upgrade:* `/api/issuesGetter/count` for the total, if the extra round
  trip is worth it.

- **Provider switching cycles, it does not offer a picker.** `p` advances to
  the next provider. Better than a modal with 2–3 providers, annoying with 8.
  *Upgrade:* a fourth screen reusing the filters' `list.Model`.

- **Favourites are ours, not YouTrack's.** The REST API exposes no
  favourite/pinned flag on `SavedQuery` — `StarWatchFolder` stars *issues*, not
  searches — so `favorites` lives in our config. It records IDs, which buys
  surviving a rename at the cost of readability: `- 145-3` says nothing to
  someone editing the file by hand. *Upgrade:* an `{id, name}` mapping, which
  needs a custom `UnmarshalYAML` to keep reading the two older formats.

- **A raw query is not saved anywhere.** `s` runs it for the session; it is
  gone on restart and cannot be pinned, because `favorites` records IDs and an
  ad-hoc query has none. *Upgrade:* a `queries:` list on the provider, which
  would then need synthetic IDs like the built-ins have.

- **Watching only compares the first page.** A poll fetches `page_size`
  results and diffs those, so an issue that lands outside the first page never
  registers. Fine for a filter sorted newest-first, wrong for one sorted by
  priority. *Upgrade:* sort the watch query by `updated desc` explicitly rather
  than trusting the saved search's own order.

- **Only the active provider is polled.** Switching providers resets the
  watcher — what was seen, what was marked and what was being watched. *Upgrade:*
  a client and watcher per provider, which means keeping them all alive.

- **A watched issue is announced once, ever.** `watcher.seen` only grows within
  a session, so an issue that leaves a filter and comes back — resolved, then
  reopened — never notifies again. That is also what keeps a filter whose order
  churns from re-announcing the same issues. *Upgrade:* age entries out of
  `seen`, which trades the quiet for a second notification on every flap.

- **The notification is fire-and-forget.** No click-to-open, no grouping across
  filters, no rate limit beyond `watch_interval`. Two watched filters gaining
  issues in the same poll produce two popups. *Upgrade:* zenity cannot do
  actionable notifications; that needs `notify-send --action` and a listener.

- **No cache.** Every navigation re-fetches. Fine on a good link, painful over
  VPN. *Upgrade:* measure first; if it hurts, a `map[string]issueCache` with a
  short TTL on `Model` — not a caching layer.

- **A date stored in `SimpleIssueCustomField` renders as a number.** The API
  only distinguishes date from integer via
  `projectCustomField.field.fieldType`, which we do not request.
  `DateIssueCustomField` and `PeriodIssueCustomField` are handled correctly.
  *Upgrade:* add `projectCustomField(field(fieldType(id)))` to the fields spec.

- **The app does not download attachments itself.** It builds the signed URL
  and emits OSC 8; the terminal and browser do the rest. A terminal without
  OSC 8 support (or with mouse tracking left on by another app) cannot click.
  *Upgrade:* a `d` key calling `openInBrowser` on the selected attachment —
  the launcher already exists, what is missing is a cursor inside the detail
  viewport to select one.

- **`o` needs a desktop.** Over SSH or on a headless box `xdg-open` fails; the
  dialog then shows the URL to copy out. *Upgrade:* an OSC 52 clipboard write,
  which works through an SSH session where a browser does not.

- **Descriptions and comments render through glamour at a fixed width.** Wide
  markdown tables overflow. *Upgrade:* none that is cheap; live with it.

- **Permissions of a config we did not write are not checked.** `Save` creates
  0600, but a hand-written 0644 file with a plaintext token draws no warning.
  *Upgrade:* `os.Stat` in `Load` plus a warning when `mode&0o077 != 0`.

- **The setup screen verifies with `savedQueries`, not a dedicated endpoint.**
  A token valid but lacking that permission reads as a bad token. *Upgrade:*
  probe `/api/users/me` first and report the two failures differently.

- **The dialog is the only modal and has one action.** No confirm/cancel pair,
  no stacking, no queue — a second error replaces the first. Enough for the
  errors this program can produce. *Upgrade:* give `dialog` an `actions
  []dialogAction` slice when a second one shows up.

- **Accepting an untrusted certificate does not pin it.** `insecure_skip_verify`
  keeps skipping verification forever, for any certificate that host presents.
  *Upgrade:* store the certificate fingerprint and verify against it, which is
  what accepting once should really mean.

- **`ca_file` is not watched.** A rotated CA bundle is picked up on the next
  launch, not live. Fine; restarting a TUI is cheap.

- **Stale responses are dropped by a generation counter, not cancelled.** A
  provider switch bumps `Model.gen`; the in-flight request still completes and
  its result is discarded. *Upgrade:* a `context.CancelFunc` on `Model`, if
  request volume ever justifies it.

## Commands

Everything goes through [Task](https://taskfile.dev) — `task` on its own lists
the targets.

```sh
task check                    # lint + test, what CI runs
task test                     # go test -race ./...
task cover                    # coverage summary
task build                    # bin/youtrack-bar, skipped when sources are unchanged
task run -- -provider acme    # build then run; `interactive: true` keeps the TUI usable
task fmt tidy clean install
```

`task run` must stay `interactive: true`. Without it Task buffers stdio and the
TUI never gets a raw terminal.
