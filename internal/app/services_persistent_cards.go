package app

// The persistent agent's Configuration as cards: what each card is drawn
// with beyond the roster entry (its icon, what its Set up asks for), the
// servers the agent suggests, and the Add cards' bindings for a suggested
// server, a skill from a folder or a .zip and a stored secret. Every change
// goes through configure, so whatever widens the roster asks first in the
// native dialog; a secret's value travels inward only and is never read
// back.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/persistent/assistant"
	"github.com/blaktron/agentry-cli/pkg/persistent/offer"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/servericon"
)

// PersistentSuggestionView is a server the agent suggests and does not
// have: the Add card lists it first, added in one click.
type PersistentSuggestionView struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Line    string `json:"line"`
	IconKey string `json:"iconKey,omitempty"`
}

// glyphs are the bundled glyphs a built-in's server is drawn with, by a
// word of its name. They are Agentry's own: no brand's mark is drawn.
var glyphs = []struct{ word, key string }{
	{"github", "git"}, {"gitlab", "git"}, {"wikipedia", "encyclopedia"}, {"context7", "docs"},
	{"semgrep", "scan"}, {"weather", "weather"}, {"maps", "map"}, {"google", "mail"}, {"microsoft", "mail"}, {"fastmail", "mail"}, {"brave", "search"},
}

// glyphFor is the glyph of a built-in's server named name, "" for none.
func glyphFor(name string) string {
	n := strings.ToLower(name)
	for _, g := range glyphs {
		if strings.Contains(n, g.word) {
			return g.key
		}
	}
	return ""
}

// cardFacts are what the roster's server entries are drawn with beyond
// the entry: the glyph of each server the built-in offers or suggests (a
// suggested server once added is the person's, and keeps its glyph), and
// what an offered server's Set up asks for.
type cardFacts struct {
	glyph   map[string]string
	secret  map[string]string
	account map[string]string
	// google are the entries whose Set up is Google Workspace's: its
	// server and the switches its sign-in grants; microsoft, Microsoft
	// 365's; fastmail, Fastmail's: its token.
	google, microsoft, fastmail map[string]bool
}

func factsOf(a persistent.Agent, cfg persistent.Config) cardFacts {
	f := cardFacts{glyph: map[string]string{}, secret: map[string]string{}, account: map[string]string{}, google: map[string]bool{}, microsoft: map[string]bool{}, fastmail: map[string]bool{}}
	// The built-in offers some servers for the git hosts its accounts are
	// on (a self-managed GitLab's), as the CLI draws its roster.
	hosts := make([]offer.GitHost, 0, len(cfg.Accounts))
	for _, g := range cfg.Accounts {
		hosts = append(hosts, offer.GitHost{Host: g.Host, Kind: string(g.Kind)})
	}
	for _, o := range a.Builtin.OfferedServers(hosts) {
		id := persistent.ServerEntryID(o.Server)
		g := glyphFor(o.Name)
		f.glyph[id] = g
		f.glyph[persistent.ServerWritesID(o.Server)] = g
		f.secret[id], f.account[id] = o.Secret, o.Account
		f.fastmail[id] = o.MailProvider == assistant.MailFastmail
	}
	for _, s := range a.Builtin.ImageServers(policy.Images{}) {
		id := persistent.ImageServerEntryID(s.Key)
		f.glyph[id] = glyphFor(s.Name)
		f.google[id] = s.Key == assistant.GogKey
		f.microsoft[id] = s.Key == assistant.MS365Key
		for _, w := range s.Switches {
			sw := persistent.ImageSwitchEntryID(s.Key, w.Key)
			f.glyph[sw], f.google[sw], f.microsoft[sw] = f.glyph[id], f.google[id], f.microsoft[id]
		}
	}
	for _, s := range a.Builtin.SuggestedServers() {
		f.glyph[persistent.ServerEntryID(s.Server)] = glyphFor(s.Key + " " + s.Name)
	}
	return f
}

