// VoiceAPI: the Voice page. A person keeps a Writing Voice in two places,
// never synced on their own: this computer's (the CLI's local store, free,
// never reviewed) and, signed in, the account's (reviewed by AI Review
// before it is live). Upload to Cloud (Submit) sends this computer's up for
// review; Download Cloud version (UseAccount) keeps the account's live one
// here, each warning first when it would put an older voice over a newer
// one. Edit with AI (WriteWithAI) runs the Voice Writer on this computer for
// no credits: an AI Edit with a note and optional samples, or a Create where
// this computer keeps no voice. The Cloud Voice page is the site's Voice
// settings page: the account's editor, Discard of a pending version, Restore
// previous, Delete, and Edit with AI run In Agentry (HostedWithAI) as a
// hosted run at the registry's flat price.
//
// The gates are the Knowledge panel's: samples come only from the native
// pickers; a download over a different voice here, an upload after a newer
// version in the account, a Discard and both Deletes are asked in a native
// dialog (a Save or a Voice Writer run replaces this computer's voice at the
// person's own press, as the page says beside it); and a Voice Writer run
// here takes the run lock.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/voice/build"
	"github.com/blaktron/agentry-cli/pkg/voice/remote"

	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

// eventVoiceLog carries a Voice Writer run on this computer, line by line.
const eventVoiceLog = "voice:write-log"

// eventVoiceHosted carries a Voice Writer run In Agentry's every state
// change and its final word.
const eventVoiceHosted = "voice:hosted"

// voiceRunRef names a Voice Writer run as the run in flight.
const voiceRunRef = "the Voice Writer"

// VoiceAPI is the Voice page's bound surface.
type VoiceAPI struct {
	app *App
	run *RunAPI
	// build is the Voice Writer seam a test substitutes; nil is the CLI's.
	build voice.Builder
	// hosted is the Voice Writer run In Agentry this app follows.
	hosted hostedSlot
}

func (v *VoiceAPI) env() voice.Env { return v.run.voiceEnv() }

// VoiceView is the panel: this computer's voice, and, signed in to a
// registry that keeps voices, the account's.
type VoiceView struct {
	Registry string       `json:"registry"`
	Local    *voice.Local `json:"local"`
	SignedIn bool         `json:"signedIn"`
	// AccountOffered says the registry keeps voices for this account;
	// Account is its voice then. AccountError is why it could not be read.
	AccountOffered bool           `json:"accountOffered"`
	Account        *voice.Account `json:"account,omitempty"`
	AccountError   string         `json:"accountError,omitempty"`
	// Model is the model the Voice Writer runs on here, from the runner
	// policy.
	Model string `json:"model"`
	// ReviewCredits is what AI Review of a voice saved or submitted costs
	// here (the runs status route's voiceBuild.aiReviewCredits; 0 where the
	// registry charges nothing or did not say). AttachEnabled says a run In
	// Agentry may write in the account's voice here (voiceBuild.attach).
	ReviewCredits int64 `json:"reviewCredits"`
	AttachEnabled bool  `json:"attachEnabled"`
	// Sync is how this computer's voice stands against the account's, for
	// Download Cloud version and Upload to Cloud.
	Sync voice.Sync `json:"sync"`
}

// View is the Voice page for reg.
func (v *VoiceAPI) View(reg string) (*VoiceView, error) {
	reg = settings.Normalize(reg)
	ctx := v.app.ctx
	e := v.env()
	local, err := e.Local(ctx, reg)
	if err != nil {
		return nil, err
	}
	out := &VoiceView{Registry: reg, Local: local, SignedIn: e.SignedIn(reg), Model: voice.Model()}
	if !out.SignedIn {
		return out, nil
	}
	a, err := e.Account(ctx, reg)
	switch {
	case errors.Is(err, remote.ErrUnconfigured):
	case err != nil:
		out.AccountOffered, out.AccountError = true, "Your account's voice could not be read: "+err.Error()
	default:
		out.AccountOffered, out.Account, out.Sync = true, a, voice.SyncOf(local, a)
	}
	// The token is found by host alone: the status route is asked only at a
	// URL Settings' rule admits.
	if out.AccountOffered && settings.ValidateURL(reg) == nil {
		if st, err := hostedrun.New(reg, e.TokenFor(reg)).Status(ctx); err == nil && st.VoiceBuild != nil {
			out.ReviewCredits, out.AttachEnabled = st.VoiceBuild.AIReviewCredits, st.VoiceBuild.Attach.Enabled
		}
	}
	return out, nil
}

