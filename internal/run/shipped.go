package run

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry/pkg/manifest"
)

// withShippedKnowledge loads the knowledge packs a release ships from the
// materialized release's own files and puts them before the packs the person
// attached, as the CLI's run does: each is verified with knowledge.Load and
// pinned by digest, and the engine stages it from the run's own copy of the
// workspace and holds it to that digest, so what is mounted is what the archive
// covers. A pack that does not load is a verification failure; an attached pack
// that takes a shipped pack's name is refused, since both would mount at
// /knowledge/<name>. It repeats the CLI's shippedKnowledge, which lives in its
// package main.
func withShippedKnowledge(workspaceDir string, m *manifest.Manifest, meta *installdir.Meta, attached []engine.AttachedPack, logw io.Writer) ([]engine.AttachedPack, error) {
	ships := m.ShippedPacks()
	if len(ships) == 0 {
		return attached, nil
	}
	release := "archive " + meta.Digest
	if meta.Source == "local" {
		release = "local package tree " + meta.Digest
	}
	out := make([]engine.AttachedPack, 0, len(ships)+len(attached))
	for _, name := range ships {
		p, err := knowledge.Load(filepath.Join(workspaceDir, manifest.KnowledgeDir, name))
		if err != nil {
			return nil, engine.Classify(engine.ExitVerify, fmt.Errorf("the release ships the knowledge pack %s, and it does not load from the release's files: %w", name, err))
		}
		if p.Manifest.Name != name {
			return nil, engine.Classify(engine.ExitVerify, fmt.Errorf("the release ships the knowledge pack %s, and its manifest.json names %s", name, p.Manifest.Name))
		}
		// A shipped pack is Agentry's to carry, so Agentry's cap holds, as in the
		// CLI's shippedKnowledge: Load takes a pack of up to this computer's 1 GB.
		if p.Manifest.Bytes > knowledge.MaxBytes {
			return nil, engine.Classify(engine.ExitVerify, fmt.Errorf("the release ships the knowledge pack %s at %d bytes; a shipped pack is at most %s", name, p.Manifest.Bytes, knowledge.FormatCap(knowledge.MaxBytes)))
		}
		a := engine.AttachPack(p)
		a.Shipped = &engine.ShippedFrom{InWorkspace: manifest.KnowledgeDir + "/" + name, Release: release}
		_, _ = fmt.Fprintf(logw, "knowledge pack %s: %s, %d bytes, %s, shipped with the release — mounted read-only at %s/%s\n",
			name, knowledge.CountPages(a.Pages), a.Bytes, a.Digest, spec.KnowledgeMount, name)
		out = append(out, a)
	}
	for _, a := range attached {
		for _, s := range out {
			if s.Name == a.Name {
				return nil, engine.Classify(engine.ExitCLI, fmt.Errorf("the knowledge pack %s you attached has the name of a pack the release ships, mounted at %s/%s; attach a pack with another name", a.Name, spec.KnowledgeMount, a.Name))
			}
		}
	}
	return append(out, attached...), nil
}
