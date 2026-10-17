package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FromRelease reads a QuickAgent's published release into the form for
// Edit, as the site's Edit does:
//   - The namespace and name are the QuickAgent's, never the manifest's, so
//     Submit adds a version to it. The version is the one given, the next
//     minor (NextMinor), which the person can change.
//   - The release's licence is kept; a release with none gets the form's.
//   - What the site's Builder cannot hold is dropped without a word
//     (they are regenerated for a new version): homepage, authors, the
//     idle timeout, knowledge.ships, runtime (back to the form's own),
//     dependencies and publish.channel and include. The instruction file is written as AGENT.md, whatever
//     the release named it (a v1 skill's SKILL.md included). Fields the
//     schema does not know never reach the form.
//
// A release is registry-validated data, read as text into fields the form
// already has; nothing is opened, run or written here.
func FromRelease(manifestJSON []byte, instructions, namespace, name, version string) (*Model, error) {
	if len(manifestJSON) > maxManifestBytes {
		return nil, fmt.Errorf("the release's manifest is %d bytes; the Build surface reads at most %d", len(manifestJSON), maxManifestBytes)
	}
	if len(instructions) > maxInstructionBytes {
		return nil, fmt.Errorf("the release's instructions are %d bytes; the Build surface reads at most %d", len(instructions), maxInstructionBytes)
	}
	if !utf8.ValidString(instructions) {
		return nil, errors.New("the release's instructions are not text (UTF-8)")
	}
	var obj map[string]any
	if err := json.Unmarshal(manifestJSON, &obj); err != nil || obj == nil {
		return nil, errors.New("the release's manifest could not be read")
	}
	m, err := decodeManifest(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("the release's manifest does not fit the form: %w", err)
	}
	fresh := Default(namespace)
	m.Namespace, m.Name, m.Version = namespace, name, version
	if strings.TrimSpace(m.License) == "" {
		m.License = fresh.License
	}
	m.Homepage = ""
	m.Authors = fresh.Authors
	m.IdleTimeoutSeconds = 0
	m.KnowledgeShips = nil
	m.RuntimeModels, m.RuntimeInterfaces = fresh.RuntimeModels, fresh.RuntimeInterfaces
	m.Dependencies = fresh.Dependencies
	m.PublishChannel, m.PublishInclude = "", fresh.PublishInclude
	m.InstructionFile = defaultInstructionFile
	m.Instructions = instructions
	m.HadComments = false
	return m, nil
}

// ReleaseInstructionPath is the file a release's instructions are in: its
// instruction entrypoint, a v1 manifest's skill entrypoint, or AGENT.md, as
// the site reads it.
func ReleaseInstructionPath(manifestJSON []byte) string {
	var doc struct {
		Entrypoints struct {
			Instruction string `json:"instruction"`
			Skill       string `json:"skill"`
		} `json:"entrypoints"`
	}
	if json.Unmarshal(manifestJSON, &doc) == nil {
		for _, p := range []string{doc.Entrypoints.Instruction, doc.Entrypoints.Skill} {
			if strings.TrimSpace(p) != "" {
				return p
			}
		}
	}
	return defaultInstructionFile
}

var versionCoreRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:[-+].*)?$`)

// NextMinor is the minor after the highest version a QuickAgent has, in any
// state: a version still in review holds its number, so an edit that reused
// it would be refused at upload. A pre-release counts by its core.
// With no version it is the form's first.
func NextMinor(versions []string) string {
	found := false
	var major, minor uint64
	for _, v := range versions {
		g := versionCoreRE.FindStringSubmatch(v)
		if g == nil {
			continue
		}
		ma, err1 := strconv.ParseUint(g[1], 10, 64)
		mi, err2 := strconv.ParseUint(g[2], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if !found || ma > major || (ma == major && mi > minor) {
			found, major, minor = true, ma, mi
		}
	}
	if !found {
		return Default("").Version
	}
	return fmt.Sprintf("%d.%d.0", major, minor+1)
}
