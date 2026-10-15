package build

import (
	"strconv"
	"strings"
)

// FromRevision reads a revision the Drafter wrote of a release (Edit with
// AI) into the form as the release itself is read for Edit (FromRelease):
// the QuickAgent's own namespace and name, the version given (the next minor), and without a
// word what the site's Builder cannot hold. The licence is the release's,
// unless the person's own words name the one the draft declares, as the
// site's revision import keeps it (agentry-web lib/draft-import.ts).
func FromRevision(manifestJSON []byte, instructions, namespace, name, version, releaseLicense, words string) (*Model, error) {
	m, err := FromRelease(manifestJSON, instructions, namespace, name, version)
	if err != nil {
		return nil, err
	}
	release := strings.TrimSpace(releaseLicense)
	drafted := strings.TrimSpace(m.License)
	if release != "" && !strings.EqualFold(drafted, release) && (drafted == "" || !namesLicence(words, drafted)) {
		m.License = release
	}
	return m, nil
}

// DiffLine is one line of a diff: Op is ' ' (in both), '-' (only in the
// release), '+' (only in the revision), or '~' for a fold of unchanged
// lines, whose Text says how many.
type DiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// DiffFile is one file's diff, as the revision view draws it.
type DiffFile struct {
	Path  string     `json:"path"`
	Lines []DiffLine `json:"lines"`
	// Same says the file did not change.
	Same bool `json:"same"`
}

// diffContext is how many unchanged lines stand beside a change; longer
// runs fold.
const diffContext = 3

// maxDiffCells bounds the line diff's table; past it the diff is the whole
// release removed and the whole revision added, as the site's is.
const maxDiffCells = 4_000_000

// LineDiff is a line diff of a against b, the site's (agentry-web
// lib/revision.ts lineDiff and fold): the common head and tail trimmed, a
// longest-common-subsequence diff of what is left, and unchanged runs
// folded to diffContext lines either side of a change.
func LineDiff(path, a, b string) DiffFile {
	al, bl := splitLines(a), splitLines(b)
	f := DiffFile{Path: path}
	if strings.Join(al, "\n") == strings.Join(bl, "\n") {
		f.Same = true
		f.Lines = []DiffLine{}
		return f
	}
	head := 0
	for head < len(al) && head < len(bl) && al[head] == bl[head] {
		head++
	}
	tail := 0
	for tail < len(al)-head && tail < len(bl)-head && al[len(al)-1-tail] == bl[len(bl)-1-tail] {
		tail++
	}
	var out []DiffLine
	for _, l := range al[:head] {
		out = append(out, DiffLine{Op: " ", Text: l})
	}
	out = append(out, lcsDiff(al[head:len(al)-tail], bl[head:len(bl)-tail])...)
	for _, l := range al[len(al)-tail:] {
		out = append(out, DiffLine{Op: " ", Text: l})
	}
	f.Lines = fold(out)
	return f
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

func lcsDiff(a, b []string) []DiffLine {
	if len(a)*len(b) > maxDiffCells {
		out := make([]DiffLine, 0, len(a)+len(b))
		for _, l := range a {
			out = append(out, DiffLine{Op: "-", Text: l})
		}
		for _, l := range b {
			out = append(out, DiffLine{Op: "+", Text: l})
		}
		return out
	}
	// t[i][j] is the LCS length of a[i:] and b[j:].
	t := make([][]int32, len(a)+1)
	for i := range t {
		t[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				t[i][j] = t[i+1][j+1] + 1
			case t[i+1][j] >= t[i][j+1]:
				t[i][j] = t[i+1][j]
			default:
				t[i][j] = t[i][j+1]
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, DiffLine{Op: " ", Text: a[i]})
			i, j = i+1, j+1
		case t[i+1][j] >= t[i][j+1]:
			out = append(out, DiffLine{Op: "-", Text: a[i]})
			i++
		default:
			out = append(out, DiffLine{Op: "+", Text: b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, DiffLine{Op: "-", Text: a[i]})
	}
	for ; j < len(b); j++ {
		out = append(out, DiffLine{Op: "+", Text: b[j]})
	}
	return out
}

// fold keeps diffContext unchanged lines beside each change and replaces
// a longer unchanged run with one '~' line saying how many it hides.
func fold(lines []DiffLine) []DiffLine {
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.Op == " " {
			continue
		}
		for k := max(0, i-diffContext); k <= min(len(lines)-1, i+diffContext); k++ {
			keep[k] = true
		}
	}
	out := []DiffLine{}
	for i := 0; i < len(lines); {
		if keep[i] {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && !keep[j] {
			j++
		}
		n := j - i
		word := "lines"
		if n == 1 {
			word = "line"
		}
		out = append(out, DiffLine{Op: "~", Text: strconv.Itoa(n) + " unchanged " + word})
		i = j
	}
	return out
}

// ManifestYAML is the agent.yaml the form writes for m, the revision view's
// diff of both sides.
func (m *Model) ManifestYAML() string { return m.manifestYAML() }
