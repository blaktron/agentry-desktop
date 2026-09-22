package selfupdate_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crypto/sha256"

	"aead.dev/minisign"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

// A channel, as the site serves one: a manifest naming a sums file and a
// signature, and the two files beside it.
type channel struct {
	t        *testing.T
	sums     []byte
	sig      []byte
	manifest map[string]any
	// hits records what was fetched, so a test can assert the check stopped
	// before fetching something it had no business fetching.
	hits []string
}

func newChannel(t *testing.T, priv minisign.PrivateKey, version, commit string, artifacts string) *channel {
	t.Helper()
	sums := []byte(artifacts)
	sig := minisign.SignWithComments(priv, sums, selfupdate.TrustedComment(version, commit), "signed by the agentry-desktop release workflow")
	digest := sha256.Sum256(sums)
	c := &channel{t: t, sums: sums, sig: sig}
	c.manifest = map[string]any{
		"desktop": map[string]any{
			"version":   version,
			"checksums": map[string]any{"file": "agentry-desktop-" + version + "-SHA256SUMS", "size": len(sums), "sha256": hex.EncodeToString(digest[:])},
			"signature": map[string]any{"file": "agentry-desktop-" + version + "-SHA256SUMS.minisig", "size": len(sig)},
		},
	}
	return c
}

func (c *channel) serve() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		c.hits = append(c.hits, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "manifest.json"):
			_ = json.NewEncoder(w).Encode(c.manifest)
		case strings.HasSuffix(r.URL.Path, ".minisig"):
			_, _ = w.Write(c.sig)
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
			_, _ = w.Write(c.sums)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	c.t.Cleanup(srv.Close)
	return srv
}

// check runs against a server, with localhost allowed in place of https.
func check(t *testing.T, srv *httptest.Server, current string, set *selfupdate.KeySet) (*selfupdate.Release, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return selfupdate.Check(ctx, srv.Client(), srv.URL+"/downloads/manifest.json", current, set)
}

const sumsBody = "aa1f56ffcb84076209f4e5cf3eb0282c  agentry-desktop-0.2.0-darwin-universal.dmg\n"

func TestCheckOffersANewerSignedRelease(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	srv := newChannel(t, priv, "0.2.0", "abc1234", sumsBody).serve()

	rel, err := check(t, srv, "0.1.0", set)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rel == nil {
		t.Fatal("no release offered although the channel carries a newer one")
	}
	if rel.Version != "0.2.0" || rel.Commit != "abc1234" || rel.SignedBy != "prod" {
		t.Errorf("offered %+v", rel)
	}
	if rel.Checksums != sumsBody {
		t.Error("the verified checksums were not kept for M7 to use")
	}
}

func TestCheckOffersNothingForTheSameOrAnOlderVersion(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	for _, tc := range []struct{ offered, current string }{
		{"0.2.0", "0.2.0"}, // the same
		{"0.1.0", "0.2.0"}, // older
		{"0.1.0", "0.1.0-11-gabc123"} /* a build already past that tag */, {"1.0.0", "1.0.1"},
	} {
		t.Run(tc.offered+" offered to "+tc.current, func(t *testing.T) {
			srv := newChannel(t, priv, tc.offered, "abc1234", sumsBody).serve()
			rel, err := check(t, srv, tc.current, set)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if rel != nil {
				t.Errorf("offered %s to a build running %s", rel.Version, tc.current)
			}
		})
	}
}

// The heart of it: the manifest is written by the site and is not signed, so
// the version must come from the signed comment. A channel that claims a
// different version from the one its signature covers is refused rather than
// silently believed either way.
func TestCheckRefusesAManifestThatDisagreesWithTheSignature(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
	c.manifest["desktop"].(map[string]any)["version"] = "9.9.9"
	srv := c.serve()

	_, err := check(t, srv, "0.1.0", set)
	if err == nil {
		t.Fatal("a manifest claiming a version its signature does not cover was accepted")
	}
	if !strings.Contains(err.Error(), "refusing to choose between them") {
		t.Errorf("message %q does not say what the disagreement was", err)
	}
}

func TestCheckRefusesTamperedSums(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
	c.sums = []byte(strings.Replace(sumsBody, "aa1f", "bb1f", 1))
	// The manifest's digest is updated too, so this reaches the signature
	// rather than stopping at the cheaper check.
	d := sha256.Sum256(c.sums)
	c.manifest["desktop"].(map[string]any)["checksums"].(map[string]any)["sha256"] = hex.EncodeToString(d[:])
	srv := c.serve()

	_, err := check(t, srv, "0.1.0", set)
	if err == nil {
		t.Fatal("tampered checksums were accepted")
	}
	if !strings.Contains(err.Error(), "not one this build accepts") {
		t.Errorf("message %q does not name the refusal", err)
	}
}