// VoiceSaveView is a voice kept on this computer, and whether it was the
// first, which is now on for every run here.
type VoiceSaveView struct {
	Voice   *voice.Local `json:"voice"`
	Created bool         `json:"created"`
}

// SaveLocal keeps text as this computer's voice: free and immediate.
func (v *VoiceAPI) SaveLocal(reg, text string) (*VoiceSaveView, error) {
	l, created, err := v.env().SaveLocal(v.app.ctx, settings.Normalize(reg), text)
	if err != nil {
		return nil, err
	}
	return &VoiceSaveView{Voice: l, Created: created}, nil
}

// SetLocalAttach sets whether every run on this computer writes in its
// voice (Run defaults).
func (v *VoiceAPI) SetLocalAttach(reg string, on bool) error {
	return v.env().SetLocalAttach(settings.Normalize(reg), on)
}

// DeleteLocal removes this computer's voice after a native dialog. The
// account's is not touched.
func (v *VoiceAPI) DeleteLocal(reg string) error {
	if err := v.app.askConsent("Delete this computer's voice?",
		"This deletes the Writing Voice kept on this computer and its key. Runs here no longer write in it. "+
			"Your account's voice in Agentry is not touched. Continue?"); err != nil {
		return err
	}
	return v.env().DeleteLocal(settings.Normalize(reg))
}

// SaveAccount sends text as the account's pending version: AI Review reads
// it before it is live, and the registry may charge a credit for that.
func (v *VoiceAPI) SaveAccount(reg, text string) (*voice.Account, error) {
	return v.env().SaveAccount(v.app.ctx, settings.Normalize(reg), text)
}

// ErrAlreadyTheSame is a download or an upload of the voice the other
// place already keeps.
var ErrAlreadyTheSame = errors.New("already the same: this computer and your account keep this voice")

// Submit is Upload to Cloud: it sends this computer's voice to the account
// for review, asking first in a native dialog when the account's newest
// version was set after this computer's voice was saved. Nothing on this
// computer changes.
func (v *VoiceAPI) Submit(reg string) (*voice.Account, error) {
	reg = settings.Normalize(reg)
	ctx := v.app.ctx
	e := v.env()
	here, err := e.Local(ctx, reg)
	if err != nil {
		return nil, err
	}
	a, err := e.Account(ctx, reg)
	if err != nil {
		return nil, err
	}
	s := voice.SyncOf(here, a)
	if s.UploadSame {
		return nil, ErrAlreadyTheSame
	}
	if title, msg, ask := uploadWarning(here, a, s, v.reviewCredits(ctx, reg)); ask {
		if err := v.app.askConsent(title, msg); err != nil {
			return nil, err
		}
	}
	return e.Submit(ctx, reg)
}

// uploadWarning is Upload to Cloud's native confirm: asked only when the
// account's newest version is the newer one.
func uploadWarning(here *voice.Local, a *voice.Account, s voice.Sync, credits int64) (title, msg string, ask bool) {
	if !s.AccountAfterLocal {
		return "", "", false
	}
	t, _ := here.UpdatedAt()
	cost := "It goes to AI Review."
	if credits > 0 {
		cost = "It goes to AI Review for " + creditsText(credits) + "."
	}
	return "Upload this computer's voice?",
		fmt.Sprintf("The cloud version was set after the one on this computer (%s against %s). Upload this computer's voice anyway? %s",
			confirmStamp(a.Newest().CreatedAt), confirmStamp(t), cost), true
}

