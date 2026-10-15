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
	"path"
	"strings"
	"time"

	"crypto/sha256"

	"aead.dev/minisign"
)

// ManifestURL is the update channel this build was made for: the downloads
// manifest of the site that serves it, stamped at build time beside the
// default registry URL
// (-X github.com/blaktron/agentry-desktop/internal/selfupdate.ManifestURL=…).
//
// Empty in a build of the open core, which anyone can make and which has no
// channel to check — Settings says so and nothing is fetched. A self-hosted
// registry's operator serves their own manifest and builds against it; GitHub
// Releases is an archive, not a channel.
var ManifestURL = ""

// ErrNoChannel means this build has no update channel, which is a state to
// report rather than a failure to retry.
var ErrNoChannel = errors.New("this build has no update channel, so it cannot check for a newer Agentry Desktop")

// ErrNothingSigned means the channel answered but published no signed
// release. Ordinarily that is simply a site with nothing to offer yet; it is
// also indistinguishable from a channel withholding one, which is why it is
// reported rather than treated as "up to date".
var ErrNothingSigned = errors.New("the update channel publishes no signed release")

// ErrNotApplicable means this platform does not install a newer Agentry
// Desktop from inside the app. On Linux that is permanent: a .deb or .rpm
// belongs to the package manager, and an app that overwrote a file dpkg owns
// would leave the package database describing something that is no longer
// there.
var ErrNotApplicable = errors.New("this platform does not install a newer Agentry Desktop from inside the app")

// Release is a release the channel is offering, as far as the signature
// proves it. Version and Commit are covered by the update-signing key; the rest
// describes the key that covered them.
type Release struct {
	// Version and Commit come from the signed trusted comment, never from the
	// manifest — see release.go for why that distinction is the whole point.
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// SignedBy names the key whose signature carried them.
	SignedBy string `json:"signedBy"`
	// Retired is true when that key has been replaced. The release is still
	// genuinely ours; installing refuses it.
	Retired bool `json:"retired"`
	// Checksums is the release's SHA256SUMS, verified, kept so that the
	// install can check a downloaded artifact against it without fetching it
	// again.
	Checksums string `json:"-"`
	// Signature is the signature over those checksums, kept for the same
	// reason and for a better one: installing re-runs the verification rather
	// than trusting Retired, a field set when the check ran. See
	// AuthoriseInstall.
	Signature string `json:"-"`
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
// returns it only when Agentry's own signature says so.
//
// The order is the point, and it is the invariant: nothing is
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
	devLoopback := devLoopbackChannel && isLoopback(base.Hostname())
	if base.Scheme != "https" && !devLoopback {
		// An update channel over plain http would let anyone on the path
		// choose which release is offered. They could not forge the
		// signature, but they could withhold a newer one indefinitely, which
		// is enough to keep someone on a version with a known problem. A
		// shipped build makes no exception, loopback included; only a build
		// with the agentry_dev_loopback tag accepts http on this machine
		// (loopback_dev.go).
		return nil, fmt.Errorf("the update channel %q is not https", manifestURL)
	}
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	// Redirects are followed only within the channel's own origin. Without
	// this, every check below binds the FIRST hop and nothing else: a 302 from
	// the channel would send the app to any host, over any scheme, and the
	// careful refusals above would have bound nothing at all.
	client = withSameOriginRedirects(client, base)

	var m Manifest
	body, err := fetch(ctx, client, base.String())
	if err != nil {
		return nil, fmt.Errorf("reading the update channel: %w", err)
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("the update channel did not answer with a manifest: %w", err)
	}
	if m.Desktop.Checksums == nil || m.Desktop.Signature == nil {
		// The site publishes both or neither, so neither normally means
		// there is nothing signed yet — not an error.
		//
		// It is also what a hostile channel would serve to keep someone on an
		// old version for ever: withholding is the one thing a signature
		// cannot prevent, here and in every update channel that has ever
		// existed. ErrNothingSigned rather than a silent "up to date" is what
		// this package can do about it — Settings shows the state, so a
		// channel that has gone quiet is visible instead of indistinguishable
		// from good news.
		return nil, ErrNothingSigned
	}

	sums, err := fetchSidecar(ctx, client, base, m.Desktop.Checksums.File)
	if err != nil {
		return nil, fmt.Errorf("reading the release's checksums: %w", err)
	}
	// The manifest's digest of the sums is checked for what it is worth: it
	// catches a truncated or mis-served file early, with a clearer message
	// than a signature failure would give. It proves nothing on its own — the
	// manifest is unsigned — which is why the signature is checked too.
	if err := describes(sums, m.Desktop.Checksums.Size, m.Desktop.Checksums.SHA256, "checksums"); err != nil {
		return nil, err
	}
	sig, err := fetchSidecar(ctx, client, base, m.Desktop.Signature.File)
	if err != nil {
		return nil, fmt.Errorf("reading the release's signature: %w", err)
	}
	if err := describes(sig, m.Desktop.Signature.Size, "", "signature"); err != nil {
		return nil, err
	}

	key, err := keys.Verify(sums, sig)
	if err != nil {
		return nil, fmt.Errorf("the release the channel offers is not one this build accepts: %w", err)
	}
	// The library already parsed the comment while verifying its signature;
	// reading the file a second time here would be a second parser to
	// disagree with the first, which is how a verifier and its caller end up
	// believing different things about the same bytes.
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(sig); err != nil {
		return nil, fmt.Errorf("the release's signature verified but could not be read back: %w", err)
	}
	version, commit, err := ParseTrustedComment(parsed.TrustedComment)
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
		Signature: string(sig),
	}, nil
}

