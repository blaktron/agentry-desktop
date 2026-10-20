package knowledge

// The Shipper over the stores: a pack kept here, a pack kept in Agentry, and
// an organization's shared pack, each laid into a QuickAgent's folder with
// the files the registry's rules read, the origin recorded only where it is
// true, and the organization's two rules held: its administrator, its
// namespace.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/ship"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// folderFor saves a QuickAgent in namespace ns that ships nothing yet.
func folderFor(t *testing.T, ns string) string {
	t.Helper()
	m := build.Default(ns)
	m.Name, m.Summary, m.FSRead = "ship-check", "Ships packs.", "none"
	dir := t.TempDir()
	if err := build.Save(dir, m); err != nil {
		t.Fatal(err)
	}
	return dir
}

func asAdmin(e Env, org string, admin bool) Env {
	e.Orgs = func(context.Context, string) ([]Membership, error) {
		return []Membership{{Org: org, Admin: admin}}, nil
	}
	return e
}

// A pack kept only here is laid whole, with a manifest.json a registry
// accepts, and claims no origin; once it has a copy in Agentry that is the
// pack as it stands, it claims the copy, and once it changes it claims
// nothing again.
func TestAKeptPackIsLaidWithTheOriginOnlyWhileItIsTrue(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	en := keep(t, e, reg.URL, "handbook")
	dir := folderFor(t, "ana")
	sh := Shipper{Env: e}
	src := build.ShipSource{Name: "handbook", Kind: build.ShipOwn}

	laid, err := sh.Lay(ctx, reg.URL, dir, src)
	if err != nil || laid.Bytes == 0 || laid.Origin != nil {
		t.Fatalf("laid %+v, %v", laid, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "knowledge", "handbook", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if o, _ := sh.Origins(dir, reg.URL); len(o) != 0 {
		t.Fatalf("a pack kept only here claims %v", o)
	}
	noPlaintextLeft(t, e)

	// Copied to Agentry, it claims the copy.
	if _, err := e.Upload(ctx, reg.URL, "handbook"); err != nil {
		t.Fatal(err)
	}
	if laid, err = sh.Lay(ctx, reg.URL, dir, src); err != nil || laid.Origin == nil || laid.Origin.Kind != "own" || laid.Origin.PackID == "" {
		t.Fatalf("laid %+v, %v", laid, err)
	}
	o, err := sh.Origins(dir, reg.URL)
	if err != nil || o["handbook"] == nil || o["handbook"].PackID != laid.Origin.PackID {
		t.Fatalf("origins %v, %v", o, err)
	}
	// The origin belongs to a registry: another claims none.
	if o, _ := sh.Origins(dir, "https://other.example"); len(o) != 0 {
		t.Fatalf("another registry: %v", o)
	}
	// Files edited by hand are no longer the stored pack.
	rec, _ := ship.ReadRecord(dir)
	rec.Packs[0].SHA256 = "sha256:changed"
	if err := ship.WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	if o, _ := sh.Origins(dir, reg.URL); len(o) != 0 {
		t.Fatalf("a changed pack claims %v", o)
	}
	_ = en
}

// The pack kept in Agentry only is downloaded first; the folder's other
// packs are left alone by a prune, and a laid one that is no longer shipped
// goes with its record.
func TestAPackKeptOnlyInAgentryIsDownloadedAndPruneTakesOutOnlyWhatWasLaid(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	keep(t, e, reg.URL, "handbook")
	if _, err := e.Upload(ctx, reg.URL, "handbook"); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete(ctx, reg.URL, "handbook", false); err != nil {
		t.Fatal(err)
	}
	dir := folderFor(t, "ana")
	sh := Shipper{Env: e}
	laid, err := sh.Lay(ctx, reg.URL, dir, build.ShipSource{Name: "handbook", Kind: build.ShipOwn})
	if err != nil || laid.Origin == nil {
		t.Fatalf("%+v, %v", laid, err)
	}
	// A folder the person put there is not the shipper's.
	if err := os.MkdirAll(filepath.Join(dir, "knowledge", "by-hand"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sh.Prune(dir, []string{"handbook"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "knowledge", "handbook")); err != nil {
		t.Fatal("a pack to keep was taken out")
	}
	if err := sh.Prune(dir, nil); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"handbook": false, "by-hand": true} {
		if _, err := os.Stat(filepath.Join(dir, "knowledge", name)); (err == nil) != want {
			t.Errorf("knowledge/%s present = %v, want %v", name, err == nil, want)
		}
	}
	if rec, _ := ship.ReadRecord(dir); len(rec.Packs) != 0 {
		t.Fatalf("record %+v", rec)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(ship.RecordFile))); err == nil {
		t.Fatal("the record outlived its last pack")
	}
}

func TestAPackPastWhatAReleaseShipsIsRefusedAndLeavesTheFolderAlone(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	s, _ := e.Store(reg.URL)
	big := knowledgetest.MakeCapped(t, "bulk", map[string]string{"sources/a.md": strings.Repeat("x", 11<<20)}, 64<<20)
	if _, err := s.Keep(ctx, big.Dir); err != nil {
		t.Skipf("the fixture could not be kept: %v", err)
	}
	dir := folderFor(t, "ana")
	if _, err := (Shipper{Env: e}).Lay(ctx, reg.URL, dir, build.ShipSource{Name: "bulk", Kind: build.ShipOwn}); err == nil {
		t.Fatal("a pack past 10 MB was laid")
	}
	if _, err := os.Stat(filepath.Join(dir, manifest.KnowledgeDir, "bulk")); err == nil {
		t.Fatal("a refused pack left files")
	}
	noPlaintextLeft(t, e)
}

// An organization's pack ships from its administrator into the
// organization's namespace and no other, and claims the organization's pack
// as its origin.
func TestAnOrganizationsPackShipsFromItsAdminIntoItsNamespaceOnly(t *testing.T) {
	ctx := context.Background()
	reg, e, _, id := sharedEnv(t)
	src := build.ShipSource{Name: "handbook", Kind: build.ShipOrg, Org: "acme"}

	// A member who is not an administrator.
	if _, err := (Shipper{Env: e}).Lay(ctx, reg.URL, folderFor(t, "acme"), src); err == nil || !strings.Contains(err.Error(), "only an administrator of acme") {
		t.Fatalf("a member shipped: %v", err)
	}
	admin := asAdmin(e, "acme", true)
	reg.Join("acme", "amy", "admin")
	if _, err := (Shipper{Env: admin}).Lay(ctx, reg.URL, folderFor(t, "ana"), src); err == nil || !strings.Contains(err.Error(), "ships only with a QuickAgent in acme's own namespace") {
		t.Fatalf("another namespace: %v", err)
	}
	dir := folderFor(t, "acme")
	laid, err := Shipper{Env: admin}.Lay(ctx, reg.URL, dir, src)
	if err != nil {
		t.Fatal(err)
	}
	if laid.Origin == nil || laid.Origin.Kind != "org" || laid.Origin.Org != "acme" || laid.Origin.PackID != id {
		t.Fatalf("origin %+v, want acme's pack %s", laid.Origin, id)
	}
	b, err := os.ReadFile(filepath.Join(dir, "knowledge", "handbook", "wiki", "topics", "rota.md"))
	if err != nil || !strings.Contains(string(b), "rota changes on Mondays") {
		t.Fatalf("the pack's pages were not laid: %v", err)
	}
	o, err := Shipper{Env: admin}.Origins(dir, reg.URL)
	if err != nil || o["handbook"] == nil || o["handbook"].Org != "acme" {
		t.Fatalf("origins %v, %v", o, err)
	}
	noPlaintextLeft(t, admin)
}

func TestAPackThatIsNotTheKindItsSourceSaysIsRefused(t *testing.T) {
	ctx := context.Background()
	reg, e, _, _ := sharedEnv(t)
	if _, err := e.DownloadShared(ctx, reg.URL, "acme", "handbook"); err != nil {
		t.Fatal(err)
	}
	dir := folderFor(t, "amy")
	if _, err := (Shipper{Env: e}).Lay(ctx, reg.URL, dir, build.ShipSource{Name: "handbook", Kind: build.ShipOwn}); err == nil || !strings.Contains(err.Error(), "shared pack, not one of yours") {
		t.Fatalf("%v", err)
	}
	for _, name := range []string{"../x", "a b"} {
		if _, err := (Shipper{Env: e}).Lay(ctx, reg.URL, dir, build.ShipSource{Name: name, Kind: build.ShipOwn}); err == nil {
			t.Errorf("%q was laid", name)
		}
	}
	if _, err := (Shipper{Env: e}).Lay(ctx, reg.URL, dir, build.ShipSource{Name: "x", Kind: "other"}); err == nil {
		t.Error("an unknown kind was laid")
	}
}

// A draft ships the packs it read, whole, each with the origin it claims.
func TestADraftsShipsAreTheWholePacksWithTheirOrigins(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	keep(t, e, reg.URL, "handbook")
	keep(t, e, reg.URL, "notes")
	if _, err := e.Upload(ctx, reg.URL, "handbook"); err != nil {
		t.Fatal(err)
	}
	d, err := e.ForDraft(ctx, reg.URL, []string{"handbook", "notes"})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Done()
	ins, err := d.ShipIns(e, reg.URL, []string{"handbook", "notes"})
	if err != nil || len(ins) != 2 {
		t.Fatalf("%+v, %v", ins, err)
	}
	if ins[0].Origin == nil || ins[0].Origin.Kind != ship.KindOwn || ins[1].Origin != nil {
		t.Fatalf("origins %+v %+v", ins[0].Origin, ins[1].Origin)
	}
	if _, err := d.ShipIns(e, reg.URL, []string{"missing"}); err == nil || !strings.Contains(err.Error(), "not among the packs it reads") {
		t.Fatalf("%v", err)
	}
	if none, err := d.ShipIns(e, reg.URL, nil); err != nil || none != nil {
		t.Fatalf("%v %v", none, err)
	}
	if _, err := d.ShipIns(e, reg.URL, slices.Repeat([]string{"handbook"}, 6)); err == nil {
		t.Fatal("six ships")
	}
}
