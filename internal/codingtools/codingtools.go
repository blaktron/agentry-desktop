// Package codingtools finds the coding tools on this computer (command-line
// coding agents, their desktop apps, and the editors built around them) and
// installs the Agentry plugin into one with that tool's own command.
//
// Finding a tool reads the file system and nothing else: PATH, then the
// tool's usual install locations, each a template in the grammar below. No
// tool is executed until the person asks for the plugin to be installed, and
// then only the command the catalog names for that tool, as an argv, never
// through a shell.
//
// A location template is a slash-separated path with an optional leading
// "~" (the home directory) and "${NAME}" environment variables; a template
// whose variable is unset or empty is skipped. A template ending in "/*"
// names a directory of version-named builds, of which the newest is the one
// that counts (a desktop app keeps the builds it downloaded side by side).
package codingtools

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Kind is what sort of tool a catalog entry is; the values are shown.
type Kind string

const (
	// KindCLI is a coding agent run in a terminal.
	KindCLI Kind = "command-line tool"
	// KindApp is a desktop app that runs coding agents.
	KindApp Kind = "desktop app"
	// KindEditor is a code editor with a coding agent built in.
	KindEditor Kind = "editor"
)

// Source says where a tool was found; the values are shown.
type Source string

const (
	SourcePath  Source = "PATH"
	SourceUsual Source = "usual location"
)

// PluginState is what the file system says about the Agentry plugin in a
// tool: present, absent, or unknown when the tool keeps its plugins where
// a file check cannot tell (or keeps none of its own).
type PluginState string

const (
	PluginInstalled PluginState = "installed"
	PluginAbsent    PluginState = "not installed"
	PluginUnknown   PluginState = "unknown"
)

// Paths are location templates per operating system ("darwin", "windows",
// "linux"); "*" applies to every system without an entry of its own.
type Paths map[string][]string

func (p Paths) For(goos string) []string {
	if v, ok := p[goos]; ok {
		return v
	}
	return p["*"]
}

// Marker says the plugin is present when its path exists and, if Contains
// is set, the file holds that text.
type Marker struct {
	Path     string
	Contains string
}

// Tool is one catalog entry.
type Tool struct {
	ID   string
	Name string
	Kind Kind
	// Site is the vendor's page, for a tool that is not found.
	Site string
	// Commands are the executable's names looked up on PATH (without
	// ".exe"; on Windows the lookup adds the extensions).
	Commands []string
	// Locations are where the tool usually lives when PATH does not have
	// it: an executable for a command-line tool, the app bundle or its
	// program for an app or an editor.
	Locations Paths
	// Package is the published plugin package the tool installs, the
	// repository blaktron/agentry-plugin-<Package>; "" for a tool that
	// loads another tool's plugins (Uses) or has none.
	Package string
	// Installer is the tool whose command installs the plugin here: ""
	// for the tool itself, or another entry's ID (an app whose plugins
	// are its command-line tool's).
	Installer string
	// Bundled are the locations of a command-line build an app carries,
	// used for the install when the Installer tool itself is not found.
	Bundled Paths
	// Steps are the install's argv, each run with the installer's
	// executable as argv[0], in order; the first failure stops the rest.
	Steps [][]string
	// Repeatable is true when running Steps again over an installed
	// plugin is harmless (the tool answers "already installed" and exits
	// 0); otherwise an installed plugin is not installed again.
	Repeatable bool
	// Markers are the files that exist when the plugin is installed; with
	// none, the plugin's state is unknown.
	Markers Paths
	// MarkerText, when set, must appear in the marker file.
	MarkerText string
	// Manual is how to install the plugin in the tool's own window, for a
	// tool whose plugins are added there; ManualURL opens the page or
	// deep link it names.
	Manual    string
	ManualURL string
	// Uses names the tools whose plugins this one loads, for a tool with
	// no plugin system of its own.
	Uses []string
	// Note is one sentence the list shows under the tool.
	Note string
}