// fetchSidecar resolves a file the manifest names, relative to the manifest
// itself, and refuses a name that would leave the channel's directory. The
// manifest is unsigned input, so a name in it that climbed to another host or
// another path would be the manifest choosing what this app fetches.
func fetchSidecar(ctx context.Context, client *http.Client, base *url.URL, name string) ([]byte, error) {
	ref, err := sidecarURL(base, name)
	if err != nil {
		return nil, err
	}
	return fetch(ctx, client, ref.String())
}

// sidecarURL resolves a file name the channel gave, beside the manifest, and
// refuses anything that would reach elsewhere. Shared by the sums, the
// signature and the downloaded artifact so that one rule governs every fetch
// this package makes.
func sidecarURL(base *url.URL, name string) (*url.URL, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("the channel names no file")
	}
	ref, err := base.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("the channel names %q, which is not a usable file name: %w", name, err)
	}
	// Containment is checked on the DECODED path, and the decoded path must
	// already be clean. Neither half is optional, and the reason is worth
	// keeping because it is not obvious:
	//
	// Go resolves a reference against the ESCAPED path, so "%2e%2e%2fSUMS"
	// stays one literal segment here and looks perfectly contained — while
	// the server that receives it decodes it to "../SUMS" and serves
	// something else. Comparing directories on the escaped form therefore
	// proves nothing about what will actually be fetched. Comparing on the
	// decoded form (url.Path) sees "/downloads/../SUMS", whose cleaned
	// directory is "/", and refuses it.
	//
	// Requiring the decoded path to equal its own cleaned form then refuses
	// anything that needed normalising at all — "SUMS%2f..%2fother" resolves
	// inside the directory but is not the file it appears to name, and a
	// manifest has no reason to name a file that way.
	if ref.Scheme != base.Scheme || ref.Host != base.Host {
		return nil, fmt.Errorf("the channel names %q, which is not on the channel's own host", name)
	}
	if ref.RawQuery != "" || ref.Fragment != "" {
		return nil, fmt.Errorf("the channel names %q, which carries a query or a fragment; a file beside the manifest has neither", name)
	}
	if ref.Path != path.Clean(ref.Path) || path.Dir(ref.Path) != path.Dir(base.Path) {
		return nil, fmt.Errorf("the channel names %q, which is not a plain file beside the manifest", name)
	}
	return ref, nil
}

// describes checks a fetched file against the size and digest the manifest
// states for it. The manifest is unsigned, so this proves nothing about who
// made the file — the signature does that. What it catches is the ordinary
// failure the signature would report as a mystery: a truncated download, a
// proxy's error page served with a 200, a stale file behind a cache.
func describes(body []byte, size int64, sha string, what string) error {
	if size > 0 && int64(len(body)) != size {
		return fmt.Errorf("the release's %s is %d bytes and the channel describes %d", what, len(body), size)
	}
	if want := strings.ToLower(strings.TrimSpace(sha)); want != "" {
		got := sha256.Sum256(body)
		if hex.EncodeToString(got[:]) != want {
			return fmt.Errorf("the release's %s is not the file the channel describes", what)
		}
	}
	return nil
}

// withSameOriginRedirects returns a copy of client that refuses any redirect
// leaving the channel's origin. A copy, because the caller's client is not
// this package's to reconfigure.
func withSameOriginRedirects(client *http.Client, base *url.URL) *http.Client {
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("the update channel redirected too many times")
		}
		if req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
			return fmt.Errorf("the update channel redirected to %s://%s, which is not where this build looks", req.URL.Scheme, req.URL.Host)
		}
		return nil
	}
	return &c
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
