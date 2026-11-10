package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// holdManaged makes e the managed settings held on this computer for the
// test.
func holdManaged(t *testing.T, e managed.Effective) {
	t.Helper()
	was := managedCalls.current
	managedCalls.current = func() managed.Effective { return e }
	t.Cleanup(func() { managedCalls.current = was })
}

// managedDoc is a verified document of org's at version, locking
// run.unsafeMode to value.
func managedDoc(reg, org string, version int64, value string) managed.State {
	return managed.State{
		Registry:  reg,
		FetchedAt: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC),
		Document: managedsettings.Document{
			Registry: reg, Org: org, Version: version,
			Settings: map[string]managedsettings.Setting{
				managedsettings.UnsafeMode: {Value: json.RawMessage(`"` + value + `"`), Locked: true},
			},
		},
	}
}

func forbidding(org string) managed.Effective {
	return managed.Effective{Documents: []managed.State{managedDoc("https://r.example", org, 3, managedsettings.UnsafeModeForbidden)}}
}

// While an organization's managed settings forbid Unsafe Mode, turning it on
// is refused with their reason before the native dialog or the image's pull,
// and the switch reads off and locked, naming the organization. Turning it
// off is never refused.
func TestUnsafeModeIsRefusedOnWhileManagedSettingsForbidIt(t *testing.T) {
	u := newUnsafeApp(t)
	u.answer = true
	holdManaged(t, forbidding("acme"))

	_, err := u.r.SetUnsafeMode(true)
	if err == nil || !strings.Contains(err.Error(), "Unsafe Mode is forbidden by acme's managed settings") {
		t.Fatalf("turning it on under the lock: err = %v, want the managed settings' refusal", err)
	}
	if len(u.asked) != 0 || u.pulls != 0 || savedUnsafeMode(t) {
		t.Fatalf("under the lock: dialogs %d, pulls %d, saved %v; want none, none, off", len(u.asked), u.pulls, savedUnsafeMode(t))
	}

	v, err := (&SettingsAPI{app: u.r.app}).RunDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if v.UnsafeMode || v.UnsafeLock == nil {
		t.Fatalf("the view under the lock: %+v; want off and locked", v)
	}
	if v.UnsafeLock.ManagedBy != "acme" || v.UnsafeLock.SavedOn || !strings.Contains(v.UnsafeLock.Reason, "run.unsafeMode") {
		t.Fatalf("the lock: %+v; want managed by acme, the reason, not saved on", *v.UnsafeLock)
	}

	v, err = u.r.SetUnsafeMode(false)
	if err != nil || v.UnsafeMode || v.UnsafeLock == nil {
		t.Fatalf("turning it off under the lock: view %+v, err %v; want off, still locked", v, err)
	}
}

// Unsafe Mode already on in desktop.yaml when a lock arrives: every view
// reads it off and locked, saying the file still holds it on, no run asks
// for it, and the file keeps what the person chose until they turn it off.
func TestUnsafeModeSavedOnIsHeldOffByTheLock(t *testing.T) {
	u := newUnsafeApp(t)
	switchOn(t)
	holdManaged(t, forbidding("acme"))

	v, err := runDefaultsView()
	if err != nil {
		t.Fatal(err)
	}
	if v.UnsafeMode || v.UnsafeLock == nil || !v.UnsafeLock.SavedOn {
		t.Fatalf("the view with the switch saved on under the lock: %+v, lock %+v", v, v.UnsafeLock)
	}
	if sv, err := settingsView(); err != nil || sv.UnsafeMode {
		t.Fatalf("the shell's view under the lock: %+v, %v; want Unsafe Mode off, so no red edge", sv, err)
	}
	if unsafeModeOn() {
		t.Fatal("a run under the lock reads Unsafe Mode on")
	}
	if n, err := u.r.UnsafeNotice("https://r.example", "acme/x@1.0.0"); err != nil || n.On {
		t.Fatalf("the run setup's notice under the lock: %+v, %v; want off", n, err)
	}
	var got []run.Request
	u.r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		got = append(got, req)
		return &run.Result{}, nil
	}
	u.answer = true
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, true, nil, false, nil, nil, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Unsafe {
		t.Fatalf("a run under the lock was compiled with Unsafe %v", got[0].Unsafe)
	}
	if !savedUnsafeMode(t) {
		t.Fatal("the lock rewrote desktop.yaml")
	}

	if _, err := u.r.SetUnsafeMode(false); err != nil || savedUnsafeMode(t) {
		t.Fatalf("turning it off under the lock: err %v, saved %v", err, savedUnsafeMode(t))
	}
}