// suggestionsView are the servers the agent suggests and does not have.
func suggestionsView(a persistent.Agent, c persistent.Config) []PersistentSuggestionView {
	out := []PersistentSuggestionView{}
	for _, s := range persistent.Suggestions(a, c) {
		out = append(out, PersistentSuggestionView{Key: s.Key, Name: s.Name, Line: s.Note, IconKey: glyphFor(s.Key + " " + s.Name)})
	}
	return out
}

// iconStore keeps the icons of the servers people added, under the
// config directory.
func iconStore() (servericon.Store, error) {
	base, err := configdir.Dir()
	if err != nil {
		return servericon.Store{}, err
	}
	return servericon.Store{Dir: filepath.Join(base, "server-icons")}, nil
}

// iconClient fetches the registry's entries and the icons they name, from
// public addresses alone.
var iconClient = servericon.Client(20 * time.Second)

// findIcon looks a server's icon up in the official MCP Registry; tests
// put their own here, so none reaches the network.
var findIcon = func(ctx context.Context, srv manifest.MCPServer) ([]byte, string, error) {
	return servericon.Find(ctx, iconClient, servericon.RegistryBase, srv)
}

// serverIcons fills in the kept icon of each server the person added. It
// only reads what is kept: a server's icon is looked up when the person
// adds it (lookUpIcon), never because a page was drawn.
func serverIcons(cfg persistent.Config, entries []PersistentRosterEntry) {
	store, err := iconStore()
	if err != nil {
		return
	}
	for i := range entries {
		e := &entries[i]
		if e.Kind != string(persistent.KindServer) || e.GrantedBy != persistent.GrantedByYou {
			continue
		}
		for _, srv := range cfg.Servers {
			if persistent.ServerEntryID(srv) == e.ID {
				e.Icon, _ = store.Get(srv)
				break
			}
		}
	}
}

// lookUpIcon looks up and keeps the icon of a server the person just
// added, in the background: once kept (an icon, or none it may draw), it
// is never looked up again; a failure to reach the registry or the icon is
// tried again when the server is next added.
func (p *PersistentAPI) lookUpIcon(agent string, srv manifest.MCPServer) {
	store, err := iconStore()
	if err != nil {
		return
	}
	if _, known := store.Get(srv); known {
		return
	}
	p.mu.Lock()
	if p.iconLookups == nil {
		p.iconLookups = map[string]bool{}
	}
	if p.iconLookups[srv.SourceID()] {
		p.mu.Unlock()
		return
	}
	p.iconLookups[srv.SourceID()] = true
	p.mu.Unlock()
	p.lookups.Add(1)
	go func() {
		defer p.lookups.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		b, mt, err := findIcon(ctx, srv)
		switch {
		case errors.Is(err, servericon.ErrNotAnImage):
			b = nil
		case err != nil:
			p.mu.Lock()
			delete(p.iconLookups, srv.SourceID())
			p.mu.Unlock()
			return
		}
		if store.Put(srv, b, mt) == nil && b != nil {
			p.stateChanged(agent)
		}
	}()
}

// AddSuggested adds the server the agent suggests under key, as the
// person's own, on: one click on the Add card, through the consent dialog.
func (p *PersistentAPI) AddSuggested(name, key string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	srv, err := persistent.Suggested(a, key)
	if err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{AddServer: &srv}); err != nil {
		return nil, err
	}
	p.lookUpIcon(a.Name, srv)
	return p.Get(name)
}

// The skill pickers: a folder, or a .zip.
const (
	SkillFromFolder = "folder"
	SkillFromZip    = "zip"
)

// pickSkill opens the native picker for a skill: a folder, or a .zip file.
// "" is a cancel.
func (p *PersistentAPI) pickSkill(from string) (string, error) {
	if p.pickSkillSource != nil {
		return p.pickSkillSource(from)
	}
	if from == SkillFromZip {
		return nativeOpenFile(p.app.ctx, pickOptions{
			Title:   "Choose a skill (.zip)",
			Filters: []fileFilter{{Name: "Skill (.zip)", Pattern: "*.zip"}},
		})
	}
	return nativePickDirectory(p.app.ctx, "Choose a skill's folder (it holds SKILL.md)")
}

