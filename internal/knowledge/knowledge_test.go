package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
)

// memKeys is a keychain in memory: no test touches a person's own.
type memKeys struct {
	mu sync.Mutex
	m  map[string]string
	// sets is every account written, in order: what a test reads to say
	// a key never reached the keychain.
	sets []string
}

func newKeys() *memKeys { return &memKeys{m: map[string]string{}} }

func (k *memKeys) Get(a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[a]
	if !ok {
		return "", store.ErrNoEntry
	}
	return v, nil
}

func (k *memKeys) Set(a, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[a] = v
	k.sets = append(k.sets, a)
	return nil
}

func (k *memKeys) Delete(a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, a)
	return nil
}

func (k *memKeys) has(a string) bool {
	_, err := k.Get(a)
	return err == nil
}

// testEnv is the store wired as this app wires it, to a runner home of its
// own, a keychain in memory, and a token for reg when signedIn.
func testEnv(t *testing.T, reg *knowledgetest.Registry, token string, keys store.Keychain) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		Home: func() (string, error) { return home, nil },
		Keys: keys,
		TokenFor: func(u string) string {
			if token != "" && strings.TrimRight(u, "/") == reg.URL {
				return token
			}
			return ""
		},
		HTTP: reg.Client(),
	}
}

func homeOf(t *testing.T, e Env) string {
	t.Helper()
	h, err := e.Home()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func keep(t *testing.T, e Env, reg, name string) store.Entry {
	t.Helper()
	s, err := e.Store(reg)
	if err != nil {
		t.Fatal(err)
	}
	en, err := s.Keep(context.Background(), knowledgetest.Make(t, name, nil).Dir)
	if err != nil {
		t.Fatalf("keep %s: %v", name, err)
	}
	return en
}

// noPlaintextLeft fails if the store holds anything but the packs' own
// three files: a scratch directory left behind would be pages in the clear.
func noPlaintextLeft(t *testing.T, e Env) {
	t.Helper()
	dir := filepath.Join(homeOf(t, e), store.Dir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range entries {
		if strings.HasPrefix(d.Name(), ".") {
			t.Errorf("the store keeps %s: a private directory left behind", d.Name())
			continue
		}
		files, _ := os.ReadDir(filepath.Join(dir, d.Name()))
		for _, f := range files {
			switch f.Name() {
			case kn.ManifestFile, store.SealedFile, store.KeyFile:
			default:
				t.Errorf("pack %s holds %s beside its sealed tree", d.Name(), f.Name())
			}
		}
	}
}

// A pack kept on one runner home opens on a fresh one after sign-in, with
// nothing carried between the two but the account (plan D4), against a
// core-only registry, whose wrapper is the deployment's key (the fake is
// the core's routes and shapes; knowledge.md §3's core-only row). Signed
// out, with no key cached, it does not open.
func TestAPackKeptOnOneHomeOpensOnAFreshOneAfterSignIn(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	tok := reg.Token("ana")

	a := testEnv(t, reg, tok, newKeys())
	en := keep(t, a, reg.URL, "handbook")
	if en.Key.Wrapper != store.WrapperRegistry || en.Key.WrappedKey.Kind != "deployment" {
		t.Fatalf("kept signed in to a core registry: wrapper %q kind %q, want the registry's deployment wrap", en.Key.Wrapper, en.Key.WrappedKey.Kind)
	}
	if _, err := a.Upload(ctx, reg.URL, "handbook"); err != nil {
		t.Fatal(err)
	}

	// A fresh runner home and a fresh keychain, signed in to the same
	// account: Agentry lists the pack, it downloads, and it opens.
	b := testEnv(t, reg, tok, newKeys())
	l, err := b.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Packs) != 1 || l.Packs[0].Local || !l.Packs[0].InAgentry || l.Packs[0].Where != "Agentry" {
		t.Fatalf("fresh home's list = %+v, want the one pack, in Agentry only", l.Packs)
	}
	if _, err := b.Download(ctx, reg.URL, "handbook"); err != nil {
		t.Fatal(err)
	}
	pv, err := b.Preview(ctx, reg.URL, "handbook")
	if err != nil {
		t.Fatalf("preview on the fresh home: %v", err)
	}
	if !strings.Contains(pv.Index.Markdown, "Buckets") || len(pv.Pages) != 1 || pv.Pages[0].Title != "Buckets" {
		t.Fatalf("preview = %+v", pv)
	}
	noPlaintextLeft(t, b)

	// The same fresh home signed out, its cache gone: the pack is kept
	// there, and does not open.
	out := b
	out.Keys, out.TokenFor = newKeys(), func(string) string { return "" }
	if _, err := out.Preview(ctx, reg.URL, "handbook"); !errors.Is(err, store.ErrSignedOut) {
		t.Fatalf("signed out with no cached key: %v, want ErrSignedOut", err)
	}
	// Another account at the same registry cannot open it either.
	other := b
	other.Keys, other.TokenFor = newKeys(), func(string) string { return reg.Token("bo") }
	if _, err := other.Preview(ctx, reg.URL, "handbook"); !errors.Is(err, store.ErrKeyNotYours) {
		t.Fatalf("another account: %v, want ErrKeyNotYours", err)
	}
	noPlaintextLeft(t, out)
}

// Kept signed out, a pack is wrapped by this computer's key, which is in
// this computer's keychain only: a copy of the store on another computer
// does not open without it, and the next sign-in rewraps it (D20).
func TestAPackKeptSignedOutNeedsThisComputersKeyUntilSignIn(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	keys := newKeys()
	a := testEnv(t, reg, "", keys)
	en := keep(t, a, reg.URL, "notes")
	if en.Key.Wrapper != store.WrapperMachine {
		t.Fatalf("kept signed out: wrapper %q, want this computer's key", en.Key.Wrapper)
	}
	l, err := a.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p := l.Packs[0]; !strings.Contains(p.Key, "this computer's key") || !p.OpensOffline || l.SignedIn {
		t.Fatalf("signed-out pack = %+v (signed in %v)", p, l.SignedIn)
	}

	// The store's directory copied to another computer's runner home,
	// with that computer's keychain: it does not open.
	c := testEnv(t, reg, "", newKeys())
	if err := os.CopyFS(filepath.Join(homeOf(t, c), store.Dir), os.DirFS(filepath.Join(homeOf(t, a), store.Dir))); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Preview(ctx, reg.URL, "notes"); err == nil || !strings.Contains(err.Error(), "machine key") {
		t.Fatalf("another computer without the machine key: %v", err)
	}

	// Signing in on the first computer moves it to the account's key.
	in := a
	tok := reg.Token("ana")
	in.TokenFor = func(u string) string { return tok }
	moved, err := in.Rewrap(ctx, reg.URL)
	if err != nil || len(moved) != 1 || moved[0] != "notes" {
		t.Fatalf("rewrap = %v, %v", moved, err)
	}
	l, err = in.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p := l.Packs[0]; p.Wrapper != string(store.WrapperRegistry) || !strings.Contains(p.Key, "keeps for your account") {
		t.Fatalf("after sign-in = %+v", p)
	}
}

// The panel's words for each pack: where it is, what holds its key, and
// what read it, never "safe", "secure" or "private" (plan §3).
func TestTheListSaysWhereEachPackIsAndWhatHoldsItsKey(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	tok := reg.Token("ana")
	keys := newKeys()
	e := testEnv(t, reg, tok, keys)
	keep(t, e, reg.URL, "local-only")
	keep(t, e, reg.URL, "both")
	if _, err := e.Upload(ctx, reg.URL, "both"); err != nil {
		t.Fatal(err)
	}
	// A pack only Agentry keeps: kept and uploaded from another computer.
	elsewhere := testEnv(t, reg, tok, newKeys())
	keep(t, elsewhere, reg.URL, "remote-only")
	if _, err := elsewhere.Upload(ctx, reg.URL, "remote-only"); err != nil {
		t.Fatal(err)
	}

	l, err := e.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !l.SignedIn || l.Agentry != "" || len(l.Packs) != 3 {
		t.Fatalf("list = %+v", l)
	}
	byName := map[string]Pack{}
	for _, p := range l.Packs {
		byName[p.Name] = p
	}
	host := strings.TrimPrefix(reg.URL, "http://")
	// The fake is a core registry: its wrap is the deployment's, which is
	// no account key and need not be Agentry's.
	if p := byName["local-only"]; !p.Local || p.InAgentry || p.Where != "this computer" || p.ScanState != ScanNotScanned || p.Scan != "not scanned" ||
		p.Key != "a key the registry at "+host+" keeps for your account" || !p.OpensOffline {
		t.Errorf("local-only = %+v", p)
	}
	if p := byName["both"]; !p.Local || !p.InAgentry || p.Where != "this computer and Agentry" || p.ScanState != ScanScanned || p.Scan != "scanned in Agentry, 0 findings" {
		t.Errorf("both = %+v", p)
	}
	if p := byName["remote-only"]; p.Local || !p.InAgentry || p.Where != "Agentry" || p.ScanState != ScanScanned || p.Key != "" {
		t.Errorf("remote-only = %+v", p)
	}
	// What a run In Agentry may attach (agentry-desktop#275): the copies
	// Agentry keeps and has scanned, never a pack kept only here.
	if !byName["both"].AgentryReady || !byName["remote-only"].AgentryReady || byName["local-only"].AgentryReady {
		t.Errorf("agentryReady: both %v, remote-only %v, local-only %v", byName["both"].AgentryReady, byName["remote-only"].AgentryReady, byName["local-only"].AgentryReady)
	}

	// The dates the Local and Cloud Knowledge pages show (plan
	// knowledge-cards.md §4.2, §4.3; agentry-desktop#619): when the copy
	// here last changed, from its manifest, and when Agentry's did, from
	// its list. A pack kept in one place has only that place's date.
	for name, want := range map[string][2]bool{"local-only": {true, false}, "both": {true, true}, "remote-only": {false, true}} {
		p := byName[name]
		if (p.Updated != "") != want[0] || (p.AgentryUpdated != "") != want[1] {
			t.Errorf("%s: updated %q, agentryUpdated %q", name, p.Updated, p.AgentryUpdated)
		}
		for _, d := range []string{p.Updated, p.AgentryUpdated} {
			if d == "" {
				continue
			}
			if at, err := time.Parse(time.RFC3339, d); err != nil || at.IsZero() {
				t.Errorf("%s: date %q is not an RFC 3339 time (%v)", name, d, err)
			}
		}
	}

	// Signed out, the kept packs are listed and Agentry says why not.
	out := e
	out.TokenFor = func(string) string { return "" }
	l, err = out.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if l.SignedIn || len(l.Packs) != 2 || !strings.Contains(l.Agentry, "Not signed in to "+host) {
		t.Fatalf("signed-out list = %+v", l)
	}

	never := regexp.MustCompile(`(?i)\b(safe|safely|secure|securely|secured|private|privately)\b`)
	for _, p := range byName {
		for _, s := range []string{p.Where, p.Key, p.Scan} {
			if never.MatchString(s) {
				t.Errorf("%s says %q", p.Name, s)
			}
		}
	}
	if never.MatchString(l.Agentry) {
		t.Errorf("the list says %q", l.Agentry)
	}
}

// A run's attachment is the pack opened into a private directory inside
// the store, which done removes; a pack that is not kept, or a name that
// is not a pack's, is refused before anything runs.
func TestAttachOpensEachPackAndDoneRemovesIt(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	en := keep(t, e, reg.URL, "handbook")
	packs, done, err := e.Attach(ctx, reg.URL, []string{"handbook", "handbook"})
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].Name != "handbook" || packs[0].Digest != en.Manifest.SHA256 {
		t.Fatalf("attached = %+v", packs)
	}
	if _, err := os.Stat(filepath.Join(packs[0].Dir, kn.WikiDir, kn.IndexFile)); err != nil {
		t.Fatalf("the attachment holds no index: %v", err)
	}
	done()
	if _, err := os.Stat(packs[0].Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the attachment is still there after done: %v", err)
	}
	noPlaintextLeft(t, e)

	// A refusal cleans up after itself: the bindings return it without
	// calling done, and a pack opened before the one refused is gone too.
	for _, names := range [][]string{{"missing"}, {"../handbook"}, {"handbook", "missing"}} {
		if _, _, err := e.Attach(ctx, reg.URL, names); err == nil {
			t.Errorf("attach %v passed", names)
		}
	}
	noPlaintextLeft(t, e)
}

