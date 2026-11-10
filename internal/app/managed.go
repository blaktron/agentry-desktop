package app

// Managed settings: the values an organization's admins set for their
// members' clients, which the registry signs and the CLI's pkg/managed
// fetches, verifies and keeps under the config directory. The runner this
// app links reads that copy itself and refuses a run the settings forbid,
// so nothing here decides a run. The app keeps the copy current (at start,
// whenever the last answer is over managed.FetchInterval old, and at each
// sign-in and sign-out) and shows what it locks: Unsafe Mode's switch, and
// a line on the Settings landing naming the organization and the version.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventManagedChanged tells the page the managed settings held here
// changed, so the shell and Settings read their views again.
const eventManagedChanged = "managed:changed"

// managedCalls are pkg/managed's calls, a package variable purely as a test
// seam. Production code never reassigns it.
var managedCalls = struct {
	current  func() managed.Effective
	sync     func(ctx context.Context, hc *http.Client, registryURL, token string, maxAge time.Duration) (st *managed.State, fetched bool, err error)
	refresh  func(ctx context.Context, hc *http.Client, registryURL, token string) (*managed.State, error)
	signedIn func(registryURL, accountID string) error
	forget   func(registryURL string) error
}{managed.Current, managed.SyncFetched, managed.Refresh, managed.SignedIn, managed.Forget}

// managedCheckEvery is how often the app looks whether its copy is due;
// Sync asks the registry only when the last answer is over
// managed.FetchInterval old, so a copy is read again about hourly.
var managedCheckEvery = 15 * time.Minute

// managedAskTimeout bounds one registry's answer, so a registry that does
// not answer holds neither a sign-in nor the next look.
const managedAskTimeout = 30 * time.Second

// unsafeModeRefusal is why Unsafe Mode may not be on, or "" when it may.
func unsafeModeRefusal() string {
	return managedCalls.current().UnsafeModeRefusal()
}

// unsafeModeHeldOn is whether Unsafe Mode is on: saved on in desktop.yaml
// and not refused by the managed settings held here.
func unsafeModeHeldOn(saved bool, held managed.Effective) bool {
	return saved && held.UnsafeModeRefusal() == ""
}

// UnsafeLockView is Unsafe Mode's switch while managed settings refuse it:
// shown off and disabled, with the organization and the reason.
type UnsafeLockView struct {
	// ManagedBy is the organization whose managed settings forbid Unsafe
	// Mode; "" when the refusal is managed settings held here that cannot be
	// verified.
	ManagedBy string `json:"managedBy,omitempty"`
	// Reason is the refusal in the runner's words.
	Reason string `json:"reason"`
	// SavedOn says desktop.yaml still holds the switch on. The lock holds it
	// off for every run on this computer, and the file keeps what the person
	// chose.
	SavedOn bool `json:"savedOn"`
}

// unsafeLock is the switch's lock, nil when nothing refuses it.
func unsafeLock(e managed.Effective, savedOn bool) *UnsafeLockView {
	reason := e.UnsafeModeRefusal()
	if reason == "" {
		return nil
	}
	v := &UnsafeLockView{Reason: reason, SavedOn: savedOn}
	for i := range e.Documents {
		d := &e.Documents[i]
		if val, ok := d.Locked(managedsettings.UnsafeMode); ok && val == managedsettings.UnsafeModeForbidden {
			v.ManagedBy = d.Document.Org
			break
		}
	}
	return v
}

// ManagedView is one organization's managed settings held here, as the
// Settings landing names them.
type ManagedView struct {
	Registry     string `json:"registry"`
	Organization string `json:"organization"`
	Version      int64  `json:"version"`
	// Summary names the document in one line: "managed by acme (version
	// 7)".
	Summary string `json:"summary"`
	// FetchedAt is when this computer last read them from the registry,
	// RFC 3339.
	FetchedAt string `json:"fetchedAt,omitempty"`
}

