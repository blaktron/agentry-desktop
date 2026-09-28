package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// A pack a build left short can hold its index and no page (the apply step
// dropped what it wrote). Its preview lists no page as an empty list, which
// the frontend can map over, never as null (agentry-desktop#351).
func TestPreviewOfAPackWithNoPageListsNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wiki"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wiki", "index.md"), []byte("# short\n\n0 pages.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wiki", "log.md"), []byte("## stopped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	pages, err := listPages(root)
	if err != nil {
		t.Fatal(err)
	}
	if pages == nil || len(pages) != 0 {
		t.Fatalf("pages = %#v, want an empty, non-nil list", pages)
	}
	raw, err := json.Marshal(Preview{Name: "short", Pages: pages})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"pages":[]`) {
		t.Fatalf("preview encodes as %s, want \"pages\":[]", raw)
	}
}

// The same through the store: a build that stopped before any page it wrote
// was taken leaves a pack of its index alone, kept sealed, and the panel's
// preview of it lists no page as an empty list (agentry-desktop#351).
func TestAStoppedBuildsPackWithNoPagePreviews(t *testing.T) {
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	t.Setenv(policy.HomeEnv, homeOf(t, e))
	short := func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		if err := os.CopyFS(o.PackDir, os.DirFS(knowledgetest.Make(t, o.Name, map[string]string{}).Dir)); err != nil {
			return nil, err
		}
		p, err := kn.Load(o.PackDir)
		if err != nil {
			return nil, err
		}
		return &build.Result{
			Documents: []string{"guide.txt"},
			Applied:   &kn.Result{Manifest: p.Manifest, Dropped: []kn.Drop{{Path: "wiki/topics/half.md", Reason: "the page has no frontmatter"}}},
			RunErr:    errors.New("run timed out after 60s"),
		}, nil
	}
	res, err := e.Create(ctx, BuildRequest{Registry: reg.URL, Name: "short", Documents: documents(t), Build: short})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 0 || res.Stopped == "" {
		t.Fatalf("result = %+v, want a stopped build of no page", res)
	}
	pv, err := e.Preview(ctx, reg.URL, "short")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(pv)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Pages == nil || len(pv.Pages) != 0 || pv.Index.Path == "" || !strings.Contains(string(raw), `"pages":[]`) {
		t.Fatalf("preview = %s", raw)
	}
}
