package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A development build must never look older than a release: `go build` stamps
// nothing, and nagging someone who is running ahead of the tag is noise.
func TestDevBuildIsNeverBehind(t *testing.T) {
	rel := Release{Tag: "v9.9.9"}
	if rel.Newer("dev") {
		t.Error("a dev build was offered an update")
	}
	if rel.Newer("") {
		t.Error("an unstamped build was offered an update")
	}
}

func TestNewerOrdersVersions(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v0.3.0", "v0.2.0", true},
		{"v0.2.1", "v0.2.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true}, // not a string comparison
		{"v0.2.0", "v0.2.0", false},
		{"v0.2.0", "v0.3.0", false},
		{"v0.2.0", "0.2.0", false},     // the v prefix is optional
		{"v1.0.0", "v1.0.0-rc1", true}, // a release beats its pre-release
		{"v1.0.0-rc1", "v1.0.0", false},
		{"garbage", "v0.2.0", false},
		// A local `go build` records a pseudo-version in the build info. It
		// parses as a pre-release of a version nobody published, so it must
		// not be taken as this binary's version — see init in run.go.
		{"v0.2.0", "v0.2.1-0.20260818201619-543769376f58+dirty", false},
	}
	for _, c := range cases {
		if got := (Release{Tag: c.tag}).Newer(c.current); got != c.want {
			t.Errorf("Release{%q}.Newer(%q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}

func TestCheckReadsTheRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://example.test/r",
			"assets":[{"name":"youtrack-tui-0.3.0-linux-amd64.tar.gz","browser_download_url":"https://example.test/a"}]}`))
	}))
	defer srv.Close()

	rel, err := checkAt(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.3.0" || len(rel.Assets) != 1 {
		t.Fatalf("release = %+v", rel)
	}
	a, err := rel.assetFor("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if a.URL != "https://example.test/a" {
		t.Errorf("asset URL = %q", a.URL)
	}
	if _, err := rel.assetFor("linux", "riscv64"); err == nil {
		t.Error("a platform with no build resolved to an asset")
	}
}

// A repository with no releases is not an error worth showing at startup.
func TestCheckReportsNoRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := checkAt(context.Background(), srv.URL); !errors.Is(err, ErrNoRelease) {
		t.Errorf("err = %v, want ErrNoRelease", err)
	}
}

func TestChecksumForPicksTheRightLine(t *testing.T) {
	listing := "aaa  youtrack-tui-0.3.0-darwin-arm64.tar.gz\n" +
		"bbb  youtrack-tui-0.3.0-linux-amd64.tar.gz\n"
	got, err := checksumFor(listing, "youtrack-tui-0.3.0-linux-amd64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bbb" {
		t.Errorf("checksum = %q, want bbb", got)
	}
	if _, err := checksumFor(listing, "youtrack-tui-0.3.0-windows-amd64.tar.gz"); err == nil {
		t.Error("a missing entry did not error")
	}
}

// The version taken from the build info is the one `go install ...@v0.3.0`
// records, and nothing else.
func TestOnlyACleanTagCountsAsAVersion(t *testing.T) {
	for _, v := range []string{"v0.3.0", "0.3.0"} {
		if n, pre := parse(v); n == nil || pre != "" {
			t.Errorf("%q was rejected as a version", v)
		}
	}
	for _, v := range []string{
		"v0.2.1-0.20260818201619-543769376f58+dirty", // a local go build
		"(devel)",
		"dev",
		"",
	} {
		n, pre := parse(v)
		if n != nil && pre == "" {
			t.Errorf("%q was accepted as a released version", v)
		}
	}
}
