// Package library is Agentry Desktop's library of presets: popular
// applications whose set-up Agentry knows, so the person can add one to
// Settings → Your computer in one click. A preset holds the application's
// local MCP server, pinned to one exact release, with each tool's class, the
// hosts it is known to reach, the person's steps, the identifiers its
// windows carry on each system, and where it is found installed.
//
// The library sets up; it does not gate. Once the person adds a preset, the
// application is theirs like any other on the list: nothing outside this
// package and the Settings page's "Add from the library" reads a preset's
// id. A tool's class here is the library's, never the server's own tool
// annotations, which a server sets as it likes.
//
// The library is embedded data (library.yaml), released with the app.
package library

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Class is how a tool's call is answered.
type Class string

// The classes, a closed set.
const (
	// ClassRead reads the application's state.
	ClassRead Class = "read"
	// ClassAct changes the application's state or works inside it.
	ClassAct Class = "act"
	// ClassExec runs code or commands the application interprets: each
	// call asks the person first.
	ClassExec Class = "exec"
)

// Classes returns every class, in a fixed order.
func Classes() []Class { return []Class{ClassRead, ClassAct, ClassExec} }

// Ecosystem is the package registry an installed server is published in.
type Ecosystem string

// The ecosystems a preset may pin.
const (
	EcosystemPyPI Ecosystem = "pypi"
	EcosystemNPM  Ecosystem = "npm"
	// EcosystemApplication is a server the application ships itself, as a
	// plugin released with it: its name is the plugin's and its version the
	// application's release that brought it. No package registry holds it,
	// so there is nothing to launch; only a loopback-http preset may name
	// it.
	EcosystemApplication Ecosystem = "application"
)

// Attachment is how Agentry Desktop reaches an installed server.
type Attachment string

// The attachments.
const (
	// AttachmentStdio is a server Agentry Desktop launches, speaking MCP
	// over its standard input and output.
	AttachmentStdio Attachment = "stdio"
	// AttachmentLoopbackHTTP is a server the application runs inside
	// itself on a loopback address, which Agentry Desktop connects to.
	AttachmentLoopbackHTTP Attachment = "loopback-http"
)

// Platforms a preset may be offered on, as Go names them.
var platforms = []string{"linux", "darwin", "windows"}

// Preset is one application of the library and its vetted local MCP
// server.
type Preset struct {
	// ID is the id a release names the application by: lowercase
	// letters, digits and single hyphens.
	ID string `yaml:"id" json:"id"`
	// Application is the application's name, as people know it.
	Application string `yaml:"application" json:"application"`
	// Platforms are the operating systems it is offered on.
	Platforms []string `yaml:"platforms" json:"platforms"`
	// Package is the installed server, pinned to one exact release.
	Package Package `yaml:"server" json:"server"`
	// Attachment is how Agentry Desktop reaches the server.
	Attachment Attachment `yaml:"attachment" json:"attachment"`
	// Launch is the command line of a stdio server.
	Launch *Launch `yaml:"launch,omitempty" json:"launch,omitempty"`
	// Endpoint is a loopback-http server's address.
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	// Listener is the loopback port the application listens on, checked
	// before a run starts.
	Listener Listener `yaml:"listener" json:"listener"`
	// Tools are the server's tools the library classes, in its order.
	Tools []Tool `yaml:"tools" json:"tools"`
	// Withheld are tools the server offers that the preset withholds.
	Withheld []string `yaml:"withheld,omitempty" json:"withheld,omitempty"`
	// Egress are the hosts the server or the application's add-on is
	// known to reach from the person's computer.
	Egress []string `yaml:"egress,omitempty" json:"egress,omitempty"`
	// Steps are what the person does once before the first run.
	Steps []string `yaml:"steps" json:"steps"`
	// Windows are the identifiers the application's windows carry on each
	// platform, matched without regard to case: on Linux its X11 WM_CLASS
	// and its Wayland app id, on macOS its bundle identifier and process
	// name, on Windows its executable's file name.
	Windows map[string][]string `yaml:"windows" json:"windows"`
	// Installed are where the application is found installed on each
	// platform: a command on the PATH, an absolute path, or a Flatpak app
	// id written flatpak:<id>.
	Installed map[string][]string `yaml:"installed,omitempty" json:"installed,omitempty"`
	// EnvNotes say what each of the launch's variables does, for the
	// person's card.
	EnvNotes map[string]string `yaml:"envNotes,omitempty" json:"envNotes,omitempty"`
	// Screening is the launch variable that turns the server's own script
	// screening on, which earlier versions of Agentry Desktop let the
	// person turn off with a switch of its own; "" for none.
	Screening string `yaml:"screening,omitempty" json:"screening,omitempty"`
}

// Package is an installed server's pin.
type Package struct {
	Ecosystem Ecosystem `yaml:"ecosystem" json:"ecosystem"`
	Name      string    `yaml:"name" json:"name"`
	// Version is exact: a newer release is not vetted until the preset moves
	// to it.
	Version string `yaml:"version" json:"version"`
	// Source is the server's source repository, https.
	Source string `yaml:"source" json:"source"`
}

