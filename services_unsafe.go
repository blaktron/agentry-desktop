package main

import (
	"context"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Run defaults → Unsafe Mode (agentry-notes plans/unsafe-mode.md M3;
// agentry-desktop#418, D-050). While the switch is on, a run on this
// computer no longer needs this machine's exec catalogue to name each
// command the QuickAgent declares: the commands are installed from the
// Unsafe Mode image (Kali Linux) instead, and each runs with that image's
// PATH (the CLI's DR-53). Nothing else about a run changes, and a run In
// Agentry never carries it (D7).
//
// The renderer is not a principal (D-030), so the page cannot turn it on:
// SetUnsafeMode(true) is a native confirmation the page cannot answer,
// then the image's pull, and only then is unsafeMode written to
// desktop.yaml. No other binding writes it, and no run binding takes it as
// an argument: RunAgent and StartSession read it from that file.

// eventUnsafeProgress carries the image pull's lines to the card.
const eventUnsafeProgress = "runner:unsafe-progress"

// The confirmation's words. The first two sentences are the plan's (§4.2);
// the PATH sentence is the operator's decision of 2026-09-30 (DR-53).
const (
	unsafeConfirmTitle   = "Turn on Unsafe Mode?"
	unsafeConfirmMessage = "QuickAgents can run commands this computer has not approved. Each command is installed from Kali Linux's repository when the QuickAgent first runs.\n\n" +
		"A declared command also runs with that image's own PATH, so it can reach every program the image holds. Only the commands a QuickAgent declares can be called, and every other limit on a run stays: the container, its network, and each consent.\n\n" +
		"This applies to runs on this computer only, never to a run In Agentry. Agentry downloads the Unsafe Mode image now, and a red edge round the window shows it is on.\n\nTurn it on?"
)

// unsafeModeOn reads the switch. A settings file that cannot be read is
// off: nothing turns Unsafe Mode on by failing.
func unsafeModeOn() bool {
	st, err := settings.Load()
	return err == nil && st.UnsafeMode
}

// SetUnsafeMode turns Unsafe Mode on or off and returns Run defaults as
// they now stand.
//
// Off is immediate and removes nothing: the images stay until the person
// removes them (D12; Run defaults → Docker images).
//
// On asks in a native dialog first, then pulls the Unsafe Mode image under
// the run lock with its progress streamed as runner:unsafe-progress, and
// writes the setting only once the image is here and its command index
// reads. A No, a failed pull or a busy docker leaves the switch off.
func (r *RunAPI) SetUnsafeMode(on bool) (*RunDefaultsView, error) {
	if !on {
		st, err := settings.Load()
		if err != nil {
			return nil, err
		}
		if st.UnsafeMode {
			st.UnsafeMode = false
			if err := settings.Save(st); err != nil {
				return nil, err
			}
		}
		return runDefaultsView()
	}
	if unsafeModeOn() {
		return runDefaultsView()
	}
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	if err := r.app.askConsent(unsafeConfirmTitle, unsafeConfirmMessage); err != nil {
		return nil, err
	}
	logw := r.eventWriter(eventUnsafeProgress)
	pull := r.pullUnsafe
	if pull == nil {
		pull = run.PullUnsafeBase
	}
	_, err := pull(r.app.ctx, logw)
	logw.Flush()
	if err != nil {
		return nil, err
	}
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.UnsafeMode = true
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// UnsafeNoticeView is what Unsafe Mode means for one QuickAgent's run on
// this computer, for the run setup's Review step.
type UnsafeNoticeView struct {
	// On is the switch; false carries nothing else.
	On bool `json:"on"`
	// Line is the sentence the run's native consent dialog carries too.
	Line string `json:"line,omitempty"`
	// Unplaced are the declared commands the image cannot provide.
	Unplaced []string `json:"unplaced"`
}

// UnsafeNotice says whether Unsafe Mode is on and, when it is, the line a
// run of ref on this computer carries: the declared commands, and the ones
// the Unsafe Mode image cannot provide (D8). It resolves the release as a
// run would and pulls nothing. The page shows it; it decides nothing.
func (r *RunAPI) UnsafeNotice(reg, ref string) (*UnsafeNoticeView, error) {
	if !unsafeModeOn() {
		return &UnsafeNoticeView{Unplaced: []string{}}, nil
	}
	n, err := r.unsafeNoticeFor(r.app.ctx, reg, ref)
	if err != nil {
		return nil, err
	}
	return &UnsafeNoticeView{On: true, Line: n.Line(), Unplaced: emptyNotNil(n.Unplaced)}, nil
}

func (r *RunAPI) unsafeNoticeFor(ctx context.Context, reg, ref string) (*run.UnsafeNotice, error) {
	if r.unsafeNotice != nil {
		return r.unsafeNotice(ctx, reg, ref)
	}
	return run.UnsafeNoticeFor(ctx, reg, ref)
}

// unsafeConsentLine is the line a local run's native consent dialog
// carries while Unsafe Mode is on (D6), "" while it is off. Where the
// release cannot be read here (the run would refuse on the same error a
// moment later) the line still names Unsafe Mode, without the commands.
func (r *RunAPI) unsafeConsentLine(reg, ref string) string {
	if !unsafeModeOn() {
		return ""
	}
	if reg != "" {
		if n, err := r.unsafeNoticeFor(r.app.ctx, reg, ref); err == nil {
			return n.Line()
		}
	}
	return "Unsafe Mode is on: the commands this QuickAgent declares are installed from Kali Linux's repository or PyPI without asking this computer's approved list, and each runs with the image's own PATH."
}

// unsafeLineOf is the receipt's exec line when the run was an Unsafe Mode
// run, "" otherwise: the result view shows it under Run details.
func unsafeLineOf(execLine string) string {
	if strings.HasPrefix(execLine, "UNSAFE MODE:") {
		return execLine
	}
	return ""
}