// AddSkill adds a skill of the person's own from a folder or a .zip the
// native picker returns (from is SkillFromFolder or SkillFromZip): its
// files are checked (ClamAV when the policy's malware check is on), kept
// by digest, and the skill added on, which asks first. A cancel changes
// nothing; a refused or failed add leaves no files behind.
func (p *PersistentAPI) AddSkill(name, from string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if from != SkillFromFolder && from != SkillFromZip {
		return nil, fmt.Errorf("a skill is added from a folder or a .zip")
	}
	src, err := p.pickSkill(from)
	if err != nil || src == "" {
		if err != nil {
			return nil, err
		}
		return p.Get(name)
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	d := p.dockerClient()
	check := p.checkSkill
	if check == nil {
		check = func(ctx context.Context, dir string) (bool, error) {
			return localrun.CheckFolder(ctx, d, pol, home, dir, io.Discard)
		}
	}
	s, err := persistent.InstallSkill(ctx, home, a, src, check)
	if err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{AddSkill: &s}); err != nil {
		_ = persistent.DiscardSkill(home, a, s.Name)
		return nil, err
	}
	return p.Get(name)
}

// RemoveSkill takes away a skill the person added, and its files. A
// built-in skill is switched off, never removed.
func (p *PersistentAPI) RemoveSkill(name, skill string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{RemoveSkill: skill}); err != nil {
		return nil, err
	}
	if err := persistent.DiscardSkill(home, a, skill); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// SetSecret stores a secret for the agent, or sets its value again: the
// value goes into the system keychain first, then the change records the
// name, which asks first; a change refused or failed puts the keychain
// back as it was. The value is never read back to the page.
func (p *PersistentAPI) SetSecret(name, secret, value string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if !a.Builtin.KeepsSecrets() {
		return nil, fmt.Errorf("the %s keeps no secrets", a.Title)
	}
	if err := persistent.CheckSecretName(secret); err != nil {
		return nil, err
	}
	if value == "" {
		return nil, errors.New("the value is empty: nothing was stored")
	}
	if err := persistent.CheckSecretValue(value); err != nil {
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
	account := persistent.SecretAccount(a.Name, secret)
	if slices.ContainsFunc(cfg.Secrets, func(s persistent.StoredSecret) bool { return s.Name == secret }) {
		// A value set again widens nothing, so the change asks nothing: the
		// native dialog asks here, so the page cannot replace a value the
		// person kept without them.
		if err := p.app.askConsent("Replace the value of "+secret+"?", "This replaces the value of your secret "+secret+" that "+called(a, cfg)+" has, in your system keychain. The old value is not kept."); err != nil {
			return nil, err
		}
	}
	// What the keychain holds under this name already, the secret's or one
	// left there before, is put back if the change does not stand.
	previous, hadPrevious := "", false
	if v, err := keys.Get(account); err == nil {
		previous, hadPrevious = v, true
	}
	if err := keys.Set(account, value); err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{SetSecret: &persistent.StoredSecret{Name: secret, Set: time.Now().UTC()}}); err != nil {
		if hadPrevious {
			_ = keys.Set(account, previous)
		} else {
			_ = keys.Delete(account)
		}
		return nil, err
	}
	return p.Get(name)
}

// RemoveSecret takes a stored secret away from the agent, then out of the
// keychain.
func (p *PersistentAPI) RemoveSecret(name, secret string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{RemoveSecret: secret}); err != nil {
		return nil, err
	}
	if err := p.secretStore().Delete(persistent.SecretAccount(a.Name, secret)); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// secretStore is where stored secrets' values are kept: the keychain, but
// in a test.
func (p *PersistentAPI) secretStore() persistent.Secrets {
	if p.secrets != nil {
		return p.secrets
	}
	return persistent.KeychainSecrets{}
}
