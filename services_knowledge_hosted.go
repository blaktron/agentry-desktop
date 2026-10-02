// Create knowledge In Agentry (plan knowledge.md §4.5, §4.8; Knowledge M6,
// agentry-desktop#274): the Knowledge panel's second place to build a pack.
// The built-in Knowledge Creator runs on a sandbox host against the
// platform's model, for credits, and the pack it writes is kept in Agentry
// and scanned there; when the scan ends, the sealed pack is downloaded into
// the local store through the unwrap flow, exactly as Download does, so the
// panel lists it with its scan's findings.
//
// The build is a run of kind knowledge_build in the registry's runs
// extension (agentry-enterprise#171): the documents go up through the
// run's input upload under its caps, the build is created with a pack's
// name, a pack in Agentry's id, or a finished build to continue, and it is
// followed as a hosted run is, by polling. No platform key reaches this
// app; the token is the account's own, and every document path is one a
// native picker returned (D-030).
package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/knowledge/store"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventKnowledgeHosted carries a build In Agentry's every state change and
// its final word.
const eventKnowledgeHosted = "knowledge:hosted-build"

// hostedScanWait bounds how long a finished build waits for Agentry's scan
// of its pack before the download: the scan is the core's job, and a pack
// being scanned is refused a download.
var hostedScanWait = 15 * time.Minute

// hostedSlot is the one hosted job of a kind this app follows: the
// Knowledge panel's build In Agentry, or the Build tab's draft In Agentry.
// A claim holds it across a start's round trip (a follow with no id yet),
// so a double click is refused before anything is sent.
type hostedSlot struct {
	mu  sync.Mutex
	cur *hostedFollow
}

// claim takes the slot, or refuses with busy when it is held.
func (s *hostedSlot) claim(busy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil {
		return errors.New(busy)
	}
	s.cur = &hostedFollow{cancel: func() {}}
	return nil
}

// release gives back a claim that did not become a follow.
func (s *hostedSlot) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.cur.id == "" {
		s.cur = nil
	}
}

// follow makes f the slot's follow, unless another is already followed (a
// reattach that raced a start): one follower.
func (s *hostedSlot) follow(f *hostedFollow) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.cur.id != "" {
		return false
	}
	s.cur = f
	return true
}

// done empties the slot when f is still its follow.
func (s *hostedSlot) done(f *hostedFollow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == f {
		s.cur = nil
	}
}

