package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The setup screen persists what the user typed, so a ${VAR} reference must
// survive the round trip instead of being frozen into a plaintext secret.
func TestSaveRoundTrip(t *testing.T) {
	t.Setenv("YT_TOKEN", "perm:secret")
	path := filepath.Join(t.TempDir(), "nested", "config.yml")

	in := &Config{PageSize: 25, Providers: []Provider{{
		Name: "acme", URL: "https://acme.youtrack.cloud", Token: "${YT_TOKEN}",
	}}}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "perm:secret") {
		t.Error("Save wrote the resolved secret instead of the ${VAR} reference")
	}

	// POSIX only, on purpose. Confidentiality on Windows is an ACL, and Go's
	// os package maps a mode to nothing there but the read-only bit, so
	// WriteFile(0600) lands as 0666 however the file was created. The
	// secret-leak check above is the part of the invariant that holds
	// everywhere.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("file mode = %o, want 600 — it holds a token", mode)
		}
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.PageSize != 25 || out.Providers[0].Token != "perm:secret" {
		t.Errorf("round trip lost data: %+v", out)
	}
}

func TestParse(t *testing.T) {
	t.Setenv("YT_TOKEN", "perm:secret")

	c, err := Parse([]byte(`
providers:
  - name: acme
    url: https://acme.youtrack.cloud/
    token: ${YT_TOKEN}
    list_fields: [State, Priority]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := c.Providers[0].Token; got != "perm:secret" {
		t.Errorf("token not expanded from env: %q", got)
	}
	if got := c.Providers[0].URL; got != "https://acme.youtrack.cloud" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
	if c.PageSize != DefaultPageSize {
		t.Errorf("PageSize = %d, want default %d", c.PageSize, DefaultPageSize)
	}
	if c.Find("acme") != 0 || c.Find("nope") != -1 {
		t.Error("Find is wrong")
	}
}

// Accepting an untrusted certificate rewrites an existing config. If Save used
// the in-memory values, that rewrite would replace every ${VAR} with what it
// resolved to — dumping the token into the file in plain text.
func TestSaveAfterLoadKeepsReferences(t *testing.T) {
	t.Setenv("YT_TOKEN", "perm:secret")
	t.Setenv("YT_CA", "/etc/ssl/certs/corp-ca.pem")
	path := filepath.Join(t.TempDir(), "config.yml")

	if err := os.WriteFile(path, []byte(
		"providers:\n  - name: acme\n    url: https://acme.youtrack.cloud\n"+
			"    token: ${YT_TOKEN}\n    ca_file: ${YT_CA}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Providers[0].Token != "perm:secret" || c.Providers[0].CAFile != "/etc/ssl/certs/corp-ca.pem" {
		t.Fatalf("not expanded in memory: %+v", c.Providers[0])
	}

	// What the trust dialog does before saving.
	c.Providers[0].CAFile, c.Providers[0].RawCAFile = "", ""
	c.Providers[0].Insecure = true
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(got, "perm:secret") {
		t.Errorf("Save leaked the resolved token into the file:\n%s", got)
	}
	if !strings.Contains(got, "${YT_TOKEN}") {
		t.Errorf("Save dropped the ${VAR} reference:\n%s", got)
	}
	if strings.Contains(got, "ca_file") {
		t.Errorf("cleared ca_file came back:\n%s", got)
	}
	if !strings.Contains(got, "insecure_skip_verify: true") {
		t.Errorf("the accepted downgrade was not persisted:\n%s", got)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"no providers":   `page_size: 10`,
		"missing name":   "providers:\n  - url: https://x.io\n    token: t\n",
		"relative url":   "providers:\n  - name: a\n    url: acme.youtrack.cloud\n    token: t\n",
		"empty token":    "providers:\n  - name: a\n    url: https://x.io\n    token: ${UNSET_ON_PURPOSE}\n",
		"duplicate name": "providers:\n  - {name: a, url: 'https://x.io', token: t}\n  - {name: a, url: 'https://y.io', token: t}\n",
		"bogus scheme":   "providers:\n  - name: a\n    url: ftp://x.io\n    token: t\n",
	}
	for name, yml := range cases {
		if _, err := Parse([]byte(yml)); err == nil {
			t.Errorf("%s: expected error, got none", name)
		}
	}
}

// `check_updates: false` has to survive a Save, or the startup request comes
// back on the next write — and the config is written by several features.
func TestSaveKeepsTheUpdateCheckOff(t *testing.T) {
	c, err := Parse([]byte(`
check_updates: false
providers:
  - name: acme
    url: https://acme.youtrack.cloud
    token: perm:tok
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.ShouldCheckUpdates() {
		t.Fatal("check_updates: false was not read")
	}

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.ShouldCheckUpdates() {
		t.Error("saving the config turned the update check back on")
	}
}
