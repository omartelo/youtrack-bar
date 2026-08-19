// Package config loads and validates the youtrack-tui YAML configuration.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultPageSize is used when page_size is absent or non-positive.
const DefaultPageSize = 50

// Polling defaults for watched filters.
const (
	DefaultWatchInterval = 2 * time.Minute
	MinWatchInterval     = 30 * time.Second
)

// Supported values for `notifier`.
const (
	NotifierZenity     = "zenity"
	NotifierNotifySend = "notify-send"
	NotifierNone       = "none"
)

// Provider is a single YouTrack instance.
type Provider struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	// Token goes through os.ExpandEnv so config files can reference
	// ${YOUTRACK_TOKEN} instead of holding the secret in plain text.
	Token string `yaml:"token"`
	// ListFields names the custom fields shown on the issue list summary
	// line. Field names differ per YouTrack instance, hence the knob.
	ListFields []string `yaml:"list_fields,omitempty"`

	// Favorites are saved-search IDs pinned to the top of the filters screen,
	// in the order they were pinned. YouTrack's REST API exposes no favourite
	// or pinned flag on a SavedQuery, so this is ours and lives here. Names
	// written by an earlier version still work and are rewritten to IDs on the
	// next pin.
	Favorites []string `yaml:"favorites,omitempty"`

	// Watch are the saved-search IDs polled in the background for new issues,
	// in the order they were turned on. Like Favorites it is ours to write:
	// `w` toggles a filter and persists the result, so what you were watching
	// is what you come back to.
	Watch []string `yaml:"watch,omitempty"`

	// Marked are issue IDs the user has ticked off with `x`, in the order they
	// were ticked. Ours like Favorites and Watch, and deliberately without a
	// meaning of its own: reviewed, read, answered, come back later — whatever
	// the person pressing the key decided it means.
	Marked []string `yaml:"marked,omitempty"`

	// CAFile is a PEM bundle to trust on top of the system roots — the right
	// answer for an instance behind a private or corporate CA.
	CAFile string `yaml:"ca_file,omitempty"`

	// Insecure turns off certificate verification for this provider. It is
	// the last resort: anyone able to intercept the connection then gets the
	// token. Deliberately per-provider, so trusting one internal host never
	// weakens the others.
	Insecure bool `yaml:"insecure_skip_verify,omitempty"`

	// RawToken and RawCAFile hold what the file — or the setup form — actually
	// contained, before ${VAR} expansion. Save writes these back, so a
	// reference is never frozen into the value it resolved to. For a token
	// that would mean writing the secret to disk on the next save.
	RawToken  string `yaml:"-"`
	RawCAFile string `yaml:"-"`
}

// Config is the whole config.yml.
type Config struct {
	PageSize int `yaml:"page_size"`

	// WatchInterval is how often watched filters are polled, as a Go duration
	// ("2m", "30s"). Notifier picks the command that puts a notification on
	// screen — that one genuinely differs per machine.
	WatchInterval string `yaml:"watch_interval,omitempty"`
	Notifier      string `yaml:"notifier,omitempty"`

	// WatchEvery is WatchInterval parsed.
	WatchEvery time.Duration `yaml:"-"`

	// CommentsNewestFirst opens an issue with its newest comment on top. The
	// API returns them oldest first, which is a lot of scrolling on a busy
	// issue. `S` on an open issue flips it and writes the file, the same way
	// `f` and `w` persist what they toggle.
	CommentsNewestFirst bool `yaml:"comments_newest_first,omitempty"`

	// CheckUpdates turns the startup release check off. A pointer so that
	// absent means on and `check_updates: false` survives a Save — the zero
	// value of a bool cannot tell those two apart.
	CheckUpdates *bool `yaml:"check_updates,omitempty"`

	Providers []Provider `yaml:"providers"`
}

// ShouldCheckUpdates reports whether to ask GitHub for a newer release on
// startup. On unless the config says otherwise: the check is one request, it
// never blocks anything, and a release nobody hears about is not much of one.
func (c *Config) ShouldCheckUpdates() bool {
	return c == nil || c.CheckUpdates == nil || *c.CheckUpdates
}

// DefaultPath is $XDG_CONFIG_HOME/youtrack-tui/config.yml.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.yml"
	}
	return filepath.Join(dir, "youtrack-tui", "config.yml")
}

// Load reads and validates the config file at path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse unmarshals and validates raw YAML.
func Parse(raw []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate normalizes the config in place and rejects anything unusable.
// Everything crossing this boundary is untrusted: a bad URL here becomes an
// Authorization header sent to an arbitrary host. The setup screen calls this
// on hand-typed input, so the rules live here and not in Parse.
func (c *Config) Validate() error {
	if c.PageSize <= 0 {
		c.PageSize = DefaultPageSize
	}

	c.WatchEvery = DefaultWatchInterval
	if s := strings.TrimSpace(c.WatchInterval); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("`watch_interval`: %w", err)
		}
		// A tighter loop is a self-inflicted denial of service on the
		// instance, not a feature.
		if d < MinWatchInterval {
			return fmt.Errorf("`watch_interval` %s is below the %s minimum", d, MinWatchInterval)
		}
		c.WatchEvery = d
	}

	switch c.Notifier {
	case "", NotifierZenity, NotifierNotifySend, NotifierNone:
	default:
		return fmt.Errorf("`notifier` %q: want %q, %q or %q",
			c.Notifier, NotifierZenity, NotifierNotifySend, NotifierNone)
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("no provider declared under `providers`")
	}

	seen := make(map[string]bool, len(c.Providers))
	for i := range c.Providers {
		p := &c.Providers[i]
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			return fmt.Errorf("provider #%d: `name` is required", i+1)
		}
		if seen[p.Name] {
			return fmt.Errorf("provider %q: duplicated name", p.Name)
		}
		seen[p.Name] = true

		p.URL = strings.TrimRight(strings.TrimSpace(p.URL), "/")
		u, err := url.Parse(p.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("provider %q: `url` must be an absolute http(s) URL, got %q", p.Name, p.URL)
		}

		p.RawToken = strings.TrimSpace(p.Token)
		p.Token = strings.TrimSpace(os.ExpandEnv(p.RawToken))
		if p.Token == "" {
			return fmt.Errorf("provider %q: `token` is empty (unset environment variable?)", p.Name)
		}

		blank := func(f string) bool { return strings.TrimSpace(f) == "" }
		p.Favorites = slices.DeleteFunc(p.Favorites, blank)
		p.Watch = slices.DeleteFunc(p.Watch, blank)
		p.Marked = slices.DeleteFunc(p.Marked, blank)

		p.RawCAFile = strings.TrimSpace(p.CAFile)
		p.CAFile = strings.TrimSpace(os.ExpandEnv(p.RawCAFile))
		if p.CAFile != "" && p.Insecure {
			return fmt.Errorf("provider %q: set `ca_file` or `insecure_skip_verify`, not both", p.Name)
		}
	}
	return nil
}

// Save writes the config as YAML. The file holds a token, so it is created
// 0600 inside a 0700 directory and written atomically.
func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	// Persist references, not what they expanded to.
	out := *c
	out.Providers = slices.Clone(c.Providers)
	for i := range out.Providers {
		p := &out.Providers[i]
		if p.RawToken != "" {
			p.Token = p.RawToken
		}
		if p.RawCAFile != "" {
			p.CAFile = p.RawCAFile
		}
	}

	raw, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find returns the index of the named provider, or -1.
func (c *Config) Find(name string) int {
	for i, p := range c.Providers {
		if p.Name == name {
			return i
		}
	}
	return -1
}