// Ref is the pin as ecosystem:name@version.
func (p Package) Ref() string { return string(p.Ecosystem) + ":" + p.Name + "@" + p.Version }

// Launch is a stdio server's command line and environment.
type Launch struct {
	// Command is the program and its arguments; one of them holds the
	// pinned version.
	Command []string `yaml:"command" json:"command"`
	// Env is set for the server's process.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// Listener is where the application listens for its server.
type Listener struct {
	Port int `yaml:"port" json:"port"`
}

// Tool is one tool a run may call.
type Tool struct {
	Name  string `yaml:"name" json:"name"`
	Class Class  `yaml:"class" json:"class"`
	// Egress marks a tool that reaches the entry's hosts.
	Egress bool `yaml:"egress,omitempty" json:"egress,omitempty"`
}

// Tool returns the preset's tool of this name.
func (s Preset) Tool(name string) (Tool, bool) {
	for _, t := range s.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// ToolNames are the preset's tools, in the library's order.
func (s Preset) ToolNames() []string {
	out := make([]string, len(s.Tools))
	for i, t := range s.Tools {
		out[i] = t.Name
	}
	return out
}

// Offers reports whether the server offers a tool of this name, given to a
// run or withheld.
func (s Preset) Offers(name string) bool {
	_, ok := s.Tool(name)
	return ok || slices.Contains(s.Withheld, name)
}

// Library is a validated library.
type Library struct {
	version int
	entries []Preset
}

//go:embed library.yaml
var builtinData []byte

var builtin = func() Library {
	c, err := Parse(builtinData)
	if err != nil {
		panic("library: the embedded library: " + err.Error())
	}
	return c
}()

// Builtin is the library released with this build.
func Builtin() Library { return builtin }

// Version is the library's version, raised on every change to it.
func (c Library) Version() int { return c.version }

// All is every preset, in the library's order.
func (c Library) All() []Preset {
	out := make([]Preset, len(c.entries))
	for i, e := range c.entries {
		out[i] = e.clone()
	}
	return out
}

// Lookup is the preset with this id.
func (c Library) Lookup(id string) (Preset, bool) {
	for _, e := range c.entries {
		if e.ID == id {
			return e.clone(), true
		}
	}
	return Preset{}, false
}

func (s Preset) clone() Preset {
	s.Platforms = slices.Clone(s.Platforms)
	s.Tools = slices.Clone(s.Tools)
	s.Withheld = slices.Clone(s.Withheld)
	s.Egress = slices.Clone(s.Egress)
	s.Steps = slices.Clone(s.Steps)
	s.Windows = cloneLists(s.Windows)
	s.Installed = cloneLists(s.Installed)
	s.EnvNotes = maps.Clone(s.EnvNotes)
	if s.Launch != nil {
		l := *s.Launch
		l.Command = slices.Clone(l.Command)
		l.Env = maps.Clone(l.Env)
		s.Launch = &l
	}
	return s
}

func cloneLists(m map[string][]string) map[string][]string {
	if m == nil {
		return nil
	}
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = slices.Clone(v)
	}
	return out
}

// WindowsOn are the identifiers the application's windows carry on goos.
func (s Preset) WindowsOn(goos string) []string { return slices.Clone(s.Windows[goos]) }

// OfferedOn reports whether the library offers the preset on goos.
func (s Preset) OfferedOn(goos string) bool { return slices.Contains(s.Platforms, goos) }

// Classes of the preset's tools, by name.
func (s Preset) ClassOf(name string) (Class, bool) {
	t, ok := s.Tool(name)
	return t.Class, ok
}

var (
	idRE      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	toolRE    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	versionRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*([.+-][0-9A-Za-z.+-]+)?$`)
	hostRE    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	envRE     = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// Parse reads and validates a library. It refuses unknown fields, a
// library without a version, a repeated or malformed id, an entry
// without an application, a platform or a step, a server without an exact
// version, a source that is not https, a launch that does not hold the
// pin, an attachment other than stdio or loopback-http, a stdio server of
// the application's own, a loopback address that is not loopback or not on
// the listener's port, a port out of range, a tool of no class or one
// outside the closed set, a tool listed twice or both given and withheld,
// an egress host that is not a DNS name, a platform offered with no window
// identifiers, and an environment note for a variable the launch does not
// set.
func Parse(data []byte) (Library, error) {
	var doc struct {
		Version int      `yaml:"version"`
		Presets []Preset `yaml:"presets"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return Library{}, fmt.Errorf("library: %w", err)
	}
	if doc.Version < 1 {
		return Library{}, errors.New("library: version is a positive number")
	}
	seen := map[string]bool{}
	for _, e := range doc.Presets {
		if err := e.validate(); err != nil {
			return Library{}, fmt.Errorf("library: %q: %w", e.ID, err)
		}
		if seen[e.ID] {
			return Library{}, fmt.Errorf("library: %q is listed twice", e.ID)
		}
		seen[e.ID] = true
	}
	return Library{version: doc.Version, entries: doc.Presets}, nil
}