// Managed settings held here that cannot be verified might forbid Unsafe
// Mode, so it is refused too, with the reason and no organization, and the
// landing warns that no run starts.
func TestUnsafeModeIsRefusedWhileTheManagedSettingsCannotBeVerified(t *testing.T) {
	u := newUnsafeApp(t)
	u.answer = true
	holdManaged(t, managed.Effective{Problems: []managed.Problem{{Registry: "https://r.example", Err: errors.New("signature does not verify")}}})

	if _, err := u.r.SetUnsafeMode(true); err == nil || !strings.Contains(err.Error(), "cannot be verified") {
		t.Fatalf("err = %v, want the unverified copy's refusal", err)
	}
	if len(u.asked) != 0 || u.pulls != 0 {
		t.Fatalf("dialogs %d, pulls %d; want none", len(u.asked), u.pulls)
	}
	v, err := runDefaultsView()
	if err != nil {
		t.Fatal(err)
	}
	if v.UnsafeLock == nil || v.UnsafeLock.ManagedBy != "" || !strings.Contains(v.UnsafeLock.Reason, "cannot be verified") {
		t.Fatalf("the lock: %+v", v.UnsafeLock)
	}
	sv, err := settingsView()
	if err != nil {
		t.Fatal(err)
	}
	if len(sv.ManagedProblems) != 1 || !strings.Contains(sv.ManagedProblems[0], "https://r.example") || !strings.Contains(sv.ManagedProblems[0], "No run starts") {
		t.Fatalf("the landing's problems: %q", sv.ManagedProblems)
	}
}

// Managed settings that allow Unsafe Mode, or none at all, leave the switch
// as it always was.
func TestUnsafeModeAllowedByManagedSettingsIsUnlocked(t *testing.T) {
	u := newUnsafeApp(t)
	u.answer = true
	holdManaged(t, managed.Effective{Documents: []managed.State{managedDoc("https://r.example", "acme", 2, managedsettings.UnsafeModeAllowed)}})

	v, err := u.r.SetUnsafeMode(true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.UnsafeMode || v.UnsafeLock != nil || len(u.asked) != 1 || u.pulls != 1 {
		t.Fatalf("allowed: view %+v, dialogs %d, pulls %d", v, len(u.asked), u.pulls)
	}
}

// The Settings landing names each organization whose settings are held, with
// the version.
func TestSettingsViewNamesTheManagedSettingsHeld(t *testing.T) {
	_ = newUnsafeApp(t)
	sv, err := settingsView()
	if err != nil {
		t.Fatal(err)
	}
	if sv.Managed == nil || len(sv.Managed) != 0 || sv.ManagedProblems == nil {
		t.Fatalf("with none held: %+v; want empty lists, not null", sv)
	}
	holdManaged(t, forbidding("acme"))
	sv, err = settingsView()
	if err != nil {
		t.Fatal(err)
	}
	want := ManagedView{Registry: "https://r.example", Organization: "acme", Version: 3, Summary: "managed by acme (version 3)", FetchedAt: "2026-11-08T09:00:00Z"}
	if len(sv.Managed) != 1 || sv.Managed[0] != want {
		t.Fatalf("held: %+v, want %+v", sv.Managed, want)
	}
}

// managedLog records pkg/managed's calls a test makes through the seam.
type managedLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *managedLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, s)
}

