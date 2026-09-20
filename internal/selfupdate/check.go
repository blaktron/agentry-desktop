package selfupdate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"crypto/sha256"
)

// ManifestURL is the update channel this build was made for: the downloads
// manifest of the site that serves it, stamped at build time beside the
// default registry URL
// (-X github.com/blaktron/agentry-desktop/internal/selfupdate.ManifestURL=…).
//
// Empty in a build of the open core, which anyone can make and which has no
// channel to check — Settings says so and nothing is fetched. A self-hosted
// registry's operator serves their own manifest and builds against it; GitHub
// Releases is an archive, not a channel (plan desktop-installers.md §7, D5).
var ManifestURL = ""

// ErrNoChannel means this build has no update channel, which is a state to
// report rather than a failure to retry.
var ErrNoChannel = errors.New("this build has no update channel, so it cannot check for a newer Agentry Desktop")

// Release is a release the channel is offering, as far as the signature
// proves it. Version and Commit are covered by the estate's key; the rest
// describes the key that covered them.
type Release struct {
	// Version and Commit come from the signed trusted comment, never from the
	// manifest — see release.go for why that distinction is the whole point.
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// SignedBy names the key whose signature carried them.
	SignedBy string `json:"signedBy"`
	// Retired is true when that key has been replaced. The release is still
	// genuinely ours; M7 will refuse to install from it.
	Retired bool `json:"retired"`
	// Checksums is the release's SHA256SUMS, verified, kept so that M7 can
	// check a downloaded artifact against it without fetching it again.
	Checksums string `json:"-"`
}

// Manifest is the part of the downloads manifest an update check reads.
// Nothing in it stands on its own: it says where to look, and the signature
// says what is true.
type Manifest struct {
	Desktop struct {
		Version   string `json:"version"`
		Checksums *struct {
			File   string `json:"file"`
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		} `json:"checksums"`
		Signature *struct {
			File string `json:"file"`
			Size int64  `json:"size"`
		} `json:"signature"`
	} `json:"desktop"`
}

// maxSidecar caps what the check will read from the channel. A SHA256SUMS for
// this product is a few hundred bytes and a signature is about three hundred;
// a megabyte is far past anything legitimate and stops a hostile or broken
// server from making the app read until it runs out of memory.
const maxSidecar = 1 << 20

// Check asks the channel whether there is a newer Agentry Desktop, and
// returns it only when the estate's own signature says so.
//
// The order is the point, and it is the plan's §3 invariant: nothing is
// believed until the signature over it verifies. The manifest is fetched
// first because it says where the sums and the signature are, but nothing it
// claims — least of all the version — is used until those two have been
// fetched and checked against the key set compiled into this build.
//
// A nil result with a nil error means the channel has nothing newer.
func Check(ctx context.Context, client *http.Client, manifestURL, current string, keys *KeySet) (*Release, error) {
	if strings.TrimSpace(manifestURL) == "" {
		return nil, ErrNoChannel
	}
	base, err := url.Parse(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("the update channel %q is not a URL: %w", manifestURL, err)
	}
	if base.Scheme != "https" && base.Hostname() != "localhost" && base.Hostname() != "127.0.0.1" {
		// An update channel over plain http would let anyone on the path
		// choose which release is offered. They could not forge the
		// signature, but they could withhold a newer one indefinitely, which
		// is enough to keep someone on a version with a known problem.
		return nil, fmt.Errorf("the update channel %q is not https", manifestURL)
	}
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	var m Manifest
	body, err := fetch(ctx, client, base.String())
	if err != nil {
		return nil, fmt.Errorf("reading the update channel: %w", err)
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("the update channel did not answer with a manifest: %w", err)
	}
	if m.Desktop.Checksums == nil || m.Desktop.Signature == nil {
		// The site publishes both or neither (agentry-web#154). Neither means
		// there is nothing signed to offer, which is not an error to show.
		return nil, nil
	}

	sums, err := fetchSidecar(ctx, client, base, m.Desktop.Checksums.File)
	if err != nil {
		return nil, fmt.Errorf("reading the release's checksums: %w", err)
	}
	// The manifest's digest of the sums is checked for what it is worth: it
	// catches a truncated or mis-served file early, with a clearer message
	// than a signature failure would give. It proves nothing on its own — the
	// manifest is unsigned — which is why the signature is checked too.
	if want := strings.ToLower(strings.TrimSpace(m.Desktop.Checksums.SHA256)); want != "" {
		got := sha256.Sum256(sums)
		if hex.EncodeToString(got[:]) != want {
			return nil, fmt.Errorf("the release's checksums are not the file the channel describes")
		}
	}
	sig, err := fetchSidecar(ctx, client, base, m.Desktop.Signature.File)
	if err != nil {
		return nil, fmt.Errorf("reading the release's signature: %w", err)
	}

	key, err := keys.Verify(sums, sig)
	if err != nil {
		return nil, fmt.Errorf("the release the channel offers is not one this build accepts: %w", err)
	}
	comment, err := trustedComment(sig)
	if err != nil {
		return nil, err
	}
	version, commit, err := ParseTrustedComment(comment)
	if err != nil {
		return nil, err
	}
	// The manifest said a version too. It is unsigned, so the signed one wins;
	// a disagreement means the channel is serving a manifest that does not
	// match its own release, which is worth refusing rather than papering over.
	if mv := strings.TrimSpace(m.Desktop.Version); mv != "" && mv != version {
		return nil, fmt.Errorf("the channel names version %s but the signature covers %s; refusing to choose between them", mv, version)
	}

	newer, err := IsNewer(version, current)
	if err != nil {
		return nil, err
	}
	if !newer {
		return nil, nil
	}
	return &Release{
		Version:   version,
		Commit:    commit,
		SignedBy:  key.Name,
		Retired:   key.Retired,
		Checksums: string(sums),
	}, nil
}

// fetchSidecar resolves a file the manifest names, relative to the manifest
// itself, and refuses a name that would leave the channel's directory. The
// manifest is unsigned input, so a name in it that climbed to another host or
// another path would be the manifest choosing what this app fetches.
func fetchSidecar(ctx context.Context, client *http.Client, base *url.URL, name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("the channel names no file")
	}
	if strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return nil, fmt.Errorf("the channel names %q, which is not a plain file name beside the manifest", name)
	}
	ref, err := base.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("the channel names %q, which is not a usable file name: %w", name, err)
	}
	if ref.Host != base.Host || ref.Scheme != base.Scheme {
		return nil, fmt.Errorf("the channel names %q, which is not on the channel's own host", name)
	}
	return fetch(ctx, client, ref.String())
}

func fetch(ctx context.Context, client *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", u, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSidecar))
}

// trustedComment pulls the trusted comment line out of a signature file. The
// signature over it has already been checked by Verify; this only reads it.
func trustedComment(sig []byte) (string, error) {
	for _, line := range strings.Split(string(sig), "\n") {
		if after, ok := strings.CutPrefix(line, "trusted comment: "); ok {
			return strings.TrimRight(after, "\r"), nil
		}
	}
	return "", errors.New("the release's signature carries no trusted comment, so it says which release it is nowhere this build can check")
}
