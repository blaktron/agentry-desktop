// Describe it In Agentry (plan builder-rework.md §7.1, §7.4, §7.5, D5,
// D15, D16; Builder M7, agentry-desktop#281): the Build tab's draft made on
// a sandbox host by the built-in Drafter against the platform's model, for
// credits, with every text it reads scanned by AI Review through the
// registry. It needs an account, since it spends credits.
//
// The draft is a run of kind draft in the registry's runs extension
// (agentry-enterprise#183): created with the description and hints, as
// StartHostedRun creates a run, and followed by polling, as a run In
// Agentry is, with every state change sent as build:hosted-draft. When it
// is over, its sealed output.tar is unpacked into the run's own folder as
// any run In Agentry's is, and its draft.json is decoded into the Build
// form's Model as data (internal/draft ReadHosted, D-041): nothing in it
// is opened or launched, and nothing reaches a QuickAgent folder until the
// person saves into one the native picker gives. No platform key reaches
// this app; the token is the account's own.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventDraftHosted carries a draft In Agentry's every state change and its
// final word.
const eventDraftHosted = "build:hosted-draft"

// DescribeHostedView answers "is In Agentry offered for Describe it here,
// can it be chosen" and what the page states before anything is created:
// the hold in the site's sentence, the caps, the balance, and the honest
// line about where the words go. Every number is the registry's.
type DescribeHostedView struct {
	Offered   bool   `json:"offered"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Remedy    string `json:"remedy,omitempty"`
	SignedIn  bool   `json:"signedIn"`

	HoldCredits    int64  `json:"holdCredits"`
	HoldSentence   string `json:"holdSentence,omitempty"`
	InputsSentence string `json:"inputsSentence"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	MaxTurns       int    `json:"maxTurns"`
	MaxPages       int    `json:"maxPages"`

	// Balance is the credits the draft is paid from; Pool names the
	// organization for a member; BalanceKnown is false when billing did
	// not answer. Short says a metered payer has fewer credits than the
	// hold, so the registry will refuse the draft.
	Balance      int64  `json:"balance"`
	BalanceKnown bool   `json:"balanceKnown"`
	Pool         string `json:"pool,omitempty"`
	Unmetered    bool   `json:"unmetered"`
	Short        bool   `json:"short"`
}

// HostedDraftView is one draft In Agentry as the page shows it: where it
// stands while it moves, and once it is over how it ended, what it cost,
// and the draft decoded into the form's Model, or why there is none.
type HostedDraftView struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	State           string `json:"state"`
	StatusLine      string `json:"statusLine"`
	QueuePosition   int    `json:"queuePosition,omitempty"`
	CancelRequested bool   `json:"cancelRequested"`

	Outcome     string `json:"outcome,omitempty"`
	OutcomeLine string `json:"outcomeLine,omitempty"`
	OutcomeTone string `json:"outcomeTone,omitempty"`
	Error       string `json:"error,omitempty"`

	HoldCredits int64  `json:"holdCredits"`
	CreditsLine string `json:"creditsLine"`
	// Draft is the finished draft, for Review; DraftError says why a draft
	// that ended has none the form can open.
	Draft      *DraftView `json:"draft,omitempty"`
	DraftError string     `json:"draftError,omitempty"`
	// Final says this is the last word on the draft.
	Final bool `json:"final"`
}

// HostedDraftStartView is what starting or reattaching returns: the draft,
// or the registry's refusal as a slug and a sentence.
type HostedDraftStartView struct {
	Draft    *HostedDraftView `json:"draft,omitempty"`
	Refusal  string           `json:"refusal,omitempty"`
	Detail   string           `json:"detail,omitempty"`
	Balance  int64            `json:"balance,omitempty"`
	Hold     int64            `json:"hold,omitempty"`
	ResetsAt string           `json:"resetsAt,omitempty"`
}

