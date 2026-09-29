// Self-update bindings: whether a newer Agentry Desktop exists, and the
// toggle that decides whether the app looks (agentry-desktop#87; plan
// desktop-installers.md §7, D5, D6, D7).
//
// Notify only. Nothing here downloads or replaces anything; applying an
// update is M7 (#88) on Windows and macOS, and on Linux it is the package
// manager's job for good (D7), so this surface is the whole feature there.
//
// The name is SELF-UPDATE throughout, never "update" alone: internal/install
// already owns that word for an installed QuickAgent, and a person reading
// either surface should not have to work out which one they are looking at.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// How often the app looks, and how long it is willing to wait. The timeout is
// short on purpose: this runs on launch, and a channel that is slow or gone
// must cost the person nothing. Neither is configurable — a person who does
// not want the check turns it off.
const (
	selfUpdateInterval = 24 * time.Hour
	selfUpdateTimeout  = 3 * time.Second
	// Downloading is a different question from checking: tens of megabytes
	// over whatever connection the person has, started because they pressed a
	// button. Three seconds would be absurd here.
	selfUpdateDownloadTimeout = 10 * time.Minute
)

// SelfUpdateView is what the chrome line and the Settings card render. It is
// a snapshot of the last check: SelfUpdateStatus never starts one, so a paint
// costs nothing. "Check now" does start one, because the person asked.
type SelfUpdateView struct {
	// Channel is the manifest URL this build checks, empty in a build of the
	// open core — which is a state Settings shows, not a failure.
	Channel string `json:"channel"`
	// Current is the version running now.
	Current string `json:"current"`
	// Enabled is the Settings toggle.
	Enabled bool `json:"enabled"`
	// Checked is when the last check finished; zero means none has.
	Checked string `json:"checked,omitempty"`
	// Available is the newer release, when there is one. Its version and
	// commit come from the signature, not from the manifest.
	Available *selfupdate.Release `json:"available,omitempty"`
	// Err is a sentence for the person when the last check could not be
	// completed or could not be believed. A verification failure says what it
	// is; it is never retried quietly and never falls back to "the checksum
	// matched" (plan §3).
	Err string `json:"err,omitempty"`
	// DownloadURL is where to get the release: the download page of the site
	// this build checks, derived from the channel rather than configured
	// separately — one fewer thing to point at the wrong place. Releases are
	// on GitHub too since DEV-148 (agentry-desktop#192), but the repository
	// is private until the flip, so the site's page is the destination a
	// person can open.
	DownloadURL string `json:"downloadUrl,omitempty"`
	// Nothing is set when the channel answered but publishes no signed
	// release at all. Distinct from "nothing newer", because they are
	// different facts and only one of them means the channel is working.
	Nothing bool `json:"nothing,omitempty"`
	// CanApply is true when this platform installs a newer Agentry Desktop
	// from inside the app AND this copy is somewhere the person can replace.
	// False on Linux always (D7), and on a copy installed for everyone by an
	// administrator — in which case Why says so.
	CanApply bool `json:"canApply"`
	// CannotApplyWhy is the sentence to show when CanApply is false and the
	// person might reasonably expect otherwise. Empty on Linux, where the
	// card simply does not offer to install.
	CannotApplyWhy string `json:"cannotApplyWhy,omitempty"`
	// OnQuit is Settings → "Install automatically when I quit".
	OnQuit bool `json:"onQuit"`
	// Applying is set while a download or a swap is in flight, so the button
	// can say so and cannot be pressed twice.
	Applying bool `json:"applying,omitempty"`
	// Dismissed is set by "Not now": the chrome line goes until the next
	// launch. Deliberately not written to the settings file — a person saying
	// "not now" is saying it about today, and D-019's banner has the same
	// shape.
	Dismissed bool `json:"dismissed,omitempty"`
}

// downloadPage turns the update channel into the page a person downloads
// from: .../downloads/manifest.json → .../download. Empty when the channel is
// not a URL of that shape, in which case the card simply offers no link.
func downloadPage(channel string) string {
	u, err := url.Parse(channel)
	if err != nil || u.Host == "" {
		return ""
	}
	u.Path, u.RawQuery, u.Fragment = "/download", "", ""
	return u.String()
}

