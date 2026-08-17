# youtrack-bar

A read-only terminal UI for browsing YouTrack issues. Built with
[Bubble Tea v2](https://charm.land/bubbletea/v2).

Pick a saved filter, list issues, open one, read its body, comments and every
custom field the instance defines. Attachments and issue links are OSC 8
hyperlinks — **Ctrl+Click** opens them in your browser.

Nothing is ever written back to YouTrack.

## Install

```sh
go install github.com/omartelo/youtrack-bar@latest
```

## First run

Just run it. With no config file, `youtrack-bar` opens a setup screen asking
for a provider name, the instance URL and a permanent token. It verifies them
against the API and only then writes
`$XDG_CONFIG_HOME/youtrack-bar/config.yml` with `0600` permissions.

Get a token under *Profile → Account Security → New permanent token* with the
`YouTrack` scope — read access is enough.

## Configure by hand

For a second provider, or to pick which fields show on the list, edit the file.
Copy `config.example.yml` to `$XDG_CONFIG_HOME/youtrack-bar/config.yml`:

```yaml
page_size: 50

providers:
  - name: acme
    url: https://acme.youtrack.cloud
    token: ${YOUTRACK_TOKEN_ACME}
    list_fields: [State, Priority, Assignee]
```

`${VAR}` in `token` is expanded from the environment, so the secret can stay
out of the file — the setup screen accepts that form too.

### Instances whose certificate does not verify

Point at the PEM bundle that does trust it:

```yaml
    ca_file: /etc/ssl/certs/corp-ca.pem
```

If you already export one for other tools, reference it instead of copying the
path — the setup screen prefills this from `NODE_EXTRA_CA_CERTS` or
`SSL_CERT_FILE` when either is set:

```yaml
    ca_file: ${NODE_EXTRA_CA_CERTS}
```

If that is not an option, `insecure_skip_verify: true` turns certificate
verification off **for that provider only**. It means anyone able to intercept
the connection gets your token, so the header shows `!insecure` the whole time
it is on. The error dialog offers this with `i` when a certificate fails to
verify, and writes the choice to the config. The two settings are mutually
exclusive.

## Run

```sh
youtrack-bar                       # first provider in the config
youtrack-bar -provider acme        # a specific one
youtrack-bar -config ./config.yml  # a specific config file
```

## Watching filters

`w` on a filter polls it in the background. Watched filters carry a `◉` in the
gutter next to the `★` for pinned ones, and the header counts them. When an
issue turns up that was not there before, you get a desktop notification and
the issue is marked `●` in the list until you open it.

```yaml
watch_interval: 2m     # minimum 30s
notifier: zenity       # or notify-send, or none

providers:
  - name: acme
    watch:
      - 145-3
```

The first poll of a filter only seeds — starting the program never announces
issues that were already there. Nothing about watching is persisted: `watch:`
seeds the session, `w` changes it for that session only, and a restart starts
over. Only the active provider is polled.

## Develop

[Task](https://taskfile.dev) drives everything; run `task` for the full list.

```sh
task check                    # lint + test
task run -- -provider acme    # build and launch
task cover                    # coverage summary
```

## Keys

| Key      | Action                                    |
| -------- | ----------------------------------------- |
| `enter`  | open the selected filter / issue          |
| `esc`    | back one screen                           |
| `↑` `↓`  | move / scroll                             |
| `o`      | open the issue in YouTrack (browser)      |
| `m`      | load the next page of issues              |
| `s`      | type a raw YouTrack query                 |
| `f`      | pin/unpin the selected filter (favourite) |
| `w`      | watch/unwatch the selected filter         |
| `/`      | fuzzy-search the current list             |
| `r`      | reload the current screen                 |
| `p`      | switch to the next provider               |
| `?`      | toggle full help                          |
| `q`      | quit                                      |

On the setup screen: `tab` next field, `enter` verify and save, `ctrl+r`
reveal the token, `ctrl+c` quit. Paste the token with your terminal's usual
shortcut (`ctrl+shift+v`, middle-click, `cmd+v`); `ctrl+v` reads the system
clipboard directly.

`s` and `/` are different things: `s` asks YouTrack a new question, `/` narrows
what is already on screen. `o` works from the issue list and from an open issue. Attachments and links
still go through Ctrl+Click: mouse tracking is deliberately disabled so the
terminal keeps handling OSC 8 hyperlinks. See `CLAUDE.md` for the full invariants and the list
of known ceilings.

## Screens

See [docs/tui-mockup.md](docs/tui-mockup.md).
