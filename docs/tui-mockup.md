# TUI mockup

Reference layout. Three screens, one at a time — no split panes, no focus
juggling. Header is always two lines (title + rule), footer is always one
(context help).

Underlined text is an OSC 8 hyperlink: **Ctrl+Click** opens it in the browser.
This is why the app never enables mouse tracking.

## 0 — Setup (first run only)

Shown when the config file does not exist yet. `enter` on the last field
verifies the credentials against the instance; the file is written only if
that call succeeds, 0600, and the app drops straight into Filters.

The token is masked — this screen gets screen-shared. `ctrl+r` reveals it.
Paste works: the terminal's own paste (`ctrl+shift+v`, middle-click, `cmd+v`)
arrives as bracketed paste, and `ctrl+v` reads the system clipboard directly.

```
 youtrack-tui  setup                                                        first run
────────────────────────────────────────────────────────────────────────────────────

  No config file yet — let's make one.
  It will be written to /home/you/.config/youtrack-tui/config.yml with 0600 permissions.

▌ Provider name
    acme
    any label you like — it shows up in the header

  YouTrack URL
    https://acme.youtrack.cloud
    instance root, without /api

  Permanent token
    ••••••••••••
    Profile → Account Security → New permanent token · ${ENV_VAR} works too

  CA certificate (optional)
    ${NODE_EXTRA_CA_CERTS}
    only needed when the certificate does not verify · ${ENV_VAR} works too


  tab next field · enter verify and save · ctrl+r reveal token · ctrl+c quit
```

The CA field comes prefilled when `NODE_EXTRA_CA_CERTS` or `SSL_CERT_FILE` is
exported — a bundle already trusted by other tools is the likeliest answer. It
is a visible, editable suggestion, and what lands in the config is the `${VAR}`
reference, not the path it resolved to.

## 0b — Error dialog

Every failure lands in a modal drawn over the current screen, never squeezed
into the header line. It owns the keyboard until dismissed, so it carries its
own keys and the footer goes quiet. The screen behind it keeps its layout — it
is a lipgloss layer, not a replacement.

An untrusted certificate is the one error with an action attached:

```
 youtrack-tui  sankhya                                                      first run
────────────────────────────────────────────────────────────────────────────────────

  No config file yet — let's make one.
  It will be written to /home/you/.config/youtrack-tui/config.yml with 0600 permissions.

  Provider na╭────────────────────────────────────────────────────────────────────╮
    sankhya  │                                                                    │
    any label│  Certificate not trusted                                           │
             │                                                                    │
  YouTrack UR│  GET /savedQueries: tls: failed to verify certificate: x509:       │
    https://y│  certificate signed by unknown authority                           │
    instance │                                                                    │
             │  No CA on this machine signs this certificate — normal for an      │
▌ Permanent t│  internal instance.                                                │
    •••••••••│                                                                    │
    Profile →│  Clean fix: add `ca_file: /path/to/ca.pem` to this provider in     │
             │  the config.                                                       │
             │  Skipping verification sends your token to whoever answers on      │
             │  that address. It applies to this provider only and stays marked   │
             │  in the header.                                                    │
             │                                                                    │
             │  i  skip verification and retry  ·  esc  dismiss                   │
             │                                                                    │
             ╰────────────────────────────────────────────────────────────────────╯
```

Press `i` and the retry goes through, the choice is written to the config, and
the header carries the downgrade for the rest of the session:

```
 youtrack-tui  sankhya  !insecure                                       pick a filter
```

Anything else is the same box with a plain title and only `esc  dismiss`:

```
             ╭────────────────────────────────────────────────────────────────────╮
             │  Request failed                                                    │
             │  HTTP 401 Unauthorized: token expired (invalid token, or missing   │
             │  permission)                                                       │
             │  esc  dismiss                                                      │
             ╰────────────────────────────────────────────────────────────────────╯
```

## 1 — Filters

Landing screen. Favourites first, then two built-ins, then the user's YouTrack
saved searches in the order the API returned them. `f` pins or unpins the
selected one and writes its ID to the config; `/` fuzzy-filters this list
locally; `s` skips the list entirely and asks YouTrack a question directly.

The star sits in a two-column gutter every row reserves, so pinning something
never shifts the list sideways.

```
 youtrack-tui  acme                                                    pick a filter
────────────────────────────────────────────────────────────────────────────────────

  FILTERS

▌ ★ Sprint 42 — backend
    project: PAY Sprint: {Sprint 42} #Unresolved

  ★ Waiting on me
    for: me State: {Waiting for review}

    My open issues
    for: me #Unresolved

    All unresolved
    #Unresolved

    Reported by me
    reported by: me #Unresolved


  enter open · f pin/unpin · w watch/unwatch · s query YouTrack · / search list · ? help · q quit
```

## 1c — Watching

`w` polls a filter in the background. Each marker gets its own gutter column —
`★` pinned, `◉` watched — so a row never shifts when either is toggled, and the
name is never pushed around by a badge. The header names and counts them —
the only place the glyph gets explained — and a poll that fails turns it orange
rather than raising a modal over what is on screen.