func TestCheckRefusesATamperedSignature(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(c.sig); err != nil {
		t.Fatal(err)
	}
	parsed.TrustedComment = selfupdate.TrustedComment("9.9.9", "deadbee")
	forged, err := parsed.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	c.sig = forged
	srv := c.serve()

	if _, err := check(t, srv, "0.1.0", set); err == nil {
		t.Fatal("a signature whose trusted comment was rewritten was accepted")
	}
}

func TestCheckRefusesAnUnknownKey(t *testing.T) {
	stranger, _ := keystest.Key(t, "stranger", false)
	_, mine := keystest.Key(t, "prod", false)
	set := keystest.Set(t, mine)
	srv := newChannel(t, stranger, "0.2.0", "abc1234", sumsBody).serve()

	_, err := check(t, srv, "0.1.0", set)
	if !errors.Is(err, selfupdate.ErrUnknownKey) {
		t.Fatalf("err = %v, want ErrUnknownKey", err)
	}
}

// A retired key is REPORTED, not refused: the release is genuinely ours and
// the person should know it exists. M7 is what refuses to install from it.
func TestCheckReportsARetiredKey(t *testing.T) {
	priv, entry := keystest.Key(t, "dev", true)
	set := keystest.Set(t, entry)
	srv := newChannel(t, priv, "0.2.0", "abc1234", sumsBody).serve()

	rel, err := check(t, srv, "0.1.0", set)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rel == nil || !rel.Retired {
		t.Fatalf("a release signed by a retired key was not reported as such: %+v", rel)
	}
}

func TestCheckWithNoChannelDoesNothing(t *testing.T) {
	_, entry := keystest.Key(t, "prod", false)
	_, err := selfupdate.Check(context.Background(), nil, "", "0.1.0", keystest.Set(t, entry))
	if !errors.Is(err, selfupdate.ErrNoChannel) {
		t.Fatalf("err = %v, want ErrNoChannel", err)
	}
}

func TestCheckRefusesAChannelThatIsNotHTTPS(t *testing.T) {
	_, entry := keystest.Key(t, "prod", false)
	_, err := selfupdate.Check(context.Background(), nil, "http://downloads.example.test/manifest.json", "0.1.0", keystest.Set(t, entry))
	if err == nil || !strings.Contains(err.Error(), "not https") {
		t.Fatalf("err = %v, want the plain-http channel refused", err)
	}
}

// The manifest is untrusted input, so a file name in it must not be able to
// send the app somewhere else.
func TestCheckRefusesASidecarNameThatLeavesTheChannel(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	for _, name := range []string{
		"../../etc/passwd",
		"https://elsewhere.example.test/SHA256SUMS",
		"/absolute/SHA256SUMS",
		"",
	} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
			c.manifest["desktop"].(map[string]any)["checksums"].(map[string]any)["file"] = name
			srv := c.serve()
			if _, err := check(t, srv, "0.1.0", set); err == nil {
				t.Fatalf("the channel named %q and it was fetched", name)
			}
		})
	}
}

// A channel publishing nothing signed is reported as that, not as "up to
// date". Withholding a release is the one thing a signature cannot prevent,
// so the state a hostile channel would produce has to be visible rather than
// indistinguishable from good news.
func TestCheckOffersNothingWhenTheChannelPublishesNoSignature(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
	delete(c.manifest["desktop"].(map[string]any), "signature")
	srv := c.serve()

	rel, err := check(t, srv, "0.1.0", set)
	if !errors.Is(err, selfupdate.ErrNothingSigned) {
		t.Fatalf("err = %v, want ErrNothingSigned", err)
	}
	if rel != nil {
		t.Fatal("a release was offered with no signature to back it")
	}
	// And nothing beyond the manifest was fetched.
	for _, h := range c.hits {
		if !strings.HasSuffix(h, "manifest.json") {
			t.Errorf("fetched %s although there was no signature to check", h)
		}
	}
}