func (s Preset) validate() error {
	switch {
	case !idRE.MatchString(s.ID):
		return errors.New("an id is lowercase letters, digits and single hyphens")
	case s.Application == "":
		return errors.New("an application name is required")
	case len(s.Platforms) == 0:
		return errors.New("a preset names the platforms it is offered on")
	case len(s.Steps) == 0:
		return errors.New("a preset says what the person does before the first run")
	case len(s.Tools) == 0:
		return errors.New("a preset gives a run at least one tool")
	}
	for _, p := range s.Platforms {
		if !slices.Contains(platforms, p) {
			return fmt.Errorf("unknown platform %q (one of %s)", p, strings.Join(platforms, ", "))
		}
		if len(s.Windows[p]) == 0 {
			return fmt.Errorf("the preset is offered on %s and names no window identifier there", p)
		}
	}
	if s.Screening != "" && (s.Launch == nil || s.Launch.Env[s.Screening] == "") {
		return fmt.Errorf("the screening variable %s is not one the launch sets", s.Screening)
	}
	for k := range s.EnvNotes {
		if s.Launch == nil || s.Launch.Env[k] == "" {
			return fmt.Errorf("the environment note %s names a variable the launch does not set", k)
		}
	}
	if err := s.Package.validate(); err != nil {
		return err
	}
	if err := s.validateAttachment(); err != nil {
		return err
	}
	if s.Listener.Port < 1 || s.Listener.Port > 65535 {
		return fmt.Errorf("listener port %d is not a port", s.Listener.Port)
	}
	given := map[string]bool{}
	for _, t := range s.Tools {
		switch {
		case !toolRE.MatchString(t.Name):
			return fmt.Errorf("tool name %q is not a tool name", t.Name)
		case given[t.Name]:
			return fmt.Errorf("tool %s is listed twice", t.Name)
		case !slices.Contains(Classes(), t.Class):
			return fmt.Errorf("tool %s has class %q: a class is read, act or exec", t.Name, t.Class)
		}
		given[t.Name] = true
	}
	withheld := map[string]bool{}
	for _, n := range s.Withheld {
		switch {
		case !toolRE.MatchString(n):
			return fmt.Errorf("withheld tool %q is not a tool name", n)
		case given[n]:
			return fmt.Errorf("tool %s is both given and withheld", n)
		case withheld[n]:
			return fmt.Errorf("withheld tool %s is listed twice", n)
		}
		withheld[n] = true
	}
	hosts := map[string]bool{}
	for _, h := range s.Egress {
		switch {
		case !hostRE.MatchString(h):
			return fmt.Errorf("egress host %q is not a lowercase DNS name", h)
		case hosts[h]:
			return fmt.Errorf("egress host %s is listed twice", h)
		}
		hosts[h] = true
	}
	return nil
}

func (p Package) validate() error {
	switch {
	case p.Ecosystem != EcosystemPyPI && p.Ecosystem != EcosystemNPM && p.Ecosystem != EcosystemApplication:
		return fmt.Errorf("unknown ecosystem %q", p.Ecosystem)
	case p.Name == "":
		return errors.New("the server needs its package name")
	case !versionRE.MatchString(p.Version):
		return fmt.Errorf("the server's version %q is not one exact release", p.Version)
	case !plainHTTPS(p.Source):
		return errors.New("the server's source is a plain https address")
	}
	return nil
}

// pinnedArg is the launch argument that names the pinned release, as the
// ecosystem's runner spells it: uvx name==version, npx name@version.
func (p Package) pinnedArg() string {
	if p.Ecosystem == EcosystemNPM {
		return p.Name + "@" + p.Version
	}
	return p.Name + "==" + p.Version
}

func (s Preset) validateAttachment() error {
	switch s.Attachment {
	case AttachmentStdio:
		switch {
		case s.Package.Ecosystem == EcosystemApplication:
			return errors.New("a stdio server is launched from a package: the application's own plugin is reached on loopback")
		case s.Launch == nil || len(s.Launch.Command) == 0:
			return errors.New("a stdio server needs its launch command")
		case s.Endpoint != "":
			return errors.New("a stdio server has no endpoint")
		case !slices.Contains(s.Launch.Command, s.Package.pinnedArg()):
			return fmt.Errorf("the launch command does not hold the pin %s", s.Package.pinnedArg())
		}
		for k := range s.Launch.Env {
			if !envRE.MatchString(k) {
				return fmt.Errorf("launch environment name %q is not an environment variable name", k)
			}
		}
	case AttachmentLoopbackHTTP:
		if s.Launch != nil {
			return errors.New("a loopback-http server is not launched")
		}
		u, err := url.Parse(s.Endpoint)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("endpoint %q is not an http://127.0.0.1 address", s.Endpoint)
		}
		if u.Port() != fmt.Sprint(s.Listener.Port) {
			return fmt.Errorf("endpoint %q is not on the listener's port %d", s.Endpoint, s.Listener.Port)
		}
	default:
		return fmt.Errorf("unknown attachment %q (stdio or loopback-http)", s.Attachment)
	}
	return nil
}

// plainHTTPS reports whether s is an https address with a host and no user
// information, query or fragment.
func plainHTTPS(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