```
 youtrack-tui  acme  ◉ watching 2                                      pick a filter
────────────────────────────────────────────────────────────────────────────────────
   Filters                                                                  5 items

  ★ ◉ Sprint 42 — backend
      project: PAY Sprint: {Sprint 42} #Unresolved

    ◉ Waiting on me
      State: {Waiting for review}

      My open issues
      for: me #Unresolved
```

Failing that, the count says so and stays out of the way:

```
 youtrack-tui  acme  ◉ watching 2 (failed)                             pick a filter
```

An issue that turns up in a watched filter raises one notification per filter
per poll, and is marked `●` in the list until it is opened:

```
   Issues                                                                  3 issues

▌ ● PAY-1421  Checkout retries duplicate the charge
    In Progress · Critical · Ana Souza · updated 2m

    PAY-1388  Cache invalidation misses on blue/green deploy
    Open · Major · Bruno Lima · updated 1d
```

The marker sits in the same two-column gutter the star uses on the filters
screen, so an arrival never shifts the rows around it.

## 1b — Raw query

`s` opens a one-line prompt above the list, seeded with the query on screen so
an existing one can be tweaked rather than retyped. It owns the keyboard while
open — `for: me` contains four command keys.

```
 youtrack-tui  acme                                                    pick a filter
────────────────────────────────────────────────────────────────────────────────────
 query  project: PAY State: Open

  FILTERS

  ★ Sprint 42 — backend
    project: PAY Sprint: {Sprint 42} #Unresolved
    …

  enter  run the query  ·  esc  cancel
```

## 2 — Issues

`page_size` issues from one request. The summary line under each title is
built from `list_fields` in the config — those field names come from your
instance, nothing is hardcoded.

```
 youtrack-tui  acme                                            for: me #Unresolved
────────────────────────────────────────────────────────────────────────────────────

  ISSUES                                                                 23 issues

▌ PAY-1421  Checkout retries duplicate the charge
  In Progress · Critical · Ana Souza · updated 2h

  PAY-1388  Cache invalidation misses on blue/green deploy
  Open · Major · Bruno Lima · updated 1d

  PAY-1355  Flaky test: TestSettlementWindow
  Open · Minor · — · updated 3d

  INF-208   Rotate the staging signing key
  Waiting for review · Major · Ana Souza · updated 5d


  enter open · o open in YouTrack · m load more · s query YouTrack · / search list · esc back
```

`m` appends the next `page_size` issues. It only shows up while a full page came
back — once a short page arrives, the key retires rather than sitting there
doing nothing.

`o` hands the selected issue's URL to the desktop's browser. It works here and
on the detail screen; on a machine with no handler (SSH, headless) the dialog
shows the URL to copy instead.

## 3 — Detail

One scrollable document, blocks stacked in reading order. Fields first because
that is what the YouTrack card shows; every field the API returned is listed,
in API order.

```
 youtrack-tui  acme                                                          PAY-1421
────────────────────────────────────────────────────────────────────────────────────

  PAY-1421  Checkout retries duplicate the charge
  reported by Ana Souza · created 4d · updated 2h
  ──────────────────────────────────────────────────────────────────────────────────

  ▌ Fields
    State        In Progress
    Priority     Critical
    Assignee     Ana Souza
    Type         Bug
    Subsystem    payments-api
    Sprint       Sprint 42
    Due Date     2026-08-22 00:00
    Spent time   3h 20m
    Tags         regression, customer-impact

  ▌ Description
    The retry middleware re-sends the authorization request when the gateway
    answers 504, but the idempotency key is regenerated per attempt, so the
    gateway treats each retry as a new charge.

      1. Pay with a card that triggers a gateway timeout
      2. Watch two authorizations land for one order

  ▌ Attachments
    ◆ gateway-trace.har        412.7 KB
    ◆ duplicate-charge.png      88.1 KB

  ▌ Links
    relates to       PAY-1102  Idempotency keys are not persisted
    is required for  PAY-1500  Q3 payments hardening
    duplicates       PAY-1390  Double charge on timeout

  ▌ Comments (3)
    Bruno Lima  2d
    Reproduced on staging. The key is built inside the retry loop instead of
    outside it.

    Ana Souza  1d
    Patch in review, moving the key generation up one frame.
    ◆ patch-v2.diff              4.2 KB

    Carla Dias  2h
    Confirmed on staging after the fix — single authorization.

  ↑/↓ scroll · o open in YouTrack · esc back · r reload · ? help · q quit
```

## Notes on the layout

- `▌ Section` markers instead of boxes: boxes cost two columns per side and
  break the moment a markdown table renders wider than expected.
- The provider name lives in the header chip. `p` cycles to the next one and
  drops you back on the Filters screen.
- Error and loading state both land on the right side of the header line, so
  no layout shifts when something fails.
- Field labels are padded to the widest name in that issue, so the column
  alignment adapts per issue instead of assuming a schema.