func (l *managedLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

func recordManaged(t *testing.T) *managedLog {
	t.Helper()
	l := &managedLog{}
	was := managedCalls
	t.Cleanup(func() { managedCalls = was })
	managedCalls.sync = func(_ context.Context, hc *http.Client, reg, token string, maxAge time.Duration) (*managed.State, error) {
		if hc == nil {
			t.Error("sync without the app's client")
		}
		l.add("sync " + reg + " " + token + " " + maxAge.String())
		return nil, nil
	}
	managedCalls.refresh = func(_ context.Context, hc *http.Client, reg, token string) (*managed.State, error) {
		if hc == nil {
			t.Error("refresh without the app's client")
		}
		l.add("refresh " + reg + " " + token)
		return nil, nil
	}
	managedCalls.signedIn = func(reg, accountID string) error {
		l.add("signedIn " + reg + " " + accountID)
		return nil
	}
	managedCalls.forget = func(reg string) error {
		l.add("forget " + reg)
		return nil
	}
	return l
}

func recordEvents(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var names []string
	was := emitEvent
	emitEvent = func(_ *App, name string, _ ...any) {
		mu.Lock()
		names = append(names, name)
		mu.Unlock()
	}
	t.Cleanup(func() { emitEvent = was })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), names...)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A sign-in records the account (which deletes another account's copy and
// keeps this one's) and then reads the settings with its token; it never
// forgets the copy itself, so the same account signing in again while the
// registry cannot answer keeps its organization's lock. A sign-out, and
// removing the registry, delete it.
func TestSignInAndSignOutReplaceTheManagedSettings(t *testing.T) {
	s := twoSignInApp(t)
	self := newRegistryServer(t, "agy_selfhosted")
	l := recordManaged(t)
	events := recordEvents(t)

	if _, err := s.SignIn(self.URL, "agy_selfhosted"); err != nil {
		t.Fatal(err)
	}
	reg := settings.Normalize(self.URL)
	const account = "00000000-0000-0000-0000-000000000002"
	if want := []string{"signedIn " + reg + " " + account, "refresh " + reg + " agy_selfhosted"}; !equalStrings(l.list(), want) {
		t.Fatalf("at sign-in: %q, want %q", l.list(), want)
	}
	if got := events(); len(got) == 0 || got[len(got)-1] != eventManagedChanged {
		t.Fatalf("events at sign-in: %q, want %s", got, eventManagedChanged)
	}

	if _, err := s.SignOut(self.URL); err != nil {
		t.Fatal(err)
	}
	if got := l.list(); got[len(got)-1] != "forget "+reg {
		t.Fatalf("at sign-out: %q, want the copy forgotten", got)
	}

	// Signed in again: recorded and read, never forgotten.
	n := len(l.list())
	if _, err := s.SignIn(self.URL, "agy_selfhosted"); err != nil {
		t.Fatal(err)
	}
	if got := l.list()[n:]; !equalStrings(got, []string{"signedIn " + reg + " " + account, "refresh " + reg + " agy_selfhosted"}) {
		t.Fatalf("signing in again: %q", got)
	}
	n = len(l.list())
	if _, err := s.RemoveRegistry(self.URL); err != nil {
		t.Fatal(err)
	}
	if got := l.list(); len(got) != n+1 || got[n] != "forget "+reg {
		t.Fatalf("removing the registry: %q, want the copy forgotten", got)
	}
}

// A registry that cannot answer at sign-in does not fail the sign-in.
func TestASignInStandsWhenTheManagedSettingsCannotBeRead(t *testing.T) {
	s := twoSignInApp(t)
	self := newRegistryServer(t, "agy_selfhosted")
	_ = recordManaged(t)
	managedCalls.refresh = func(context.Context, *http.Client, string, string) (*managed.State, error) {
		return nil, errors.New("connection refused")
	}
	if _, err := s.SignIn(self.URL, "agy_selfhosted"); err != nil {
		t.Fatalf("the sign-in failed on the managed settings: %v", err)
	}
	if settings.TokenFor(self.URL) == "" {
		t.Fatal("the token was not kept")
	}
}

// At start and on every look after, each registry signed in here is synced
// with its token, held to managed.FetchInterval; a registry with no sign-in,
// or one whose token it refused, is not asked. The page is told only when
// what it shows changed.
func TestSyncAsksEachSignedInRegistry(t *testing.T) {
	s := twoSignInApp(t)
	l := recordManaged(t)
	events := recordEvents(t)
	const secure = "https://secureagentry.net"
	const self = "https://self.example"
	if _, err := settings.StoreToken(secure, "agy_secure"); err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Upsert(settings.Registry{URL: secure})
	cfg.Upsert(settings.Registry{URL: self})
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}

	s.syncManaged(context.Background())
	if want := []string{"sync " + secure + " agy_secure " + managed.FetchInterval.String()}; !equalStrings(l.list(), want) {
		t.Fatalf("synced: %q, want %q", l.list(), want)
	}
	if got := events(); len(got) != 0 {
		t.Fatalf("nothing changed, yet the page was told: %q", got)
	}

	// The sync brought a document: the page is told.
	held := managed.Effective{}
	managedCalls.current = func() managed.Effective { return held }
	managedCalls.sync = func(context.Context, *http.Client, string, string, time.Duration) (*managed.State, error) {
		held = forbidding("acme")
		return nil, nil
	}
	s.syncManaged(context.Background())
	if got := events(); !equalStrings(got, []string{eventManagedChanged}) {
		t.Fatalf("after a sync that changed the copy: %q", got)
	}

	// A token the registry refused is not sent again.
	managedCalls.sync = func(_ context.Context, _ *http.Client, reg, _ string, _ time.Duration) (*managed.State, error) {
		t.Errorf("synced %s with a refused token", reg)
		return nil, nil
	}
	markTokenRejected(secure)
	t.Cleanup(func() { tokenAccepted(secure) })
	s.syncManaged(context.Background())
}