func TestCheckRefusesASumsFileTheManifestMisdescribes(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
	c.manifest["desktop"].(map[string]any)["checksums"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
	srv := c.serve()

	_, err := check(t, srv, "0.1.0", set)
	if err == nil || !strings.Contains(err.Error(), "not the file the channel describes") {
		t.Fatalf("err = %v, want the mis-described sums refused", err)
	}
	// It stopped before fetching the signature: a cheaper check first.
	for _, h := range c.hits {
		if strings.HasSuffix(h, ".minisig") {
			t.Error("the signature was fetched although the checksums were already wrong")
		}
	}
}

// A slow or dead channel must leave the app alone rather than hanging it.
func TestCheckGivesUpOnASlowChannel(t *testing.T) {
	_, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := selfupdate.Check(ctx, srv.Client(), srv.URL+"/downloads/manifest.json", "0.1.0", set)
	if err == nil {
		t.Fatal("a channel that never answers was not given up on")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("took %s to give up; the check must not hold anything up", elapsed)
	}
}

func TestCheckRefusesAnAnswerThatIsNotAManifest(t *testing.T) {
	_, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>a captive portal, say</html>"))
	}))
	defer srv.Close()

	_, err := selfupdate.Check(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json", "0.1.0", set)
	if err == nil || !strings.Contains(err.Error(), "did not answer with a manifest") {
		t.Fatalf("err = %v, want the non-manifest answer refused", err)
	}
}

func TestTrustedCommentRoundTrips(t *testing.T) {
	s := selfupdate.TrustedComment("0.2.0", "a7d2e603")
	v, c, err := selfupdate.ParseTrustedComment(s)
	if err != nil {
		t.Fatalf("ParseTrustedComment(%q): %v", s, err)
	}
	if v != "0.2.0" || c != "a7d2e603" {
		t.Errorf("round trip gave %q %q", v, c)
	}

	for _, bad := range []string{
		"",
		"agentry-desktop 0.2.0",
		"agentry-desktop 0.2.0 (commit )",
		"agentry 0.2.0 (commit abc1234)",
		"agentry-desktop  (commit abc1234)",
		"signed by the agentry-desktop release workflow",
		"agentry-desktop 0.2.0 (commit abc1234) and something else",
	} {
		if _, _, err := selfupdate.ParseTrustedComment(bad); err == nil {
			t.Errorf("ParseTrustedComment(%q) accepted a comment that is not the format", bad)
		}
	}
}

// The estate signs more than the desktop's releases with this key (the CLI,
// agentry-desktop#171): a comment names its product, the app reads only its
// own, and the general reader gives every product back exactly.
func TestTrustedCommentNamesItsProductAndTheAppReadsOnlyItsOwn(t *testing.T) {
	cli := selfupdate.TrustedCommentFor("agentry-cli", "0.1.0", "abc1234")
	if cli != "agentry-cli 0.1.0 (commit abc1234)" {
		t.Fatalf("TrustedCommentFor(agentry-cli) = %q", cli)
	}
	// The desktop's own comment is byte-identical to what every release
	// before #171 signed: an older build reads a newer release's comment.
	if got := selfupdate.TrustedComment("0.2.0", "a7d2e603"); got != "agentry-desktop 0.2.0 (commit a7d2e603)" {
		t.Errorf("the desktop's comment changed shape: %q", got)
	}
	// The app's update path must never read a CLI release as a desktop one.
	if _, _, err := selfupdate.ParseTrustedComment(cli); err == nil {
		t.Errorf("ParseTrustedComment accepted a CLI comment as the desktop's")
	}
	p, v, c, err := selfupdate.ParseAnyTrustedComment(cli)
	if err != nil || p != "agentry-cli" || v != "0.1.0" || c != "abc1234" {
		t.Errorf("ParseAnyTrustedComment(%q) = %q %q %q %v", cli, p, v, c, err)
	}
	p, v, c, err = selfupdate.ParseAnyTrustedComment(selfupdate.TrustedComment("0.2.0", "a7d2e603"))
	if err != nil || p != "agentry-desktop" || v != "0.2.0" || c != "a7d2e603" {
		t.Errorf("ParseAnyTrustedComment(desktop) = %q %q %q %v", p, v, c, err)
	}
	for _, bad := range []string{
		"Agentry-CLI 0.1.0 (commit abc1234)",
		"agentry cli 0.1.0 (commit abc1234)",
		"agentry-cli (commit abc1234)",
		"agentry-cli 0.1.0 (commit )",
		"",
	} {
		if _, _, _, err := selfupdate.ParseAnyTrustedComment(bad); err == nil {
			t.Errorf("ParseAnyTrustedComment(%q) accepted a comment that is not the format", bad)
		}
	}
}