// Export writes the pack in the clear into a new folder named for it in
// the folder given, and refuses a folder of that name already there,
// leaving it as it was.
func TestExportWritesANewFolderAndNeverIntoOneThere(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	en := keep(t, e, reg.URL, "handbook")
	parent := t.TempDir()
	dir, err := e.Export(ctx, reg.URL, "handbook", parent)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(parent, "handbook") {
		t.Fatalf("exported to %s", dir)
	}
	p, err := kn.Load(dir)
	if err != nil || p.Manifest.SHA256 != en.Manifest.SHA256 {
		t.Fatalf("the export is not the pack: %v", err)
	}
	if _, err := e.Export(ctx, reg.URL, "handbook", parent); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second export into the same folder: %v", err)
	}
	if _, err := kn.Load(dir); err != nil {
		t.Fatalf("the refused export touched the first: %v", err)
	}
	if _, err := e.Export(ctx, reg.URL, "../handbook", parent); err == nil {
		t.Fatal("an export named out of the folder passed")
	}
	noPlaintextLeft(t, e)
}

// A preview reads only the pack's pages, never another file of it and
// never a path out of it; a page comes without its frontmatter.
func TestReadPageReadsOnlyAPage(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	keep(t, e, reg.URL, "handbook")
	p, err := e.ReadPage(ctx, reg.URL, "handbook", "wiki/topics/buckets.md")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Buckets" || p.Summary != "Naming" || strings.Contains(p.Markdown, "title:") || !strings.Contains(p.Markdown, "globally unique") {
		t.Fatalf("page = %+v", p)
	}
	for _, bad := range []string{"manifest.json", "sources/guide.txt", "../manifest.json", "wiki/../manifest.json", "/etc/passwd", "wiki/./topics/buckets.md", "wiki/topics/../../sources/guide.txt"} {
		if _, err := e.ReadPage(ctx, reg.URL, "handbook", bad); err == nil {
			t.Errorf("ReadPage %q passed", bad)
		}
	}
	noPlaintextLeft(t, e)
}

