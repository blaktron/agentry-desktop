package main

import (
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

// The confirmation's words. The first two sentences are the plan's (§4.2);
// the rest says what DR-53 added (PyPI for the tools Kali does not package,
// the image's PATH in an image that keeps everything it installed) and what
// stays, without saying more than is enforced: run_command's allowlist is
// the declared names, and a declared interpreter can start anything.
const (
	unsafeConfirmTitle   = "Turn on Unsafe Mode?"
	unsafeConfirmMessage = "QuickAgents can run commands this computer has not approved. Each command is installed from Kali Linux's repository when the QuickAgent first runs. Tools Kali does not package come from PyPI.\n\n" +
		"A declared command also runs with that image's own PATH, in an image that keeps everything it installed, so it can start any program the image holds. run_command still takes only the commands a QuickAgent declares, and every other limit on a run stays: the container's isolation, its network limits, and each consent.\n\n" +
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
		if err := settings.SetUnsafeMode(false); err != nil {
			return nil, err
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
	err := pull(r.app.ctx, logw)
	logw.Flush()
	if err != nil {
		return nil, err
	}
	if err := settings.SetUnsafeMode(true); err != nil {
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
	n := r.unsafeNoticeFor(reg, ref)
	return &UnsafeNoticeView{On: true, Line: n.Line(), Unplaced: emptyNotNil(n.Unplaced)}, nil
}

// unsafeNoticeFor is the notice for a run of ref. A release that cannot be
// read here (the run would refuse on the same error a moment later) still
// gets one that names Unsafe Mode, with nothing listed.
func (r *RunAPI) unsafeNoticeFor(reg, ref string) *run.UnsafeNotice {
	read := r.unsafeNotice
	if read == nil {
		read = run.UnsafeNoticeFor
	}
	n, err := read(r.app.ctx, reg, ref)
	if err != nil {
		return run.UnsafeNoticeUnread()
	}
	return n
}