// The watch syncs once at start, then again on each look, until the app
// ends.
func TestWatchManagedSyncsAtStartAndThenAgain(t *testing.T) {
	s := twoSignInApp(t)
	l := recordManaged(t)
	if _, err := settings.StoreToken("https://secureagentry.net", "agy_secure"); err != nil {
		t.Fatal(err)
	}
	was := managedCheckEvery
	managedCheckEvery = 10 * time.Millisecond
	t.Cleanup(func() { managedCheckEvery = was })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.watchManaged(ctx)
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(l.list()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if got := l.list(); len(got) < 2 || !strings.HasPrefix(got[0], "sync https://secureagentry.net ") {
		t.Fatalf("the watch synced %q; want at start and again", got)
	}
}

// The setting the lock holds off stays in desktop.yaml as the person set it:
// the lock is read, never written.
func TestTheLockNeverWritesDesktopYAML(t *testing.T) {
	_ = newUnsafeApp(t)
	switchOn(t)
	path, err := settings.Path()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	holdManaged(t, forbidding("acme"))
	if _, err := runDefaultsView(); err != nil {
		t.Fatal(err)
	}
	if _, err := settingsView(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("desktop.yaml changed under the lock:\n%s\n→\n%s", before, after)
	}
}

// A runhost's Unsafe Mode is refused on as this computer's is, before its
// record is read or anything is asked, while the managed settings held here
// forbid it.
func TestARunhostsUnsafeModeIsRefusedOnWhileManagedSettingsForbidIt(t *testing.T) {
	u := newUnsafeApp(t)
	u.answer = true
	holdManaged(t, forbidding("acme"))
	rh := &RunhostsAPI{app: u.r.app, run: u.r}
	err := rh.SetUnsafeMode("lab", true, RunhostAdmin{})
	if err == nil || !strings.Contains(err.Error(), "Unsafe Mode is forbidden by acme's managed settings") {
		t.Fatalf("err = %v, want the managed settings' refusal", err)
	}
	if len(u.asked) != 0 {
		t.Fatalf("dialogs %d; want none", len(u.asked))
	}
	if err := rh.SetUnsafeMode("lab", false, RunhostAdmin{}); err != nil && strings.Contains(err.Error(), "managed settings") {
		t.Fatalf("turning it off was refused by the managed settings: %v", err)
	}
}

// While managed settings are held here, only an agentry that applies them
// may schedule a run, since a schedule fires through it; with none held,
// any agentry the app otherwise accepts may.
func TestSchedulingWhileManagedNeedsAnAgentryThatAppliesThem(t *testing.T) {
	old, current := "agentry v0.7.34 (abc)", "agentry v0.7.35-dev.1 (def)"
	if why := managedCLITooOld(old, managed.Effective{}); why != "" {
		t.Fatalf("nothing held refused an older agentry: %s", why)
	}
	if why := managedCLITooOld(old, forbidding("acme")); !strings.Contains(why, "v0.7.35") {
		t.Fatalf("an older agentry under managed settings: %q", why)
	}
	unverified := managed.Effective{Problems: []managed.Problem{{Registry: "https://r.example", Err: errors.New("signature does not verify")}}}
	if why := managedCLITooOld(old, unverified); why == "" {
		t.Fatal("an older agentry was accepted while held settings cannot be applied")
	}
	if why := managedCLITooOld(current, forbidding("acme")); why != "" {
		t.Fatalf("a current agentry was refused: %s", why)
	}
}

// A key setter is refused while a lock decides that no key of the member's
// own pays for the service, in the runner's words; a lock to the member's
// own key, a default, or no row leaves it to them.
func TestAKeySetterIsRefusedWhileALockPaysAnotherWay(t *testing.T) {
	doc := func(id, value string, locked bool) managed.Effective {
		return managed.Effective{Documents: []managed.State{{Registry: "https://r.example", Document: managedsettings.Document{
			Registry: "https://r.example", Org: "acme", Version: 4,
			Settings: map[string]managedsettings.Setting{id: {Value: json.RawMessage(value), Locked: locked}},
		}}}}
	}
	for _, c := range []struct {
		id, value string
		locked    bool
		want      string
	}{
		{managedsettings.ReviewSource, `{"source":"agentry-credits"}`, true, "acme's managed settings pay for AI Review with its credits (review.source)"},
		{managedsettings.ReviewSource, `{"source":"organization-key","key":{"id":"k","rev":1}}`, true, "use a key acme provides for AI Review (review.source)"},
		{managedsettings.WebSearchSource, `{"source":"off"}`, true, "Web search is off by acme's managed settings (webSearch.source)"},
		{managedsettings.ReviewSource, `{"source":"member"}`, true, ""},
		{managedsettings.ReviewSource, `{"source":"agentry-credits"}`, false, ""},
	} {
		holdManaged(t, doc(c.id, c.value, c.locked))
		what := map[string]string{managedsettings.ReviewSource: "AI Review", managedsettings.WebSearchSource: "web search"}[c.id]
		err := refuseManagedKeySource(c.id, what)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s %s locked %v: refused: %v", c.id, c.value, c.locked, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s %s: %v, want %q", c.id, c.value, err, c.want)
		}
	}
	holdManaged(t, managed.Effective{})
	if err := refuseManagedKeySource(managedsettings.ReviewSource, "AI Review"); err != nil {
		t.Errorf("no document: %v", err)
	}
	r := &RunAPI{}
	holdManaged(t, doc(managedsettings.ReviewSource, `{"source":"agentry-credits"}`, true))
	if _, err := r.StoreJevKey("ts-should-not-be-kept"); err == nil || !strings.Contains(err.Error(), "review.source") {
		t.Fatalf("StoreJevKey under a credits lock: %v", err)
	}
}