// downloadWarning is Download Cloud version's native confirm, which folds in
// the confirm Use always asked before it replaced a different voice here:
// asked when it would replace one, worded for a voice here saved after the
// live version, and saying so when a newer version is in review in the
// account.
func downloadWarning(here *voice.Local, a *voice.Account, s voice.Sync) (title, msg string, ask bool) {
	replaces := here.Exists && !s.DownloadSame
	inReview := ""
	if s.InReview {
		inReview = " A newer version is in review in your account and is not live yet."
	}
	switch {
	case replaces && s.LocalAfterLive:
		t, _ := here.UpdatedAt()
		return "Replace this computer's voice?", fmt.Sprintf("The voice on this computer was saved after the cloud version (%s against %s).%s "+
			"Replace it with the cloud version? The one here cannot be recovered.", confirmStamp(t), confirmStamp(a.Live.CreatedAt), inReview), true
	case replaces:
		return "Replace this computer's voice?", "This replaces the Writing Voice kept on this computer with your account's live one." + inReview +
			" The one here cannot be recovered. Continue?", true
	case s.InReview:
		return "Download the cloud version?", strings.TrimSpace(inReview) + " Download the live version anyway?", true
	}
	return "", "", false
}

// confirmStamp is a version's time in a native confirm: "4 Oct 14:02",
// this computer's local time.
func confirmStamp(t time.Time) string { return t.Local().Format("2 Jan 15:04") }

// reviewCredits is what AI Review of a voice costs at reg (the runs status
// route's voiceBuild.aiReviewCredits); 0 where it charges nothing or did not
// say. The token is found by host alone, so the URL is held to Settings'
// rule before it is looked up.
func (v *VoiceAPI) reviewCredits(ctx context.Context, reg string) int64 {
	if settings.ValidateURL(reg) != nil {
		return 0
	}
	st, err := hostedrun.New(reg, v.env().TokenFor(reg)).Status(ctx)
	if err != nil || st.VoiceBuild == nil {
		return 0
	}
	return st.VoiceBuild.AIReviewCredits
}

// UseAccount is Download Cloud version: it keeps the account's live voice
// on this computer, asking first in a native dialog when it replaces a
// different one here or a newer version is in review (downloadWarning).
func (v *VoiceAPI) UseAccount(reg string) (*VoiceSaveView, error) {
	reg = settings.Normalize(reg)
	ctx := v.app.ctx
	e := v.env()
	a, err := e.Account(ctx, reg)
	if err != nil {
		return nil, err
	}
	if a.Live == nil {
		return nil, voice.ErrNoLiveVoice
	}
	here, err := e.Local(ctx, reg)
	if err != nil {
		return nil, err
	}
	if title, msg, ask := downloadWarning(here, a, voice.SyncOf(here, a)); ask {
		if err := v.app.askConsent(title, msg); err != nil {
			return nil, err
		}
	}
	l, created, err := e.UseAccount(ctx, reg)
	if err != nil {
		return nil, err
	}
	return &VoiceSaveView{Voice: l, Created: created}, nil
}

// SetAccountAttach sets whether runs In Agentry write in the account's live
// voice (Profile).
func (v *VoiceAPI) SetAccountAttach(reg string, on bool) (*voice.Account, error) {
	return v.env().SetAccountAttach(v.app.ctx, settings.Normalize(reg), on)
}

// DiscardAccount drops the account's pending version, in review or flagged,
// after a native dialog (the Cloud Voice page's Discard). The live voice
// stays live.
func (v *VoiceAPI) DiscardAccount(reg string) (*voice.Account, error) {
	if err := v.app.askConsent("Discard your new version?",
		"This drops the version of your voice waiting in your account, in review or flagged by AI Review. Your live voice, if you have one, stays live. "+
			"The discarded text cannot be recovered. Continue?"); err != nil {
		return nil, err
	}
	return v.env().DiscardAccount(v.app.ctx, settings.Normalize(reg))
}

// VoiceRestoreView is the account after Restore previous, and whether the
// restored version went to AI Review again rather than live at once.
type VoiceRestoreView struct {
	Account  *voice.Account `json:"account"`
	InReview bool           `json:"inReview"`
}

// RestoreAccount puts the account's previous version back (Restore
// previous). It is asked nowhere, as on the site: the line beside it says
// whether it goes live at once or to AI Review, and a swap is undone by
// restoring again.
func (v *VoiceAPI) RestoreAccount(reg string) (*VoiceRestoreView, error) {
	a, inReview, err := v.env().RestoreAccount(v.app.ctx, settings.Normalize(reg))
	if err != nil {
		return nil, err
	}
	return &VoiceRestoreView{Account: a, InReview: inReview}, nil
}

