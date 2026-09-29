package build

import (
	"reflect"
	"strings"
	"testing"
)

func drafted() *Model {
	m := Default("demo1")
	m.Name, m.Version, m.License = "csv-quality-check", "1.0.0", "Apache-2.0"
	m.Summary = "Checks a CSV file for empty rows."
	m.InstructionFile = "INSTRUCTIONS.md"
	m.PublishInclude = []string{"agent.yaml", "INSTRUCTIONS.md", "README.md", "LICENSE", "extra/**"}
	return m
}

// What does not depend on the task is the Builder's (plan
// builder-simplify.md §4.2, D10): a first release's version, the licence
// whose text Save writes unless the person named another, AGENT.md, the
// include list Save writes, and a name none of the person's own QuickAgents
// has. Each change is named.
func TestTheBuilderSetsWhatDoesNotDependOnTheTask(t *testing.T) {
	m := drafted()
	notes := SetByBuilder(m, "Check a CSV file for empty rows.", []string{"csv-quality-check", "csv-quality-check-2"})
	if m.Version != BuilderVersion || m.License != BuilderLicense || m.InstructionFile != "AGENT.md" || len(m.PublishInclude) != 0 {
		t.Errorf("model = version %q licence %q file %q include %v", m.Version, m.License, m.InstructionFile, m.PublishInclude)
	}
	if m.Name != "csv-quality-check-3" {
		t.Errorf("name = %q", m.Name)
	}
	want := []string{
		`The draft's version "1.0.0" is 0.1.0 here, where a first release starts.`,
		`The draft's licence "Apache-2.0" is MIT here, because your description does not name it.`,
		`The draft named its instruction file "INSTRUCTIONS.md"; the Builder writes the instructions as AGENT.md.`,
		`The draft's include list is the Builder's own here: agent.yaml, AGENT.md, README.md and LICENSE.`,
		`You already have a QuickAgent named "csv-quality-check", so this draft is named "csv-quality-check-3".`,
	}
	if !reflect.DeepEqual(notes, want) {
		t.Errorf("notes =\n%s\nwant\n%s", strings.Join(notes, "\n"), strings.Join(want, "\n"))
	}
	if c := m.Check(); c.First != nil {
		t.Errorf("the Builder's settings fail the form's rules: %+v", c.First)
	}
}

// A draft that already has the Builder's values gets no note.
func TestTheBuilderNamesNothingItDidNotChange(t *testing.T) {
	m := Default("demo1")
	m.Name = "fresh-name"
	if notes := SetByBuilder(m, "anything", []string{"other"}); len(notes) != 0 {
		t.Errorf("notes = %q", notes)
	}
}

// The person's own words keep the draft's licence when they name it as a
// word of its own, case folded, a space for the hyphen: "Apache 2.0" names
// Apache-2.0, and "Apache-2.01" or "MIT-0" do not name it.
func TestTheLicenceStaysWhenThePersonNamesIt(t *testing.T) {
	for words, keep := range map[string]bool{
		"Publish it under the Apache-2.0 licence.": true,
		"apache 2.0, please":                       true,
		"under Apache-2.01":                        false,
		"no licence said":                          false,
		"apache2.0":                                false,
	} {
		m := drafted()
		SetByBuilder(m, words, nil)
		if got := m.License == "Apache-2.0"; got != keep {
			t.Errorf("%q: licence %q, want kept=%v", words, m.License, keep)
		}
	}
}

func TestUniqueNameStaysInTheNameGrammar(t *testing.T) {
	long := strings.Repeat("a", 63) + "-b"
	for _, tc := range []struct {
		name  string
		taken []string
		want  string
	}{
		{"free", []string{"other"}, "free"},
		{"x", []string{"x"}, "x-2"},
		{"x", []string{"x", "x-2", "x-3"}, "x-4"},
		{long, []string{long}, strings.Repeat("a", 63) + "-2"},
		{"end-", nil, "end-"},
	} {
		got := UniqueName(tc.name, tc.taken)
		if got != tc.want {
			t.Errorf("UniqueName(%q, %v) = %q, want %q", tc.name, tc.taken, got, tc.want)
		}
		if len(got) > maxNameChars {
			t.Errorf("%q is past %d characters", got, maxNameChars)
		}
	}
}
