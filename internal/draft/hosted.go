package draft

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry-cli/pkg/drafter"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// A draft In Agentry (plan builder-rework.md §7.5, D15, D16; #281): the
// sandbox host's output.tar is unpacked into the run's folder as any run
// In Agentry's is, and the draft.json at its root is read here as data.
// That file was written on a sandbox host by a model that read the web, so
// it is held to bounds before any of it reaches the page, and it is never
// opened, launched or written anywhere: the form's Model comes from it
// through build.FromDraft, and a QuickAgent folder gets it only when the
// person saves into one they pick natively.

// The bounds a draft.json is held to. A draft over one is refused whole
// rather than cut, since a cut value would read as the Drafter's.
const (
	// MaxDraftJSONBytes is the file: a manifest and instructions of 1 MiB
	// each (the form's own limits), the notes, and the lists.
	MaxDraftJSONBytes = 4 << 20
	maxDraftNotes     = 1 << 20
	maxDraftList      = 100
	maxDraftValue     = 4096
	maxDraftURL       = 2048
	maxDraftReason    = 1000
	maxDraftShort     = 300
	maxManifestError  = 16 << 10
)

// draftJSONFields are the top-level fields of pkg/drafter's Draft this app
// reads; any other is dropped and named.
var draftJSONFields = map[string]bool{
	"manifest": true, "instructions": true, "notes": true, "matches": true, "sources": true, "openQuestions": true,
	"drafter": true, "aiReview": true, "validation": true, "manifestError": true,
}

// ReadHosted reads a hosted draft's draft.json from the run folder its
// output.tar was unpacked into, decodes it, and decodes its manifest into
// the Build form's Model under namespace. It writes nothing.
func ReadHosted(outDir, namespace string) (*Result, error) {
	raw, err := readDraftFile(filepath.Join(outDir, "draft.json"))
	if err != nil {
		return nil, err
	}
	d, model, notes, err := Decode(raw, namespace)
	if err != nil {
		return nil, fmt.Errorf("the draft could not be opened in the form (its files are in %s): %w", outDir, err)
	}
	return &Result{Draft: d, Model: model, Notes: notes, OutDir: outDir}, nil
}

// readDraftFile reads draft.json: a regular file, not a link, at most
// MaxDraftJSONBytes.
func readDraftFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("the draft's output holds no draft.json, so there is no draft to open")
	}
	if err != nil {
		return nil, fmt.Errorf("read the draft: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("the draft's draft.json is not a file, so it was not read")
	}
	if info.Size() > MaxDraftJSONBytes {
		return nil, fmt.Errorf("the draft's draft.json is %d bytes; the Build surface reads at most %d", info.Size(), MaxDraftJSONBytes)
	}
	f, err := os.Open(path) //nolint:gosec // the run folder this app unpacked the draft's output into
	if err != nil {
		return nil, fmt.Errorf("read the draft: %w", err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, MaxDraftJSONBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the draft: %w", err)
	}
	if len(raw) > MaxDraftJSONBytes {
		return nil, fmt.Errorf("the draft's draft.json is over %d bytes; the Build surface reads at most that", MaxDraftJSONBytes)
	}
	return raw, nil
}

// Decode reads an agent-written draft.json as data: known fields only
// (each other one named in the notes), every value of the shape
// pkg/drafter writes and within its bound, or the draft is refused; then
// its manifest and instructions into the form's Model (build.FromDraft,
// whose notes follow). It opens, launches, reads and writes nothing.
func Decode(raw []byte, namespace string) (*drafter.Draft, *build.Model, []string, error) {
	if len(raw) > MaxDraftJSONBytes {
		return nil, nil, nil, fmt.Errorf("draft.json is %d bytes; the Build surface reads at most %d", len(raw), MaxDraftJSONBytes)
	}
	if !utf8.Valid(raw) {
		return nil, nil, nil, errors.New("draft.json is not text (UTF-8)")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, nil, nil, fmt.Errorf("draft.json is not a JSON object: %w", err)
	}
	var notes []string
	var dropped []string
	for k := range top {
		if !draftJSONFields[k] {
			dropped = append(dropped, k)
		}
	}
	if len(dropped) > 0 {
		sort.Strings(dropped)
		notes = append(notes, "The draft carried fields this app does not read, so they were left out: "+strings.Join(dropped, ", ")+".")
	}
	var d drafter.Draft
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, nil, nil, fmt.Errorf("draft.json does not have the shape of a draft: %w", err)
	}
	if len(d.Manifest) == 0 || string(d.Manifest) == "null" {
		return nil, nil, nil, errors.New("draft.json holds no manifest")
	}
	if err := holdDraft(&d); err != nil {
		return nil, nil, nil, err
	}
	model, more, err := build.FromDraft(d.Manifest, d.Instructions, namespace)
	if err != nil {
		return nil, nil, nil, err
	}
	return &d, model, append(notes, more...), nil
}

// holdDraft holds the draft's own texts, which the page renders beside
// Review, to their bounds: the notes, each list and each value in it.
func holdDraft(d *drafter.Draft) error {
	if len(d.Notes) > maxDraftNotes {
		return fmt.Errorf("the draft's notes are %d bytes; the Build surface reads at most %d", len(d.Notes), maxDraftNotes)
	}
	if len(d.ManifestError) > maxManifestError {
		return fmt.Errorf("the draft's manifestError is %d bytes; the Build surface reads at most %d", len(d.ManifestError), maxManifestError)
	}
	list := func(name string, n int) error {
		if n > maxDraftList {
			return fmt.Errorf("the draft lists %d %s; the Build surface reads at most %d", n, name, maxDraftList)
		}
		return nil
	}
	value := func(name, v string, max int) error {
		if len(v) > max {
			return fmt.Errorf("a value in the draft's %s is %d bytes; the Build surface reads at most %d", name, len(v), max)
		}
		return nil
	}
	if err := list("matches", len(d.Matches)); err != nil {
		return err
	}
	for _, m := range d.Matches {
		if err := errors.Join(value("matches", m.Ref, maxDraftShort), value("matches", m.Why, maxDraftValue)); err != nil {
			return err
		}
	}
	if err := list("sources", len(d.Sources)); err != nil {
		return err
	}
	for _, s := range d.Sources {
		if err := errors.Join(value("sources", s.URL, maxDraftURL), value("sources", s.Used, maxDraftValue)); err != nil {
			return err
		}
	}
	if err := list("open questions", len(d.OpenQuestions)); err != nil {
		return err
	}
	for _, q := range d.OpenQuestions {
		if err := value("openQuestions", q, maxDraftValue); err != nil {
			return err
		}
	}
	if m := d.Drafter; m != nil {
		if err := errors.Join(value("drafter", m.Ref, maxDraftShort), value("drafter", m.Build, maxDraftShort),
			value("drafter", m.Model, maxDraftShort), value("drafter", m.RunID, maxDraftShort)); err != nil {
			return err
		}
	}
	if a := d.AIReview; a != nil {
		if err := list("withheld sources", len(a.WithheldSources)); err != nil {
			return err
		}
		for _, w := range a.WithheldSources {
			if err := errors.Join(value("aiReview", w.Tool, maxDraftShort), value("aiReview", w.URL, maxDraftURL),
				value("aiReview", w.Reason, maxDraftReason)); err != nil {
				return err
			}
		}
		if a.Read < 0 || a.Passed < 0 || a.Withheld < 0 || a.NotScanned < 0 {
			return errors.New("the draft's AI Review counts are not counts")
		}
	}
	return nil
}
