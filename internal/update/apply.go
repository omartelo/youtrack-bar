package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// maxAsset caps what will be pulled into memory. The archive has to be read
// twice — once to hash it, once to unpack it — and a release tarball is a few
// megabytes, so it is held in a buffer rather than spooled to disk.
const maxAsset = 64 << 20

// binaryName is what the archive is expected to contain.
var binaryName = func() string {
	if runtime.GOOS == "windows" {
		return repo + ".exe"
	}
	return repo
}()

// Apply downloads the release's archive for this platform, verifies it against
// the release's checksums.txt, and replaces the running binary with rollback
// on failure.
//
// Callers must have ruled out a package-managed install first: see
// DetectManager. Overwriting a file Homebrew or pacman owns is what this is
// meant to avoid.
func Apply(ctx context.Context, rel Release) error {
	asset, err := rel.assetFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	archive, err := download(ctx, asset.URL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", asset.Name, err)
	}

	// The checksum file is published alongside the archives; without it the
	// download is unverified, which is not a trade worth making for a binary
	// that is about to replace this process.
	sums, err := rel.assetNamed("checksums.txt")
	if err != nil {
		return err
	}
	raw, err := download(ctx, sums.URL)
	if err != nil {
		return fmt.Errorf("downloading checksums.txt: %w", err)
	}
	want, err := checksumFor(string(raw), asset.Name)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s: refusing to install it", asset.Name)
	}

	bin, err := extract(archive, binaryName)
	if err != nil {
		return err
	}

	// selfupdate writes next to the target and renames over it, keeping the
	// old file until the new one is in place — which is also the only way this
	// works on Windows, where the running executable cannot be unlinked.
	if err := selfupdate.Apply(bytes.NewReader(bin), selfupdate.Options{}); err != nil {
		if rollback := selfupdate.RollbackError(err); rollback != nil {
			return fmt.Errorf("update failed and the old binary could not be restored: %w", rollback)
		}
		return err
	}
	return nil
}

// assetFor picks the archive for a platform. Names come from GoReleaser's
// name_template: youtrack-tui-0.3.0-linux-amd64.tar.gz.
func (r Release) assetFor(goos, goarch string) (Asset, error) {
	suffix := fmt.Sprintf("-%s-%s.tar.gz", goos, goarch)
	for _, a := range r.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s has no build for %s/%s", r.Tag, goos, goarch)
}

func (r Release) assetNamed(name string) (Asset, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s is missing %s", r.Tag, name)
}

// checksumFor reads one line out of a `sha256  filename` listing.
func checksumFor(listing, name string) (string, error) {
	for line := range strings.Lines(listing) {
		sum, file, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		// The separator is two spaces, and binary mode adds a `*` marker.
		if strings.TrimLeft(strings.TrimSpace(file), "*") == name {
			return sum, nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", repo+"/"+Version)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxAsset))
}

// extract pulls one file out of a .tar.gz. The archive also carries the
// licence and readme, which are of no use here.
func extract(archive []byte, want string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || path.Base(hdr.Name) != want {
			continue
		}
		bin, err := io.ReadAll(io.LimitReader(tr, maxAsset))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", want, err)
		}
		return bin, nil
	}
	return nil, fmt.Errorf("archive does not contain %s", want)
}
