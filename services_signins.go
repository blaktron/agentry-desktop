package main

// Model setup's sign-ins and connects (plan model-sign-ins.md §4.1;
// agentry-desktop#734, M9): the page's side of internal/run/signins.go. A
// sign-in blocks until the vendor answers, the person cancels, or it times
// out, telling the page as it goes; one sign-in runs at a time in this app,
// an MCP connection's consent included (consentNow), so two browser tabs
// never compete. Nothing here carries a token, a key or an authorization
// code to the page: GitHub's device code, the one code Sign in with GitHub
// asks the person to enter on GitHub's own page, is the exception, shown
// and put on the clipboard for them (operator, 2026-10-11).

import (
	"context"
	"errors"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// Events a model sign-in emits while it waits on the person.
const (
	// signInSayEvent is a line the sign-in says (SignInSayView).
	signInSayEvent = "signin:say"
	// signInURLEvent is the vendor's page the browser was sent to, for
	// the page to offer again should the browser not have opened.
	signInURLEvent = "signin:url"
	// signInCodeEvent is GitHub's device code and the page to enter it on.
	signInCodeEvent = "signin:code"
)

// SignInSayView is one line a sign-in says.
type SignInSayView struct {
	Vendor string `json:"vendor"`
	Line   string `json:"line"`
}

// SignInURLView is the page a sign-in opened.
type SignInURLView struct {
	Vendor string `json:"vendor"`
	URL    string `json:"url"`
}

// SignInCodeView is GitHub's device code, to enter at Page.
type SignInCodeView struct {
	Vendor string `json:"vendor"`
	Page   string `json:"page"`
	Code   string `json:"code"`
}

// SignInView is how a sign-in or connect ended: what was kept, or a
// cancel.
type SignInView struct {
	SignedIn  *run.SignedIn `json:"signedIn,omitempty"`
	Cancelled bool          `json:"cancelled,omitempty"`
}

// signInUI is the run package's view of the page for vendor.
func (r *RunAPI) signInUI(vendor string) run.SignInUI {
	return run.SignInUI{
		Say: func(line string) { emitEvent(r.app, signInSayEvent, SignInSayView{Vendor: vendor, Line: line}) },
		Open: func(u string) {
			emitEvent(r.app, signInURLEvent, SignInURLView{Vendor: vendor, URL: u})
			if err := r.openURL(u); err != nil {
				emitEvent(r.app, signInSayEvent, SignInSayView{Vendor: vendor,
					Line: "The browser could not be opened (" + err.Error() + "): open the page below yourself."})
			}
		},
		Code: func(page, code string) {
			emitEvent(r.app, signInCodeEvent, SignInCodeView{Vendor: vendor, Page: page, Code: code})
		},
	}
}

// signIn runs one sign-in under the app's one-consent lock, cancellable by
// CancelSignIn (or CancelConnect, the same lock).
func (r *RunAPI) signIn(f func(ctx context.Context) (*run.SignedIn, error)) (*SignInView, error) {
	if !consentNow.mu.TryLock() {
		return nil, errors.New("a sign-in is already under way: finish it in the browser, or cancel it")
	}
	defer consentNow.mu.Unlock()
	ctx, cancel := context.WithCancel(r.ctx())
	defer cancel()
	setConsent(cancel, nil)
	defer setConsent(nil, nil)
	got, err := f(ctx)
	switch {
	case err != nil && errors.Is(ctx.Err(), context.Canceled) && r.ctx().Err() == nil:
		return &SignInView{Cancelled: true}, nil
	case err != nil:
		return nil, err
	}
	return &SignInView{SignedIn: got}, nil
}

// ModelSignIns lists the model sign-ins and connected accounts this
// computer keeps, the CLI's included: never a token or a key.
func (r *RunAPI) ModelSignIns() ([]run.ModelSignIn, error) {
	return run.ModelSignIns()
}

// SignInChatGPT is Sign in with ChatGPT; newAccount signs in another
// account than this computer's.
func (r *RunAPI) SignInChatGPT(newAccount bool) (*SignInView, error) {
	return r.signIn(func(ctx context.Context) (*run.SignedIn, error) {
		return run.SignInChatGPT(ctx, r.signInUI("chatgpt"), newAccount)
	})
}

// SignInClaude is Use your Claude subscription (runs Claude Code): Claude
// Code signs the person in itself, in their browser. The Claude Code image
// is built on this computer first when it is not here, which takes the
// app's one docker job.
func (r *RunAPI) SignInClaude() (*SignInView, error) {
	if err := r.acquire(jobSetup, "the Claude sign-in"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	return r.signIn(func(ctx context.Context) (*run.SignedIn, error) {
		return run.SignInClaude(ctx, r.signInUI("claude"), logw)
	})
}

// SignInCopilot is Sign in with GitHub, for a Copilot plan; oauthApp goes
// through Agentry's OAuth App rather than its GitHub App (the fallback).
func (r *RunAPI) SignInCopilot(oauthApp bool) (*SignInView, error) {
	return r.signIn(func(ctx context.Context) (*run.SignedIn, error) {
		return run.SignInCopilot(ctx, r.signInUI("copilot"), oauthApp)
	})
}

// ConnectAccount is Connect your account for openrouter, poe or
// huggingface.
func (r *RunAPI) ConnectAccount(vendor string) (*SignInView, error) {
	return r.signIn(func(ctx context.Context) (*run.SignedIn, error) {
		return run.ConnectAccount(ctx, r.signInUI(vendor), vendor)
	})
}

// CancelSignIn ends the sign-in in progress, if there is one, and says
// whether there was.
func (r *RunAPI) CancelSignIn() bool { return r.CancelConnect() }

// ChatGPTModels lists the models the kept ChatGPT sign-in's plan offers.
func (r *RunAPI) ChatGPTModels() ([]string, error) {
	return run.ChatGPTModels(r.ctx())
}

// CopilotModels lists the models the kept Copilot sign-in's plan offers,
// asked of Copilot CLI (agentry-cli#808). It may build the runner and
// Copilot CLI images first, so it takes the docker job and reports their
// progress on the setup log.
func (r *RunAPI) CopilotModels(plan string) ([]run.CopilotModel, error) {
	if err := r.acquire(jobSetup, "the listing of your Copilot plan's models"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	return run.CopilotModels(r.ctx(), plan, logw)
}

// RemoveModelSignIn removes one kept sign-in or connected account and
// says what became of it at the vendor. A Claude sign-in's removal runs
// Claude Code's own logout in its image, which takes the docker job.
func (r *RunAPI) RemoveModelSignIn(vendor string) (string, error) {
	if err := r.acquire(jobSetup, "the removal of a model sign-in"); err != nil {
		return "", err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	return run.RemoveModelSignIn(r.ctx(), vendor, logw)
}

// ApplySignInSetup makes a kept model sign-in or connected account model
// setup's active choice: kind is chatgpt-plan, claude-signin or
// copilot-signin, or connect is true for a connected account; copilotPlan
// names a Copilot plan (individual, business, enterprise). pullImages
// prepares that choice's images now. A choice nothing is signed in to is
// refused.
func (r *RunAPI) ApplySignInSetup(provider, model, kind string, connect bool, copilotPlan string, pullImages bool) (*SetupStatusView, error) {
	if err := r.acquire(jobSetup, "the model and image setup"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.ApplySetup(r.ctx(), run.SetupOpts{
		Provider: provider, Model: model, APIKeyKind: kind, Connect: connect,
		CopilotPlan: copilotPlan, PullImages: pullImages,
	}, logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SetupStatus()
}