// DescribeInAgentryStatus answers the page's question for reg: may a
// draft be made In Agentry, and what would it hold.
func (b *BuildAPI) DescribeInAgentryStatus(reg string) (*DescribeHostedView, error) {
	reg = settings.Normalize(reg)
	// The token is kept by host, so it goes only to a registry URL that
	// Settings would accept: https, or plain http on this machine.
	if err := settings.ValidateURL(reg); err != nil {
		return nil, err
	}
	ctx := b.app.ctx
	token := settings.TokenFor(reg)
	v := &DescribeHostedView{SignedIn: token != "", InputsSentence: hostedrun.DraftInputsSentence}
	f := hostedrun.DraftFacts{SignedIn: v.SignedIn}
	if f.SignedIn {
		if exts, err := extensionsOf(ctx, reg); err == nil {
			f.Runs = hasString(exts, "runs")
		}
	}
	if f.SignedIn && f.Runs {
		f.Scopes, f.ScopesKnown = tokenScopes(ctx, reg, token)
		client := hostedrun.New(reg, token)
		if st, err := client.Status(ctx); err == nil {
			f.StatusKnown, f.Terms = true, st.Draft
			if st.Caller != nil {
				f.Hold = st.Caller.Draft
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
		if f.Terms != nil && f.Hold != nil {
			v.HoldCredits = f.Hold.HoldCredits
			v.HoldSentence = hostedrun.DraftHoldSentence(f.Hold.HoldCredits, *f.Terms)
			v.TimeoutSeconds, v.MaxTurns, v.MaxPages = f.Hold.Caps.TimeoutSeconds, f.Hold.Caps.MaxTurns, f.Terms.MaxPages
		}
		if sub, err := fetchSubscription(ctx, reg, token); err == nil && sub != nil {
			v.Balance, v.BalanceKnown, v.Pool, v.Unmetered = sub.Balance, true, sub.MemberOf, sub.Unmetered
			v.Short = !sub.Unmetered && v.HoldCredits > 0 && sub.Balance < v.HoldCredits
		}
	}
	o := hostedrun.DecideDraft(f)
	v.Offered, v.Available, v.Reason, v.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
	return v, nil
}

// DescribeInAgentry drafts a QuickAgent from a description In Agentry.
// The description and hints are held to the registry's bounds before
// anything is sent, and the draft is created and followed; it is this
// app's one draft In Agentry while it lasts.
func (b *BuildAPI) DescribeInAgentry(reg, description string, hints []string) (*HostedDraftStartView, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, err
	}
	ctx := b.app.ctx
	token := settings.TokenFor(reg)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to draft In Agentry: a draft spends credits", regHost(reg))
	}
	description, hints, err := hostedrun.CheckDraft(description, hints)
	if err != nil {
		return nil, err
	}
	if err := b.claimHosted(); err != nil {
		return nil, err
	}
	claimed := true
	defer func() {
		if claimed {
			b.releaseHosted()
		}
	}()
	client := hostedrun.New(reg, token)
	created, err := client.Create(ctx, hostedrun.CreateRequest{Kind: hostedrun.KindDraft, Description: description, Hints: hints})
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return hostedDraftRefusal(refusal), nil
		}
		return nil, err
	}
	if !created.IsDraft() {
		return nil, fmt.Errorf("the registry answered the draft with a %q, not a draft", created.Kind)
	}
	if !validRunID(created.ID) {
		return nil, fmt.Errorf("the registry named this draft %q, which is not a run id", created.ID)
	}
	claimed = false
	b.followHostedDraft(reg, token, created)
	return &HostedDraftStartView{Draft: hostedDraftView(created, false)}, nil
}

// AttachHostedDraft follows this account's draft in flight, if it has one,
// so a reopened page shows it rather than offering a second.
func (b *BuildAPI) AttachHostedDraft(reg string) (*HostedDraftStartView, error) {
	reg = settings.Normalize(reg)
	token := settings.TokenFor(reg)
	if settings.ValidateURL(reg) != nil || token == "" {
		return &HostedDraftStartView{}, nil
	}
	client := hostedrun.New(reg, token)
	if cur := b.hostedCurrent(); cur != nil {
		if cur.id == "" {
			// A start or another attach holds the slot; its own events
			// report the draft.
			return &HostedDraftStartView{}, nil
		}
		if fresh, err := client.Get(b.app.ctx, cur.id); err == nil {
			return &HostedDraftStartView{Draft: hostedDraftView(fresh, false)}, nil
		}
		return &HostedDraftStartView{}, nil
	}
	if err := b.claimHosted(); err != nil {
		return &HostedDraftStartView{}, nil //nolint:nilerr // another call took the slot first
	}
	runs, err := client.List(b.app.ctx)
	if err != nil {
		b.releaseHosted()
		return &HostedDraftStartView{}, nil //nolint:nilerr // no list, nothing to reattach to
	}
	for i := range runs {
		r := &runs[i]
		if r.IsDraft() && hostedrun.Active(r.Status) && validRunID(r.ID) {
			b.followHostedDraft(reg, token, r)
			return &HostedDraftStartView{Draft: hostedDraftView(r, false)}, nil
		}
	}
	b.releaseHosted()
	return &HostedDraftStartView{}, nil
}

