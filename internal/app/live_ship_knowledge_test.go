package app

// Shipping a knowledge pack and making the QuickAgent public, live: a pack
// taken from a release that already ships one is laid into a new
// QuickAgent's folder, which Submit sends with the publisher's confirmation
// (one native dialog per pack), and the version records who confirmed it;
// Make public on the QuickAgent made private again needs the confirmation
// too, and the registry refuses it without. The QuickAgent is deleted at the
// end. Skipped unless the variables are set; the token needs agent:publish,
// agent:manage and user:profile in AGENTRY_LIVE_SHIP_NAMESPACE.
//
//	AGENTRY_LIVE_SHIP_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_SHIP_TOKEN=… \
//	AGENTRY_LIVE_SHIP_NAMESPACE=<the token's handle> \
//	AGENTRY_LIVE_SHIP_PACK_REF=chris/m9-ships-check@0.2.0 \
//	go test -run TestLiveShipKnowledge -v ./internal/app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// copyShipper lays a pack from a tree the test extracted, in place of a
// stored pack.
type copyShipper struct {
	fromDir string
	pack    string
}

func (c copyShipper) Lay(_ context.Context, _, dir string, src build.ShipSource) (*build.Laid, error) {
	dest := filepath.Join(dir, "knowledge", src.Name)
	if err := os.RemoveAll(dest); err != nil {
		return nil, err
	}
	if err := copyTree(filepath.Join(c.fromDir, "knowledge", c.pack), dest); err != nil {
		return nil, err
	}
	return &build.Laid{}, nil
}
func (c copyShipper) Prune(string, []string) error                                 { return nil }
func (c copyShipper) Origins(string, string) (map[string]*build.PackOrigin, error) { return nil, nil }

func copyTree(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dest := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		b, err := os.ReadFile(p) //nolint:gosec // a tree this test extracted
		if err != nil {
			return err
		}
		return os.WriteFile(dest, b, 0o644) //nolint:gosec // a test fixture
	})
}