// managedViews are the verified documents held here.
func managedViews(e managed.Effective) []ManagedView {
	out := []ManagedView{}
	for i := range e.Documents {
		d := &e.Documents[i]
		v := ManagedView{Registry: d.Registry, Organization: d.Document.Org, Version: d.Document.Version, Summary: d.Summary()}
		if !d.FetchedAt.IsZero() {
			v.FetchedAt = d.FetchedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out
}

// managedProblems say which managed settings held here cannot be applied: a
// copy that does not verify, one older than a version already applied, or
// one that was accepted and is no longer there. While there is one, no run
// starts on this computer.
func managedProblems(e managed.Effective) []string {
	out := []string{}
	for _, p := range e.Problems {
		where := p.Registry
		if where == "" {
			where = "this computer"
		}
		out = append(out, fmt.Sprintf("The managed settings held for %s cannot be applied: %v. No run starts on this computer until they are read again.", where, p.Err))
	}
	return out
}

// managedFingerprint is what the page shows of the managed settings, to
// tell whether a sync changed it.
func managedFingerprint(e managed.Effective) string {
	var b strings.Builder
	for i := range e.Documents {
		d := &e.Documents[i]
		fmt.Fprintf(&b, "%s %s %d\n", d.Registry, d.Document.Org, d.Document.Version)
	}
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "! %s %v\n", p.Registry, p.Err)
	}
	return b.String()
}

// syncManaged reads each signed-in registry's managed settings again when
// the last answer is over managed.FetchInterval old. A registry that
// cannot be asked leaves its last verified copy in force. The page is told
// when what it shows changed.
func (s *SettingsAPI) syncManaged(ctx context.Context) {
	cfg, err := settings.Load()
	if err != nil {
		return
	}
	before := managedFingerprint(managedCalls.current())
	for _, r := range cfg.Registries {
		reg := settings.Normalize(r.URL)
		token := settings.TokenFor(reg)
		if token == "" || tokenRejected(reg) {
			continue
		}
		actx, cancel := context.WithTimeout(ctx, managedAskTimeout)
		st, fetched, err := managedCalls.sync(actx, registry.HTTPClient, reg, token, managed.FetchInterval)
		cancel()
		if err != nil {
			slog.Warn("managed settings: not read again; the last verified copy applies", "registry", originOf(reg), "err", err)
		}
		if fetched {
			s.installRequired(ctx, st)
		}
	}
	if managedFingerprint(managedCalls.current()) != before {
		emitEvent(s.app, eventManagedChanged)
	}
}

// watchManaged keeps the managed settings current until ctx ends: once at
// start, then every managedCheckEvery.
func (s *SettingsAPI) watchManaged(ctx context.Context) {
	s.syncManaged(ctx)
	t := time.NewTicker(managedCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.syncManaged(ctx)
		}
	}
}

// signedInManaged reads the registry's managed settings at sign-in, after
// its signing keys are pinned. Another account's copy is deleted first; the
// same account's stays until the registry answers, so a sign-in the registry
// cannot answer at that moment does not lift a lock. A registry that cannot
// answer is not a failed sign-in; the next look asks again.
func (s *SettingsAPI) signedInManaged(reg, accountID, token string) {
	if err := managedCalls.signedIn(reg, accountID); err != nil {
		slog.Warn("managed settings: the sign-in was not recorded", "registry", originOf(reg), "err", err)
	}
	ctx, cancel := context.WithTimeout(s.app.ctxOrBackground(), managedAskTimeout)
	defer cancel()
	st, err := managedCalls.refresh(ctx, registry.HTTPClient, reg, token)
	if err != nil {
		slog.Warn("managed settings: not read at sign-in", "registry", originOf(reg), "err", err)
	} else {
		s.installRequired(s.app.ctxOrBackground(), st)
	}
	emitEvent(s.app, eventManagedChanged)
}

// signedOutManaged deletes the registry's managed settings with its
// sign-in.
func (s *SettingsAPI) signedOutManaged(reg string) {
	if err := managedCalls.forget(reg); err != nil {
		slog.Warn("managed settings: not deleted at sign-out", "registry", originOf(reg), "err", err)
	}
	emitEvent(s.app, eventManagedChanged)
}