func (s *hostedSlot) current() *hostedFollow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// KnowledgeHostedAvailabilityView answers "is In Agentry offered for
// Create knowledge here, can it be chosen" and what the panel states before
// anything is created: the hold with the scan's credit inside it, the
// build's caps, the balance and the input limits.
type KnowledgeHostedAvailabilityView struct {
	Offered   bool   `json:"offered"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Remedy    string `json:"remedy,omitempty"`
	// UpgradeURL is the registry's subscription page, set where the way out
	// is Pro (DEV-162); the page opens it through App.OpenExternal, in the
	// system browser.
	UpgradeURL string `json:"upgradeUrl,omitempty"`

	// HoldCredits is what a build of this account's holds (0 for a member
	// of an unmetered organization); ScanCredits is the scan's credit
	// inside it. Both are the registry's.
	HoldCredits    int64 `json:"holdCredits"`
	ScanCredits    int64 `json:"scanCredits"`
	TimeoutSeconds int   `json:"timeoutSeconds"`
	MaxTurns       int   `json:"maxTurns"`
	// Balance is the credits the build is paid from; Pool names the
	// organization for a member; BalanceKnown is false when billing did
	// not answer.
	Balance      int64  `json:"balance"`
	BalanceKnown bool   `json:"balanceKnown"`
	Pool         string `json:"pool,omitempty"`
	Unmetered    bool   `json:"unmetered"`

	MaxInputFiles int   `json:"maxInputFiles"`
	MaxInputBytes int64 `json:"maxInputBytes"`
}

// KnowledgeHostedBuildView is one build In Agentry as the panel shows it:
// where it stands while it moves, and once it is over what it did to the
// pack, what it cost, and — once the pack is downloaded — its scan.
type KnowledgeHostedBuildView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	State           string `json:"state"`
	StatusLine      string `json:"statusLine"`
	QueuePosition   int    `json:"queuePosition,omitempty"`
	CancelRequested bool   `json:"cancelRequested"`

	Outcome     string `json:"outcome,omitempty"`
	OutcomeLine string `json:"outcomeLine,omitempty"`
	OutcomeTone string `json:"outcomeTone,omitempty"`
	Error       string `json:"error,omitempty"`

	HoldCredits    int64 `json:"holdCredits"`
	SettledCredits int64 `json:"settledCredits"`
	Settled        bool  `json:"settled"`
	// ScanCredits is the scan's credit in the settlement, 0 when none was
	// owed.
	ScanCredits int64 `json:"scanCredits"`

	// Applied says the pack now holds the tree this build wrote: Pages and
	// Bytes are that tree's. Note says why not, when it is not.
	Applied bool   `json:"applied"`
	Pages   int    `json:"pages"`
	Bytes   int64  `json:"bytes"`
	Note    string `json:"note,omitempty"`

	// Turns is the build's use as the person is told it; its tokens stay
	// with the registry (agentry-desktop#457).
	Turns int `json:"turns"`

	Documents []HostedArtefactView `json:"documents"`
	// CanContinue says Continue is offered: the build stopped short.
	CanContinue bool `json:"canContinue"`

	// Downloaded says the pack is kept on this computer now, with Kept
	// the store's sentence; DownloadError says why it is not. ScanState and
	// Scan are the kept pack's, as the panel's list shows them.
	Downloaded    bool   `json:"downloaded"`
	Kept          string `json:"kept,omitempty"`
	DownloadError string `json:"downloadError,omitempty"`
	ScanState     string `json:"scanState,omitempty"`
	Scan          string `json:"scan,omitempty"`
	// Final says this is the last word on the build.
	Final bool `json:"final"`
}

// KnowledgeHostedStartView is what starting, continuing or reattaching
// returns: the build, or the registry's refusal as a slug and a sentence.
type KnowledgeHostedStartView struct {
	Build    *KnowledgeHostedBuildView `json:"build,omitempty"`
	Refusal  string                    `json:"refusal,omitempty"`
	Detail   string                    `json:"detail,omitempty"`
	Balance  int64                     `json:"balance,omitempty"`
	Hold     int64                     `json:"hold,omitempty"`
	ResetsAt string                    `json:"resetsAt,omitempty"`
}

// HostedAvailability answers the panel's question for reg: may a pack be
// built In Agentry, and what would it hold.
func (k *KnowledgeAPI) HostedAvailability(reg string) (*KnowledgeHostedAvailabilityView, error) {
	reg = settings.Normalize(reg)
	ctx := k.app.ctx
	token := k.env().TokenFor(reg)
	v := &KnowledgeHostedAvailabilityView{MaxInputFiles: hostedrun.MaxInputFiles, MaxInputBytes: hostedrun.MaxInputBytes}
	f := hostedrun.BuildFacts{SignedIn: token != ""}
	if !f.SignedIn {
		return v, nil
	}
	if exts, err := extensionsOf(ctx, reg); err == nil {
		f.Knowledge, f.Runs = hasString(exts, "knowledge"), hasString(exts, "runs")
	}
	if !f.Knowledge || !f.Runs {
		o := hostedrun.DecideBuild(f)
		v.Offered, v.Available, v.Reason, v.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
		return v, nil
	}
	f.Scopes, f.ScopesKnown = tokenScopes(ctx, reg, token)
	client := hostedrun.New(reg, token)
	if st, err := client.Status(ctx); err == nil {
		f.StatusKnown, f.Price = true, st.KnowledgeBuild
		if st.KnowledgeBuild != nil {
			v.HoldCredits, v.ScanCredits = st.KnowledgeBuild.HoldCredits, st.KnowledgeBuild.ScanCredits
		}
		if st.Caller != nil && st.Caller.Build != nil {
			v.HoldCredits = st.Caller.Build.HoldCredits
			v.TimeoutSeconds, v.MaxTurns = st.Caller.Build.Caps.TimeoutSeconds, st.Caller.Build.Caps.MaxTurns
			f.Allowed = st.Caller.Build.Allowed
		}
	}
	if runs, err := client.List(ctx); err == nil {
		for i := range runs {
			if hostedrun.Active(runs[i].Status) {
				f.Active = &runs[i]
				break
			}
		}
	}
	if sub, err := fetchSubscription(ctx, reg, token); err == nil && sub != nil {
		v.Balance, v.BalanceKnown, v.Pool, v.Unmetered = sub.Balance, true, sub.MemberOf, sub.Unmetered
	}
	o := hostedrun.DecideBuild(f)
	v.Offered, v.Available, v.Reason, v.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
	if o.Upgrade {
		v.UpgradeURL = reg + "/settings/subscription?upgrade=pro"
	}
	return v, nil
}

// StartHostedBuild builds In Agentry: into a new pack named name, or, with
// addTo, into that pack's copy in Agentry. The documents are files and
// folders a native picker returned; nothing is created until they are
// staged and uploaded.
func (k *KnowledgeAPI) StartHostedBuild(reg, name, addTo string, documents []string) (*KnowledgeHostedStartView, error) {
	reg = settings.Normalize(reg)
	ctx := k.app.ctx
	token := k.env().TokenFor(reg)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to build In Agentry", regHost(reg))
	}
	for _, p := range documents {
		if err := k.app.requirePickedResolved("document", p); err != nil {
			return nil, err
		}
	}
	// The slot is claimed before anything is uploaded, so a second Start
	// (a double click) is refused here rather than after its upload.
	if err := k.claimHosted(); err != nil {
		return nil, err
	}
	claimed := true
	defer func() {
		if claimed {
			k.releaseHosted()
		}
	}()
	req := hostedrun.CreateRequest{Kind: hostedrun.KindKnowledgeBuild}
	target, err := k.hostedTarget(ctx, reg, name, addTo)
	if err != nil {
		return nil, err
	}
	req.Name, req.Pack = target.name, target.pack
	files, err := hostedrun.StageDocuments(documents)
	if err != nil {
		return nil, err
	}
	client := hostedrun.New(reg, token)
	ids, err := client.UploadInputs(ctx, files)
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return hostedRefusal(refusal), nil
		}
		return nil, err
	}
	req.Inputs = ids
	v, err := k.createHosted(ctx, reg, token, target.label, req)
	claimed = v == nil || v.Build == nil
	return v, err
}

// ContinueHostedBuild starts a new build from one of this account's that
// stopped short: the pack as it is now, the documents the registry kept.
func (k *KnowledgeAPI) ContinueHostedBuild(reg, id string) (*KnowledgeHostedStartView, error) {
	reg = settings.Normalize(reg)
	ctx := k.app.ctx
	token := k.env().TokenFor(reg)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to build In Agentry", regHost(reg))
	}
	if !validRunID(id) {
		return nil, fmt.Errorf("%q is not a build's id", id)
	}
	if err := k.claimHosted(); err != nil {
		return nil, err
	}
	claimed := true
	defer func() {
		if claimed {
			k.releaseHosted()
		}
	}()
	client := hostedrun.New(reg, token)
	prev, err := client.Get(ctx, id)
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return hostedRefusal(refusal), nil
		}
		return nil, err
	}
	if !hostedrun.CanContinue(prev) {
		return nil, fmt.Errorf("that build did not stop short, so there is nothing to continue")
	}
	label := id
	if prev.Build != nil {
		label = prev.Build.Name
	}
	v, err := k.createHosted(ctx, reg, token, label, hostedrun.CreateRequest{Kind: hostedrun.KindKnowledgeBuild, Continue: id})
	claimed = v == nil || v.Build == nil
	return v, err
}

// AttachHostedBuild follows this account's build in flight, if it has one,
// so a reopened panel shows it rather than offering a second.
func (k *KnowledgeAPI) AttachHostedBuild(reg string) (*KnowledgeHostedStartView, error) {
	reg = settings.Normalize(reg)
	token := k.env().TokenFor(reg)
	if token == "" {
		return &KnowledgeHostedStartView{}, nil
	}
	client := hostedrun.New(reg, token)
	if cur := k.hostedCurrent(); cur != nil {
		if cur.id == "" {
			// A start or another attach holds the slot; its own events
			// report the build.
			return &KnowledgeHostedStartView{}, nil
		}
		if fresh, err := client.Get(k.app.ctx, cur.id); err == nil {
			return &KnowledgeHostedStartView{Build: hostedBuildView(fresh, false)}, nil
		}
		return &KnowledgeHostedStartView{}, nil
	}
	if err := k.claimHosted(); err != nil {
		return &KnowledgeHostedStartView{}, nil //nolint:nilerr // another call took the slot first
	}
	runs, err := client.List(k.app.ctx)
	if err != nil {
		k.releaseHosted()
		return &KnowledgeHostedStartView{}, nil //nolint:nilerr // no list, nothing to reattach to
	}
	for i := range runs {
		r := &runs[i]
		if r.IsBuild() && hostedrun.Active(r.Status) {
			name := r.ID
			if r.Build != nil {
				name = r.Build.Name
			}
			k.followHostedBuild(reg, token, name, r)
			return &KnowledgeHostedStartView{Build: hostedBuildView(r, false)}, nil
		}
	}
	k.releaseHosted()
	return &KnowledgeHostedStartView{}, nil
}

// CancelHostedBuild asks the registry to stop the build this app follows.
func (k *KnowledgeAPI) CancelHostedBuild() (*KnowledgeHostedBuildView, error) {
	cur := k.hostedCurrent()
	if cur == nil {
		return nil, fmt.Errorf("no build In Agentry is in progress")
	}
	token := k.env().TokenFor(cur.registry)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to cancel this build", regHost(cur.registry))
	}
	client := hostedrun.New(cur.registry, token)
	if r, err := client.Get(k.app.ctx, cur.id); err == nil && r.Terminal() {
		// The build is over and the app is waiting for Agentry's scan:
		// stop waiting. The pack stays in Agentry, to Download later.
		cur.cancel()
		return hostedBuildView(r, true), nil
	}
	r, err := client.Cancel(k.app.ctx, cur.id)
	if err != nil {
		return nil, err
	}
	return hostedBuildView(r, false), nil
}

// buildTarget is what a build builds into, and the pack's name for the
// panel.
type buildTarget struct{ name, pack, label string }

// hostedTarget checks, before anything is uploaded, that the pack the
// build will write can come back to this computer: a new pack's name must
// be free here, and a pack to add to must be kept in Agentry with the tree
// this computer holds (otherwise the build would start from pages this
// computer does not have, and its download would be refused over a copy
// changed here).
func (k *KnowledgeAPI) hostedTarget(ctx context.Context, reg, name, addTo string) (buildTarget, error) {
	list, err := k.env().List(ctx, reg)
	if err != nil {
		return buildTarget{}, err
	}
	if addTo == "" {
		if err := validPackName(name); err != nil {
			return buildTarget{}, err
		}
		for _, p := range list.Packs {
			if p.Name == name {
				return buildTarget{}, fmt.Errorf("a pack named %s is already kept here or in Agentry: add documents to it, or choose another name", name)
			}
		}
		return buildTarget{name: name, label: name}, nil
	}
	if list.Agentry != "" {
		return buildTarget{}, fmt.Errorf("add documents In Agentry: %s", list.Agentry)
	}
	for _, p := range list.Packs {
		if p.Name != addTo || !p.InAgentry {
			continue
		}
		if p.Changed {
			return buildTarget{}, fmt.Errorf("%s changed on this computer since it was uploaded: Upload to Agentry first, so the build starts from the pages this computer holds", addTo)
		}
		return buildTarget{pack: p.AgentryID, label: addTo}, nil
	}
	return buildTarget{}, fmt.Errorf("%s is not kept in Agentry: Upload to Agentry first, then add documents In Agentry", addTo)
}

// validPackName is the pack name rule the CLI and the registry hold a name
// to: 1-64 lowercase letters, digits, '.', '-' or '_', starting with a
// letter or a digit.
func validPackName(name string) error {
	ok := name != "" && len(name) <= 64
	for i, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case (c == '.' || c == '-' || c == '_') && i > 0:
		default:
			ok = false
		}
	}
	if !ok {
		return fmt.Errorf("a pack's name is 1-64 lowercase letters, digits, '.', '-' or '_', starting with a letter or a digit")
	}
	return nil
}

func (k *KnowledgeAPI) createHosted(ctx context.Context, reg, token, name string, req hostedrun.CreateRequest) (*KnowledgeHostedStartView, error) {
	created, err := hostedrun.New(reg, token).Create(ctx, req)
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return hostedRefusal(refusal), nil
		}
		return nil, err
	}
	if created.Build != nil && created.Build.Name != "" {
		name = created.Build.Name
	}
	k.followHostedBuild(reg, token, name, created)
	return &KnowledgeHostedStartView{Build: hostedBuildView(created, false)}, nil
}

// followHostedBuild watches the build in the background, reports every
// status change as knowledge:hosted-build, and once it is over and its tree
// is the pack's, waits for Agentry's scan and downloads the pack into the
// local store.
func (k *KnowledgeAPI) followHostedBuild(reg, token, name string, started *hostedrun.Run) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &hostedFollow{registry: reg, id: started.ID, cancel: cancel, done: make(chan struct{})}
	if !k.hosted.follow(f) {
		cancel()
		return
	}
	client := hostedrun.New(reg, token)
	go func() {
		defer close(f.done)
		defer cancel()
		defer k.hosted.done(f)
		final, err := client.Watch(ctx, started.ID, hostedrun.WatchOptions{
			Poll:     hostedPoll,
			OnStatus: func(r *hostedrun.Run) { emitEvent(k.app, eventKnowledgeHosted, hostedBuildView(r, false)) },
		})
		if err != nil {
			last, gerr := client.Get(context.Background(), started.ID)
			if gerr != nil || !last.Terminal() {
				v := hostedBuildView(started, true)
				if gerr == nil {
					v = hostedBuildView(last, true)
				}
				v.Error = "lost touch with the registry while the build was in flight (" + err.Error() + "); reopen Create knowledge to follow it again"
				emitEvent(k.app, eventKnowledgeHosted, v)
				return
			}
			final = last
		}
		emitEvent(k.app, eventKnowledgeHosted, k.landHostedBuild(ctx, reg, name, final))
	}()
}

// landHostedBuild is a finished build's last step on this computer: when
// its tree is the pack's, wait for Agentry's scan to end and download the
// sealed pack into the local store, then read the kept pack's scan.
func (k *KnowledgeAPI) landHostedBuild(ctx context.Context, reg, name string, final *hostedrun.Run) *KnowledgeHostedBuildView {
	v := hostedBuildView(final, true)
	if final.Build == nil || !final.Build.Applied {
		return v
	}
	if final.Build.Name != "" {
		name = final.Build.Name
	}
	env := k.env()
	waiting := hostedBuildView(final, false)
	waiting.StatusLine = "The build is done. Waiting for Agentry's scan of " + name + " to end, then it is downloaded here."
	emitEvent(k.app, eventKnowledgeHosted, waiting)
	deadline := time.Now().Add(hostedScanWait)
	transient := 0
	for {
		kept, err := env.Download(ctx, reg, name)
		if err == nil {
			v.Downloaded, v.Kept = true, kept
			break
		}
		if ctx.Err() == nil && !errors.Is(err, store.ErrScanning) && transient < 3 && hostedrun.RefusalOf(err) == nil && !isStoreRefusal(err) {
			// A network blip after the build: a few more tries, as the
			// watch allows.
			transient++
		} else if !errors.Is(err, store.ErrScanning) || time.Now().After(deadline) {
			v.DownloadError = err.Error()
			if errors.Is(err, store.ErrScanning) {
				v.DownloadError = "Agentry is still scanning " + name + "; Download it from the Knowledge panel once the scan ends."
			}
			return v
		}
		select {
		case <-ctx.Done():
			v.DownloadError = "Stopped waiting for Agentry's scan of " + name + "; Download it from the Knowledge panel once the scan ends."
			return v
		case <-time.After(hostedPoll):
		}
	}
	if list, err := env.List(ctx, reg); err == nil {
		for _, p := range list.Packs {
			if p.Name == name && p.Local {
				v.ScanState, v.Scan = p.ScanState, p.Scan
				break
			}
		}
	}
	return v
}

// claimHosted takes the follow slot for a build about to be started or
// reattached, or refuses when one is already followed or being started.
func (k *KnowledgeAPI) claimHosted() error {
	return k.hosted.claim("a build In Agentry is already in progress: wait for it to finish, or cancel it")
}

// releaseHosted gives back a claim that did not become a follow.
func (k *KnowledgeAPI) releaseHosted() { k.hosted.release() }

// isStoreRefusal is a refusal of the local store's that a retry will not
// change: a name or a pack it will not replace.
func isStoreRefusal(err error) bool {
	return errors.Is(err, store.ErrNameTaken) || errors.Is(err, store.ErrRemoteNotFound)
}

func (k *KnowledgeAPI) hostedCurrent() *hostedFollow { return k.hosted.current() }

// hostedRefusal is the registry's refusal as the panel renders it.
func hostedRefusal(r *hostedrun.Refusal) *KnowledgeHostedStartView {
	v := &KnowledgeHostedStartView{Refusal: r.Slug, Detail: r.Detail}
	if v.Detail == "" {
		v.Detail = r.Sentence()
	}
	if r.Known {
		v.Balance, v.Hold, v.ResetsAt = r.Balance, r.Hold, r.ResetsAt
	}
	return v
}

// runWord is the word "run" alone, never inside "running".
var runWord = regexp.MustCompile(`\brun\b`)

// hostedBuildView turns the registry's record into the panel's words. The
// state and outcome words are a hosted run's (hostedRunView), so a build
// reads as the runs do.
func hostedBuildView(r *hostedrun.Run, final bool) *KnowledgeHostedBuildView {
	rv := hostedRunView(r, "", nil, "", "", final)
	v := &KnowledgeHostedBuildView{
		ID: r.ID, Status: r.Status, State: rv.State, StatusLine: runWord.ReplaceAllString(rv.StatusLine, "build"),
		QueuePosition: r.QueuePosition, CancelRequested: r.CancelRequested,
		Outcome: r.Outcome, OutcomeLine: rv.OutcomeLine, OutcomeTone: rv.OutcomeTone, Error: r.Error,
		HoldCredits: r.HoldCredits, SettledCredits: rv.SettledCredits, Settled: rv.Settled,
		Turns:     rv.Turns,
		Documents: []HostedArtefactView{}, CanContinue: hostedrun.CanContinue(r), Final: final,
	}
	for _, in := range r.Inputs {
		v.Documents = append(v.Documents, HostedArtefactView{Name: in.Name, Bytes: in.Bytes})
	}
	if line := buildOutcomeLine(r); line != "" {
		v.OutcomeLine = line
	}
	if b := r.Build; b != nil {
		v.Name, v.Applied, v.Pages, v.Bytes = b.Name, b.Applied, b.Pages, b.Bytes
		if b.Applied {
			v.ScanCredits = b.ScanCredits
		}
		v.Note = hostedrun.NoteSentence(b.Note)
	}
	return v
}

// regHost is a registry's host, for sentences.
func regHost(reg string) string {
	h := strings.TrimPrefix(strings.TrimPrefix(reg, "https://"), "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	return h
}

// buildOutcomeLine is how a build ended, in a build's words: a run's lines
// speak of an agent and its outputs, and a build has neither.
func buildOutcomeLine(r *hostedrun.Run) string {
	if r.Status == hostedrun.StatusCancelled {
		return "Cancelled before the build finished."
	}
	switch r.Outcome {
	case "succeeded":
		return "The Knowledge Creator finished its pages."
	case "failed":
		return "The Knowledge Creator ran and did not finish its pages."
	case "refused":
		return "Refused before it ran; the hold was refunded."
	case "timeout":
		return "Stopped at its time limit."
	case "budget":
		return "Stopped at its turn or cost limit."
	case "interrupted":
		return "Interrupted before it finished, by your cancel or at its limit on model use."
	case "error":
		return "The platform could not run it; the hold was refunded."
	}
	return ""
}