// DeleteAccount deletes the account's voice, every version and its key,
// after a native dialog. This computer's voice is not touched.
func (v *VoiceAPI) DeleteAccount(reg string) error {
	if err := v.app.askConsent("Delete your account's voice?",
		"This deletes your Writing Voice in Agentry: the live version, any version waiting for AI Review, the previous one, and its key. "+
			"Runs In Agentry no longer write in it. It cannot be undone. The voice kept on this computer is not touched. Continue?"); err != nil {
		return err
	}
	return v.env().DeleteAccount(v.app.ctx, settings.Normalize(reg))
}

// voiceJob reads the panel's word for a Voice Writer job.
func voiceJob(job string) (build.Job, error) {
	switch job {
	case hostedrun.VoiceCreate:
		return build.Create, nil
	case hostedrun.VoiceImprove:
		return build.Improve, nil
	case hostedrun.VoiceEdit:
		return build.Edit, nil
	}
	return "", fmt.Errorf("%q is not create, improve or edit", job)
}

// checkVoiceJob holds a job's samples, addresses and brief to the rule both
// places share: Create and Improve take samples or web addresses of the
// person's writing, an AI Edit a note and either if the person adds some,
// and a brief or a note is at most 4 KiB. Every sample is one a native
// picker returned, and every address one the build would fetch.
func (v *VoiceAPI) checkVoiceJob(job build.Job, samples, urls []string, brief string) error {
	if p := voice.CheckAddresses(urls).First(); p != "" {
		return fmt.Errorf("the web addresses: %s", p)
	}
	switch {
	case job == build.Edit && strings.TrimSpace(brief) == "":
		return errors.New("say what should change in your voice")
	case job != build.Edit && len(samples) == 0 && len(knowledge.TrimmedLines(urls)) == 0:
		return errors.New("choose samples of your writing, or give web addresses of it, first")
	case len(strings.TrimSpace(brief)) > hostedrun.MaxVoiceBriefBytes:
		return errors.New("a brief or a note is at most 4 KiB")
	}
	for _, p := range samples {
		if err := v.app.requirePicked("writing sample", p); err != nil {
			return err
		}
	}
	return nil
}

// VoiceAddressesView is the step's check of the web addresses: each line,
// and on this computer whether AI Review can read the pages, which takes
// the person's own TypeSafe key (ReviewWhy says what is missing). Answered
// from presence alone: no key is read.
type VoiceAddressesView struct {
	Check       *voice.AddressCheck `json:"check"`
	ReviewReady bool                `json:"reviewReady"`
	ReviewWhy   string              `json:"reviewWhy,omitempty"`
}

// CheckAddresses answers VoiceAddressesView for the addresses, with the
// CLI's own parse.
func (v *VoiceAPI) CheckAddresses(urls []string) (*VoiceAddressesView, error) {
	st, err := draft.GetStatus()
	if err != nil {
		return nil, err
	}
	out := &VoiceAddressesView{Check: voice.CheckAddresses(urls), ReviewReady: st.Key.Ready()}
	if !out.ReviewReady {
		out.ReviewWhy = voice.NoReviewKey
	}
	return out, nil
}

// Write runs the Voice Writer on this computer (job create, improve or
// edit) and keeps what it wrote as this computer's voice. It takes the run
// lock; its log streams as voice:write-log.
func (v *VoiceAPI) Write(reg, job string, samples, urls []string, brief string) (*voice.BuildResult, error) {
	j, err := voiceJob(job)
	if err != nil {
		return nil, err
	}
	if err := v.checkVoiceJob(j, samples, urls, brief); err != nil {
		return nil, err
	}
	reg = settings.Normalize(reg)
	if j == build.Create {
		// A Create starts from nothing: over a voice kept here it is a
		// replacement, asked first as Use asks.
		here, err := v.env().Local(v.app.ctx, reg)
		if err != nil {
			return nil, err
		}
		if here.Exists {
			if err := v.app.askConsent("Replace this computer's voice?",
				"Create writes a new voice from your samples, in place of the Writing Voice kept on this computer, which cannot be recovered. Improve builds on the one you have instead. Continue?"); err != nil {
				return nil, err
			}
		}
	}
	var res *voice.BuildResult
	err = v.run.asRun(jobBuild, "a run of the Voice Writer", voiceRunRef, eventVoiceLog, func(ctx context.Context, runID string, logw io.Writer) error {
		var err error
		res, err = v.env().Build(ctx, voice.BuildRequest{
			Registry: reg, Job: j, Samples: samples, URLs: urls, Brief: brief, RunID: runID, Build: v.build, Log: logw,
		})
		return err
	})
	return res, err
}