func TestLiveShipKnowledge(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_SHIP_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_SHIP_TOKEN")
	ns := os.Getenv("AGENTRY_LIVE_SHIP_NAMESPACE")
	ref := os.Getenv("AGENTRY_LIVE_SHIP_PACK_REF")
	if reg == "" || token == "" || ns == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_SHIP_REGISTRY, _TOKEN, _NAMESPACE and _PACK_REF to run the live check of shipping a pack")
	}
	keyring.MockInit()
	setUserHome(t, t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	// A build signs in to the one SecureAgentry registry it is for.
	was := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = settings.Normalize(reg)
	t.Cleanup(func() { settings.DefaultRegistryURL = was })
	if _, err := settings.StoreToken(settings.Normalize(reg), token); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	client, err := registry.New(settings.Normalize(reg), token)
	if err != nil {
		t.Fatal(err)
	}

	// A valid pack: the tree of a release that ships one.
	res, err := registry.Resolve(ctx, client, ref, false)
	if err != nil {
		t.Fatalf("resolve %s: %v", ref, err)
	}
	archive, err := registry.Download(ctx, settings.Normalize(reg), token, res)
	if err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	if err := pkgarchive.Extract(bytes.NewReader(archive), tree, int64(len(archive)), registry.ExtractLimits()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(tree, "knowledge"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("%s ships no pack: %v", ref, err)
	}
	packName := entries[0].Name()

	var raw [4]byte
	_, _ = rand.Read(raw[:])
	name := "ship-live-" + hex.EncodeToString(raw[:])
	var dialogs []string
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, message string) (bool, error) {
		dialogs = append(dialogs, title+"\n"+message)
		return true, nil
	}}
	b := &BuildAPI{app: app, run: &RunAPI{app: app}, shipper: copyShipper{fromDir: tree, pack: packName}}
	p := &PublisherAPI{app: app}
	dir := t.TempDir()
	app.markPicked(dir)
	t.Cleanup(func() {
		if _, err := p.Delete(reg, ns, name, name); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})

	m := build.Default(ns)
	m.Name, m.Summary, m.FSRead = name, "Ships a knowledge pack, for a live check; deleted when it ends.", "none"
	if err := m.TickShip(build.ShipSource{Name: packName, Kind: build.ShipOwn}); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(reg, dir, *m); err != nil {
		t.Fatalf("save: %v", err)
	}
	pub, err := b.Submit(reg, dir)
	if err != nil || pub == nil {
		t.Fatalf("submit: %+v, %v (dialogs %q)", pub, err, dialogs)
	}
	t.Logf("submit: %+v", pub)
	// The submission's dialog, then the pack's.
	if len(dialogs) != 2 || !strings.Contains(dialogs[1], "becomes public.") || !strings.Contains(dialogs[1], packName) {
		t.Fatalf("dialogs %q", dialogs)
	}
	if pub.Status != "published" && pub.Status != "needs_review" {
		t.Fatalf("the version is %s: %s", pub.Status, pub.Detail)
	}

	// The version records who confirmed the pack, and how.
	vr, err := client.GetVersionInfoWithResponse(ctx, ns, name, "0.1.0")
	if err != nil || vr.JSON200 == nil {
		t.Fatalf("version: %v %s", err, vr.Body)
	}
	rec := vr.JSON200.KnowledgeShipping
	if rec == nil || string(rec.Via) != "client" || len(rec.Packs) != 1 || rec.Packs[0].Name != packName || rec.By == nil || *rec.By != ns {
		t.Fatalf("the version's record: %+v", rec)
	}

	// Make public: private first, then the registry refuses without the
	// confirmation and the desktop asks for it.
	if _, err := p.SetVisibility(reg, ns, name, "private"); err != nil {
		t.Fatal(err)
	}
	pub2 := apiclient.UpdateAgentJSONBodyVisibilityPublic
	r, err := client.UpdateAgentWithResponse(ctx, ns, name, apiclient.UpdateAgentJSONRequestBody{Visibility: &pub2})
	if err != nil || r.StatusCode() != http.StatusConflict || !strings.Contains(string(r.Body), "shipped-knowledge-unacknowledged") {
		t.Fatalf("without the confirmation: %v %d %s", err, r.StatusCode(), r.Body)
	}
	dialogs = nil
	done, err := p.SetVisibility(reg, ns, name, "public")
	if err != nil || done.Cancelled {
		t.Fatalf("make public: %+v, %v", done, err)
	}
	if len(dialogs) != 2 || !strings.Contains(dialogs[1], packName+" (") || !strings.Contains(dialogs[1], "becomes public.") || !strings.Contains(dialogs[1], "version 0.1.0") {
		t.Fatalf("make public dialogs %q", dialogs)
	}
}

