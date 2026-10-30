package highlights

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheCompiledFileKeepsEveryRule(t *testing.T) {
	entries, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("the compiled file holds no version")
	}
}

// The version the next release carries is wails.json's productVersion, so
// the change that moves it adds that version's lines.
func TestTheProductVersionHasItsHighlights(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "wails.json"))
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	entries, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := For(entries, w.Info.ProductVersion); !ok {
		t.Fatalf("highlights.json has no entry for productVersion %s: add one to four lines saying what this version brings", w.Info.ProductVersion)
	}
}

func TestALineIsOnePlainSentence(t *testing.T) {
	good := []string{
		"Meeting notes now tell speakers apart when everyone shares one microphone.",
		"Can a run keep its place while you scroll back?",
		"The Run log keeps up with a busy agent!",
		"A persistent agent's page has a View menu with five views: Terminal, Compact and three more.",
		strings.Repeat("a", MaxLineLength-1) + ".",
	}
	for _, line := range good {
		if err := CheckLine(line); err != nil {
			t.Errorf("refused %q: %v", line, err)
		}
	}
	bad := map[string]string{
		"too long":          strings.Repeat("a", MaxLineLength) + ".",
		"empty":             "",
		"leading space":     " Faster runs.",
		"two lines":         "Faster runs.\nAnd more.",
		"no full stop":      "Faster runs",
		"bold":              "**Faster** runs.",
		"code":              "Run `agentry run` faster.",
		"html":              "Faster <script>alert(1)</script> runs.",
		"link":              "See [the docs](x) for runs.",
		"list item":         "- Faster runs.",
		"numbered item":     "1. Faster runs.",
		"issue reference":   "Faster runs (#123).",
		"register id":       "Faster runs, as DEV-264 decided.",
		"commit prefix":     "feat: faster runs.",
		"scoped prefix":     "fix(run card): a key names no credits.",
		"address":           "Read more at https://example.org/runs.",
		"host name":         "Runs reach dev.secureagentry.net faster.",
		"internal host":     "Built on agentry-runner2 faster.",
		"underscore symbol": "The agent_view setting is kept.",
	}
	for name, line := range bad {
		if err := CheckLine(line); err == nil {
			t.Errorf("%s: accepted %q", name, line)
		}
	}
}

func TestAFileIsNewestFirstEachVersionOnce(t *testing.T) {
	ok := `{"versions":[{"version":"0.6.12","highlights":["A."]},{"version":"0.6.9","highlights":["B.","C.","D.","E."]}]}`
	if _, err := Parse([]byte(ok)); err != nil {
		t.Fatalf("refused a good file: %v", err)
	}
	bad := map[string]string{
		"not json":        `{`,
		"unknown field":   `{"versions":[],"next":[]}`,
		"oldest first":    `{"versions":[{"version":"0.6.9","highlights":["A."]},{"version":"0.6.12","highlights":["B."]}]}`,
		"twice":           `{"versions":[{"version":"0.6.9","highlights":["A."]},{"version":"0.6.9","highlights":["B."]}]}`,
		"a prerelease":    `{"versions":[{"version":"0.6.9-dev.1","highlights":["A."]}]}`,
		"a v":             `{"versions":[{"version":"v0.6.9","highlights":["A."]}]}`,
		"two parts":       `{"versions":[{"version":"0.6","highlights":["A."]}]}`,
		"leading zero":    `{"versions":[{"version":"0.06.9","highlights":["A."]}]}`,
		"no lines":        `{"versions":[{"version":"0.6.9","highlights":[]}]}`,
		"five lines":      `{"versions":[{"version":"0.6.9","highlights":["A.","B.","C.","D.","E."]}]}`,
		"a bad line":      `{"versions":[{"version":"0.6.9","highlights":["fix: a."]}]}`,
		"two json values": `{"versions":[]} {}`,
	}
	for name, f := range bad {
		if _, err := Parse([]byte(f)); err == nil {
			t.Errorf("%s: accepted %s", name, f)
		}
	}
}

func TestVersionNamesTheReleaseABuildBelongsTo(t *testing.T) {
	for in, want := range map[string]string{
		"0.6.12": "0.6.12", "v0.6.12": "0.6.12", "0.6.12-dev.3": "0.6.12", "0.6.12-4-gabc1234": "0.6.12", "0.6.12-dirty": "0.6.12",
	} {
		if got, ok := Version(in); !ok || got != want {
			t.Errorf("Version(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"0.0.0-dev", "", "abc1234", "0.6"} {
		if got, ok := Version(in); ok {
			t.Errorf("Version(%q) = %q, want no release", in, got)
		}
	}
}

func TestCompareReadsTheNumbersNotTheText(t *testing.T) {
	if Compare("0.6.1004", "0.6.999") <= 0 || Compare("0.6.9", "0.6.10") >= 0 || Compare("1.0.0", "0.9.99") <= 0 {
		t.Fatal("versions compared as text")
	}
	if Compare("0.6.9-dev.2", "0.6.9") != 0 {
		t.Fatal("a prerelease is a different version from its X.Y.Z")
	}
}

func TestBetweenIsNewestFirstAndCounted(t *testing.T) {
	var entries []Entry
	for _, v := range []string{"0.6.20", "0.6.19", "0.6.18", "0.6.17", "0.6.16", "0.6.15", "0.6.14", "0.6.13"} {
		entries = append(entries, Entry{Version: v, Highlights: []string{"A."}})
	}
	versions := func(es []Entry) string {
		var s []string
		for _, e := range es {
			s = append(s, e.Version)
		}
		return strings.Join(s, " ")
	}
	cases := []struct {
		after, upTo string
		limit       int
		want        string
		more        int
	}{
		{"0.6.17", "0.6.20", 5, "0.6.20 0.6.19 0.6.18", 0},
		{"0.6.12", "0.6.20", 5, "0.6.20 0.6.19 0.6.18 0.6.17 0.6.16", 3},
		{"0.6.12", "0.6.18", 5, "0.6.18 0.6.17 0.6.16 0.6.15 0.6.14", 1},
		{"0.6.20", "0.6.20", 5, "", 0},
		{"", "0.6.14", 5, "0.6.14 0.6.13", 0},
		{"0.6.17", "0.6.21-dev.1", 5, "0.6.20 0.6.19 0.6.18", 0},
	}
	for _, c := range cases {
		got, more := Between(entries, c.after, c.upTo, c.limit)
		if versions(got) != c.want || more != c.more {
			t.Errorf("Between(after %s, up to %s) = %q +%d; want %q +%d", c.after, c.upTo, versions(got), more, c.want, c.more)
		}
	}
}

// What's new reads only what is compiled in: nothing in this package can
// reach the network.
func TestThePackageReachesNoNetwork(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "net" || strings.HasPrefix(path, "net/") {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
}