// WriteWithAI is the Edit with AI dialog on the Local Voice page: the Voice
// Writer on this computer, for no credits. Where this computer keeps a voice
// it is an AI Edit, the text being what should change and the samples
// optional; where it keeps none it is a Create, the samples required and the
// text read as the brief.
func (v *VoiceAPI) WriteWithAI(reg, text string, samples, urls []string) (*voice.BuildResult, error) {
	reg = settings.Normalize(reg)
	here, err := v.env().Local(v.app.ctx, reg)
	if err != nil {
		return nil, err
	}
	return v.Write(reg, dialogJob(here), samples, urls, text)
}

// dialogJob is the Voice Writer's job behind Edit with AI: Edit over a
// voice kept here, even one that will not open (the Edit then says why
// rather than a Create writing over it), Create where there is none.
func dialogJob(here *voice.Local) string {
	if here != nil && here.Exists {
		return hostedrun.VoiceEdit
	}
	return hostedrun.VoiceCreate
}

// StopWrite stops the Voice Writer run on this computer, never a run or a
// build of another kind.
func (v *VoiceAPI) StopWrite() error {
	r := v.run
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.live == nil || r.live.ref != voiceRunRef {
		return errors.New("the Voice Writer is not running on this computer")
	}
	r.live.cancel()
	return nil
}