// SelfUpdateAPI is the bound surface.
//
// The channel and the key set are fields rather than reads of the package
// globals, so that a test can point them somewhere without the trust root
// growing a settable key set. An exported "use these keys instead" on
// internal/selfupdate would be a hole in the one thing this feature exists to
// protect; a constructor argument is not.
type SelfUpdateAPI struct {
	app *App

	channel string
	keys    func() (*selfupdate.KeySet, error)
	// applier installs a verified artifact. A field so a test can observe
	// what would have been installed rather than replacing the test binary.
	applier selfupdate.Applier
	// goos and goarch name the platform whose artifact is chosen. Fields, not
	// runtime constants, so that the install path can be exercised on a
	// machine that does not install — otherwise the two most important tests
	// here would skip on Linux, and a skip reads as a pass.
	goos, goarch string

	// checking serialises checks. Without it the daily round and a "Check
	// now" race to finish, and the one that happens to finish LAST wins —
	// so a slow failure could overwrite a fresh success and the card would
	// show an error that is no longer true.
	checking sync.Mutex

	mu   sync.Mutex
	last SelfUpdateView
	// dismissed is "Not now", for this run of the app only.
	dismissed bool
	// applying is set while a download or swap is in flight.
	applying bool
	// client is replaced in tests; nil means the default with its own timeout.
	client *http.Client
	now    func() time.Time
}

func newSelfUpdateAPI(app *App) *SelfUpdateAPI {
	return &SelfUpdateAPI{
		app:     app,
		channel: selfupdate.ManifestURL,
		keys:    selfupdate.Carried,
		applier: selfupdate.ApplierFor(),
		goos:    runtime.GOOS,
		goarch:  runtime.GOARCH,
		now:     time.Now,
	}
}

// SelfUpdateStatus returns the last check's result without starting one, so
// the chrome and Settings can render immediately on any paint.
func (u *SelfUpdateAPI) SelfUpdateStatus() (*SelfUpdateView, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	v := u.last
	v.Channel = u.channel
	v.Current = version.Version
	v.Enabled = u.enabled()
	v.Dismissed = u.dismissed
	v.Applying = u.applying
	if v.Available != nil {
		v.DownloadURL = downloadPage(u.channel)
	}
	if err := u.applier.Ready(); err != nil {
		if !errors.Is(err, selfupdate.ErrNotApplicable) {
			v.CannotApplyWhy = err.Error()
		}
	} else {
		v.CanApply = true
	}
	s, serr := settings.Load()
	v.OnQuit = serr == nil && s.InstallUpdatesOnQuit
	return &v, nil
}

// SelfUpdateCheckNow runs a check and returns the result. Settings offers it
// so a person who has just turned the toggle on does not wait a day to see
// anything.
func (u *SelfUpdateAPI) SelfUpdateCheckNow() (*SelfUpdateView, error) {
	u.check(context.Background())
	return u.SelfUpdateStatus()
}

// SetSelfUpdateCheckEnabled writes the toggle and, when it is turned on,
// checks at once.
func (u *SelfUpdateAPI) SetSelfUpdateCheckEnabled(enabled bool) (*SelfUpdateView, error) {
	s, err := settings.Load()
	if err != nil {
		return nil, err
	}
	s.CheckForUpdates = &enabled
	if err := settings.Save(s); err != nil {
		return nil, err
	}
	if enabled {
		u.check(context.Background())
	} else {
		// Turning it off clears what the last check found: leaving the card up
		// after someone asked not to be told would be the opposite of the
		// toggle's promise.
		u.mu.Lock()
		u.last = SelfUpdateView{}
		u.mu.Unlock()
	}
	return u.SelfUpdateStatus()
}

// DismissSelfUpdate is "Not now": the chrome line goes for this run. Settings
// still shows the card, because Settings is where someone goes to look —
// hiding it there would mean the person could not find it again.
func (u *SelfUpdateAPI) DismissSelfUpdate() (*SelfUpdateView, error) {
	u.mu.Lock()
	u.dismissed = true
	u.mu.Unlock()
	return u.SelfUpdateStatus()
}

// enabled reads the toggle. A settings file that cannot be read is treated as
// "on", which is the shippable default — the check is harmless and being told
// about a release is the point.
func (u *SelfUpdateAPI) enabled() bool {
	s, err := settings.Load()
	if err != nil {
		return true
	}
	return s.UpdateCheckEnabled()
}

// start runs the check on launch and every day after, in its own goroutine.
//
// It is deliberately the last thing app startup does and it holds nothing up:
// the first-run wizard, the first paint and every binding are unaffected
// whether the channel answers in a millisecond, in three seconds, or never.
func (u *SelfUpdateAPI) start(ctx context.Context) {
	go func() {
		for {
			u.check(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(selfUpdateInterval):
			}
		}
	}()
}

