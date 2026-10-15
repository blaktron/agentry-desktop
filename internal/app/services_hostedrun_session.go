// Talk to it In Agentry: a hosted session of a release's interactive mode,
// started with the app's token and followed over the registry's session
// route.
//
// SessionChat is one component with two transports. On this machine the
// engine's session feeds it runner:session-event (services_run.go); In
// Agentry the registry's stream feeds it runner:hosted-session-event, in the
// same shape, and runner:hosted-session-state with the run's content-free
// state and its session record for the status strip. The app keeps no frame:
// each is emitted as it arrives, and a chat that opens on a session already
// under way asks the stream to start again from the first frame the host
// still holds (ReplayHostedSession), so reopening the run replays the
// conversation from the host's memory, never from ours. Only the account's
// token travels (internal/hostedrun), and a file added to a session is one a
// native picker returned.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The events a hosted session sends the chat.
const (
	// eventHostedSessionEvent carries one display event, as a
	// SessionEventView with its sequence number.
	eventHostedSessionEvent = "runner:hosted-session-event"
	// eventHostedSessionState carries the run's state and session record.
	eventHostedSessionState = "runner:hosted-session-state"
)

// SessionTransportSentence is the copy shown once beside a hosted session's
// input and nowhere else. It is true of the transport between the sandbox
// host and the registry's opening of each frame, and claims nothing more:
// Agentry holds each person's key.
const SessionTransportSentence = "Transport encrypted by the user key, messages not stored; the transcript comes back only in your sealed output."

