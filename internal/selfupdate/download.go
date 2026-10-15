package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Artifact is one file of a release, as the release's own SHA256SUMS names it.
//
// Both fields come from the signed sums and nothing else. That is the whole
// design: the manifest is written by the site and is not signed, so a name or
// a digest taken from it would be a name or a digest whoever serves the site
// chose. The sums are covered by the update-signing key, so a release can only
// offer the files it actually signed, under the digests it actually signed.
type Artifact struct {
	Name   string
	SHA256 string
}

// maxArtifact caps a download. The largest thing this product ships is the
// universal macOS bundle, tens of megabytes; 512 MB is far past anything
// legitimate and stops a hostile or broken channel from filling the disk.
const maxArtifact = 512 << 20

// ArtifactFor picks the file this platform would install from a release's
// signed sums.
//
// It matches on the exact name the release workflow's matrix produces, so a
// channel cannot offer a different file by inventing a name: the name is
// looked up, never taken.
//
//   - Windows installs from the per-user setup executable, which is what
//     makes a silent reinstall possible without elevation.
//   - macOS installs from the zip rather than the dmg: a zip is unpacked with
//     no mounting and no privileges, where a dmg needs hdiutil and leaves a
//     mount to clean up. The dmg is what a person downloads; the zip is what
//     the app swaps.
//   - Linux gets nothing. A .deb or .rpm belongs to the package manager,
//     and this returns the refusal rather than a file.
func ArtifactFor(sums, version, goos, goarch string) (Artifact, error) {
	var want string
	switch goos {
	case "windows":
		want = fmt.Sprintf("agentry-desktop-%s-windows-%s-setup.exe", version, goarch)
	case "darwin":
		want = fmt.Sprintf("agentry-desktop-%s-darwin-universal.zip", version)
	default:
		return Artifact{}, fmt.Errorf("%w: on %s a newer Agentry Desktop is installed by the package manager", ErrNotApplicable, goos)
	}

	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		digest, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if name != want {
			continue
		}
		if len(digest) != 64 {
			return Artifact{}, fmt.Errorf("the release's checksums give %s a digest that is not a SHA-256", name)
		}
		return Artifact{Name: name, SHA256: strings.ToLower(digest)}, nil
	}
	return Artifact{}, fmt.Errorf("the release's checksums name no %s, so this release has nothing for this platform", want)
}

// Download fetches an artifact from the channel and writes it to a file in
// dir, returning the path.
//
// It refuses unless the bytes hash to the digest the signed sums gave, and on
// a mismatch it removes what it wrote before returning: a rejected download
// must not be left on disk where a later step, or a person, could mistake it
// for a release.
//
// The artifact is fetched from beside the manifest, under the same rule the
// sums and the signature are — the channel cannot point this anywhere else.
func Download(ctx context.Context, client *http.Client, manifestURL string, a Artifact, dir string) (path string, err error) {
	base, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("the update channel %q is not a URL: %w", manifestURL, err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	client = withSameOriginRedirects(client, base)

	ref, err := sidecarURL(base, a.Name)
	if err != nil {
		return "", err
	}

	// A name from the signed sums is already one of ours, but it still becomes
	// a path here, so it is reduced to its last element and checked — a defence
	// that costs nothing and would matter if a release ever named a file with a
	// slash in it.
	name := filepath.Base(filepath.Clean("/" + a.Name))
	if name == "." || name == string(filepath.Separator) || name != a.Name {
		return "", fmt.Errorf("the release names %q, which is not a plain file name", a.Name)
	}
	path = filepath.Join(dir, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", a.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: the channel answered %s", a.Name, resp.Status)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	// Removed on every failure below, including a digest mismatch: what this
	// wrote is not a release until it has been proven to be one.
	//
	// written, not the named return: `return "", err` sets path to "" BEFORE
	// the deferred function runs, so a defer that used path would remove ""
	// and leave the rejected bytes on disk. A test holds it.
	written := path
	defer func() {
		if err != nil {
			_ = os.Remove(written)
		}
	}()

	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(resp.Body, maxArtifact+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", a.Name, err)
	}
	if n > maxArtifact {
		return "", fmt.Errorf("%s is larger than %d bytes, which no release of this product is", a.Name, int64(maxArtifact))
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != a.SHA256 {
		return "", fmt.Errorf("%s is not the file this release signed: its digest is %s and the release's checksums say %s", a.Name, got, a.SHA256)
	}
	return path, nil
}

// ThisPlatform names the artifact this build would install, for a caller that
// has a verified release in hand.
func (r *Release) ThisPlatform() (Artifact, error) {
	return ArtifactFor(r.Checksums, r.Version, runtime.GOOS, runtime.GOARCH)
}

// AuthoriseInstall re-runs the whole verification and returns the key that
// authorises installing this release.
//
// Re-running it rather than reading Release.Retired is the point. That field
// was set when the check ran, which may have been a day ago in a process that
// has been asleep; and a boolean a caller has to remember to read is the
// shape a function replaces precisely because it is forgettable.
// Installing is the operation where forgetting it installs something, so the
// question is asked again, of the same bytes, through the call that refuses a
// retired key on its own.
func (r *Release) AuthoriseInstall(keys *KeySet) (Key, error) {
	if r.Checksums == "" || r.Signature == "" {
		return Key{}, errors.New("this release was not carried with its checksums and signature, so it cannot be re-verified before installing")
	}
	return keys.VerifyForInstall([]byte(r.Checksums), []byte(r.Signature))
}
