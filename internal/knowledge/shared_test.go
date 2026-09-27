package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
)

// An organization's shared packs in the panel (Knowledge M8,
// agentry-desktop#276; plan §4.7, D13, D26): listed beside the person's
// own, kept and opened through the organization's route with nothing of
// the key kept on this computer, and exported as the organization allows.

var handbook = map[string]string{
	"sources/handbook.md": "# Handbook\nThe rota changes on Mondays.\n",
	"wiki/index.md":       "# Index\n\n- [Rota](topics/rota.md)\n",
	"wiki/log.md":         "# Log\n",
	"wiki/topics/rota.md": "---\ntitle: Rota\n---\nThe rota changes on Mondays. [src: doc:handbook.md]\n",
}

func sharedEnv(t *testing.T) (*knowledgetest.Registry, Env, *memKeys, string) {
	t.Helper()
	reg := knowledgetest.NewRegistry(t)
	reg.Join("acme", "amy", "member")
	id := reg.SeedShared(t, "acme", "handbook", handbook)
	keys := newKeys()
	e := testEnv(t, reg, reg.Token("amy"), keys)
	e.Orgs = func(context.Context, string) ([]Membership, error) {
		return []Membership{{Org: "acme"}}, nil
	}
	return reg, e, keys, id
}

func keyWrites(k *memKeys, id string) []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []string
	for _, a := range k.sets {
		if strings.Contains(a, id) {
			out = append(out, a)
		}
	}
	return out
}

func find(l *List, name string) (Pack, bool) {
	for _, p := range l.Packs {
		if p.Name == name {
			return p, true
		}
	}
	return Pack{}, false
}

// TestASharedPackOpensForAMemberAndNotOnceTheyAreRemoved is the issue's
// Go test: a shared pack opens for a member, the fake registry then refuses
// the unwrap, and the pack no longer opens, with nothing cached in the
// keychain (the keychain records no write for a shared pack's key).
func TestASharedPackOpensForAMemberAndNotOnceTheyAreRemoved(t *testing.T) {
	reg, e, keys, _ := sharedEnv(t)
	ctx := context.Background()
	l, err := e.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := find(l, "handbook")
	if !ok || !p.Shared || p.Org != "acme" || p.Local || !p.InAgentry {
		t.Fatalf("the list offers the organization's pack as %+v (%v)", p, ok)
	}
	if _, err := e.DownloadShared(ctx, reg.URL, "acme", "handbook"); err != nil {
		t.Fatal(err)
	}
	if l, err = e.List(ctx, reg.URL); err != nil {
		t.Fatal(err)
	}
	p, _ = find(l, "handbook")
	if !p.Local || !p.Shared || p.OpensOffline || p.CanExport || !strings.Contains(p.Key, "acme's organization key") {
		t.Fatalf("the kept shared pack reads %+v", p)
	}
	if len(l.Packs) != 1 {
		t.Fatalf("the kept pack is listed twice: %+v", l.Packs)
	}
	for i := 0; i < 2; i++ {
		if _, err := e.Preview(ctx, reg.URL, "handbook"); err != nil {
			t.Fatalf("a member's preview %d: %v", i, err)
		}
	}
	packs, done, err := e.Attach(ctx, reg.URL, []string{"handbook"})
	if err != nil || len(packs) != 1 {
		t.Fatalf("a member's attach: %v", err)
	}
	done()
	en, err := e.Find(reg.URL, "handbook")
	if err != nil {
		t.Fatal(err)
	}
	if w := keyWrites(keys, en.ID()); len(w) != 0 {
		t.Fatalf("a shared pack's key reached the keychain: %v", w)
	}

	reg.Remove("acme", "amy")
	if _, err := e.Preview(ctx, reg.URL, "handbook"); err == nil {
		t.Fatal("a removed member previewed the organization's pack")
	}
	if _, _, err := e.Attach(ctx, reg.URL, []string{"handbook"}); err == nil {
		t.Fatal("a removed member attached the organization's pack to a run")
	}
	if w := keyWrites(keys, en.ID()); len(w) != 0 {
		t.Fatalf("a shared pack's key reached the keychain: %v", w)
	}
	noPlaintextLeft(t, e)
}

// TestASharedPacksExportIsTheOrganizations: the panel offers Export of a
// shared pack only to its admins or once it is marked exportable (D26), and
// the export itself is the registry's to allow.
func TestASharedPacksExportIsTheOrganizations(t *testing.T) {
	reg, e, _, id := sharedEnv(t)
	ctx := context.Background()
	if _, err := e.DownloadShared(ctx, reg.URL, "acme", "handbook"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Export(ctx, reg.URL, "handbook", t.TempDir()); !errors.Is(err, store.ErrNotExportable) {
		t.Fatalf("a member's export of an unmarked pack: %v", err)
	}
	reg.SetExportable("acme", id, true)
	l, err := e.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := find(l, "handbook"); !p.CanExport || !p.Exportable {
		t.Fatalf("a marked pack reads %+v", p)
	}
	if _, err := e.Export(ctx, reg.URL, "handbook", t.TempDir()); err != nil {
		t.Fatalf("a member's export of a marked pack: %v", err)
	}
	// An admin may export an unmarked one, and the panel says so.
	reg.SetExportable("acme", id, false)
	reg.Join("acme", "amy", "admin")
	e.Orgs = func(context.Context, string) ([]Membership, error) {
		return []Membership{{Org: "acme", Admin: true}}, nil
	}
	if l, err = e.List(ctx, reg.URL); err != nil {
		t.Fatal(err)
	}
	if p, _ := find(l, "handbook"); !p.CanExport || p.Exportable {
		t.Fatalf("an admin's unmarked pack reads %+v", p)
	}
	if _, err := e.Export(ctx, reg.URL, "handbook", t.TempDir()); err != nil {
		t.Fatalf("an admin's export: %v", err)
	}
}

// TestTheMembershipsReadHoldsTheURLRule: GET /user carries the token, so
// it is held to Settings' URL rule before a token is looked up, as every
// other registry call here is (agentry-desktop#307's class).
func TestTheMembershipsReadHoldsTheURLRule(t *testing.T) {
	reg := knowledgetest.NewRegistry(t)
	asked := []string{}
	e := testEnv(t, reg, reg.Token("amy"), newKeys())
	e.TokenFor = func(u string) string { asked = append(asked, u); return "tok" }
	if _, err := e.memberships(context.Background(), "http://registry.example"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("a plain-http registry's memberships: %v", err)
	}
	if len(asked) != 0 {
		t.Fatalf("a token was looked up for %v before the URL was refused", asked)
	}
}

// TestASharedPackTakesNoDocumentsFromThisComputer: Add refuses a shared
// pack before the Creator runs; its admins add to it in Agentry.
func TestASharedPackTakesNoDocumentsFromThisComputer(t *testing.T) {
	reg, e, _, _ := sharedEnv(t)
	ctx := context.Background()
	if _, err := e.DownloadShared(ctx, reg.URL, "acme", "handbook"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Add(ctx, BuildRequest{Registry: reg.URL, Name: "handbook"}); !errors.Is(err, store.ErrShared) {
		t.Fatalf("Add documents to a shared pack: %v", err)
	}
	noPlaintextLeft(t, e)
}
