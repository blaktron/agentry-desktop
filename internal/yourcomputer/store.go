// Package yourcomputer keeps the person's approved applications: the
// applications on this computer that agents may use, each approved by the
// person for its local MCP server, its windows, both or neither. Settings →
// Your computer edits the list, the link service and computer use read it,
// and a release names an application by its id, which resolves here.
//
// Nothing on the list is approved by default: adding an application, from
// the library, by hand or by the discovery at the first start of this
// version, leaves both approvals off. A tool nobody has classed is exec,
// and so asks the person before each call.
//
// The list is a file of its own in the app's config folder,
// your-computer.yaml, readable by the person alone.
package yourcomputer

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"

	"github.com/blaktron/agentry-desktop/internal/library"
)

// FileName is the list's file in the app's config folder.
const FileName = "your-computer.yaml"

// From says where an application's set-up came from: a preset of the
// library, at the library's version, or the person.
type From struct {
	// Preset is the library preset's id, "" for an application the person
	// set up.
	Preset string `yaml:"preset,omitempty" json:"preset,omitempty"`
	// Library is the library's version the preset was taken from.
	Library int `yaml:"library,omitempty" json:"library,omitempty"`
	// Edited says the person changed the preset's server since: its
	// launch, its environment, its endpoint or its listener.
	Edited bool `yaml:"edited,omitempty" json:"edited,omitempty"`
}

// Protocol is the link protocol's word for it: library, library-edited or
// person.
func (f From) Protocol() string {
	switch {
	case f.Preset == "":
		return desktop.FromPerson
	case f.Edited:
		return desktop.FromLibraryEdited
	}
	return desktop.FromLibrary
}

// Words say it as the card and the consent do.
func (f From) Words() string {
	switch f.Protocol() {
	case desktop.FromLibrary:
		return "from Agentry's library"
	case desktop.FromLibraryEdited:
		return "from Agentry's library, changed by you; Agentry has not vetted the change"
	}
	return "set up by you; Agentry has not vetted it"
}

// Server is an application's local MCP server.
type Server struct {
	// Attachment is stdio (Agentry Desktop starts it) or loopback-http
	// (the application serves it).
	Attachment library.Attachment `yaml:"attachment" json:"attachment"`
	// Command is a stdio server's program and arguments, and Env the
	// variables set for it over the app's environment.
	Command []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	// Endpoint is a loopback-http server's address.
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	// Listener is the loopback port the application listens on for its
	// server, checked before a run starts; 0 for none.
	Listener int `yaml:"listener,omitempty" json:"listener,omitempty"`
	// Package names a preset's pinned server, "pypi:mcp-for-blender@2.2.7".
	Package string `yaml:"package,omitempty" json:"package,omitempty"`
}

// Tool is one of an application's tools as the person classed it.
type Tool struct {
	Name string `yaml:"name" json:"name"`
	// Class is read, act or exec; "" is a tool nobody has classed, which
	// is answered as exec.
	Class library.Class `yaml:"class,omitempty" json:"class,omitempty"`
	// Withheld tools are never given to a run.
	Withheld bool `yaml:"withheld,omitempty" json:"withheld,omitempty"`
}

// Approval is what the person approved of an application.
type Approval struct {
	// MCP lets agents use its local MCP server.
	MCP bool `yaml:"mcp,omitempty" json:"mcp"`
	// Computer lets agents see and use its windows.
	Computer bool `yaml:"computer,omitempty" json:"computer"`
}

// Application is one application on the person's list.
type Application struct {
	// ID is what a release names it by, unique on this computer.
	ID string `yaml:"id" json:"id"`
	// Name is what the person and the consent call it.
	Name string `yaml:"name" json:"name"`
	From From   `yaml:"from,omitempty" json:"from"`
	// Windows are the identifiers its windows carry on this computer's
	// system; none means no computer use.
	Windows []string `yaml:"windows,omitempty" json:"windows"`
	// Server is its local MCP server; nil means none.
	Server  *Server  `yaml:"server,omitempty" json:"server,omitempty"`
	Tools   []Tool   `yaml:"tools,omitempty" json:"tools"`
	Approve Approval `yaml:"approve" json:"approve"`
}