// Repository is the plugin package's public repository.
func (t Tool) Repository() string {
	if t.Package == "" {
		return ""
	}
	return "https://github.com/blaktron/agentry-plugin-" + t.Package
}

// The seams the tests set.
var (
	homeDir  = os.UserHomeDir
	goos     = runtime.GOOS
	lookPath = exec.LookPath
	getenv   = os.Getenv
)

// Found is where a tool was found on this computer.
type Found struct {
	Path   string
	Source Source
}

// Find looks for t: PATH for its commands, then its usual locations. It
// reads the file system and runs nothing.
func Find(t Tool) (Found, bool) {
	for _, c := range t.Commands {
		if p, err := lookPath(c); err == nil {
			return Found{Path: p, Source: SourcePath}, true
		}
	}
	if p, ok := firstExisting(t.Locations.For(goos), t.Kind == KindCLI); ok {
		return Found{Path: p, Source: SourceUsual}, true
	}
	return Found{}, false
}

// firstExisting returns the first template that expands to a path that
// exists: a regular file when file is true (an executable), anything when
// it is false (an app bundle is a directory on macOS).
func firstExisting(templates []string, file bool) (string, bool) {
	for _, tpl := range templates {
		p, ok := expand(tpl)
		if !ok {
			continue
		}
		if strings.HasSuffix(p, string(filepath.Separator)+"*") {
			p, ok = newestVersion(strings.TrimSuffix(p, string(filepath.Separator)+"*"))
			if !ok {
				continue
			}
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if file && !st.Mode().IsRegular() {
			continue
		}
		return p, true
	}
	return "", false
}

var bracedVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expand resolves a template for this computer; false when a variable is
// unset or empty, or the home directory is unknown.
func expand(tpl string) (string, bool) {
	skip := false
	tpl = bracedVar.ReplaceAllStringFunc(tpl, func(m string) string {
		v := getenv(m[2 : len(m)-1])
		if v == "" {
			skip = true
		}
		return filepath.ToSlash(v)
	})
	if skip {
		return "", false
	}
	if tpl == "~" || strings.HasPrefix(tpl, "~/") {
		home, err := homeDir()
		if err != nil || home == "" {
			return "", false
		}
		tpl = filepath.ToSlash(home) + tpl[1:]
	}
	return filepath.FromSlash(tpl), true
}

// newestVersion picks, in a directory of version-named entries, the one
// whose name sorts highest as a dotted version ("2.1.321-c6c6b56" reads as
// 2.1.321).
func newestVersion(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	type build struct {
		name string
		ver  []int
	}
	var builds []build
	for _, e := range entries {
		if v := versionOf(e.Name()); v != nil {
			builds = append(builds, build{e.Name(), v})
		}
	}
	if len(builds) == 0 {
		return "", false
	}
	sort.SliceStable(builds, func(i, j int) bool { return compareVersions(builds[i].ver, builds[j].ver) > 0 })
	return filepath.Join(dir, builds[0].name), true
}

func versionOf(name string) []int {
	name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	var v []int
	for _, part := range strings.Split(name, ".") {
		digits := part
		if i := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
			digits = part[:i]
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			break
		}
		v = append(v, n)
		if len(digits) != len(part) {
			break
		}
	}
	return v
}

func compareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// Plugin reads the plugin's state in t from its marker files.
func Plugin(t Tool) PluginState {
	tpls := t.Markers.For(goos)
	if len(tpls) == 0 {
		return PluginUnknown
	}
	for _, tpl := range tpls {
		p, ok := expand(tpl)
		if !ok {
			continue
		}
		if t.MarkerText == "" {
			if _, err := os.Stat(p); err == nil {
				return PluginInstalled
			}
			continue
		}
		b, err := readSmall(p)
		if err == nil && strings.Contains(string(b), t.MarkerText) {
			return PluginInstalled
		}
	}
	return PluginAbsent
}

// readSmall reads a marker file of at most 4 MiB; a larger one is cut
// there, which a plugin list never reaches.
func readSmall(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, 4<<20))
}