// check does one round and stores the result. It never panics the caller and
// never leaves a stale "available" beside a fresh error: whatever this round
// concluded is what the surface shows.
func (u *SelfUpdateAPI) check(ctx context.Context) {
	if !u.enabled() {
		return
	}
	u.checking.Lock()
	defer u.checking.Unlock()

	var v SelfUpdateView

	keys, err := u.keys()
	if err != nil {
		v.Err = "this build's signing keys could not be read, so a release could not be checked"
	} else if u.channel == "" {
		// Not an error to show as one: a build of the open core has no
		// channel by design, and Settings says so from Channel being empty.
		v = SelfUpdateView{}
	} else { //nolint:revive // the branch below is the whole of the work
		ctx, cancel := context.WithTimeout(ctx, selfUpdateTimeout)
		defer cancel()
		rel, cerr := selfupdate.Check(ctx, u.client, u.channel, version.Version, keys)
		switch {
		case errors.Is(cerr, selfupdate.ErrNothingSigned):
			// Said plainly rather than shown as a failure: a site with
			// nothing signed yet is the ordinary case today, and a person
			// should still be able to tell it apart from "you are up to
			// date" (D-027).
			v.Nothing = true
		case cerr != nil:
			v.Err = cerr.Error()
		case rel != nil:
			v.Available = rel
		}
	}

	// Stamped when the round FINISHED, not when it started: "checked at" is a
	// statement about an answer, and an answer that took three seconds to
	// arrive did not arrive three seconds ago.
	if v.Channel != "" || v.Err != "" || v.Available != nil || u.channel != "" {
		v.Checked = u.now().UTC().Format(time.RFC3339)
	}

	u.mu.Lock()
	// A check that found a DIFFERENT release is news again: "not now" was
	// about the release the person was shown, not about every release for
	// ever.
	if u.last.Available == nil || v.Available == nil || u.last.Available.Version != v.Available.Version {
		u.dismissed = false
	}
	u.last = v
	u.mu.Unlock()
}

// InstallSelfUpdate downloads the release this build was offered and installs
// it: the "Install and relaunch" button (agentry-desktop#88).
//
// Every refusal below happens BEFORE a byte is downloaded where it can, so a
// person is not made to wait for a download that was never going to be
// installable. The download itself is verified against the digest in the
// release's signed checksums (internal/selfupdate.Download) and removed if it
// does not match, so nothing unverified ever reaches the applier.
//
// On success this does not return: the platform applier replaces the
// application and relaunches it.
func (u *SelfUpdateAPI) InstallSelfUpdate() (*SelfUpdateView, error) {
	return u.install(true)
}

func (u *SelfUpdateAPI) install(relaunch bool) (*SelfUpdateView, error) {
	// Claimed under ONE lock: reading "am I busy" and then setting it under a
	// second lock lets two presses — or a press racing the quit hook — both
	// see false and both proceed.
	u.mu.Lock()
	rel := u.last.Available
	if u.applying {
		u.mu.Unlock()
		return u.SelfUpdateStatus()
	}
	u.applying = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.applying = false
		u.mu.Unlock()
	}()

	if rel == nil {
		return nil, errors.New("there is no newer Agentry Desktop to install; check again first")
	}
	// The whole verification again, over the same bytes, through the call
	// that refuses a retired key on its own. Not a read of rel.Retired: that
	// was decided when the check ran, and this is the operation where
	// forgetting to read it installs something (D-026).
	keys, err := u.keys()
	if err != nil {
		return nil, err
	}
	if _, err := rel.AuthoriseInstall(keys); err != nil {
		return nil, err
	}
	if err := u.applier.Ready(); err != nil {
		return nil, err
	}
	artifact, err := selfupdate.ArtifactFor(rel.Checksums, rel.Version, u.goos, u.goarch)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "agentry-selfupdate-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ctx, cancel := context.WithTimeout(context.Background(), selfUpdateDownloadTimeout)
	defer cancel()
	path, err := selfupdate.Download(ctx, u.client, u.channel, artifact, dir)
	if err != nil {
		return nil, err
	}
	// Past here the application is replaced and this process ends, so there
	// is no view to return on success.
	if err := u.applier.Apply(path, relaunch); err != nil {
		return nil, err
	}
	return u.SelfUpdateStatus()
}

// SetInstallOnQuit writes Settings → "Install automatically when I quit".
//
// Off by default and off until someone says otherwise (D6): an application
// that replaced itself without ever having been asked would be a surprise
// the next time it opened, however well the bytes verified.
func (u *SelfUpdateAPI) SetInstallOnQuit(on bool) (*SelfUpdateView, error) {
	s, err := settings.Load()
	if err != nil {
		return nil, err
	}
	s.InstallUpdatesOnQuit = on
	if err := settings.Save(s); err != nil {
		return nil, err
	}
	return u.SelfUpdateStatus()
}

// applyOnQuit is called as the app shuts down. It installs a release only if
// the person asked for that, one is waiting, and it would have been
// installable anyway — and it swallows every failure, because a shutdown is
// not a place to show anything: the release is still there at the next
// launch, and the card will say so then.
func (u *SelfUpdateAPI) applyOnQuit() {
	s, err := settings.Load()
	if err != nil || !s.InstallUpdatesOnQuit {
		return
	}
	// relaunch=false: the person is quitting. Reopening the application they
	// just closed, because it happened to install an update on the way out,
	// would be the opposite of what they asked for.
	if _, err := u.install(false); err != nil {
		return
	}
}