// CancelHostedDraft asks the registry to stop the draft this app follows.
func (b *BuildAPI) CancelHostedDraft() (*HostedDraftView, error) {
	cur := b.hostedCurrent()
	if cur == nil || cur.id == "" {
		return nil, fmt.Errorf("no draft In Agentry is in progress")
	}
	token := settings.TokenFor(cur.registry)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to cancel this draft", regHost(cur.registry))
	}
	r, err := hostedrun.New(cur.registry, token).Cancel(b.app.ctx, cur.id)
	if err != nil {
		return nil, err
	}
	return hostedDraftView(r, false), nil
}

// DescribeAgainText is the description a new draft starts from, as the
// site composes it (plan D11; internal/draft DescribeAgain): what the
// person first asked, what they want changed now, and the form's draft as
// it stands, written by the form's own renderer. The page sends it to
// DescribeInAgentry or DescribeOnThisComputer. It reads and writes nothing.
func (b *BuildAPI) DescribeAgainText(m build.Model, followUp, firstDescription string) (string, error) {
	followUp = strings.TrimSpace(followUp)
	if followUp == "" {
		return "", errors.New("describe again: say what should change")
	}
	if len(followUp) > draft.MaxFollowUp {
		return "", fmt.Errorf("describe again: what should change is at most %d bytes", draft.MaxFollowUp)
	}
	return draft.DescribeAgain(followUp, draft.FirstDescription(firstDescription), m.Check().YAML, m.Instructions), nil
}

// followHostedDraft watches the draft in the background, reports every
// status change as build:hosted-draft, and once it is over unpacks its
// output into the run's folder and decodes the draft.
func (b *BuildAPI) followHostedDraft(reg, token string, started *hostedrun.Run) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &hostedFollow{registry: reg, id: started.ID, cancel: cancel, done: make(chan struct{})}
	if !b.hosted.follow(f) {
		cancel()
		return
	}
	client := hostedrun.New(reg, token)
	go func() {
		defer close(f.done)
		defer cancel()
		defer b.hosted.done(f)
		final, err := client.Watch(ctx, started.ID, hostedrun.WatchOptions{
			Poll:     hostedPoll,
			OnStatus: func(r *hostedrun.Run) { emitEvent(b.app, eventDraftHosted, hostedDraftView(r, false)) },
		})
		if err != nil {
			last, gerr := client.Get(context.Background(), started.ID)
			if gerr != nil || !last.Terminal() {
				v := hostedDraftView(started, true)
				if gerr == nil {
					v = hostedDraftView(last, true)
				}
				v.Error = "lost touch with the registry while the draft was in flight (" + err.Error() + "); reopen Describe it to follow it again"
				emitEvent(b.app, eventDraftHosted, v)
				return
			}
			final = last
		}
		emitEvent(b.app, eventDraftHosted, b.landHostedDraft(ctx, client, reg, token, started.ID, final))
	}()
}

// landHostedDraft is a finished draft's last step on this computer: its
// output unpacked into the run's own folder, and its draft.json decoded
// into the form's Model under the person's namespace. The folder is named
// by id, the draft's id as the create (or the reattach) answered and this
// app checked it, never by what a later poll says: a registry that names
// the draft otherwise at the end gets nothing unpacked.
func (b *BuildAPI) landHostedDraft(ctx context.Context, client *hostedrun.Client, reg, token, id string, final *hostedrun.Run) *HostedDraftView {
	v := hostedDraftView(final, true)
	if final.Status != hostedrun.StatusFinished || final.Outcome != hostedrun.OutcomeSucceeded {
		return v
	}
	if final.ID != id || !validRunID(id) {
		v.DraftError = fmt.Sprintf("the registry answered the draft %s as %q, so its output was not downloaded", id, final.ID)
		return v
	}
	outDir, err := run.OutputDir("", drafter.Namespace, drafter.Name, id)
	if err != nil {
		v.DraftError = "there is no folder for the draft's output on this computer (" + err.Error() + ")"
		return v
	}
	if _, err := client.DownloadOutputs(ctx, final, outDir); err != nil {
		v.DraftError = "the draft's output could not be downloaded (" + err.Error() + ")"
		return v
	}
	ns := ""
	if u, err := whoami(b.app, reg, token); err == nil {
		ns = u.Handle
	}
	res, err := draft.ReadHosted(outDir, ns)
	if err != nil {
		v.DraftError = err.Error()
		return v
	}
	dv := draftView(res)
	dv.Where = draftInAgentry
	dv.Description = final.Task
	if final.Draft != nil {
		dv.Hints = append([]string{}, final.Draft.Hints...)
	}
	dv.settle(ctx, reg, token, ns)
	v.Draft = dv
	return v
}

