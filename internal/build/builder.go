package build

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// What the Builder sets on a draft, whatever the draft said: a first
// release's version, and the one licence whose full text Save writes
// (MIT, as on the site).
const (
	BuilderVersion = "0.1.0"
	BuilderLicense = "MIT"
)

// maxNameChars is the longest name the core's V1 grammar takes.
const maxNameChars = 65

// SetByBuilder writes what does not depend on the task onto a drafted
// model, as the site's import does: the version, the licence unless the
// person's own words name the draft's, the instruction file, the include
// list Save writes, and a name none of the person's own QuickAgents
// (taken) already has. The namespace is FromDraft's. The Drafter decides
// the rest (the name itself, tags, modes, data access and the contract).
// It returns one note per change, for the draft's notes.
func SetByBuilder(m *Model, words string, taken []string) []string {
	var notes []string
	if v := strings.TrimSpace(m.Version); v != BuilderVersion {
		if v != "" {
			notes = append(notes, fmt.Sprintf("The draft's version %q is %s here, where a first release starts.", v, BuilderVersion))
		}
		m.Version = BuilderVersion
	}
	if l := strings.TrimSpace(m.License); l != BuilderLicense && (l == "" || !namesLicence(words, l)) {
		if l != "" {
			notes = append(notes, fmt.Sprintf("The draft's licence %q is %s here, because your description does not name it.", l, BuilderLicense))
		}
		m.License = BuilderLicense
	}
	if f := m.InstructionFile; f != defaultInstructionFile {
		if f != "" {
			notes = append(notes, fmt.Sprintf("The draft named its instruction file %q; the Builder writes the instructions as %s.", f, defaultInstructionFile))
		}
		m.InstructionFile = defaultInstructionFile
	}
	if inc := compact(m.PublishInclude); len(inc) > 0 {
		if own := m.defaultInclude(); !slices.Equal(inc, own) {
			notes = append(notes, "The draft's include list is the Builder's own here: "+strings.Join(own[:len(own)-1], ", ")+" and "+own[len(own)-1]+".")
		}
		m.PublishInclude = []string{}
	}
	if name := UniqueName(m.Name, taken); name != m.Name {
		notes = append(notes, fmt.Sprintf("You already have a QuickAgent named %q, so this draft is named %q.", m.Name, name))
		m.Name = name
	}
	return notes
}

// defaultInclude is the include list Save's files make: the manifest, the
// instruction file, and the README and LICENSE Save writes.
func (m *Model) defaultInclude() []string {
	return []string{"agent.yaml", m.instructionFile(), "README.md", "LICENSE"}
}

// UniqueName is name, or name with the first free -2, -3… when one of
// taken already has it, cut so it stays within the name grammar's length.
func UniqueName(name string, taken []string) string {
	if !slices.Contains(taken, name) {
		return name
	}
	for n := 2; n <= len(taken)+2; n++ {
		tail := fmt.Sprintf("-%d", n)
		head := name
		if len(head) > maxNameChars-len(tail) {
			head = head[:maxNameChars-len(tail)]
		}
		candidate := strings.TrimRight(head, "-") + tail
		if !slices.Contains(taken, candidate) {
			return candidate
		}
	}
	return name
}

// namesLicence says whether the person's words name an SPDX id as a word
// of its own, case folded, with a space allowed where the id has a hyphen:
// "under the Apache-2.0 licence" and "Apache 2.0" both name Apache-2.0.
// The site's lib/draft-import.ts namesLicence, the same rule.
func namesLicence(words, id string) bool {
	pattern := strings.ReplaceAll(regexp.QuoteMeta(strings.ToLower(id)), "-", "[- ]")
	re, err := regexp.Compile(`(^|[^a-z0-9.+-])` + pattern + `($|[^a-z0-9+-])`)
	return err == nil && re.MatchString(strings.ToLower(words))
}
