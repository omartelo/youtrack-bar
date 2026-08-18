# youtrack-tui

A read-only terminal UI for browsing YouTrack issues. Built with
[Bubble Tea v2](https://charm.land/bubbletea/v2).

Pick a saved filter, list issues, open one, read its body, comments and every
custom field the instance defines. Attachments and issue links are OSC 8
hyperlinks — **Ctrl+Click** opens them in your browser.

Nothing is ever written back to YouTrack.

## Install

Linux, macOS, or Git Bash on Windows:

```sh
curl -fsSL https://raw.githubusercontent.com/omartelo/youtrack-tui/main/install.sh | sh
```

The installer verifies the GitHub release checksum and writes to
`$HOME/.local/bin`. Override that directory with `INSTALL_DIR`.

Homebrew on macOS:

```sh
brew install --cask omartelo/tap/youtrack-tui
```

Arch Linux:

```sh
yay -S youtrack-tui-bin
```

## First run

Just run it. With no config file, `youtrack-tui` opens a setup screen asking
for a provider name, the instance URL and a permanent token. It verifies them
against the API and only then writes
`$XDG_CONFIG_HOME/youtrack-tui/config.yml` with `0600` permissions.

Get a token under *Profile → Account Security → New permanent token* with the
`YouTrack` scope — read access is enough.

## Configure by hand

For a second provider, or to pick which fields show on the list, edit the file.
Copy `config.example.yml` to `$XDG_CONFIG_HOME/youtrack-tui/config.yml`:

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
youtrack-tui                       # first provider in the config
youtrack-tui -provider acme        # a specific one
youtrack-tui -config ./config.yml  # a specific config file
youtrack-tui update                # install the latest release
youtrack-tui -version              # what this binary is
```

## Watching filters

`w` on a filter polls it in the background. Watched filters carry a `◉` in the
gutter next to the `★` for pinned ones, and the header shows `◉ watching N`.
When an issue turns up that was not there before, you get a desktop
notification and the issue is marked `●` in the list until you open it.

```yaml
watch_interval: 2m     # minimum 30s
notifier: zenity       # or notify-send, or none

providers:
  - name: acme
    watch:
      - 145-3
```

The first poll of a filter only seeds — starting the program never announces
issues that were already there, so a restart is quiet even though `w` is
remembered: it writes the config, and `watch:` is what the next run picks up.
Only the active provider is polled.

## Updating

The TUI checks GitHub for a newer release once, when it opens, and says so in
the header. It never installs anything on its own — `youtrack-tui update` does
that:

```sh
youtrack-tui update      # check, then install
youtrack-tui -version    # what this binary is
```

If the binary came from a package manager, `update` runs that manager instead
of overwriting its file — replacing it would leave the manager holding a
version that is not there, and its next upgrade would put the old one back:

| Installed with            | What `update` runs                          |
|---------------------------|---------------------------------------------|
| Homebrew tap              | `brew upgrade --cask omartelo/tap/youtrack-tui` |
| AUR (`youtrack-tui-bin`)  | `paru -S youtrack-tui-bin` (or `yay`)       |
| `install.sh`, tarball, `go install` | replaces the binary in place, after checking its SHA-256 against the release's `checksums.txt` |

The AUR helper runs as you, not as root, and is pointed at `pkexec` for the
install step, so the password is asked for in a polkit dialog rather than at a
prompt underneath the output. What is about to run is printed before it runs.

To stop the startup check:

```yaml
check_updates: false
```

## Develop

[Task](https://taskfile.dev) drives everything; run `task` for the full list.

```sh
task check                    # lint, tests, installer smoke test
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