// claimHosted, releaseHosted and hostedCurrent are the draft's follow
// slot (hostedSlot).
func (b *BuildAPI) claimHosted() error {
	return b.hosted.claim("a draft In Agentry is already in progress: wait for it to finish, or cancel it")
}

func (b *BuildAPI) releaseHosted() { b.hosted.release() }

func (b *BuildAPI) hostedCurrent() *hostedFollow { return b.hosted.current() }

// hostedDraftRefusal is the registry's refusal as the page renders it.
func hostedDraftRefusal(r *hostedrun.Refusal) *HostedDraftStartView {
	v := &HostedDraftStartView{Refusal: r.Slug, Detail: r.Detail}
	if v.Detail == "" {
		v.Detail = r.Sentence()
	}
	if r.Known {
		v.Balance, v.Hold, v.ResetsAt = r.Balance, r.Hold, r.ResetsAt
	}
	return v
}

// hostedDraftView turns the registry's record into the page's words. The
// state words are a hosted run's (hostedRunView), with "draft" for "run".
func hostedDraftView(r *hostedrun.Run, final bool) *HostedDraftView {
	rv := hostedRunView(r, "", nil, "", "", final)
	v := &HostedDraftView{
		ID: r.ID, Status: r.Status, State: rv.State, StatusLine: runWord.ReplaceAllString(rv.StatusLine, "draft"),
		QueuePosition: r.QueuePosition, CancelRequested: r.CancelRequested,
		Outcome: r.Outcome, OutcomeTone: rv.OutcomeTone, Error: r.Error,
		HoldCredits: r.HoldCredits, CreditsLine: draftCreditsLine(r), Final: final,
	}
	if r.Status == hostedrun.StatusRunning || r.Status == hostedrun.StatusClaimed {
		v.StatusLine = "It is drafting the QuickAgent from your description."
	}
	if r.Terminal() {
		v.OutcomeLine = draftOutcomeLine(r)
		v.StatusLine = v.OutcomeLine
	}
	return v
}

// draftOutcomeLine is how a draft ended, in a draft's words.
func draftOutcomeLine(r *hostedrun.Run) string {
	if r.Status == hostedrun.StatusCancelled {
		return "Cancelled before the draft started. No draft came of it."
	}
	switch r.Outcome {
	case hostedrun.OutcomeSucceeded:
		return "The Drafter finished the draft."
	case hostedrun.OutcomeFailed:
		return "The Drafter ran and did not finish a draft. No draft came of it."
	case hostedrun.OutcomeRefused:
		return "Refused before the Drafter started: AI Review reads the description first and holds back one it flags. The hold was refunded. No draft came of it."
	case hostedrun.OutcomeTimeout:
		return "Stopped at its time limit. No draft came of it."
	case hostedrun.OutcomeBudget:
		return "Stopped at its token or credit limit. No draft came of it."
	case hostedrun.OutcomeInterrupted:
		return "Cancelled while it ran; what it used was settled. No draft came of it."
	case hostedrun.OutcomeError:
		return "The platform could not run it; the hold was refunded. No draft came of it."
	}
	return "Done."
}

// draftCreditsLine is the site's sentence about a draft's ledger
// (agentry-web lib/runs.ts creditsSentence): the hold while it is in
// flight, then what it cost, naming the searches' and AI Review's parts
// the record wrote.
func draftCreditsLine(r *hostedrun.Run) string {
	if r.SettledCredits == nil || !r.Terminal() {
		return fmt.Sprintf("%d credits are held for this draft.", r.HoldCredits)
	}
	if *r.SettledCredits == 0 {
		return fmt.Sprintf("%d credits were held and all of them were released: this draft cost nothing.", r.HoldCredits)
	}
	of := ""
	if parts := hostedrun.DraftSettledParts(r.Draft); len(parts) > 0 {
		of = " (" + strings.Join(parts, ", ") + ")"
	}
	return fmt.Sprintf("%d credits were held; %d settled%s and the rest was released.", r.HoldCredits, *r.SettledCredits, of)
}