// ClassOf is how a call of the tool is answered: the class the person or
// the preset gave it, exec for a tool nobody has classed.
func (a Application) ClassOf(tool string) library.Class {
	for _, t := range a.Tools {
		if t.Name == tool && t.Class != "" {
			return t.Class
		}
	}
	return library.ClassExec
}

// Classed reports whether someone gave the tool a class.
func (a Application) Classed(tool string) bool {
	for _, t := range a.Tools {
		if t.Name == tool {
			return t.Class != ""
		}
	}
	return false
}

// Withholds reports whether the tool is never given to a run.
func (a Application) Withholds(tool string) bool {
	for _, t := range a.Tools {
		if t.Name == tool {
			return t.Withheld
		}
	}
	return false
}

// UsesMCP says agents may use its server: it has one and the person
// approved it.
func (a Application) UsesMCP() bool { return a.Server != nil && a.Approve.MCP }

// UsesWindows says agents may see and use its windows: it names them and
// the person approved it.
func (a Application) UsesWindows() bool { return len(a.Windows) > 0 && a.Approve.Computer }

// Launch is a stdio server's launch line as the card and the receipt say
// it: the variables set, then the command.
func (s Server) Launch() string {
	keys := slices.Sorted(maps.Keys(s.Env))
	parts := make([]string, 0, len(keys)+len(s.Command))
	for _, k := range keys {
		parts = append(parts, k+"="+s.Env[k])
	}
	return strings.Join(append(parts, s.Command...), " ")
}

// Doc is the list's file.
type Doc struct {
	Version int `yaml:"version"`
	// Discovered says the first start of this version looked for the
	// library's applications already set up here; it runs once.
	Discovered   bool          `yaml:"discovered,omitempty"`
	Applications []Application `yaml:"applications,omitempty"`
	// Choices are the applications the person chose for a QuickAgent:
	// its namespace/name, then the entry (an id the list did not have, or
	// chosen for an application the person chooses) to the application's
	// id.
	Choices map[string]map[string]string `yaml:"choices,omitempty"`
}

// docVersion is the file's version.
const docVersion = 1

// Lookup is the application with this id.
func (d Doc) Lookup(id string) (Application, bool) {
	for _, a := range d.Applications {
		if a.ID == id {
			return clone(a), true
		}
	}
	return Application{}, false
}

// Choice is the application the person chose for a QuickAgent's entry, if
// it is still on the list.
func (d Doc) Choice(ref, entry string) (Application, bool) {
	id := d.Choices[releaseKey(ref)][entry]
	if id == "" {
		return Application{}, false
	}
	return d.Lookup(id)
}

// Choose remembers the person's choice for a QuickAgent's entry.
func (d *Doc) Choose(ref, entry, id string) {
	if d.Choices == nil {
		d.Choices = map[string]map[string]string{}
	}
	k := releaseKey(ref)
	if d.Choices[k] == nil {
		d.Choices[k] = map[string]string{}
	}
	d.Choices[k][entry] = id
}

// releaseKey is a QuickAgent's namespace/name, its version left off, so a
// choice holds across its versions.
func releaseKey(ref string) string {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

func clone(a Application) Application {
	a.Windows = slices.Clone(a.Windows)
	a.Tools = slices.Clone(a.Tools)
	if a.Server != nil {
		s := *a.Server
		s.Command = slices.Clone(s.Command)
		s.Env = maps.Clone(s.Env)
		a.Server = &s
	}
	return a
}

var (
	idRE   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	toolRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	envRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// MaxIDLength is the longest id a release can name.
const MaxIDLength = 40

// reserved ids name the desktop link's own entries.
var reserved = []string{"computer", "chosen"}

// ValidID says why id cannot name an application, nil when it can.
func ValidID(id string) error {
	switch {
	case !idRE.MatchString(id) || len(id) > MaxIDLength:
		return fmt.Errorf("an id is lowercase letters and digits in words joined by single hyphens, at most %d characters: %q is not", MaxIDLength, id)
	case slices.Contains(reserved, id):
		return fmt.Errorf("%q names the desktop link's own entry: choose another id", id)
	}
	return nil
}

// IDFor is an id for an application named name: its words, lowercase,
// joined by hyphens.
func IDFor(name string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
		default:
			hyphen = true
		}
		if b.Len() >= MaxIDLength {
			break
		}
	}
	id := strings.Trim(b.String(), "-")
	if len(id) > MaxIDLength {
		id = strings.Trim(id[:MaxIDLength], "-")
	}
	if slices.Contains(reserved, id) {
		id += "-app"
	}
	return id
}

