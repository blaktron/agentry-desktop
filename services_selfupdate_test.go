package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
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

// These drive the bound surface rather than the checker underneath it: the
// toggle's persistence, that a check never blocks the caller, and that a
// verification failure reaches the person as a sentence instead of silence.

// configHome points the settings file at a temporary directory so a test never
// touches the machine's own.
func configHome(t *testing.T) {
	t.Helper()
	t.Setenv("AGENTRY_CONFIG_DIR", t.TempDir())
}

// signedChannel serves a manifest, sums and signature for one release.
func signedChannel(t *testing.T, version, commit string, retired bool) (*httptest.Server, *selfupdate.KeySet) {
	t.Helper()
	priv, entry := keystest.Key(t, "prod", retired)
	sums := []byte("aa  agentry-desktop-" + version + "-linux-amd64\n")
	sig := minisign.SignWithComments(priv, sums, selfupdate.TrustedComment(version, commit), "x")
	digest := sha256.Sum256(sums)
	manifest := map[string]any{"desktop": map[string]any{
		"version":   version,
		"checksums": map[string]any{"file": "SUMS", "size": len(sums), "sha256": hex.EncodeToString(digest[:])},
		"signature": map[string]any{"file": "SUMS.minisig", "size": len(sig)},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "manifest.json"):
			_ = json.NewEncoder(w).Encode(manifest)
		case strings.HasSuffix(r.URL.Path, ".minisig"):
			_, _ = w.Write(sig)
		default:
			_, _ = w.Write(sums)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, keystest.Set(t, entry)
}

// api builds the bound surface pointed at a test channel and key set. The
// service takes both as fields precisely so a test needs no global to swap.
func api(t *testing.T, channel string, keys *selfupdate.KeySet, client *http.Client) *SelfUpdateAPI {
	t.Helper()
	u := newSelfUpdateAPI(&App{})
	u.channel = channel
	u.keys = func() (*selfupdate.KeySet, error) { return keys, nil }
	u.client = client
	return u
}

func TestSelfUpdateStatusReportsNoChannelForACoreBuild(t *testing.T) {
	configHome(t)
	u := api(t, "", keystest.Set(t), nil)

	u.check(context.Background())
	got, err := u.SelfUpdateStatus()
	if err != nil {
		t.Fatal(err)
	}
	if got.Channel != "" {
		t.Errorf("channel = %q, want empty for a build of the open core", got.Channel)
	}
	if got.Err != "" {
		t.Errorf("having no channel is a state, not an error: %q", got.Err)
	}
	if got.Available != nil {
		t.Error("a release was offered by a build with no channel")
	}
}

func TestSelfUpdateStatusDoesNotFetch(t *testing.T) {
	configHome(t)
	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched = true
		http.NotFound(w, r)
	}))
	defer srv.Close()

	u := api(t, srv.URL+"/downloads/manifest.json", keystest.Set(t), srv.Client())
	if _, err := u.SelfUpdateStatus(); err != nil {
		t.Fatal(err)
	}
	if fetched {
		t.Error("reading the status started network traffic; the frontend must not be able to")
	}
}

func TestSelfUpdateCheckOffersANewerRelease(t *testing.T) {
	configHome(t)
	srv, keys := signedChannel(t, "9.9.9", "abc1234", false)
	u := api(t, srv.URL+"/downloads/manifest.json", keys, srv.Client())

	got, err := u.SelfUpdateCheckNow()
	if err != nil {
		t.Fatal(err)
	}
	if got.Err != "" {
		t.Fatalf("check reported %q", got.Err)
	}
	if got.Available == nil || got.Available.Version != "9.9.9" {
		t.Fatalf("no release offered: %+v", got.Available)
	}
	if got.Checked == "" {
		t.Error("the check recorded no time, so the card cannot say when it last looked")
	}
}

func TestSelfUpdateReportsAVerificationFailureAsASentence(t *testing.T) {
	configHome(t)
	srv, _ := signedChannel(t, "9.9.9", "abc1234", false)
	// A key set that carries a DIFFERENT key: the release is genuine to
	// somebody, and not to this build.
	_, stranger := keystest.Key(t, "someone-else", false)
	u := api(t, srv.URL+"/downloads/manifest.json", keystest.Set(t, stranger), srv.Client())

	got, err := u.SelfUpdateCheckNow()
	if err != nil {
		t.Fatal(err)
	}
	if got.Available != nil {
		t.Fatal("a release this build cannot verify was offered anyway")
	}
	if got.Err == "" {
		t.Fatal("a verification failure was swallowed; it must be shown as what it is")
	}
	if !strings.Contains(got.Err, "not one this build accepts") {
		t.Errorf("the sentence shown is %q, which does not say what happened", got.Err)
	}
}

func TestSelfUpdateTogglePersistsAndDefaultsOn(t *testing.T) {
	configHome(t)
	u := api(t, "", keystest.Set(t), nil)

	got, err := u.SelfUpdateStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Fatal("the check is off for someone who has never opened Settings; it defaults on")
	}

	if _, err := u.SetSelfUpdateCheckEnabled(false); err != nil {
		t.Fatal(err)
	}
	got, _ = u.SelfUpdateStatus()
	if got.Enabled {
		t.Fatal("turning the toggle off did not stick")
	}
	// And a fresh service reads the same file.
	got, _ = api(t, "", keystest.Set(t), nil).SelfUpdateStatus()
	if got.Enabled {
		t.Fatal("the toggle did not persist to the settings file")
	}

	if _, err := u.SetSelfUpdateCheckEnabled(true); err != nil {
		t.Fatal(err)
	}
	got, _ = api(t, "", keystest.Set(t), nil).SelfUpdateStatus()
	if !got.Enabled {
		t.Fatal("turning the toggle back on did not stick")
	}
}