// The update path itself, not only the reader: a channel serving a CLI
// release's sums, signed by the estate's key with the CLI's comment, is
// refused as another product's before an artifact is chosen or fetched.
func TestCheckRefusesAnotherProductsRelease(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "9.9.9", "abc1234", sumsBody)
	c.sig = minisign.SignWithComments(priv, c.sums, selfupdate.TrustedCommentFor("agentry-cli", "9.9.9", "abc1234"), "signed by the agentry-cli release workflow")
	c.manifest["desktop"].(map[string]any)["signature"].(map[string]any)["size"] = len(c.sig)
	srv := c.serve()

	rel, err := check(t, srv, "0.1.0", set)
	if err == nil || !strings.Contains(err.Error(), "a release of agentry-cli, not of agentry-desktop") {
		t.Fatalf("Check = %+v, %v; want the CLI release refused as another product's", rel, err)
	}
	for _, hit := range c.hits {
		if !strings.HasSuffix(hit, "manifest.json") && !strings.HasSuffix(hit, "SHA256SUMS") && !strings.HasSuffix(hit, ".minisig") {
			t.Errorf("the check fetched %s after refusing the release", hit)
		}
	}
}

func TestIsNewerReadsGitDescribeTheWayGitMeansIt(t *testing.T) {
	// The suffix means "past this tag", not semver's "before it".
	for _, tc := range []struct {
		offered, current string
		want             bool
	}{
		{"0.2.0", "0.1.0", true},
		{"1.0.0", "0.9.9", true},
		{"0.1.1", "0.1.0", true},
		{"0.2.0", "0.1.0-11-g5ac8c0d", true},
		{"0.1.0", "0.1.0-11-g5ac8c0d", false},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"v0.2.0", "0.1.0", true},
		{"0.1.0", "0.1.0-11-g5ac8c0d-dirty", false},
	} {
		got, err := selfupdate.IsNewer(tc.offered, tc.current)
		if err != nil {
			t.Errorf("IsNewer(%q, %q): %v", tc.offered, tc.current, err)
			continue
		}
		if got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.offered, tc.current, got, tc.want)
		}
	}
}

// A build with no tag in its history has no ordering, so it is never told an
// upgrade exists rather than being guessed at.
func TestIsNewerRefusesAVersionItCannotOrder(t *testing.T) {
	for _, tc := range [][2]string{
		{"0.2.0", "5ac8c0d"},
		{"not-a-version", "0.1.0"},
		{"", "0.1.0"},
		{"0.2.0", ""},
	} {
		if _, err := selfupdate.IsNewer(tc[0], tc[1]); err == nil {
			t.Errorf("IsNewer(%q, %q) claimed an ordering it cannot have", tc[0], tc[1])
		}
	}
}

// The host and scheme checks bind the first hop only unless redirects are
// bound too. A channel that answers 302 to another origin must not be
// followed, or every refusal above it means nothing.
func TestCheckDoesNotFollowARedirectOffTheChannel(t *testing.T) {
	_, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the check followed a redirect to another host")
		http.NotFound(w, r)
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusFound)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := selfupdate.Check(ctx, srv.Client(), srv.URL+"/downloads/manifest.json", "0.1.0", set)
	if err == nil {
		t.Fatal("a redirect to another host was followed")
	}
	if !strings.Contains(err.Error(), "not where this build looks") {
		t.Errorf("message %q does not name the redirect", err)
	}
}

// A percent-encoded slash carries no slash to a string check and is still a
// path traversal once a server has decoded it, so containment is checked on
// the resolved path.
func TestCheckRefusesAnEncodedTraversalInASidecarName(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)

	for _, name := range []string{
		"SHA256SUMS%2f..%2fother",
		"%2e%2e%2fSHA256SUMS",
		"SHA256SUMS?x=1",
		"SHA256SUMS#frag",
		"   ",
	} {
		t.Run(name, func(t *testing.T) {
			c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
			c.manifest["desktop"].(map[string]any)["checksums"].(map[string]any)["file"] = name
			srv := c.serve()
			if _, err := check(t, srv, "0.1.0", set); err == nil {
				t.Fatalf("the channel named %q and it was fetched", name)
			}
		})
	}
}

// The manifest states a size as well as a digest; a file that is not the
// length it claims is caught before the signature, with a message that says
// which of the two files was wrong.
func TestCheckRefusesASidecarOfTheWrongLength(t *testing.T) {
	priv, entry := keystest.Key(t, "prod", false)
	set := keystest.Set(t, entry)
	c := newChannel(t, priv, "0.2.0", "abc1234", sumsBody)
	c.manifest["desktop"].(map[string]any)["signature"].(map[string]any)["size"] = 99999
	srv := c.serve()

	_, err := check(t, srv, "0.1.0", set)
	if err == nil || !strings.Contains(err.Error(), "signature is") {
		t.Fatalf("err = %v, want the wrong-length signature refused by name", err)
	}
}
