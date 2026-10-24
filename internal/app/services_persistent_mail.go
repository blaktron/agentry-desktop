package app

import (
	"fmt"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// The Assistant's mail providers in the app: the services whose mail and
// calendar it reaches, each enabled or disabled in its Configuration's Mail
// and calendar section. Only an enabled provider's cards are drawn; a
// disabled one keeps its sign-in, so enabling it again needs no new one.
// Every change is the CLI's own (persistent.SetMailProvider).

// PersistentMailProviderView is one mail provider as the page draws it:
// whether it is enabled, its servers' roster ids, the accounts its servers
// are signed in as, and what a server not yet signed in needs. Never a
// credential.
type PersistentMailProviderView struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Servers  []string `json:"servers"`
	Accounts []string `json:"accounts"`
	SetUp    []string `json:"setUp"`
}

// mailProviderViews are the agent's mail providers, none for an agent that
// reaches no mail.
func mailProviderViews(a persistent.Agent, cfg persistent.Config) []PersistentMailProviderView {
	out := []PersistentMailProviderView{}
	for _, p := range persistent.MailProviders(a, cfg) {
		out = append(out, PersistentMailProviderView{Key: p.Key, Name: p.Name, Enabled: p.Enabled, Servers: nonNil(p.Servers), Accounts: nonNil(p.Accounts), SetUp: nonNil(p.SetUp)})
	}
	return out
}

// nonNil is l, or an empty list, so the page always gets an array.
func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// SetMailProvider enables or disables one of the agent's mail providers.
// Disabling takes its cards off the Configuration and its servers out of
// the agent's next start, and keeps its sign-in, keychain entry and volume.
// It is refused while the agent runs, here or elsewhere, since its servers
// hold their sign-ins.
func (p *PersistentAPI) SetMailProvider(name, key string, on bool) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	if _, here := p.running(a.Name); here {
		return nil, fmt.Errorf("stop the %s first: its mail servers hold their sign-ins while it runs", a.Title)
	}
	if err := persistent.SetMailProvider(home, a, persistent.ClientDesktop, key, on); err != nil {
		return nil, err
	}
	return p.Get(name)
}
