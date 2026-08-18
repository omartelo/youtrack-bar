// Package update checks GitHub for a newer release and installs it.
//
// It talks to api.github.com, never to a YouTrack instance, which is why it
// does not live in internal/youtrack. Like that package it only ever issues
// GET requests.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Version is the release this binary was built from. GoReleaser stamps it
// through -ldflags; a `go build` without that says "dev", and a `go install`
// picks up the module version from the build info instead. See BuildVersion.
var Version = "dev"

// repo is where releases come from. Hardcoded on purpose: an update that can
// be pointed somewhere else by a config file is a way to install someone
// else's binary.
const (
	owner = "omartelo"
	repo  = "youtrack-tui"

	latestURL = "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
)

// ErrNoRelease means GitHub answered but has no release to offer — a brand new
// repository, or every release still marked draft.
var ErrNoRelease = errors.New("no published release")

// Release is the subset of the GitHub payload this package acts on.
type Release struct {
	Tag    string  `json:"tag_name"`
	URL    string  `json:"html_url"`
	Assets []Asset `json:"assets"`
}

// Asset is one published file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Newer reports whether rel is a later version than current.
//
// A binary that does not know its own version — a local `go build`, whose
// Version is still "dev" — is never behind. It may well be ahead of the tag,
// and telling somebody to install a release over the code they just compiled
// is worse than saying nothing. `youtrack-tui update` asks explicitly and does
// not go through here.
func (r Release) Newer(current string) bool {
	if !known(current) {
		return false
	}
	return compare(r.Tag, current) > 0
}

// known reports whether a version string is a real vX.Y.Z this package can
// compare.
func known(v string) bool {
	n, _ := parse(v)
	return n != nil
}

// Check asks GitHub for the latest release. Unauthenticated, so it is subject
// to GitHub's per-IP rate limit; a caller that runs this on every launch has
// to treat a failure as "no answer", not as an error worth showing.
func Check(ctx context.Context) (Release, error) {
	return checkAt(ctx, latestURL)
}

// checkAt is Check against an arbitrary endpoint, which is what the tests use.
func checkAt(ctx context.Context, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", repo+"/"+Version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, ErrNoRelease
	default:
		return Release{}, fmt.Errorf("github: %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("github: %w", err)
	}
	if rel.Tag == "" {
		return Release{}, ErrNoRelease
	}
	return rel, nil
}

// compare orders two version strings, returning -1, 0 or 1. Tags are
// vMAJOR.MINOR.PATCH, so this is three integers and a pre-release suffix —
// small enough not to be worth a semver dependency. Anything that does not
// parse sorts below everything else.
func compare(a, b string) int {
	an, apre := parse(a)
	bn, bpre := parse(b)
	switch {
	case an == nil && bn == nil:
		return 0
	case an == nil:
		return -1
	case bn == nil:
		return 1
	}
	for i := range 3 {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	// 1.0.0 is newer than 1.0.0-rc1, and two pre-releases of the same version
	// are ordered lexically. Good enough: this only decides whether to offer
	// an update, and we do not ship pre-releases as `latest`.
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	case apre < bpre:
		return -1
	}
	return 1
}

// parse splits "v1.2.3-rc1" into [1 2 3] and "rc1". It returns a nil slice for
// anything that is not three numbers, "dev" included.
func parse(v string) ([]int, string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, ""
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, ""
		}
		out[i] = n
	}
	return out, pre
}