// The whole path with a real stored pack: a pack kept on this computer, copied
// to the registry and waited on until it is ready, offered by ShipOptions,
// laid by the real shipper at Save, confirmed and sent by Submit with the
// origin the registry checks, and recorded on the version. The pack and the
// QuickAgent are deleted at the end. Same variables as the check above,
// without AGENTRY_LIVE_SHIP_PACK_REF.
//
//	go test -run TestLiveShipStoredPack -v ./internal/app
func TestLiveShipStoredPack(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_SHIP_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_SHIP_TOKEN")
	ns := os.Getenv("AGENTRY_LIVE_SHIP_NAMESPACE")
	if reg == "" || token == "" || ns == "" {
		t.Skip("set AGENTRY_LIVE_SHIP_REGISTRY, _TOKEN and _NAMESPACE to run the live check of shipping a stored pack")
	}
	keyring.MockInit()
	setUserHome(t, t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	was := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = settings.Normalize(reg)
	t.Cleanup(func() { settings.DefaultRegistryURL = was })
	if _, err := settings.StoreToken(settings.Normalize(reg), token); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	home := t.TempDir()
	env := knowledge.Env{Home: func() (string, error) { return home, nil }, Keys: &recordingKeys{m: map[string]string{}},
		TokenFor: settings.TokenFor, HTTP: registry.HTTPClient}

	var raw [4]byte
	_, _ = rand.Read(raw[:])
	suffix := hex.EncodeToString(raw[:])
	packName, name := "ship-live-pack-"+suffix, "ship-live-"+suffix
	files := map[string]string{
		"sources/a.md":        "# Rota\nThe rota changes on Mondays.\n",
		"wiki/topics/rota.md": "---\ntitle: Rota\ntype: topic\nsummary: When the rota changes\nsources: [doc:a.md]\nupdated: 2026-09-25\n---\n\nThe rota changes on Mondays [src: doc:a.md].\n",
	}
	pack := knowledgetest.Make(t, packName, files)
	s, err := env.Store(reg)
	if err != nil {
		t.Fatal(err)
	}
	// A token with knowledge:read and knowledge:write copies the pack to the
	// registry and ships it with an origin the registry checks; one without
	// them keeps the pack on this computer's own key and ships it as the
	// person's own files.
	var agentryID string
	stored := true
	if _, err := s.Keep(ctx, pack.Dir); err != nil {
		if !strings.Contains(err.Error(), "Access denied") {
			t.Fatalf("keep: %v", err)
		}
		t.Logf("the token carries no knowledge scopes (%v): the pack is kept on this computer only", err)
		stored = false
		local, lerr := env.Store("")
		if lerr != nil {
			t.Fatal(lerr)
		}
		if _, err := local.Keep(ctx, pack.Dir); err != nil {
			t.Fatalf("keep on this computer: %v", err)
		}
	}
	if stored {
		if _, err := env.Upload(ctx, reg, packName); err != nil {
			t.Fatalf("copy to Agentry: %v", err)
		}
	}
	t.Cleanup(func() {
		if agentryID != "" {
			if err := env.DeleteFromAgentry(ctx, reg, agentryID); err != nil {
				t.Logf("cleanup of the pack in Agentry: %v", err)
			}
		}
	})
	var opt *build.ShipOption
	app := &App{ctx: ctx}
	var dialogs []string
	app.confirm = func(_ context.Context, title, message string) (bool, error) {
		dialogs = append(dialogs, title+"\n"+message)
		return true, nil
	}
	b := &BuildAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env}}
	for deadline := time.Now().Add(4 * time.Minute); stored && time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		opts, err := b.ShipOptions(reg)
		if err != nil {
			t.Fatal(err)
		}
		for i := range opts {
			if opts[i].Name == packName {
				agentryID = opts[i].AgentryID
				if opts[i].Ready && opts[i].Reason(ns) == "" {
					opt = &opts[i]
				}
			}
		}
		if opt != nil {
			break
		}
	}
	if stored && opt == nil {
		t.Fatal("the pack never became shippable")
	}
	if opt == nil {
		opt = &build.ShipOption{ShipSource: build.ShipSource{Name: packName, Kind: build.ShipOwn}}
	}
	p := &PublisherAPI{app: app}
	dir := t.TempDir()
	app.markPicked(dir)
	t.Cleanup(func() {
		if _, err := p.Delete(reg, ns, name, name); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	m := build.Default(ns)
	m.Name, m.Summary, m.FSRead = name, "Ships a stored knowledge pack, for a live check; deleted when it ends.", "none"
	if err := m.TickShip(opt.ShipSource); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(reg, dir, *m); err != nil {
		t.Fatalf("save: %v", err)
	}
	pub, err := b.Submit(reg, dir)
	if err != nil || pub == nil {
		t.Fatalf("submit: %+v, %v (dialogs %q)", pub, err, dialogs)
	}
	t.Logf("submit: status %s, %s", pub.Status, pub.Detail)
	if pub.Status != "published" && pub.Status != "needs_review" {
		t.Fatalf("the version is %s: %s", pub.Status, pub.Detail)
	}
	client, err := registry.New(settings.Normalize(reg), token)
	if err != nil {
		t.Fatal(err)
	}
	vr, err := client.GetVersionInfoWithResponse(ctx, ns, name, "0.1.0")
	if err != nil || vr.JSON200 == nil {
		t.Fatalf("version: %v %s", err, vr.Body)
	}
	rec := vr.JSON200.KnowledgeShipping
	if rec == nil || string(rec.Via) != "client" || len(rec.Packs) != 1 || rec.Packs[0].Name != packName {
		t.Fatalf("the version's record: %+v", rec)
	}
	o := rec.Packs[0].Origin
	switch {
	case stored && (o == nil || string(o.Kind) != "own" || o.PackId.String() != agentryID):
		t.Fatalf("the pack's origin: %+v, want the copy in Agentry %s", o, agentryID)
	case !stored && o != nil:
		t.Fatalf("a pack kept only here claimed an origin: %+v", o)
	}
}