// Validate says why an application cannot be on the list, nil when it can.
func (a Application) Validate() error {
	if err := ValidID(a.ID); err != nil {
		return err
	}
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("an application needs a name")
	}
	if a.Server == nil && len(a.Windows) == 0 {
		return fmt.Errorf("%s has neither an MCP server nor windows: give it one or both", a.Name)
	}
	for _, w := range a.Windows {
		if strings.TrimSpace(w) == "" || strings.ContainsAny(w, "\n\r") {
			return fmt.Errorf("%s's window identifier %q is empty or spans lines", a.Name, w)
		}
	}
	if a.Server != nil {
		if err := a.Server.Validate(); err != nil {
			return fmt.Errorf("%s's MCP server: %w", a.Name, err)
		}
	}
	seen := map[string]bool{}
	for _, t := range a.Tools {
		switch {
		case !toolRE.MatchString(t.Name):
			return fmt.Errorf("%q is not a tool name", t.Name)
		case seen[t.Name]:
			return fmt.Errorf("the tool %s is listed twice", t.Name)
		case t.Class != "" && !slices.Contains(library.Classes(), t.Class):
			return fmt.Errorf("the tool %s has class %q: a class is read, act or exec", t.Name, t.Class)
		}
		seen[t.Name] = true
	}
	return nil
}

// Validate says why a server cannot be launched or reached, nil when it
// can.
func (s Server) Validate() error {
	if s.Listener < 0 || s.Listener > 65535 {
		return fmt.Errorf("port %d is not a port", s.Listener)
	}
	switch s.Attachment {
	case library.AttachmentStdio:
		if len(s.Command) == 0 || strings.TrimSpace(s.Command[0]) == "" {
			return errors.New("a server Agentry Desktop starts needs its command")
		}
		if s.Endpoint != "" {
			return errors.New("a server Agentry Desktop starts has no address")
		}
		for k := range s.Env {
			if !envRE.MatchString(k) {
				return fmt.Errorf("%q is not an environment variable's name", k)
			}
		}
	case library.AttachmentLoopbackHTTP:
		if len(s.Command) > 0 || len(s.Env) > 0 {
			return errors.New("a server the application serves is not started by Agentry Desktop")
		}
		if err := LoopbackEndpoint(s.Endpoint); err != nil {
			return err
		}
	default:
		return fmt.Errorf("a server is started by Agentry Desktop (stdio) or served by the application on loopback (loopback-http), not %q", s.Attachment)
	}
	return nil
}

// LoopbackEndpoint says why an address is not an http address on this
// computer's loopback, nil when it is.
func LoopbackEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Port() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%q is not an http://127.0.0.1:<port> or http://[::1]:<port> address", endpoint)
	}
	return nil
}

// validate says why the file cannot be read as a list.
func (d Doc) validate() error {
	seen := map[string]bool{}
	for _, a := range d.Applications {
		if err := a.Validate(); err != nil {
			return err
		}
		if seen[a.ID] {
			return fmt.Errorf("two applications have the id %s", a.ID)
		}
		seen[a.ID] = true
	}
	return nil
}

// Store is the list's file.
type Store struct {
	path string
	mu   sync.Mutex
}

// Open is the list kept at path.
func Open(path string) *Store { return &Store{path: path} }

// InDir is the list in the app's config folder.
func InDir(dir string) *Store { return Open(filepath.Join(dir, FileName)) }

// Path is the list's file.
func (s *Store) Path() string { return s.path }