// Delete removes the sealed tree, the wrapped key and the key the keychain
// caches, marker and key both.
func TestDeleteRemovesTheSealedFileAndTheCachedKey(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	keys := newKeys()
	e := testEnv(t, reg, reg.Token("ana"), keys)
	en := keep(t, e, reg.URL, "handbook")
	if !keys.has("knowledge:"+en.ID()) || !keys.has("stored:knowledge:"+en.ID()) {
		t.Fatalf("keeping it signed in cached no key: %v", keys.m)
	}
	if err := e.Delete(ctx, reg.URL, "handbook", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(en.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the pack's directory is still there: %v", err)
	}
	if keys.has("knowledge:"+en.ID()) || keys.has("stored:knowledge:"+en.ID()) {
		t.Fatalf("the cached key is still in the keychain: %v", keys.m)
	}
}

// A pack's key is worded by what wrapped it: SecureAgentry's certificate
// is the account's key, which Agentry holds; a core registry's deployment
// wrap is a key that registry keeps for the account; this computer's key
// is the CLI's sentence.
func TestTheKeyIsWordedByItsWrapper(t *testing.T) {
	entry := func(w store.Wrapper, kind string) store.Entry {
		return store.Entry{Key: store.KeyRecord{Wrapper: w, Registry: "https://secureagentry.net", WrappedKey: store.WrappedKey{Kind: kind}}}
	}
	for _, c := range []struct {
		e    store.Entry
		want string
	}{
		{entry(store.WrapperRegistry, "certificate"), "your account's key at secureagentry.net, which Agentry holds"},
		{entry(store.WrapperRegistry, "deployment"), "a key the registry at secureagentry.net keeps for your account"},
		{entry(store.WrapperRegistry, "something-new"), "a key the registry at secureagentry.net holds (a wrapping this app does not know: something-new)"},
		{entry(store.WrapperMachine, "machine"), "this computer's key, in its keychain (not portable; rewrapped to your account at your next sign-in)"},
	} {
		if got := KeyHolder(c.e); got != c.want {
			t.Errorf("KeyHolder(%s, %s) = %q, want %q", c.e.Key.Wrapper, c.e.Key.WrappedKey.Kind, got, c.want)
		}
	}
}

// A token is found by host alone, and a wrap or an unwrap carries a raw
// data key, so neither is sent to a registry URL Settings would refuse: a
// key.json naming plain http off this machine is refused before any token
// is looked up, and so is such a registry chosen on the page.
func TestNoTokenGoesToAURLSettingsWouldRefuse(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	tok := reg.Token("ana")
	asked := []string{}
	e := testEnv(t, reg, tok, newKeys())
	e.TokenFor = func(u string) string {
		asked = append(asked, u)
		return tok // by host, as settings.TokenFor finds it: any URL here gets it
	}
	en := keep(t, e, reg.URL, "handbook")
	// The record rewritten to name the same host over plain http, off
	// this machine, with the cached key gone so the pack must be unwrapped.
	raw, err := os.ReadFile(filepath.Join(en.Dir, store.KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	forged := strings.Replace(string(raw), reg.URL, "http://registry.example", 1)
	if err := os.WriteFile(filepath.Join(en.Dir, store.KeyFile), []byte(forged), 0o600); err != nil {
		t.Fatal(err)
	}
	e.Keys = newKeys()
	asked = asked[:0]
	if _, err := e.Preview(ctx, reg.URL, "handbook"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("a key.json naming plain http: %v", err)
	}
	if len(asked) != 0 {
		t.Fatalf("a token was looked up for %v before the URL was refused", asked)
	}
	if _, err := e.Upload(ctx, "http://registry.example", "handbook"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("an upload to a plain-http registry: %v", err)
	}
	noPlaintextLeft(t, e)
}

// ForDraft (agentry-desktop#497; plan knowledge-and-drafter.md K4, D3):
// each named pack kept here opened for the Drafter, once, with a scratch
// inside the store; a pack with no scan in Agentry is named to be scanned
// first; done removes every copy. A pack not kept, a name that is not a
// pack's, or more packs than a draft reads is refused with nothing left in
// the clear.
func TestForDraftOpensThePacksAndNamesThoseToScanFirst(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	en := keep(t, e, reg.URL, "handbook")
	d, err := e.ForDraft(ctx, reg.URL, []string{"handbook", "handbook"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Packs) != 1 || d.Packs[0].Pack.Manifest.SHA256 != en.Manifest.SHA256 || len(d.Packs[0].Agentry) != 0 ||
		!reflect.DeepEqual(d.Names(), []string{"handbook"}) || !reflect.DeepEqual(d.ScanFirst, []string{"handbook"}) {
		t.Fatalf("for draft = %+v", d)
	}
	if fi, err := os.Stat(d.Scratch); err != nil || !fi.IsDir() || !strings.HasPrefix(d.Scratch, homeOf(t, e)) {
		t.Fatalf("scratch %q: %v", d.Scratch, err)
	}
	d.Done()
	noPlaintextLeft(t, e)

	for _, names := range [][]string{{"missing"}, {"../handbook"}, {"handbook", "missing"}, {"a", "b", "c", "d", "e", "f"}} {
		if _, err := e.ForDraft(ctx, reg.URL, names); err == nil {
			t.Errorf("for draft %v passed", names)
		}
	}
	noPlaintextLeft(t, e)
	none, err := e.ForDraft(ctx, reg.URL, nil)
	if err != nil || len(none.Packs) != 0 || len(none.Names()) != 0 {
		t.Fatalf("no packs: %+v %v", none, err)
	}
	none.Done()
}