// HostedSessionOfferView is Talk to it In Agentry for one release: whether
// it is offered and can be chosen, why not, and the registry's terms for a
// session — the app does no credit arithmetic of its own.
type HostedSessionOfferView struct {
	Offered   bool   `json:"offered"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Remedy    string `json:"remedy,omitempty"`
	// Upgrade says the way out is a higher tier: the panel offers the
	// registry's subscription page (SettingsAPI.OpenUpgrade).
	Upgrade bool `json:"upgrade,omitempty"`

	FreeMinutes      int   `json:"freeMinutes"`
	CreditsPerMinute int64 `json:"creditsPerMinute"`
	MinCredits       int64 `json:"minCredits"`
	IdleSeconds      int   `json:"idleSeconds"`
	// Transport is that sentence, for the chat to show.
	Transport string `json:"transport"`
}

// sessionOffer is the view of DecideSession's answer, with the registry's
// terms where it said them. Not offered is nil: the panel draws nothing.
func sessionOffer(o hostedrun.Offer, terms *hostedrun.SessionTerms) *HostedSessionOfferView {
	if !o.Offered {
		return nil
	}
	v := &HostedSessionOfferView{
		Offered: o.Offered, Available: o.Available, Reason: o.Reason, Remedy: o.Remedy, Upgrade: o.Upgrade,
		Transport: SessionTransportSentence,
	}
	if terms != nil {
		v.FreeMinutes, v.CreditsPerMinute, v.MinCredits, v.IdleSeconds = terms.FreeMinutes, terms.CreditsPerMinute, terms.MinCredits, terms.IdleSeconds
	}
	return v
}

// HostedSessionView is a session's content-free record as the panel shows
// it: what its time has cost, its model use once settled (in credits, as
// the registry names it), and the idle close.
type HostedSessionView struct {
	FreeMinutes      int   `json:"freeMinutes"`
	CreditsPerMinute int64 `json:"creditsPerMinute"`
	PaidThrough      int   `json:"paidThrough"`
	TimeCredits      int64 `json:"timeCredits"`
	TokenCredits     int64 `json:"tokenCredits"`
	TokensUnpaid     int64 `json:"tokensUnpaid"`
	OutOfCredits     bool  `json:"outOfCredits"`
	FilesAdded       int   `json:"filesAdded"`
	IdleSeconds      int   `json:"idleSeconds"`
}

func sessionView(s *hostedrun.Session) *HostedSessionView {
	if s == nil {
		return nil
	}
	return &HostedSessionView{
		FreeMinutes: s.FreeMinutes, CreditsPerMinute: s.CreditsPerMinute, PaidThrough: s.PaidThrough,
		TimeCredits: s.TimeCredits, TokenCredits: s.TokenCredits, TokensUnpaid: s.TokensUnpaid,
		OutOfCredits: s.OutOfCredits, FilesAdded: s.FilesAdded, IdleSeconds: s.IdleSeconds,
	}
}

// HostedSessionStateView is one runner:hosted-session-state payload.
type HostedSessionStateView struct {
	Status     string             `json:"status"`
	StartedAt  string             `json:"startedAt,omitempty"`
	FinishedAt string             `json:"finishedAt,omitempty"`
	Session    *HostedSessionView `json:"session,omitempty"`
}

// StartHostedSession starts a hosted session of the release's interactive
// mode: the same gates, uploads and create as StartHostedRun, with the
// interactive mode. The chat then follows it (ReplayHostedSession) and
// talks to it (SendHostedTurn, AddHostedSessionFile, EndHostedSession);
// when it ends its sealed output comes down into the folder picked, with
// transcript.md among the receipts.
func (r *RunAPI) StartHostedSession(reg, ref, task string, inputPaths []string, outDir string, intakeAnswers map[string]string, packs []string, writeInVoice, askOptional bool, sourceTokens map[string]string) (*HostedStartView, error) {
	return r.startHosted(reg, ref, task, inputPaths, outDir, intakeAnswers, packs, hostedrun.ModeInteractive, writeInVoice, askOptional, sourceTokens)
}

// follow follows a run the registry has created or this account already had:
// a session's conversation and state, or a run's state alone. It returns the
// folder the run downloads into: when another call (App's attach at start, a
// run setup's, a start) got there first, the run is already followed and its
// folder is that follow's, so no second loop downloads it again. When what
// got there first is another run, this app follows one at a time and does
// not follow this one: it says so, naming both.
func (r *RunAPI) follow(reg, token, outDir string, run *hostedrun.Run) (string, error) {
	var cur *hostedFollow
	if run.IsSession() {
		outDir, cur = r.followSession(reg, token, outDir, run)
	} else {
		outDir, cur = r.followHosted(reg, token, outDir, run)
	}
	if cur != nil && (cur.id != run.ID || cur.registry != reg) {
		return "", fmt.Errorf("this app is already following run %s and follows one run In Agentry at a time, so it is not following run %s, which Agentry has under way; Agentry's Runs page shows it", cur.id, run.ID)
	}
	return outDir, nil
}

// claimFollow makes f the follow, unless one is already under way, which
// it returns instead. A run this app lost touch with is followed again
// once, so its remembered folder goes with the claim.
func (r *RunAPI) claimFollow(f *hostedFollow) *hostedFollow {
	r.hostedMu.Lock()
	defer r.hostedMu.Unlock()
	if r.hosted != nil {
		return r.hosted
	}
	r.hosted = f
	delete(r.lostDirs, lostKey{f.registry, f.id})
	return nil
}

// followSession streams a session until it ends, then delivers its output
// as a run's is. A stream the registry cannot show (the runner's key does
// not open, a frame out of order) or cannot keep up says so in the chat
// and the run is followed by its state alone: the session goes on, and its
// transcript comes back in its output.
func (r *RunAPI) followSession(reg, token, outDir string, started *hostedrun.Run) (string, *hostedFollow) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &hostedFollow{
		registry: reg, id: started.ID, ref: started.Ref, outDir: outDir, cancel: cancel, done: make(chan struct{}),
		session: true, replay: make(chan struct{}, 1),
	}
	if started.Session != nil {
		f.idle = started.Session.IdleSeconds
	}
	if cur := r.claimFollow(f); cur != nil {
		cancel()
		return cur.outDir, cur
	}

	client := hostedrun.New(reg, token)
	// Read here, not in the goroutine: a test shortens them for its life.
	poll, retry := hostedPoll, sessionRetry
	go func() {
		defer close(f.done)
		defer cancel()
		defer func() {
			r.hostedMu.Lock()
			if r.hosted == f {
				r.hosted = nil
			}
			r.hostedMu.Unlock()
		}()
		if err := r.streamSession(ctx, client, f, retry); err != nil && ctx.Err() == nil {
			r.hostedMu.Lock()
			f.lost = streamLost(err)
			r.hostedMu.Unlock()
			emitLost(r.app, f.lost)
		}
		if ctx.Err() != nil {
			return
		}
		// The stream said done, or could not go on: the run's own state
		// says when it is over.
		r.watchAndDeliver(ctx, client, reg, started, outDir, poll)
	}()
	return outDir, nil
}

// streamSession runs the session's stream, starting it again from the
// host's first frame whenever the chat asks (a chat that has just opened),
// and reports each state change as the run's view too, so the panel's
// badge moves with it.
func (r *RunAPI) streamSession(ctx context.Context, client *hostedrun.Client, f *hostedFollow, retry time.Duration) error {
	lastStatus := ""
	handlers := hostedrun.SessionHandlers{
		State: func(st hostedrun.SessionState) {
			v := HostedSessionStateView{Status: st.Status, Session: sessionView(st.Session)}
			if st.StartedAt != nil {
				v.StartedAt = rfc3339(*st.StartedAt)
			}
			if st.FinishedAt != nil {
				v.FinishedAt = rfc3339(*st.FinishedAt)
			}
			emitEvent(r.app, eventHostedSessionState, v)
			if st.Status != lastStatus {
				lastStatus = st.Status
				if run, err := client.Get(ctx, f.id); err == nil && !run.Terminal() {
					emitEvent(r.app, eventHostedRun, hostedRunView(run, f.outDir, nil, "", "", false))
				}
			}
		},
		Frame: func(seq uint64, ev hostedrun.FrameEvent) {
			emitEvent(r.app, eventHostedSessionEvent, frameView(seq, ev))
		},
		Dropped: func(before uint64) {
			// Seq is the first frame after the gap, which the chat compares
			// with the frames it has, so a replay says it once.
			emitEvent(r.app, eventHostedSessionEvent, SessionEventView{Kind: "dropped", Seq: before, Time: time.Now().UTC().Format(time.RFC3339Nano)})
		},
	}
	for {
		sctx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- client.FollowSession(sctx, f.id, hostedrun.FollowSessionOptions{Handlers: handlers, Retry: retry})
		}()
		select {
		case err := <-done:
			stop()
			return err
		case <-f.replay:
			// Every frame again, from the first the host holds: the chat
			// that asked keeps each sequence number once.
			stop()
			<-done
			lastStatus = ""
		}
	}
}

// sessionRetry is the pause before a failed stream is asked for again; a
// test shortens it.
var sessionRetry = 5 * time.Second

// emitLost tells the chat its conversation stopped here.
func emitLost(app *App, why string) {
	emitEvent(app, eventHostedSessionEvent, SessionEventView{Kind: "stream_lost", Time: time.Now().UTC().Format(time.RFC3339Nano), Text: why})
}

// streamLost says, in the chat, why its conversation stopped here.
func streamLost(err error) string {
	var se *hostedrun.SessionStreamError
	if errors.As(err, &se) {
		return "The conversation cannot be shown here. The session goes on, and its transcript comes back in your sealed output."
	}
	return "This app lost the session's stream (" + err.Error() + "). The session goes on until you finish or stop it, or it is idle; its transcript comes back in your sealed output."
}

// frameView is a frame in the shape the engine's session events take, so the
// chat reads both transports alike: a file added is named where the agent
// finds it, and an error by the fixed words of its class, never by text the
// harness wrote. A frame's token counts are not carried: a session In
// Agentry is stated in credits.
func frameView(seq uint64, ev hostedrun.FrameEvent) SessionEventView {
	v := SessionEventView{
		Seq: seq, Kind: ev.Kind, Text: ev.Text, Synthetic: ev.Synthetic, StopReason: ev.StopReason,
		Reason: ev.Reason,
	}
	if !ev.Time.IsZero() {
		v.Time = ev.Time.UTC().Format(time.RFC3339Nano)
	}
	switch ev.Kind {
	case "input_added":
		v.Path, v.Text = "/inputs/"+ev.Text, ""
	case "error":
		v.Text = hostedrun.ErrorWords(ev.Class)
	}
	return v
}

// ReplayHostedSession asks the session this app follows for every frame
// the host still holds, for a chat that has just opened on it.
func (r *RunAPI) ReplayHostedSession() error {
	cur := r.hostedCurrent()
	if cur == nil || !cur.session {
		return errNoHostedSession
	}
	r.hostedMu.Lock()
	lost := cur.lost
	r.hostedMu.Unlock()
	if lost != "" {
		// The stream is gone and the run is followed by its state alone:
		// there is nothing to replay, and the chat is told why.
		emitLost(r.app, lost)
		return nil
	}
	select {
	case cur.replay <- struct{}{}:
	default: // one is already asked for
	}
	return nil
}

var errNoHostedSession = errors.New("no session In Agentry is being followed")

// hostedSession is the session this app follows and a client for it with
// the account's token.
func (r *RunAPI) hostedSession() (*hostedFollow, *hostedrun.Client, error) {
	cur := r.hostedCurrent()
	if cur == nil || !cur.session {
		return nil, nil, errNoHostedSession
	}
	token := settings.TokenFor(cur.registry)
	if token == "" {
		return nil, nil, fmt.Errorf("sign in to %s to talk to this session", cur.registry)
	}
	return cur, hostedrun.New(cur.registry, token), nil
}

// SendHostedTurn sends the agent one message. It returns once the host has
// it; the reply streams in as the session's events.
func (r *RunAPI) SendHostedTurn(text string) error {
	cur, client, err := r.hostedSession()
	if err != nil {
		return err
	}
	return sentence(client.SessionTurn(r.ctx(), cur.id, text))
}

// AddHostedSessionFile gives the agent one more file, read-only under
// /inputs: a file a native picker returned, and whose bytes are that file
// once links are resolved (it leaves this machine), uploaded as one of
// the account's inputs and handed to the session under the run's caps.
func (r *RunAPI) AddHostedSessionFile(path string) error {
	cur, client, err := r.hostedSession()
	if err != nil {
		return err
	}
	if err := r.app.requirePicked("file", path); err != nil {
		return err
	}
	files, err := hostedrun.StageInputs([]string{path})
	if err != nil {
		return err
	}
	ids, err := client.UploadInputs(r.ctx(), files)
	if err != nil {
		return sentence(err)
	}
	return sentence(client.SessionFile(r.ctx(), cur.id, ids[0]))
}

// EndHostedSession ends the session: finish asks the agent for its final
// result first, stop ends it now with none. A session its host has not
// started yet cannot take either, so stopping it cancels the run instead.
// The output comes back as the session's follow delivers it.
func (r *RunAPI) EndHostedSession(finish bool) error {
	cur, client, err := r.hostedSession()
	if err != nil {
		return err
	}
	err = client.SessionEnd(r.ctx(), cur.id, finish)
	if refusal := hostedrun.RefusalOf(err); refusal != nil && refusal.Slug == "session-starting" && !finish {
		_, err = client.Cancel(r.ctx(), cur.id)
	}
	return sentence(err)
}

// sentence turns the registry's refusal into its own sentence, which the
// chat shows as it is; any other error is left as it came.
func sentence(err error) error {
	if refusal := hostedrun.RefusalOf(err); refusal != nil {
		return errors.New(refusal.Sentence())
	}
	return err
}

// hostedSessionQuitNote is what quitting says about a session In Agentry the
// app is following: it goes on, and closes when idle.
func hostedSessionQuitNote(ref string, idle int) string {
	if idle <= 0 {
		idle = 300
	}
	return "A session with " + ref + " is going on In Agentry. Quitting does not end it: it goes on until it has been idle for " +
		idleWords(idle) + ", and reopening its run in this app before then picks the conversation up. Quit anyway?"
}

func idleWords(seconds int) string {
	if seconds%60 == 0 {
		if seconds == 60 {
			return "a minute"
		}
		return fmt.Sprintf("%d minutes", seconds/60)
	}
	return fmt.Sprintf("%d seconds", seconds)
}