// Load reads the list; a missing file is an empty list.
func (s *Store) Load() (Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) load() (Doc, error) {
	if s == nil || s.path == "" {
		return Doc{Version: docVersion}, nil
	}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Doc{Version: docVersion}, nil
	}
	if err != nil {
		return Doc{}, fmt.Errorf("read the list of your applications: %w", err)
	}
	var d Doc
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return Doc{}, fmt.Errorf("read the list of your applications (%s): %w", s.path, err)
	}
	if d.Version > docVersion {
		return Doc{}, fmt.Errorf("the list of your applications (%s) was written by a newer Agentry Desktop: update Agentry Desktop", s.path)
	}
	if err := d.validate(); err != nil {
		return Doc{}, fmt.Errorf("the list of your applications (%s): %w", s.path, err)
	}
	d.Version = docVersion
	return d, nil
}

// Update reads the list, changes it with f, checks it and writes it, the
// person alone able to read the file.
func (s *Store) Update(f func(*Doc) error) (Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load()
	if err != nil {
		return Doc{}, err
	}
	if err := f(&d); err != nil {
		return Doc{}, err
	}
	if err := d.validate(); err != nil {
		return Doc{}, err
	}
	if s == nil || s.path == "" {
		return d, nil
	}
	b, err := yaml.Marshal(d)
	if err != nil {
		return Doc{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return Doc{}, err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return Doc{}, err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return Doc{}, err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return Doc{}, err
	}
	return d, nil
}

// FromPreset is a preset as an application on this computer's system,
// nothing approved.
func FromPreset(p library.Preset, version int, goos string) Application {
	a := Application{ID: p.ID, Name: p.Application, From: From{Preset: p.ID, Library: version}, Windows: p.WindowsOn(goos)}
	srv := &Server{Attachment: p.Attachment, Listener: p.Listener.Port, Package: p.Package.Ref()}
	switch p.Attachment {
	case library.AttachmentStdio:
		srv.Command = slices.Clone(p.Launch.Command)
		srv.Env = maps.Clone(p.Launch.Env)
	case library.AttachmentLoopbackHTTP:
		srv.Endpoint = p.Endpoint
	}
	a.Server = srv
	for _, t := range p.Tools {
		a.Tools = append(a.Tools, Tool{Name: t.Name, Class: t.Class})
	}
	for _, w := range p.Withheld {
		a.Tools = append(a.Tools, Tool{Name: w, Withheld: true})
	}
	return a
}

// Add puts an application on the list, nothing approved.
func (d *Doc) Add(a Application) error {
	if _, taken := d.Lookup(a.ID); taken {
		return fmt.Errorf("an application with the id %s is on your list already: give this one another id", a.ID)
	}
	a.Approve = Approval{}
	d.Applications = append(d.Applications, a)
	return nil
}

// Change applies f to the application with this id.
func (d *Doc) Change(id string, f func(*Application) error) error {
	for i := range d.Applications {
		if d.Applications[i].ID == id {
			return f(&d.Applications[i])
		}
	}
	return fmt.Errorf("no application %s is on your list", id)
}

// SetServer replaces an application's server: a preset's becomes edited.
func (a *Application) SetServer(s *Server) {
	if a.From.Preset != "" && !sameServer(a.Server, s) {
		a.From.Edited = true
	}
	a.Server = s
}

func sameServer(a, b *Server) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Attachment == b.Attachment && slices.Equal(a.Command, b.Command) && maps.Equal(a.Env, b.Env) &&
		a.Endpoint == b.Endpoint && a.Listener == b.Listener
}

// Remove takes an application off the list, and the choices that named it.
func (d *Doc) Remove(id string) error {
	n := len(d.Applications)
	d.Applications = slices.DeleteFunc(d.Applications, func(a Application) bool { return a.ID == id })
	if len(d.Applications) == n {
		return fmt.Errorf("no application %s is on your list", id)
	}
	for k, m := range d.Choices {
		for e, v := range m {
			if v == id {
				delete(m, e)
			}
		}
		if len(m) == 0 {
			delete(d.Choices, k)
		}
	}
	return nil
}

// Loosens reports whether moving a tool from one class to another lets
// its calls through with less asked: exec to act or read, act to read.
func Loosens(from, to library.Class) bool {
	rank := func(c library.Class) int {
		switch c {
		case library.ClassRead:
			return 0
		case library.ClassAct:
			return 1
		}
		return 2
	}
	return rank(to) < rank(from)
}
