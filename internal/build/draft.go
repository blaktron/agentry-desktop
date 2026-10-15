package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/manifest"
)

// FromDraft decodes a draft into the form's Model: the Drafter's manifest,
// which it writes as JSON, and its instructions. It is data to the form and
// nothing else. Nothing is opened, launched, read or written; the draft
// reaches a QuickAgent folder only when the person saves it into one they
// pick natively. So:
//   - Known fields only. What the form cannot hold is dropped, and each
//     dropped field is named in the notes.
//   - A value of the wrong shape (an object where a name belongs), a key
//     named twice, a manifest or instructions over the form's limits, or
//     instructions that are not text refuse the draft.
//   - A path is text. An instruction file that is not one Markdown file at
//     the folder's root, other than the README Save writes, becomes
//     AGENT.md (the CLI's rendering holds a draft to the same rule), and the
//     notes say so: a name the draft chose must not pick which file a save
//     writes the instructions over. Paths in the contract are shown as
//     written, for the form's rules and the person to judge.
//
// The draft is published under the person's namespace: a non-empty
// namespace replaces the Drafter's guess, and the notes say so. The notes
// also say when the manifest as written does not parse under the core's
// rules, since the form shows only what it could read.
func FromDraft(manifestJSON []byte, instructions, namespace string) (*Model, []string, error) {
	if len(manifestJSON) > maxManifestBytes {
		return nil, nil, fmt.Errorf("the draft's manifest is %d bytes; the Build surface reads at most %d", len(manifestJSON), maxManifestBytes)
	}
	if len(instructions) > maxInstructionBytes {
		return nil, nil, fmt.Errorf("the draft's instructions are %d bytes; the Build surface reads at most %d", len(instructions), maxInstructionBytes)
	}
	if !utf8.ValidString(instructions) {
		return nil, nil, errors.New("the draft's instructions are not text (UTF-8)")
	}
	var loose any
	if err := json.Unmarshal(manifestJSON, &loose); err != nil {
		return nil, nil, fmt.Errorf("the draft's manifest is not JSON: %w", err)
	}
	obj, ok := loose.(map[string]any)
	if !ok {
		return nil, nil, errors.New("the draft's manifest is not an object")
	}
	m, err := decodeManifest(manifestJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("the draft's manifest does not fit the form: %w", err)
	}
	var notes []string
	dropped := unknownFields(obj, draftFields, "")
	// A draft never ships the person's pack: its knowledge.ships is dropped
	// and named, as the site's importer does, and its knowledge.accepts is
	// kept.
	if len(m.KnowledgeShips) > 0 {
		dropped = append(dropped, "knowledge.ships")
		m.KnowledgeShips = nil
	}
	if len(dropped) > 0 {
		sort.Strings(dropped)
		notes = append(notes, "The form cannot hold these fields of the draft, so they were left out: "+strings.Join(dropped, ", ")+".")
	}
	if f := m.InstructionFile; f != "" && (!draftInstructionRE.MatchString(f) || strings.EqualFold(f, "README.md")) {
		notes = append(notes, fmt.Sprintf("The draft named its instruction file %q, which is not one file in the QuickAgent's folder; here it is %s.", f, defaultInstructionFile))
		m.InstructionFile = defaultInstructionFile
	}
	if ns := strings.TrimSpace(namespace); ns != "" && ns != m.Namespace {
		if m.Namespace != "" {
			notes = append(notes, fmt.Sprintf("The draft's namespace %q is replaced with yours, %s.", m.Namespace, ns))
		}
		m.Namespace = ns
	}
	if _, perr := manifest.Parse(manifestJSON); perr != nil {
		notes = append(notes, "As the Drafter wrote it, the manifest does not pass the manifest rules: "+perr.Error()+". The form shows what it could read; its checks say what is left to fix.")
	}
	m.Instructions = instructions
	m.HadComments = false
	return m, notes, nil
}

// draftInstructionRE is an instruction file a draft may name: one Markdown
// file at the folder's root, the rule pkg/drafter's Render holds a draft to.
var draftInstructionRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}\.md$`)

// draftFields is schema v2 as the form holds it, for naming what a draft
// carries beyond it: a map is an object whose keys are known, a one-element
// slice is a list of such objects, nil is a value the form reads whole (a
// scalar, a list of scalars, or a map of names such as guide.connect).
// capabilities.filesystem.read and write, and capabilities.exec, are also
// read as a word ("none", "workspace"), which the walk passes by.
var draftFields = map[string]any{
	"schemaVersion": nil, "name": nil, "namespace": nil, "kind": nil, "modes": nil, "version": nil,
	"summary": nil, "license": nil, "homepage": nil, "tags": nil,
	"authors":     []any{map[string]any{"name": nil, "email": nil}},
	"entrypoints": map[string]any{"instruction": nil, "skill": nil},
	"intake":      []any{map[string]any{"id": nil, "prompt": nil, "type": nil, "required": nil, "choices": nil}},
	"inputs":      map[string]any{"kinds": nil, "view": nil},
	"interactive": map[string]any{"opening": nil, "idleTimeoutSeconds": nil},
	"guide":       map[string]any{"overview": nil, "prepare": nil, "files": nil, "task": nil, "connect": nil},
	"runtime":     map[string]any{"models": nil, "interfaces": nil},
	"capabilities": map[string]any{
		"network":    map[string]any{"egress": nil},
		"filesystem": map[string]any{"read": map[string]any{"paths": nil}, "write": map[string]any{"paths": nil}},
		"exec":       map[string]any{"commands": nil},
		"secrets":    nil, "optionalSecrets": nil, "dataAccess": nil,
		"mcpServers": []any{map[string]any{
			"npm": nil, "pypi": nil, "url": nil, "auth": nil, "scopes": nil, "version": nil, "tools": nil, "secrets": nil, "egress": nil, "env": nil,
			"oauth": map[string]any{"issuer": nil, "scopes": nil, "files": []any{map[string]any{"path": nil, "template": nil}}, "env": nil},
		}},
	},
	"knowledge":    map[string]any{"accepts": map[string]any{"prompt": nil, "required": nil}, "ships": nil},
	"dependencies": map[string]any{"agents": []any{map[string]any{"ref": nil, "version": nil}}},
	"publish":      map[string]any{"include": nil, "channel": nil},
}

// unknownFields names, by path, every key of v that known does not hold.
func unknownFields(v map[string]any, known map[string]any, at string) []string {
	var out []string
	for k, val := range v {
		path := k
		if at != "" {
			path = at + "." + k
		}
		schema, ok := known[k]
		if !ok {
			out = append(out, path)
			continue
		}
		switch sch := schema.(type) {
		case map[string]any:
			if obj, isObj := val.(map[string]any); isObj {
				out = append(out, unknownFields(obj, sch, path)...)
			}
		case []any:
			list, isList := val.([]any)
			elem, _ := sch[0].(map[string]any)
			if !isList || elem == nil {
				continue
			}
			for i, item := range list {
				if obj, isObj := item.(map[string]any); isObj {
					out = append(out, unknownFields(obj, elem, fmt.Sprintf("%s[%d]", path, i))...)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}
