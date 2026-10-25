package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/blaktron/agentry-cli/pkg/persistent/assistant"
	"github.com/blaktron/agentry-cli/pkg/persistent/offer"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// Fastmail's set-up in the app: the person's own Fastmail API token, typed
// in a password field and handed to Go, which asks Fastmail whether it
// accepts it (persistent.FastmailTokenAccepted) before anything is kept. An
// accepted token goes into the system keychain as the stored secret the
// server declares, and is never sent back to the page. Neither the token nor
// its removal is taken while the agent runs, as the CLI does.

// PersistentFastmailView is an agent's Fastmail set-up, as far as the app
// shows it: whether a token is kept, and when it was given. Never the token.
type PersistentFastmailView struct {
	Token bool   `json:"token"`
	Set   string `json:"set,omitempty"`
}

// hasFastmail says the agent offers Fastmail's server.
func hasFastmail(a persistent.Agent) bool {
	return slices.ContainsFunc(a.Builtin.OfferedServers(nil), func(o offer.Server) bool { return o.MailProvider == assistant.MailFastmail })
}

// fastmailView is the agent's Fastmail set-up, nil for an agent with no
// such server.
func fastmailView(a persistent.Agent, cfg persistent.Config) *PersistentFastmailView {
	if !hasFastmail(a) {
		return nil
	}
	v := &PersistentFastmailView{}
	for _, s := range cfg.Secrets {
		if s.Name == assistant.FastmailToken {
			v.Token = true
			if !s.Set.IsZero() {
				v.Set = s.Set.Format(time.RFC3339)
			}
		}
	}
	return v
}

// fastmailAgent is the agent named, if it has Fastmail and does not run
// here.
func (p *PersistentAPI) fastmailAgent(name string) (persistent.Agent, error) {
	a, err := p.agent(name)
	if err != nil {
		return persistent.Agent{}, err
	}
	if !hasFastmail(a) || !a.Builtin.KeepsSecrets() {
		return persistent.Agent{}, fmt.Errorf("the %s has no Fastmail", a.Title)
	}
	if _, running := p.running(a.Name); running {
		return persistent.Agent{}, fmt.Errorf("stop the %s first", a.Title)
	}
	return a, nil
}

// fastmailAccepted asks Fastmail whether it accepts a token: the CLI's own
// question, but in a test.
func (p *PersistentAPI) fastmailAccepted(ctx context.Context, token string) error {
	if p.fastmailCheck != nil {
		return p.fastmailCheck(ctx, token)
	}
	return persistent.FastmailTokenAccepted(ctx, token)
}

// SetFastmailToken keeps the person's Fastmail API token once Fastmail
// accepts it: replacing a kept one asks first, the Fastmail provider is
// enabled before the token is recorded (so the agreement asked for names
// the server it goes to), and a change refused or failed puts the keychain
// and the provider back as they were.
func (p *PersistentAPI) SetFastmailToken(name, token string) (*PersistentAgentView, error) {
	a, err := p.fastmailAgent(name)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("the token is empty: nothing was kept")
	}
	if err := persistent.CheckSecretValue(token); err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	keys := p.secretStore()
	account := persistent.SecretAccount(a.Name, assistant.FastmailToken)
	previous := ""
	if v := fastmailView(a, cfg); v != nil && v.Token {
		if err := p.app.askConsent("Replace your Fastmail token?", "This replaces the Fastmail API token that "+called(a, cfg)+" has, in your system keychain. The old token is not kept here; delete it in Fastmail's settings if you no longer use it."); err != nil {
			return nil, err
		}
		if previous, err = keys.Get(account); err != nil {
			return nil, fmt.Errorf("the keychain's Fastmail token could not be read, so it is left as it is: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.fastmailAccepted(ctx, token); err != nil {
		return nil, err
	}
	wasOn := slices.Contains(cfg.MailProviders, assistant.MailFastmail)
	if err := persistent.SetMailProvider(home, a, persistent.ClientDesktop, assistant.MailFastmail, true); err != nil {
		return nil, err
	}
	undo := func() {
		if previous != "" {
			_ = keys.Set(account, previous)
		} else {
			_ = keys.Delete(account)
		}
		if !wasOn {
			_ = persistent.SetMailProvider(home, a, persistent.ClientDesktop, assistant.MailFastmail, false)
		}
	}
	if err := keys.Set(account, token); err != nil {
		undo()
		return nil, err
	}
	if err := p.configure(a, persistent.Change{SetSecret: &persistent.StoredSecret{Name: assistant.FastmailToken, Set: time.Now().UTC()}}); err != nil {
		undo()
		return nil, err
	}
	return p.Get(name)
}

// RemoveFastmailToken takes the Fastmail token away from the agent, then
// out of the keychain. It stays valid at Fastmail until the person deletes
// it there.
func (p *PersistentAPI) RemoveFastmailToken(name string) (*PersistentAgentView, error) {
	a, err := p.fastmailAgent(name)
	if err != nil {
		return nil, err
	}
	return p.RemoveSecret(a.Name, assistant.FastmailToken)
}
