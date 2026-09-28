package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
