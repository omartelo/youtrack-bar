# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `w` now persists the watch list to the config, the way `f` does for
  favourites: what was being watched when you quit is what the next run polls.
  What has already been seen is still session state, so a restart stays quiet.

## [0.2.0] - 2026-08-18

### Changed

- Homebrew, AUR, and release publishing are now driven by GoReleaser's own
  `homebrew_casks` and `aurs` publishers instead of hand-rendered templates.
- Homebrew installs the macOS cask (`brew install --cask
  omartelo/tap/youtrack-tui`); the bare Linux formula is gone, so Linux is
  served by the AUR package and `install.sh`.
- CI runs the suite on Linux, macOS, and Windows, and golangci-lint now enforces
  gofmt plus `bodyclose`, `errorlint`, `nilerr`, and `revive`.

### Fixed

- `TestSaveRoundTrip` no longer asserts a 0600 config on Windows, where Go maps
  a file mode to nothing but the read-only bit.

## [0.1.0] - 2026-08-18

### Added

- Read-only TUI for saved filters, issue lists, descriptions, comments, dynamic
  custom fields, attachments, and OSC 8 links.
- Multi-provider configuration, verified first-run setup, environment-backed
  tokens, custom CA bundles, and explicit per-provider insecure TLS mode.
- Raw queries, favourite filters, pagination, provider switching, desktop
  notifications, and session-only filter watching.
- Release builds for Linux, macOS, and Windows on amd64 and arm64.
- Checksum-verified curl installer, Homebrew tap formula, and `youtrack-tui-bin`
  AUR package.
- GitHub CI with tests, race detection, golangci-lint, and release packaging.

[Unreleased]: https://github.com/omartelo/youtrack-tui/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/omartelo/youtrack-tui/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/omartelo/youtrack-tui/releases/tag/v0.1.0