func TestSelfUpdateTurnedOffChecksNothingAndShowsNothing(t *testing.T) {
	configHome(t)
	srv, keys := signedChannel(t, "9.9.9", "abc1234", false)
	u := api(t, srv.URL+"/downloads/manifest.json", keys, srv.Client())

	if _, err := u.SelfUpdateCheckNow(); err != nil {
		t.Fatal(err)
	}
	if got, _ := u.SelfUpdateStatus(); got.Available == nil {
		t.Fatal("setup: the release should have been found while the toggle was on")
	}

	if _, err := u.SetSelfUpdateCheckEnabled(false); err != nil {
		t.Fatal(err)
	}
	got, _ := u.SelfUpdateStatus()
	if got.Available != nil {
		t.Error("the card is still up after the person asked not to be told")
	}
	u.check(context.Background())
	if got, _ := u.SelfUpdateStatus(); got.Available != nil {
		t.Error("a check ran although the toggle is off")
	}
}

// The check runs on launch, and launch must not wait for it.
func TestSelfUpdateStartDoesNotBlock(t *testing.T) {
	configHome(t)
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	u := api(t, srv.URL+"/downloads/manifest.json", keystest.Set(t), srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() { u.start(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("start blocked on the channel; launch must not wait for it")
	}
	// And the status is readable while that check is still in flight.
	if _, err := u.SelfUpdateStatus(); err != nil {
		t.Fatalf("the status could not be read during a check: %v", err)
	}
}

// "Not now" hides the chrome line for this run and no longer: a person saying
// not now is saying it about today, and nothing is written to the settings
// file. Settings keeps showing the card, because that is where someone looks.
func TestSelfUpdateDismissLastsOnlyForThisRun(t *testing.T) {
	configHome(t)
	srv, keys := signedChannel(t, "9.9.9", "abc1234", false)
	u := api(t, srv.URL+"/downloads/manifest.json", keys, srv.Client())

	if _, err := u.SelfUpdateCheckNow(); err != nil {
		t.Fatal(err)
	}
	got, _ := u.SelfUpdateStatus()
	if got.Dismissed {
		t.Fatal("a fresh check starts dismissed")
	}
	if got.DownloadURL == "" || !strings.HasSuffix(got.DownloadURL, "/download") {
		t.Errorf("no download page to send the person to: %q", got.DownloadURL)
	}

	if _, err := u.DismissSelfUpdate(); err != nil {
		t.Fatal(err)
	}
	got, _ = u.SelfUpdateStatus()
	if !got.Dismissed {
		t.Fatal("Not now did not take")
	}
	if got.Available == nil {
		t.Error("Settings lost the card too; dismissing hides the chrome line, not the place you look")
	}

	// A fresh service is a fresh run: the line is back.
	again := api(t, srv.URL+"/downloads/manifest.json", keys, srv.Client())
	if _, err := again.SelfUpdateCheckNow(); err != nil {
		t.Fatal(err)
	}
	if got, _ := again.SelfUpdateStatus(); got.Dismissed {
		t.Error("Not now survived a restart; it was written somewhere it should not have been")
	}
}

// Dismissing 0.2.0 must not silence 0.3.0.
func TestSelfUpdateDismissDoesNotSilenceTheNextRelease(t *testing.T) {
	configHome(t)
	first, keys := signedChannel(t, "9.9.9", "abc1234", false)
	u := api(t, first.URL+"/downloads/manifest.json", keys, first.Client())
	if _, err := u.SelfUpdateCheckNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := u.DismissSelfUpdate(); err != nil {
		t.Fatal(err)
	}

	// The same key, a later release.
	u.channel = first.URL + "/downloads/manifest.json"
	u.last.Available = &selfupdate.Release{Version: "9.9.8"}
	u.check(context.Background())
	if got, _ := u.SelfUpdateStatus(); got.Dismissed {
		t.Error("a different release stayed dismissed; Not now was about the one on screen")
	}
}

func TestDownloadPageIsDerivedFromTheChannel(t *testing.T) {
	for channel, want := range map[string]string{
		"https://dev.secureagentry.net/downloads/manifest.json": "https://dev.secureagentry.net/download",
		"https://secureagentry.net/downloads/manifest.json":     "https://secureagentry.net/download",
		"not a url at all": "",
		"":                 "",
	} {
		if got := downloadPage(channel); got != want {
			t.Errorf("downloadPage(%q) = %q, want %q", channel, got, want)
		}
	}
}

// A channel that answers but publishes nothing signed is its own state. It is
// what a site with nothing released yet looks like, and also what a channel
// withholding a release looks like — so it is shown rather than folded into
// "up to date" (D-027).
func TestSelfUpdateReportsAChannelWithNothingSigned(t *testing.T) {
	configHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"desktop": map[string]any{"version": "0.1.0"}})
	}))
	defer srv.Close()

	u := api(t, srv.URL+"/downloads/manifest.json", keystest.Set(t), srv.Client())
	got, err := u.SelfUpdateCheckNow()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Nothing {
		t.Error("a channel with nothing signed was not reported as such")
	}
	if got.Err != "" {
		t.Errorf("it was shown as a failure instead: %q", got.Err)
	}
	if got.Available != nil {
		t.Error("a release was offered")
	}
}