// VoiceHostedAvailabilityView answers whether the Voice Writer may run In
// Agentry for a job, and what it costs: the registry's flat price, the
// balance it is paid from, and Standard's limit.
type VoiceHostedAvailabilityView struct {
	Offered    bool   `json:"offered"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	Remedy     string `json:"remedy,omitempty"`
	UpgradeURL string `json:"upgradeUrl,omitempty"`
	// Credits is the flat price; HoldCredits what this payer's run holds
	// (0 for a member of an unmetered organization).
	Credits     int64 `json:"credits"`
	HoldCredits int64 `json:"holdCredits"`
	// Limited is Standard's runs a cycle (Max of them); Left is how many
	// are left.
	Limited bool `json:"limited"`
	Max     int  `json:"max"`
	Left    int  `json:"left"`
	// AIReviewCredits is what a voice saved by hand or submitted costs.
	AIReviewCredits int64  `json:"aiReviewCredits"`
	Balance         int64  `json:"balance"`
	BalanceKnown    bool   `json:"balanceKnown"`
	Pool            string `json:"pool,omitempty"`
	MaxInputFiles   int    `json:"maxInputFiles"`
	MaxInputBytes   int64  `json:"maxInputBytes"`
	// TakesURLs says the registry's Voice Writer reads web addresses, at
	// most MaxURLs; AI Review reads each page there through the registry.
	TakesURLs bool `json:"takesUrls"`
	MaxURLs   int  `json:"maxUrls,omitempty"`
}

// HostedAvailability answers the Where step for a job at reg.
func (v *VoiceAPI) HostedAvailability(reg, job string) (*VoiceHostedAvailabilityView, error) {
	reg = settings.Normalize(reg)
	// The token is found by host alone: the URL is held to Settings' rule
	// before it is looked up.
	if err := settings.ValidateURL(reg); err != nil {
		return nil, fmt.Errorf("the registry: %w", err)
	}
	ctx := v.app.ctx
	e := v.env()
	token := e.TokenFor(reg)
	out := &VoiceHostedAvailabilityView{MaxInputFiles: hostedrun.MaxInputFiles, MaxInputBytes: hostedrun.MaxInputBytes}
	f := hostedrun.VoiceFacts{SignedIn: token != ""}
	if !f.SignedIn {
		return out, nil
	}
	a, err := e.Account(ctx, reg)
	f.Voice = !errors.Is(err, remote.ErrUnconfigured)
	f.HasVoice = err == nil && a.Exists
	if exts, err := extensionsOf(ctx, reg); err == nil {
		f.Runs = hasString(exts, "runs")
	}
	if !f.Voice || !f.Runs {
		o := hostedrun.DecideVoice(f, job)
		out.Offered, out.Available, out.Reason, out.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
		return out, nil
	}
	f.Scopes, f.ScopesKnown = tokenScopes(ctx, reg, token)
	client := hostedrun.New(reg, token)
	if st, err := client.Status(ctx); err == nil {
		f.StatusKnown, f.Terms = true, st.VoiceBuild
		if t := st.VoiceBuild; t != nil {
			out.Credits, out.HoldCredits, out.AIReviewCredits = t.Credits, t.Credits, t.AIReviewCredits
			if u, ok := t.TakesURLs(); ok {
				out.TakesURLs, out.MaxURLs = true, u.Max
			}
		}
		if st.Caller != nil && st.Caller.Voice != nil {
			c := st.Caller.Voice
			f.Caller = c
			out.HoldCredits, out.Limited, out.Max, out.Left = c.HoldCredits, c.Limited, c.Max, max(c.Max-c.Used, 0)
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
		out.Balance, out.BalanceKnown, out.Pool = sub.Balance, true, sub.MemberOf
	}
	o := hostedrun.DecideVoice(f, job)
	out.Offered, out.Available, out.Reason, out.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
	if o.Upgrade {
		out.UpgradeURL = reg + "/settings/subscription?upgrade=pro"
	}
	return out, nil
}

// VoiceHostedView is a Voice Writer run In Agentry as the panel shows it:
// where it stands while it moves, then whether its voice became the
// account's version in review, and what it cost.
type VoiceHostedView struct {
	ID              string `json:"id"`
	Op              string `json:"op"`
	Status          string `json:"status"`
	State           string `json:"state"`
	StatusLine      string `json:"statusLine"`
	CancelRequested bool   `json:"cancelRequested"`
	OutcomeLine     string `json:"outcomeLine,omitempty"`
	OutcomeTone     string `json:"outcomeTone,omitempty"`
	Error           string `json:"error,omitempty"`
	HoldCredits     int64  `json:"holdCredits"`
	SettledCredits  int64  `json:"settledCredits"`
	Settled         bool   `json:"settled"`
	// Applied says its voice is the account's pending version now, which
	// AI Review reads before it is live; Note says why not, when it is not.
	Applied bool   `json:"applied"`
	Bytes   int    `json:"bytes,omitempty"`
	Note    string `json:"note,omitempty"`
	Final   bool   `json:"final"`
	// URLs are the web addresses it was given; PagesLine what its record
	// says of their page reads, "" for a run given none.
	URLs      []string `json:"urls,omitempty"`
	PagesLine string   `json:"pagesLine,omitempty"`
}

// VoiceHostedStartView is a started run, or the registry's refusal.
type VoiceHostedStartView struct {
	Run     *VoiceHostedView `json:"run,omitempty"`
	Refusal string           `json:"refusal,omitempty"`
	Detail  string           `json:"detail,omitempty"`
}

func voiceHostedView(r *hostedrun.Run, final bool) *VoiceHostedView {
	rv := hostedRunView(r, "", nil, "", "", final)
	out := &VoiceHostedView{
		ID: r.ID, Status: r.Status, State: rv.State, StatusLine: rv.StatusLine, CancelRequested: r.CancelRequested,
		OutcomeLine: rv.OutcomeLine, OutcomeTone: rv.OutcomeTone, Error: r.Error,
		HoldCredits: r.HoldCredits, SettledCredits: rv.SettledCredits, Settled: rv.Settled, Final: final,
	}
	if b := r.VoiceBuild; b != nil {
		out.Op, out.Applied, out.Bytes = b.Op, b.Applied, b.Bytes
		out.Note = hostedrun.VoiceNoteSentence(b.Note)
		out.URLs, out.PagesLine = b.URLs, voicePagesLine(b)
	}
	if final && out.Applied {
		out.OutcomeLine = "The Voice Writer wrote a new version of your voice. AI Review reads it before it is live; until it passes, your live voice stays as it was."
	}
	return out
}

// voicePagesLine is what a run given web addresses read of them: the page
// reads admitted against its terms, and the texts AI Review withheld.
func voicePagesLine(b *hostedrun.VoiceBuild) string {
	if len(b.URLs) == 0 || b.Pages == 0 {
		return ""
	}
	s := plural(b.Pages, "page read", "page reads")
	if b.MaxPages > 0 {
		s += fmt.Sprintf(" of the %d it may make", b.MaxPages)
	}
	if b.Withheld > 0 {
		s += "; AI Review withheld " + plural(b.Withheld, "text", "texts") + " from the Voice Writer"
	}
	return s + "."
}

// plural is n with the word that fits it.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// StartHosted runs the Voice Writer In Agentry: the samples (files and
// folders a native picker returned) are staged and uploaded as the run's
// inputs, the web addresses sent as its urls, then the run is created and
// followed in the background. AI Review reads each page through the
// registry before the run sees it.
func (v *VoiceAPI) StartHosted(reg, job string, samples, urls []string, brief string) (*VoiceHostedStartView, error) {
	reg = settings.Normalize(reg)
	ctx := v.app.ctx
	j, err := voiceJob(job)
	if err != nil {
		return nil, err
	}
	if err := v.checkVoiceJob(j, samples, urls, brief); err != nil {
		return nil, err
	}
	addrs := knowledge.TrimmedLines(urls)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, fmt.Errorf("the registry: %w", err)
	}
	token := v.env().TokenFor(reg)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to run the Voice Writer In Agentry", regHost(reg))
	}
	if err := v.hosted.claim("a Voice Writer run In Agentry is already in progress: wait for it to finish, or cancel it"); err != nil {
		return nil, err
	}
	claimed := true
	defer func() {
		if claimed {
			v.hosted.release()
		}
	}()
	client := hostedrun.New(reg, token)
	if len(addrs) > 0 {
		terms, err := client.Status(ctx)
		if err != nil {
			return nil, err
		}
		u, ok := terms.VoiceBuild.TakesURLs()
		switch {
		case !ok:
			return nil, errors.New("this registry's Voice Writer does not read web addresses In Agentry: run it on this computer, or give the samples as files")
		case len(addrs) > u.Max:
			return nil, fmt.Errorf("the Voice Writer In Agentry takes at most %d web addresses", u.Max)
		}
	}
	req := hostedrun.CreateRequest{Kind: hostedrun.KindVoiceBuild, Op: job, URLs: addrs}
	if j == build.Edit {
		req.Note = strings.TrimSpace(brief)
	} else {
		req.Brief = strings.TrimSpace(brief)
	}
	// An AI Edit's samples are optional; Create's and Improve's were checked
	// present above.
	if len(samples) > 0 {
		files, err := hostedrun.StageDocuments(samples)
		if err != nil {
			return nil, err
		}
		ids, err := client.UploadInputs(ctx, files)
		if err != nil {
			if r := hostedrun.RefusalOf(err); r != nil {
				return voiceRefusal(r), nil
			}
			return nil, err
		}
		req.Inputs = ids
	}
	created, err := client.Create(ctx, req)
	if err != nil {
		if r := hostedrun.RefusalOf(err); r != nil {
			return voiceRefusal(r), nil
		}
		return nil, err
	}
	claimed = false
	v.followHosted(reg, token, created)
	return &VoiceHostedStartView{Run: voiceHostedView(created, false)}, nil
}

func voiceRefusal(r *hostedrun.Refusal) *VoiceHostedStartView {
	out := &VoiceHostedStartView{Refusal: r.Slug, Detail: r.Detail}
	if out.Detail == "" {
		out.Detail = r.Sentence()
	}
	return out
}

// HostedWithAI is Edit with AI on the Cloud Voice page: the Voice Writer In
// Agentry at the registry's flat price. Where the account keeps a voice it
// is an AI Edit of its newest version, the text being what should change and
// the samples optional; where it keeps none it is a Create, the samples
// required and the text read as the brief (accountJob).
func (v *VoiceAPI) HostedWithAI(reg, text string, samples, urls []string) (*VoiceHostedStartView, error) {
	reg = settings.Normalize(reg)
	a, err := v.env().Account(v.app.ctx, reg)
	if err != nil {
		return nil, err
	}
	return v.StartHosted(reg, accountJob(a), samples, urls, text)
}

// accountJob is the Voice Writer's job behind Edit with AI In Agentry:
// Edit over a live or a pending version (a first version in review or
// flagged is the voice to edit, as on the site), Create where there is
// neither.
func accountJob(a *voice.Account) string {
	if a != nil && (a.Live != nil || a.Pending != nil) {
		return hostedrun.VoiceEdit
	}
	return hostedrun.VoiceCreate
}

// followHosted watches the run in the background and reports every status
// change as voice:hosted. Its voice lands in the account, not here: the
// panel reads the account again when the run is over.
func (v *VoiceAPI) followHosted(reg, token string, started *hostedrun.Run) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &hostedFollow{registry: reg, id: started.ID, cancel: cancel, done: make(chan struct{})}
	if !v.hosted.follow(f) {
		cancel()
		return
	}
	client := hostedrun.New(reg, token)
	go func() {
		defer close(f.done)
		defer cancel()
		defer v.hosted.done(f)
		final, err := client.Watch(ctx, started.ID, hostedrun.WatchOptions{
			Poll:     hostedPoll,
			OnStatus: func(r *hostedrun.Run) { emitEvent(v.app, eventVoiceHosted, voiceHostedView(r, false)) },
		})
		if err != nil {
			last, gerr := client.Get(context.Background(), started.ID)
			if gerr != nil || !last.Terminal() {
				out := voiceHostedView(started, true)
				if gerr == nil {
					out = voiceHostedView(last, true)
				}
				out.Error = "lost touch with the registry while the Voice Writer ran (" + err.Error() + "); open the Voice page again to see your account's voice"
				emitEvent(v.app, eventVoiceHosted, out)
				return
			}
			final = last
		}
		emitEvent(v.app, eventVoiceHosted, voiceHostedView(final, true))
	}()
}

// AttachHosted follows this account's Voice Writer run In Agentry in flight,
// if it has one, so a Cloud Voice page opened again (left and reopened, or
// after a restart) shows it and its Cancel rather than offering a second.
// The URL is held to Settings' rule before its token is looked up.
func (v *VoiceAPI) AttachHosted(reg string) (*VoiceHostedStartView, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, fmt.Errorf("the registry: %w", err)
	}
	token := v.env().TokenFor(reg)
	if token == "" {
		return &VoiceHostedStartView{}, nil
	}
	client := hostedrun.New(reg, token)
	if cur := v.hosted.current(); cur != nil {
		if cur.id == "" {
			// A start holds the slot; its own events report the run.
			return &VoiceHostedStartView{}, nil
		}
		if fresh, err := client.Get(v.app.ctx, cur.id); err == nil {
			return &VoiceHostedStartView{Run: voiceHostedView(fresh, false)}, nil
		}
		return &VoiceHostedStartView{}, nil
	}
	if err := v.hosted.claim("busy"); err != nil {
		return &VoiceHostedStartView{}, nil //nolint:nilerr // a start took the slot first
	}
	runs, err := client.List(v.app.ctx)
	if err != nil {
		v.hosted.release()
		return &VoiceHostedStartView{}, nil //nolint:nilerr // no list, nothing to follow
	}
	for i := range runs {
		r := &runs[i]
		if r.IsVoiceBuild() && hostedrun.Active(r.Status) {
			v.followHosted(reg, token, r)
			return &VoiceHostedStartView{Run: voiceHostedView(r, false)}, nil
		}
	}
	v.hosted.release()
	return &VoiceHostedStartView{}, nil
}

// CancelHosted asks the registry to cancel the Voice Writer run In Agentry
// this app follows (the registry stops it, the app never drives it). What it
// held is settled as the registry settles a cancelled run.
func (v *VoiceAPI) CancelHosted() (*VoiceHostedView, error) {
	cur := v.hosted.current()
	if cur == nil || cur.id == "" {
		return nil, errors.New("no Voice Writer run In Agentry is in progress")
	}
	token := v.env().TokenFor(cur.registry)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to cancel this run", regHost(cur.registry))
	}
	r, err := hostedrun.New(cur.registry, token).Cancel(v.app.ctx, cur.id)
	if err != nil {
		return nil, err
	}
	return voiceHostedView(r, false), nil
}
