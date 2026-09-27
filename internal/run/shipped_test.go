package run

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry/pkg/manifest"
)

// A release's shipped knowledge packs in a desktop run (agentry-desktop#277,
// the CLI's rule from agentry-cli#389): loaded from the release's files,
// named as the release's, mounted before the person's own.

func shippingWorkspace(t *testing.T, name string) (string, *manifest.Manifest) {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := knowledge.CopyPack(knowledgetest.Make(t, name, nil).Dir, filepath.Join(ws, "knowledge", name)); err != nil {
		t.Fatal(err)
	}
	return ws, &manifest.Manifest{SchemaVersion: "2", Knowledge: &manifest.Knowledge{Ships: []manifest.ShippedKnowledge{{Name: name}}}}
}

func TestShippedKnowledgeInARun(t *testing.T) {
	meta := &installdir.Meta{Digest: "sha256:arch", Source: "registry"}
	t.Run("mounted before the attached, named as the release's", func(t *testing.T) {
		ws, m := shippingWorkspace(t, "aws-docs")
		mine := engine.AttachPack(knowledgetest.Make(t, "notes", nil))
		got, err := withShippedKnowledge(ws, m, meta, []engine.AttachedPack{mine}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Name != "aws-docs" || got[0].Shipped == nil || got[0].Shipped.InWorkspace != "knowledge/aws-docs" ||
			got[0].Shipped.Release != "archive sha256:arch" || got[1].Name != "notes" || got[1].Shipped != nil {
			t.Fatalf("packs = %+v", got)
		}
	})
	t.Run("a local package names its tree", func(t *testing.T) {
		ws, m := shippingWorkspace(t, "aws-docs")
		got, err := withShippedKnowledge(ws, m, &installdir.Meta{Digest: "sha256:tree", Source: "local"}, nil, io.Discard)
		if err != nil || got[0].Shipped.Release != "local package tree sha256:tree" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("a shipped pack changed after its manifest", func(t *testing.T) {
		ws, m := shippingWorkspace(t, "aws-docs")
		if err := os.WriteFile(filepath.Join(ws, "knowledge", "aws-docs", "sources", "guide.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := withShippedKnowledge(ws, m, meta, nil, io.Discard)
		var ce *engine.ClassifiedError
		if !errors.As(err, &ce) || ce.Code != engine.ExitVerify {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("an attached pack cannot take a shipped pack's name", func(t *testing.T) {
		ws, m := shippingWorkspace(t, "aws-docs")
		mine := engine.AttachPack(knowledgetest.Make(t, "aws-docs", nil))
		_, err := withShippedKnowledge(ws, m, meta, []engine.AttachedPack{mine}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "has the name of a pack the release ships") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a release that ships none is unchanged", func(t *testing.T) {
		mine := []engine.AttachedPack{engine.AttachPack(knowledgetest.Make(t, "notes", nil))}
		got, err := withShippedKnowledge(t.TempDir(), &manifest.Manifest{SchemaVersion: "2"}, meta, mine, io.Discard)
		if err != nil || len(got) != 1 || got[0].Name != "notes" {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	if got := capabilityLines(manifest.Capabilities{Knowledge: []string{"aws-docs"}}); len(got) == 0 || got[len(got)-1] != "knowledge: ships aws-docs" {
		t.Fatalf("capability lines = %v", got)
	}
}
