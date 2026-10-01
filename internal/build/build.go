// Package build is the Build surface's engine: author a QuickAgent as a
// form, save it as a real workspace (the same files `agentry init` lays
// down), and publish it to a registry over the same cli-upload path the
// CLI uses — the client packs a deterministic archive, the server
// re-validates and re-packs, and the version rides the full publication
// pipeline to a verdict. Nothing here bypasses a gate: the desktop gets
// exactly the scan-and-review treatment `agentry publish` gets.
package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
)

// Model is the Build form's shape: flat fields that map one-to-one onto
// the manifest's YAML. The YAML is the contract (a closed schema, V-rules
// at validate/publish); this struct is only a convenient editor for it.
//
// It covers every field of manifest schema v2 (plan builder-rework.md D19,
// agentry-desktop#280), so Load then Save keeps what a template-made or
// hand-edited workspace declares. The form edits most of them; the rest
// (authors, the runtime block, dependencies, the publish channel and extra
// include globs) are carried through unchanged. What a save does not keep
// is comments, and HadComments says when a loaded file had some.
type Model struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Modes are the run modes the release supports (manifest v2): autonomous,
	// interactive, or both.
	Modes    []string    `json:"modes"`
	Version  string      `json:"version"`
	Summary  string      `json:"summary"`
	License  string      `json:"license"`
	Authors  []AuthorRow `json:"authors"`
	Homepage string      `json:"homepage,omitempty"`
	Tags     []string    `json:"tags"`

	// InstructionFile is entrypoints.instruction (AGENT.md by default; a
	// workspace authored as a v1 skill keeps its SKILL.md).
	InstructionFile string `json:"instructionFile"`
	// Instructions is the instruction file's content.
	Instructions string `json:"instructions"`
	// Intake are the up-front questions (manifest v2 intake, plan §3.5).
	Intake []IntakeRow `json:"intake"`
	// Opening is interactive.opening: the agent's first line in a session
	// started without a task (only meaningful with interactive in Modes).
	Opening string `json:"opening"`
	// IdleTimeoutSeconds is interactive.idleTimeoutSeconds; 0 leaves it out.
	IdleTimeoutSeconds int `json:"idleTimeoutSeconds"`
	// Guide is the manifest's guide block (DEV-141): what a person reads
	// before a run, never the agent.
	Guide GuideModel `json:"guide"`
	// RuntimeModels and RuntimeInterfaces are the runtime block. A new
	// QuickAgent declares both instruction interfaces; a loaded one keeps
	// what it had, which may be nothing.
	RuntimeModels     []string `json:"runtimeModels"`
	RuntimeInterfaces []string `json:"runtimeInterfaces"`

	// The §5.4 capability contract, flattened for form controls.
	EgressMode   string   `json:"egressMode"` // none | hosts | all
	EgressHosts  []string `json:"egressHosts"`
	FSRead       string   `json:"fsRead"` // none | workspace | paths
	FSReadPaths  []string `json:"fsReadPaths"`
	FSWrite      string   `json:"fsWrite"`
	FSWritePaths []string `json:"fsWritePaths"`
	ExecCommands []string `json:"execCommands"`
	Secrets      []string `json:"secrets"`
	DataAccess   []string `json:"dataAccess"`
	// MCPServers are the declared, version-pinned MCP servers (DEV-060);
	// one structured row per server so the form never guesses at a
	// line syntax.
	MCPServers []MCPServerRow `json:"mcpServers"`

	// The knowledge block (Knowledge M9; agentry-desktop#497). The prompt
	// is knowledge.accepts: one plain line the run setup's Knowledge step
	// shows when the QuickAgent needs a pack of the person's own, empty for
	// none; KnowledgeRequired makes the step unskippable. KnowledgeShips are
	// the packs knowledge.ships names, which the form carries through a load
	// and a save and never adds: a draft never brings one (FromDraft).
	KnowledgePrompt   string   `json:"knowledgePrompt"`
	KnowledgeRequired bool     `json:"knowledgeRequired"`
	KnowledgeShips    []string `json:"knowledgeShips"`

	// Dependencies are dependencies.agents.
	Dependencies []DependencyRow `json:"dependencies"`
	// PublishChannel is publish.channel (stable when empty).
	PublishChannel string `json:"publishChannel"`
	// PublishInclude is publish.include as loaded; empty means the four
	// files the Build surface writes.
	PublishInclude []string `json:"publishInclude"`

	// HadComments reports that the loaded agent.yaml carried comments,
	// which a save does not keep. Never written.
	HadComments bool `json:"hadComments"`
}

// AuthorRow is one authors entry.
type AuthorRow struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GuideModel is the guide block as the form edits it. Connect keeps its
// rows in order; the manifest's map has none.
type GuideModel struct {
	Overview string       `json:"overview"`
	Prepare  []string     `json:"prepare"`
	Files    string       `json:"files"`
	Task     string       `json:"task"`
	Connect  []ConnectRow `json:"connect"`
}

// ConnectRow is help for one declared secret: where to create it, which
// scopes it needs.
type ConnectRow struct {
	Secret string `json:"secret"`
	Help   string `json:"help"`
}

// IntakeRow is one intake question as the form edits it.
type IntakeRow struct {
	ID       string   `json:"id"`
	Prompt   string   `json:"prompt"`
	Type     string   `json:"type"` // text | secret | choice | file
	Required bool     `json:"required"`
	Choices  []string `json:"choices"`
}

// MCPServerRow is one capabilities.mcpServers entry as the form edits it.
type MCPServerRow struct {
	// Source is "npm" or "pypi".
	Source string `json:"source"`
	// Package is the npm (optionally @scope/name) or PyPI package name.
	Package string `json:"package"`
	// Version is the exact pin (no ranges, V5).
	Version string `json:"version"`
	// Tools restricts the server to these tool names; empty = all.
	Tools []string `json:"tools"`
	// Secrets are env var NAMES this server needs (never values).
	Secrets []string `json:"secrets"`
	// Egress lists the hosts this server may reach; empty = none.
	Egress []string `json:"egress"`
	// Env is the server's plain configuration (v2, DEV-098): in the
	// contract, never a secret.
	Env []EnvRow `json:"env"`
}

// EnvRow is one env entry of a declared server.
type EnvRow struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DependencyRow is one dependencies.agents entry.
type DependencyRow struct {
	Ref     string `json:"ref"`
	Version string `json:"version"`
}

// defaultInstructionFile is the manifest's default entrypoints.instruction.
const defaultInstructionFile = "AGENT.md"

// instructionFile is the file the instructions are saved to and declared as.
func (m *Model) instructionFile() string {
	if f := strings.TrimSpace(m.InstructionFile); f != "" {
		return f
	}
	return defaultInstructionFile
}

var (
	intakeIDRE = regexp.MustCompile(`^[a-z0-9_]+$`)
	// envNameRE is the core's environment-variable grammar (pkg/manifest):
	// env names and guide help keys are written bare, so they are held to it.
	envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// instructionFileRE is a plain file name at the workspace root.
	instructionFileRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// The closed vocabularies written bare.
var (
	dataAccessWords = map[string]bool{"email": true, "calendar": true, "files": true, "tickets": true, "code": true, "messages": true, "payments": true, "other": true}
	interfaceWords  = map[string]bool{"claude-skill": true, "agents-md": true}
	channelWords    = map[string]bool{"": true, "stable": true, "beta": true}
)

func validMode(s string) bool { return s == "autonomous" || s == "interactive" }

// Default returns a fresh form model with the scaffold's defaults.
func Default(namespace string) *Model {
	return &Model{
		Modes: []string{"autonomous"}, Version: "0.1.0", License: "MIT", Namespace: namespace,
		InstructionFile: defaultInstructionFile, Intake: []IntakeRow{},
		EgressMode: "none", FSRead: "workspace", FSWrite: "none",
		Tags: []string{}, EgressHosts: []string{}, FSReadPaths: []string{}, FSWritePaths: []string{},
		ExecCommands: []string{}, Secrets: []string{}, DataAccess: []string{}, MCPServers: []MCPServerRow{},
		Authors: []AuthorRow{}, Guide: GuideModel{Prepare: []string{}, Connect: []ConnectRow{}},
		RuntimeModels: []string{}, RuntimeInterfaces: []string{"claude-skill", "agents-md"},
		Dependencies: []DependencyRow{}, PublishInclude: []string{},
		Instructions: "# New QuickAgent\n\nDescribe the one job this agent does: task in, result out.\n\n## What you are given\n\nThe task, and any files under /inputs.\n\n## Steps\n\n1. \n\n## What you return\n\nWrite the report with the write_report tool.\n",
	}
}

// Section is one step of the Step by step view (plan builder-rework.md D4).
// The Advanced view shows the same sections at once.
type Section string

const (
	SectionBasics       Section = "basics"
	SectionInstructions Section = "instructions"
	SectionTools        Section = "tools"
	SectionAccess       Section = "access"
	SectionQuestions    Section = "questions"
	SectionGuide        Section = "guide"
	SectionPublish      Section = "publish"
)

// Sections are the form's sections in the order Step by step walks them.
var Sections = []Section{SectionBasics, SectionInstructions, SectionTools, SectionAccess, SectionQuestions, SectionGuide, SectionPublish}

// maxInstructionBytes is the §19 instruction-file quota.
const maxInstructionBytes = 1 << 20

// formProblems are the rules the form holds before anything touches disk,
// each with the section whose fields break it. Two are stricter than the
// core: a declared server's "*" egress, which the runner always refuses
// (DR-20), and a secret question with no declared secret, which the runner
// refuses at intake. The rest of the V-rules come from manifest.Validate in
// Check.
func (m *Model) formProblems() []SectionProblem {
	var out []SectionProblem
	add := func(sec Section, format string, args ...any) {
		out = append(out, SectionProblem{Section: sec, Problem: fmt.Sprintf(format, args...)})
	}
	if !manifest.ValidName(m.Name) {
		add(SectionBasics, "name %q is invalid (lowercase letters, digits and inner hyphens, 1-65 characters)", m.Name)
	}
	if strings.TrimSpace(m.Summary) == "" {
		add(SectionBasics, "summary is required (one line, ≤140 chars)")
	}
	modes := compact(m.Modes)
	if len(modes) == 0 {
		add(SectionBasics, "choose at least one run mode (autonomous, interactive)")
	}
	seenMode := map[string]bool{}
	for _, mode := range modes {
		if !validMode(mode) {
			add(SectionBasics, "unknown run mode %q (want autonomous or interactive)", mode)
		}
		if seenMode[mode] {
			add(SectionBasics, "run mode %s is listed twice", mode)
		}
		seenMode[mode] = true
	}
	if len([]rune(m.Opening)) > manifest.MaxOpeningChars {
		add(SectionBasics, "the opening line is at most %d characters", manifest.MaxOpeningChars)
	}
	if m.IdleTimeoutSeconds < 0 {
		add(SectionBasics, "the idle timeout cannot be negative")
	}

	if !instructionFileRE.MatchString(m.instructionFile()) || strings.Contains(m.instructionFile(), "..") {
		add(SectionInstructions, "the instruction file must be a plain file name at the workspace root")
	}
	if strings.TrimSpace(m.Instructions) == "" {
		add(SectionInstructions, "write the instructions")
	}
	if len(m.Instructions) > maxInstructionBytes {
		add(SectionInstructions, "the instructions are at most 1 MiB (the §19 instruction-file quota)")
	}

	if m.EgressMode == "hosts" && len(compact(m.EgressHosts)) == 0 {
		add(SectionTools, "list the sites it may reach, or choose no internet")
	}
	agentSecrets := map[string]bool{}
	for _, s := range compact(m.Secrets) {
		agentSecrets[s] = true
	}
	seen := map[string]bool{}
	for i, srv := range m.MCPServers {
		if srv.Source != "npm" && srv.Source != "pypi" {
			add(SectionTools, "mcp server %d: source must be npm or pypi", i+1)
			continue
		}
		pkg := strings.TrimSpace(srv.Package)
		if pkg == "" {
			add(SectionTools, "mcp server %d: package name is required", i+1)
			continue
		}
		if strings.TrimSpace(srv.Version) == "" {
			add(SectionTools, "mcp server %d (%s): an exact version is required (no ranges)", i+1, pkg)
		}
		if id := srv.Source + ":" + pkg; seen[id] {
			add(SectionTools, "mcp server %s is declared twice", id)
		} else {
			seen[id] = true
		}
		for _, h := range compact(srv.Egress) {
			if h == "*" {
				add(SectionTools, "mcp server %s: list the hosts it reaches; the runner always refuses \"*\" for a declared server", pkg)
			}
		}
		serverSecrets := map[string]bool{}
		for _, s := range compact(srv.Secrets) {
			serverSecrets[s] = true
		}
		names := map[string]bool{}
		for _, e := range srv.Env {
			name := strings.TrimSpace(e.Name)
			switch {
			case name == "":
				add(SectionTools, "mcp server %s: an env entry needs a name", pkg)
			case !envNameRE.MatchString(name):
				add(SectionTools, "mcp server %s: env %q is not an environment variable name (letters, digits, _)", pkg, name)
			case names[name]:
				add(SectionTools, "mcp server %s: env %s is set twice", pkg, name)
			case serverSecrets[name] || agentSecrets[name]:
				add(SectionTools, "mcp server %s: %s is declared as env and as a secret; make it one or the other", pkg, name)
			case !manifest.LiteralEnvValue(e.Value):
				add(SectionTools, "mcp server %s: env %s holds a $ or a {{ }} the runner would expand; write the literal value", pkg, name)
			}
			names[name] = true
		}
	}

	if m.FSRead == "paths" && len(compact(m.FSReadPaths)) == 0 {
		add(SectionAccess, "list the paths it reads, or choose another answer")
	}
	if m.FSWrite == "paths" && len(compact(m.FSWritePaths)) == 0 {
		add(SectionAccess, "list the paths it writes, or choose another answer")
	}
	for _, d := range compact(m.DataAccess) {
		if !dataAccessWords[d] {
			add(SectionAccess, "%q is not a data category (email, calendar, files, tickets, code, messages, payments, other)", d)
		}
	}

	if len(m.Intake) > 20 {
		add(SectionQuestions, "at most 20 intake questions")
	}
	// A secret answer is carried by the QuickAgent's own declared secret of
	// the same upper-cased name. The runner counts capabilities.secrets only
	// (agentry-cli pkg/runner/intake), never a server's.
	seenID := map[string]bool{}
	for i, q := range m.Intake {
		id := strings.TrimSpace(q.ID)
		if !intakeIDRE.MatchString(id) {
			add(SectionQuestions, "intake question %d: id must match [a-z0-9_]+", i+1)
			continue
		}
		if seenID[id] {
			add(SectionQuestions, "intake question %d: id %q is used twice", i+1, id)
		}
		seenID[id] = true
		if strings.TrimSpace(q.Prompt) == "" {
			add(SectionQuestions, "intake question %s: the prompt is required", id)
		}
		switch q.Type {
		case "", "text", "file":
		case "choice":
			if len(compact(q.Choices)) == 0 {
				add(SectionQuestions, "intake question %s: a choice question needs its choices", id)
			}
		case "secret":
			if !agentSecrets[strings.ToUpper(id)] {
				add(SectionQuestions, "intake question %s: a secret answer is carried by a key of its own named %s — add it under Files, commands and data", id, strings.ToUpper(id))
			}
		default:
			add(SectionQuestions, "intake question %s: unknown type %q (text, secret, choice, file)", id, q.Type)
		}
		if q.Type != "choice" && len(compact(q.Choices)) > 0 {
			add(SectionQuestions, "intake question %s: choices only apply to a choice question", id)
		}
	}

	seenHelp := map[string]bool{}
	for _, c := range m.Guide.Connect {
		name := strings.TrimSpace(c.Secret)
		if name == "" {
			add(SectionGuide, "help for a key needs the key's name")
			continue
		}
		if !envNameRE.MatchString(name) {
			add(SectionGuide, "%q is not a key name (letters, digits, _)", name)
			continue
		}
		if seenHelp[name] {
			add(SectionGuide, "help for %s is given twice", name)
		}
		seenHelp[name] = true
	}
	// knowledge.accepts (V11): one plain line the run setup shows.
	if p := strings.TrimSpace(m.KnowledgePrompt); p != "" {
		if strings.ContainsFunc(p, func(r rune) bool { return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) }) {
			add(SectionGuide, "the knowledge it asks for is one line of plain text")
		}
		if utf8.RuneCountInString(p) > manifest.MaxKnowledgePromptChars {
			add(SectionGuide, "the knowledge it asks for is at most %d characters", manifest.MaxKnowledgePromptChars)
		}
	} else if m.KnowledgeRequired {
		add(SectionGuide, "say which knowledge pack it needs, or untick required")
	}

	if !manifest.ValidNamespace(m.Namespace) {
		add(SectionPublish, "namespace %q is invalid (lowercase letters, digits and inner hyphens, 1-39 characters)", m.Namespace)
	}
	if strings.TrimSpace(m.Version) == "" {
		add(SectionPublish, "version is required (strict semver, e.g. 0.1.0)")
	}
	if !channelWords[strings.TrimSpace(m.PublishChannel)] {
		add(SectionPublish, "publish channel %q is not stable or beta", m.PublishChannel)
	}
	for _, i := range compact(m.RuntimeInterfaces) {
		if !interfaceWords[i] {
			add(SectionPublish, "runtime interface %q is not claude-skill or agents-md", i)
		}
	}
	return out
}

// Validate applies the form-level rules before anything touches disk.
func (m *Model) Validate() error {
	if ps := m.formProblems(); len(ps) > 0 {
		return errors.New(ps[0].Problem)
	}
	return nil
}

// SectionProblem is one broken rule and the section whose fields break it.
type SectionProblem struct {
	Section Section `json:"section"`
	Problem string  `json:"problem"`
}

// CheckResult is what the Build page draws from a model before it saves
// anything: the agent.yaml it would write, the lines each section writes,
// the first problem in each section, and the V-rules' warnings. The
// problems are the form's own rules and then manifest.Validate's, the same
// static V-rules `agentry validate` runs, so a step refuses what the
// validator would.
type CheckResult struct {
	YAML      string            `json:"yaml"`
	Fragments map[string]string `json:"fragments"`
	Problems  map[string]string `json:"problems"`
	// First is the first problem in step order, or nil.
	First    *SectionProblem `json:"first"`
	Warnings []string        `json:"warnings"`
	// Public is the first rule a public submission still fails (V7), or
	// "": it holds Submit, which is public (agentry-desktop#425), and not
	// Save or a local Install, which V7 does not bind.
	Public string `json:"public,omitempty"`
}

// Check renders the model and runs the rules over it.
func (m *Model) Check() *CheckResult {
	res := &CheckResult{YAML: m.manifestYAML(), Fragments: map[string]string{}, Problems: map[string]string{}, Warnings: []string{}}
	for _, sec := range Sections {
		if f := m.fragment(sec); f != "" {
			res.Fragments[string(sec)] = f
		}
	}
	note := func(sec Section, problem string) {
		if _, ok := res.Problems[string(sec)]; !ok {
			res.Problems[string(sec)] = problem
		}
	}
	for _, p := range m.formProblems() {
		note(p.Section, p.Problem)
	}
	parsed, err := manifest.Parse([]byte(res.YAML))
	if err != nil {
		// The renderer writes the closed schema; a file it cannot parse back
		// is a value the form's rules did not catch.
		note(SectionPublish, "agent.yaml does not parse: "+err.Error())
	} else {
		for _, e := range manifest.Validate(parsed) {
			iss, ok := manifest.AsIssue(e)
			if !ok {
				note(SectionPublish, e.Error())
				continue
			}
			msg := iss.Message
			if iss.Path != "" {
				msg = iss.Path + ": " + iss.Message
			}
			if iss.Severity == manifest.SeverityWarning {
				res.Warnings = append(res.Warnings, msg)
				continue
			}
			note(sectionOf(iss.Path), msg)
		}
		if ps := publicPublishProblems(parsed, m.instructionFile()); len(ps) > 0 {
			res.Public = ps[0]
		}
	}
	for _, sec := range Sections {
		if p, ok := res.Problems[string(sec)]; ok {
			res.First = &SectionProblem{Section: sec, Problem: p}
			break
		}
	}
	return res
}

// publicPublishProblems holds the QuickAgent to V7, since Submit is public
// (agentry-desktop#425): an SPDX licence, and an archive that carries a
// LICENSE (or LICENSE.md, LICENSE.txt) and a README.md. The candidate
// files are the ones Save writes (the manifest, its instruction file,
// README.md and LICENSE when absent) and the licence file's other accepted
// names, held to the include list, so a folder whose include leaves them
// out is named before Submit rather than refused by the registry after
// the agent was created. Only V7 is read: the rest of the context needs
// the folder's real files.
func publicPublishProblems(m *manifest.Manifest, instructionFile string) []string {
	var files []manifest.FileInfo
	for _, p := range []string{"agent.yaml", instructionFile, "README.md", "LICENSE", "LICENSE.md", "LICENSE.txt"} {
		files = append(files, manifest.FileInfo{Path: p, IsText: true})
	}
	var out []string
	for _, e := range manifest.ValidateInContext(m, manifest.Context{
		Namespace: m.Namespace, Name: m.Name, Public: true,
		Files: manifest.ResolveInclude(m.Publish.Include, files),
	}) {
		iss, ok := manifest.AsIssue(e)
		if !ok || iss.Rule != "V7" || iss.Severity == manifest.SeverityWarning {
			continue
		}
		msg := iss.Message
		if iss.Path == "" {
			// A file V7 wants: the include list is where it is missing.
			msg += "; publish.include must carry it"
		}
		out = append(out, msg)
	}
	return out
}

// DeclaredServers are the rows with a package and a version, as the
// manifest declares them, for asking the registry what it knows of each.
func (m *Model) DeclaredServers() []manifest.MCPServer {
	var out []manifest.MCPServer
	for _, srv := range m.MCPServers {
		pkg, ver := strings.TrimSpace(srv.Package), strings.TrimSpace(srv.Version)
		if pkg == "" || ver == "" {
			continue
		}
		d := manifest.MCPServer{Version: ver}
		if srv.Source == "pypi" {
			d.PyPI = pkg
		} else {
			d.NPM = pkg
		}
		out = append(out, d)
	}
	return out
}

// sectionOf names the section whose fields a V-rule's path is in.
func sectionOf(path string) Section {
	head := path
	if i := strings.IndexAny(head, ".["); i >= 0 {
		head = head[:i]
	}
	switch head {
	case "name", "summary", "modes", "kind", "schemaVersion", "interactive":
		return SectionBasics
	case "entrypoints":
		return SectionInstructions
	case "intake":
		return SectionQuestions
	case "guide":
		return SectionGuide
	case "capabilities":
		if strings.HasPrefix(path, "capabilities.network") || strings.HasPrefix(path, "capabilities.mcpServers") {
			return SectionTools
		}
		return SectionAccess
	}
	return SectionPublish
}

// q YAML-quotes a scalar a person typed, so no character in it changes the
// file's shape. The values written bare are each held to a grammar in
// formProblems first: the name and namespace (manifest.ValidName and
// ValidNamespace), the run modes, the MCP source, intake ids and types, env
// and help-key names (envNameRE), data categories, runtime interfaces and
// the publish channel (closed vocabularies). Save refuses a model that
// breaks one, and refuses a file that does not parse back.
func q(s string) string {
	// A value over several lines is one double-quoted string (a JSON string
	// is a YAML double-quoted scalar). yaml.Marshal would write a block
	// scalar indented for column 0, which parses under a key at two spaces
	// (the overview) and breaks under one at four (a key's help, which may
	// span lines since DEV-167; agentry-desktop#343).
	if strings.ContainsAny(s, "\n\r") {
		out, _ := json.Marshal(s)
		return string(out)
	}
	out, _ := yaml.Marshal(s)
	return strings.TrimSpace(string(out))
}

// seq is a flow sequence of quoted values.
func seq(items []string) string { return "[" + strings.Join(quoteAll(items), ", ") + "]" }

// The manifest's blocks, each as the lines manifestYAML writes for it, so a
// section can show its own lines (plan builder-rework.md §5) and the whole
// file is their composition.
func (m *Model) nameLine() string    { return "name: " + m.Name }
func (m *Model) modesLine() string   { return "modes: [" + strings.Join(compact(m.Modes), ", ") + "]" }
func (m *Model) summaryLine() string { return "summary: " + q(m.Summary) }
func (m *Model) namespaceLine() string {
	return "namespace: " + m.Namespace
}
func (m *Model) versionLine() string { return "version: " + q(strings.TrimSpace(m.Version)) }

func (m *Model) identityLines() []string {
	return []string{m.nameLine(), m.modesLine(), m.summaryLine()}
}

// publishingLines are the namespace and version and then the rest of the
// Publishing section (publishingRest), which the file writes after the
// summary.
func (m *Model) publishingLines() []string {
	return append([]string{m.namespaceLine(), m.versionLine()}, m.publishingRest()...)
}

func (m *Model) publishingRest() []string {
	var out []string
	if m.License != "" {
		out = append(out, "license: "+q(m.License))
	}
	if len(m.Authors) > 0 {
		out = append(out, "authors:")
		for _, a := range m.Authors {
			out = append(out, "  - name: "+q(strings.TrimSpace(a.Name)))
			if e := strings.TrimSpace(a.Email); e != "" {
				out = append(out, "    email: "+q(e))
			}
		}
	}
	if m.Homepage != "" {
		out = append(out, "homepage: "+q(m.Homepage))
	}
	if tags := compact(m.Tags); len(tags) > 0 {
		out = append(out, "tags: "+seq(tags))
	}
	return out
}

func (m *Model) entrypointLines() []string {
	return []string{"entrypoints:", "  instruction: " + q(m.instructionFile())}
}

func (m *Model) interactiveLines() []string {
	opening := strings.TrimSpace(m.Opening)
	if opening == "" && m.IdleTimeoutSeconds == 0 {
		return nil
	}
	out := []string{"interactive:"}
	if opening != "" {
		out = append(out, "  opening: "+q(opening))
	}
	if m.IdleTimeoutSeconds > 0 {
		out = append(out, fmt.Sprintf("  idleTimeoutSeconds: %d", m.IdleTimeoutSeconds))
	}
	return out
}

func (m *Model) intakeLines() []string {
	if len(m.Intake) == 0 {
		return nil
	}
	out := []string{"intake:"}
	for _, qn := range m.Intake {
		out = append(out, "  - id: "+strings.TrimSpace(qn.ID), "    prompt: "+q(strings.TrimSpace(qn.Prompt)))
		if qn.Type != "" && qn.Type != "text" {
			out = append(out, "    type: "+qn.Type)
		}
		if qn.Required {
			out = append(out, "    required: true")
		}
		if qn.Type == "choice" {
			out = append(out, "    choices: "+seq(compact(qn.Choices)))
		}
	}
	return out
}

func (g GuideModel) empty() bool {
	return strings.TrimSpace(g.Overview) == "" && len(compact(g.Prepare)) == 0 && strings.TrimSpace(g.Files) == "" &&
		strings.TrimSpace(g.Task) == "" && len(g.Connect) == 0
}

func (m *Model) guideLines() []string {
	g := m.Guide
	if g.empty() {
		return nil
	}
	out := []string{"guide:"}
	if o := strings.TrimSpace(g.Overview); o != "" {
		out = append(out, "  overview: "+q(o))
	}
	if p := compact(g.Prepare); len(p) > 0 {
		out = append(out, "  prepare: "+seq(p))
	}
	if f := strings.TrimSpace(g.Files); f != "" {
		out = append(out, "  files: "+q(f))
	}
	if t := strings.TrimSpace(g.Task); t != "" {
		out = append(out, "  task: "+q(t))
	}
	if len(g.Connect) > 0 {
		out = append(out, "  connect:")
		for _, c := range g.Connect {
			out = append(out, "    "+strings.TrimSpace(c.Secret)+": "+q(strings.TrimSpace(c.Help)))
		}
	}
	return out
}

func (m *Model) runtimeLines() []string {
	models, ifaces := compact(m.RuntimeModels), compact(m.RuntimeInterfaces)
	if len(models) == 0 && len(ifaces) == 0 {
		return nil
	}
	out := []string{"runtime:"}
	if len(models) > 0 {
		out = append(out, "  models: "+seq(models))
	}
	if len(ifaces) > 0 {
		out = append(out, "  interfaces: ["+strings.Join(ifaces, ", ")+"]")
	}
	return out
}

// The capability block's two halves: where the QuickAgent and its servers
// reach (Tools and web access), and the rest of the contract (Files,
// commands and data). Indented under capabilities:.
func (m *Model) networkLines() []string {
	switch m.EgressMode {
	case "all":
		return []string{"  network:", `    egress: "*"`}
	case "hosts":
		return []string{"  network:", "    egress: " + seq(compact(m.EgressHosts))}
	}
	return []string{"  network:", "    egress: none"}
}

func (m *Model) serverLines() []string {
	if len(m.MCPServers) == 0 {
		return nil
	}
	out := []string{"  mcpServers:"}
	for _, srv := range m.MCPServers {
		out = append(out, "    - "+srv.Source+": "+q(strings.TrimSpace(srv.Package)), "      version: "+q(strings.TrimSpace(srv.Version)))
		if tools := compact(srv.Tools); len(tools) > 0 {
			out = append(out, "      tools: "+seq(tools))
		}
		if secs := compact(srv.Secrets); len(secs) > 0 {
			out = append(out, "      secrets: "+seq(secs))
		}
		if eg := compact(srv.Egress); len(eg) > 0 {
			out = append(out, "      egress: "+seq(eg))
		}
		if len(srv.Env) > 0 {
			out = append(out, "      env:")
			for _, e := range srv.Env {
				out = append(out, "        "+strings.TrimSpace(e.Name)+": "+q(e.Value))
			}
		}
	}
	return out
}

func (m *Model) accessLines() []string {
	out := []string{"  filesystem:", "    read: " + fsValue(m.FSRead, m.FSReadPaths), "    write: " + fsValue(m.FSWrite, m.FSWritePaths)}
	if cmds := compact(m.ExecCommands); len(cmds) > 0 {
		out = append(out, "  exec: { commands: "+seq(cmds)+" }")
	} else {
		out = append(out, "  exec: none")
	}
	if secs := compact(m.Secrets); len(secs) > 0 {
		out = append(out, "  secrets: "+seq(secs))
	}
	if da := compact(m.DataAccess); len(da) > 0 {
		out = append(out, "  dataAccess: ["+strings.Join(da, ", ")+"]")
	}
	return out
}

// knowledgeLines is the knowledge block: accepts when the form asks for a
// pack, and the ships list a loaded manifest carried.
func (m *Model) knowledgeLines() []string {
	prompt, ships := strings.TrimSpace(m.KnowledgePrompt), compact(m.KnowledgeShips)
	if prompt == "" && len(ships) == 0 {
		return nil
	}
	out := []string{"knowledge:"}
	if prompt != "" {
		out = append(out, "  accepts:", "    prompt: "+q(prompt))
		if m.KnowledgeRequired {
			out = append(out, "    required: true")
		}
	}
	if len(ships) > 0 {
		out = append(out, "  ships:")
		for _, s := range ships {
			out = append(out, "    - name: "+q(s))
		}
	}
	return out
}

func (m *Model) dependencyLines() []string {
	if len(m.Dependencies) == 0 {
		return nil
	}
	out := []string{"dependencies:", "  agents:"}
	for _, d := range m.Dependencies {
		out = append(out, "    - ref: "+q(strings.TrimSpace(d.Ref)))
		if v := strings.TrimSpace(d.Version); v != "" {
			out = append(out, "      version: "+q(v))
		}
	}
	return out
}

func (m *Model) publishLines() []string {
	include := compact(m.PublishInclude)
	if len(include) == 0 {
		include = m.defaultInclude()
	}
	out := []string{"publish:", "  include: " + seq(include)}
	if c := strings.TrimSpace(m.PublishChannel); c != "" {
		out = append(out, "  channel: "+c)
	}
	return out
}

// fragment is what one section writes to agent.yaml.
func (m *Model) fragment(sec Section) string {
	var lines []string
	switch sec {
	case SectionBasics:
		lines = append(m.identityLines(), m.interactiveLines()...)
	case SectionInstructions:
		lines = m.entrypointLines()
	case SectionTools:
		lines = append(append([]string{"capabilities:"}, m.networkLines()...), m.serverLines()...)
	case SectionAccess:
		lines = append([]string{"capabilities:"}, m.accessLines()...)
	case SectionQuestions:
		lines = m.intakeLines()
	case SectionGuide:
		lines = append(m.guideLines(), m.knowledgeLines()...)
	case SectionPublish:
		lines = append(m.publishingLines(), m.dependencyLines()...)
	}
	return strings.Join(lines, "\n")
}

// manifestYAML renders the model as agent.yaml. Rendered by hand rather
// than yaml.Marshal of manifest.Manifest: the manifest type's union fields
// have custom decode shapes, and the file should read like the template
// repo — stable key order, the contract last.
func (m *Model) manifestYAML() string {
	var out []string
	block := func(lines []string) {
		if len(lines) > 0 {
			out = append(out, "")
			out = append(out, lines...)
		}
	}
	// The template repo's order: identity, then the entrypoint, intake, the
	// guide, interactive hints, the runtime, the contract, dependencies and
	// publish.
	out = append(out, `schemaVersion: "2"`, m.nameLine(), m.namespaceLine(), m.modesLine(), m.versionLine(), m.summaryLine())
	out = append(out, m.publishingRest()...)
	block(m.entrypointLines())
	block(m.intakeLines())
	block(m.guideLines())
	block(m.interactiveLines())
	block(m.runtimeLines())
	caps := append([]string{"capabilities:"}, m.networkLines()...)
	caps = append(caps, m.accessLines()...)
	block(append(caps, m.serverLines()...))
	block(m.knowledgeLines())
	block(m.dependencyLines())
	block(m.publishLines())
	return strings.Join(out, "\n") + "\n"
}

func fsValue(mode string, paths []string) string {
	switch mode {
	case "workspace":
		return "workspace"
	case "paths":
		return "{ paths: " + seq(compact(paths)) + " }"
	default:
		return "none"
	}
}

// quoteAll YAML-quotes each item ("*" is a YAML alias marker unquoted).
func quoteAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, q(s))
	}
	return out
}

func compact(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Save writes the workspace: agent.yaml and the entrypoint always (this IS
// the editor for them); README.md, LICENSE, and .gitignore only when
// absent, so hand edits there survive.
func Save(dir string, m *Model) error {
	if err := m.Validate(); err != nil {
		return err
	}
	// Belt to the grammars above: a file the form writes must parse back as
	// the closed schema, so nothing typed can change its shape unseen.
	if _, err := manifest.Parse([]byte(m.manifestYAML())); err != nil {
		return fmt.Errorf("the agent.yaml the form would write does not parse: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entry := m.instructionFile()
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(m.manifestYAML()), 0o644); err != nil { //nolint:gosec // workspace files are world-readable by design
		return err
	}
	instructions := m.Instructions
	if !strings.HasSuffix(instructions, "\n") {
		instructions += "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, entry), []byte(instructions), 0o644); err != nil { //nolint:gosec
		return err
	}
	onlyIfAbsent := map[string]string{
		"README.md":  fmt.Sprintf("# %s\n\n%s\n", m.Name, m.Summary),
		"LICENSE":    mitLicense(m.Namespace),
		".gitignore": "agentry-out/\n",
	}
	// Deterministic creation order keeps logs and tests stable.
	names := make([]string, 0, len(onlyIfAbsent))
	for n := range onlyIfAbsent {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, []byte(onlyIfAbsent[n]), 0o644); err != nil { //nolint:gosec
			return err
		}
	}
	return nil
}

// Load reads dir/agent.yaml (+ the entrypoint) back into the form model.
// It parses the YAML shape directly — the manifest package's unions are a
// decode contract — and reads every field of schema v2, so a save writes
// back what the file declared (D19). Only comments are lost, and
// HadComments says so.
// maxManifestBytes caps the agent.yaml Load reads; a manifest is a page of
// YAML, and the registry's own text quota is far below this.
const maxManifestBytes = 1 << 20

func Load(dir string) (*Model, error) {
	p := filepath.Join(dir, "agent.yaml")
	if fi, err := os.Stat(p); err == nil && fi.Size() > maxManifestBytes {
		return nil, fmt.Errorf("agent.yaml is %d bytes; the Build surface reads at most %d", fi.Size(), maxManifestBytes)
	}
	raw, err := os.ReadFile(p) //nolint:gosec // user-picked workspace
	if err != nil {
		return nil, fmt.Errorf("read agent.yaml: %w", err)
	}
	m, err := decodeManifest(raw)
	if err != nil {
		return nil, fmt.Errorf("parse agent.yaml: %w", err)
	}
	m.HadComments = hasComments(raw)

	// The instruction file is read only from the picked folder itself: an
	// entrypoint naming a path outside it is not followed (the form's rules
	// then refuse to save it until it names a file at the root).
	if instructionFileRE.MatchString(m.instructionFile()) && !strings.Contains(m.instructionFile(), "..") {
		if b, rerr := os.ReadFile(filepath.Join(dir, m.instructionFile())); rerr == nil { //nolint:gosec
			m.Instructions = string(b)
		}
	}
	return m, nil
}

// decodeManifest reads a manifest's YAML shape (JSON is YAML too, which is
// how a draft's manifest comes) into the form's Model: every field of schema
// v2, and a v1 kind as its mode. It reads no file. A value of a shape the
// field cannot take (an object where a string belongs) is an error.
func decodeManifest(raw []byte) (*Model, error) {
	var doc struct {
		SchemaVersion string   `yaml:"schemaVersion"`
		Name          string   `yaml:"name"`
		Namespace     string   `yaml:"namespace"`
		Kind          string   `yaml:"kind"` // v1 only
		Modes         []string `yaml:"modes"`
		Version       string   `yaml:"version"`
		Authors       []struct {
			Name  string `yaml:"name"`
			Email string `yaml:"email"`
		} `yaml:"authors"`
		Entrypoints struct {
			Skill       string `yaml:"skill"`
			Instruction string `yaml:"instruction"`
		} `yaml:"entrypoints"`
		Intake []struct {
			ID       string   `yaml:"id"`
			Prompt   string   `yaml:"prompt"`
			Type     string   `yaml:"type"`
			Required bool     `yaml:"required"`
			Choices  []string `yaml:"choices"`
		} `yaml:"intake"`
		Interactive struct {
			Opening            string `yaml:"opening"`
			IdleTimeoutSeconds int    `yaml:"idleTimeoutSeconds"`
		} `yaml:"interactive"`
		Guide struct {
			Overview string            `yaml:"overview"`
			Prepare  []string          `yaml:"prepare"`
			Files    string            `yaml:"files"`
			Task     string            `yaml:"task"`
			Connect  map[string]string `yaml:"connect"`
		} `yaml:"guide"`
		Runtime struct {
			Models     []string `yaml:"models"`
			Interfaces []string `yaml:"interfaces"`
		} `yaml:"runtime"`
		Summary      string   `yaml:"summary"`
		License      string   `yaml:"license"`
		Homepage     string   `yaml:"homepage"`
		Tags         []string `yaml:"tags"`
		Capabilities struct {
			Network struct {
				Egress yaml.Node `yaml:"egress"`
			} `yaml:"network"`
			Filesystem struct {
				Read  yaml.Node `yaml:"read"`
				Write yaml.Node `yaml:"write"`
			} `yaml:"filesystem"`
			Exec       yaml.Node `yaml:"exec"`
			Secrets    []string  `yaml:"secrets"`
			DataAccess []string  `yaml:"dataAccess"`
			MCPServers []struct {
				NPM     string            `yaml:"npm"`
				PyPI    string            `yaml:"pypi"`
				Version string            `yaml:"version"`
				Tools   []string          `yaml:"tools"`
				Secrets []string          `yaml:"secrets"`
				Egress  []string          `yaml:"egress"`
				Env     map[string]string `yaml:"env"`
			} `yaml:"mcpServers"`
		} `yaml:"capabilities"`
		Knowledge struct {
			Accepts struct {
				Prompt   string `yaml:"prompt"`
				Required bool   `yaml:"required"`
			} `yaml:"accepts"`
			Ships []struct {
				Name string `yaml:"name"`
			} `yaml:"ships"`
		} `yaml:"knowledge"`
		Dependencies struct {
			Agents []struct {
				Ref     string `yaml:"ref"`
				Version string `yaml:"version"`
			} `yaml:"agents"`
		} `yaml:"dependencies"`
		Publish struct {
			Include []string `yaml:"include"`
			Channel string   `yaml:"channel"`
		} `yaml:"publish"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	m := Default(doc.Namespace)
	m.Name, m.Version = doc.Name, doc.Version
	switch {
	case len(doc.Modes) > 0:
		m.Modes = doc.Modes
		if doc.Entrypoints.Instruction != "" {
			m.InstructionFile = doc.Entrypoints.Instruction
		}
	case doc.Kind != "":
		// A v1 workspace: kinds became modes (DEV-088); the skill file
		// stays the instruction file so nothing moves on save.
		m.Modes = []string{"autonomous"}
		switch doc.Kind {
		case "skill":
			m.InstructionFile = "SKILL.md"
			if doc.Entrypoints.Skill != "" {
				m.InstructionFile = doc.Entrypoints.Skill
			}
		case "prompt-agent":
			m.InstructionFile = "AGENT.md"
		default:
			return nil, fmt.Errorf("the manifest declares the withdrawn kind %q; the Build surface authors instruction-file QuickAgents", doc.Kind)
		}
	}
	for _, a := range doc.Authors {
		m.Authors = append(m.Authors, AuthorRow{Name: a.Name, Email: a.Email})
	}
	for _, qn := range doc.Intake {
		m.Intake = append(m.Intake, IntakeRow{ID: qn.ID, Prompt: qn.Prompt, Type: qn.Type, Required: qn.Required, Choices: orEmpty(qn.Choices)})
	}
	m.Opening, m.IdleTimeoutSeconds = doc.Interactive.Opening, doc.Interactive.IdleTimeoutSeconds
	m.Guide = GuideModel{Overview: doc.Guide.Overview, Prepare: orEmpty(doc.Guide.Prepare), Files: doc.Guide.Files, Task: doc.Guide.Task, Connect: []ConnectRow{}}
	for _, name := range sortedKeys(doc.Guide.Connect) {
		m.Guide.Connect = append(m.Guide.Connect, ConnectRow{Secret: name, Help: doc.Guide.Connect[name]})
	}
	m.RuntimeModels, m.RuntimeInterfaces = orEmpty(doc.Runtime.Models), orEmpty(doc.Runtime.Interfaces)
	m.Summary, m.License, m.Homepage = doc.Summary, doc.License, doc.Homepage
	if doc.Tags != nil {
		m.Tags = doc.Tags
	}
	if doc.Capabilities.Secrets != nil {
		m.Secrets = doc.Capabilities.Secrets
	}
	if doc.Capabilities.DataAccess != nil {
		m.DataAccess = doc.Capabilities.DataAccess
	}
	m.EgressMode, m.EgressHosts = decodeEgress(doc.Capabilities.Network.Egress)
	m.FSRead, m.FSReadPaths = decodeFS(doc.Capabilities.Filesystem.Read)
	m.FSWrite, m.FSWritePaths = decodeFS(doc.Capabilities.Filesystem.Write)
	m.ExecCommands = decodeExec(doc.Capabilities.Exec)
	for _, srv := range doc.Capabilities.MCPServers {
		row := MCPServerRow{Source: "npm", Package: srv.NPM, Version: srv.Version,
			Tools: orEmpty(srv.Tools), Secrets: orEmpty(srv.Secrets), Egress: orEmpty(srv.Egress), Env: []EnvRow{}}
		if srv.PyPI != "" {
			row.Source, row.Package = "pypi", srv.PyPI
		}
		for _, name := range sortedKeys(srv.Env) {
			row.Env = append(row.Env, EnvRow{Name: name, Value: srv.Env[name]})
		}
		m.MCPServers = append(m.MCPServers, row)
	}
	m.KnowledgePrompt, m.KnowledgeRequired = doc.Knowledge.Accepts.Prompt, doc.Knowledge.Accepts.Required
	for _, sh := range doc.Knowledge.Ships {
		m.KnowledgeShips = append(m.KnowledgeShips, sh.Name)
	}
	for _, d := range doc.Dependencies.Agents {
		m.Dependencies = append(m.Dependencies, DependencyRow{Ref: d.Ref, Version: d.Version})
	}
	m.PublishInclude, m.PublishChannel = orEmpty(doc.Publish.Include), doc.Publish.Channel
	return m, nil
}

// hasComments reports whether a YAML document carries any comment.
func hasComments(raw []byte) bool {
	var root yaml.Node
	if yaml.Unmarshal(raw, &root) != nil {
		return false
	}
	var walk func(n *yaml.Node) bool
	walk = func(n *yaml.Node) bool {
		if n.HeadComment != "" || n.LineComment != "" || n.FootComment != "" {
			return true
		}
		for _, c := range n.Content {
			if walk(c) {
				return true
			}
		}
		return false
	}
	return walk(&root)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// orEmpty keeps JSON arrays arrays — the form maps over them.
func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func decodeEgress(n yaml.Node) (string, []string) {
	switch n.Kind {
	case yaml.SequenceNode:
		var hosts []string
		_ = n.Decode(&hosts)
		return "hosts", hosts
	case yaml.ScalarNode:
		if n.Value == "*" {
			return "all", []string{}
		}
	}
	return "none", []string{}
}

func decodeFS(n yaml.Node) (string, []string) {
	switch n.Kind {
	case yaml.MappingNode:
		var m struct {
			Paths []string `yaml:"paths"`
		}
		_ = n.Decode(&m)
		return "paths", m.Paths
	case yaml.ScalarNode:
		if n.Value == "workspace" {
			return "workspace", []string{}
		}
	}
	return "none", []string{}
}

func decodeExec(n yaml.Node) []string {
	if n.Kind == yaml.MappingNode {
		var m struct {
			Commands []string `yaml:"commands"`
		}
		_ = n.Decode(&m)
		return m.Commands
	}
	return []string{}
}

// PublishResult is the pipeline's verdict, flattened for the UI.
type PublishResult struct {
	Version string `json:"version"`
	// Status is the pipeline's verdict — published | needs_review |
	// rejected — or "refused" when the registry declined the submission
	// before storing anything (a spent credit allowance: 402
	// credits-exhausted, plan subscription-tiers.md §8); Detail then
	// carries the refusal's sentence and Refusal its slug.
	Status  string `json:"status"`
	Digest  string `json:"digest"`
	Detail  string `json:"detail,omitempty"`
	Refusal string `json:"refusal,omitempty"`
	// Account names whose subscription the refusal is about. A member
	// publishes to their own namespace but spends from their
	// organization's pool (plan organization-subscriptions.md D22), so
	// the account that is short is often not the one that signed in.
	Account       string         `json:"account,omitempty"`
	FindingCounts map[string]int `json:"findingCounts,omitempty"`
	// Visibility is the agent's once the submission is through: public, or
	// private for an agent that already was, which VisibilityNote then says
	// in words; SettingsURL is the site's page that changes it, set by the
	// desktop's binding where the registry is the site (agentry-desktop#425).
	Visibility     string `json:"visibility,omitempty"`
	VisibilityNote string `json:"visibilityNote,omitempty"`
	SettingsURL    string `json:"settingsUrl,omitempty"`
}

// StatusRefused is the PublishResult status of a submission the registry
// declined before storing it.
const StatusRefused = "refused"

// ReadManifest parses the workspace's agent.yaml.
func ReadManifest(dir string) (*manifest.Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "agent.yaml")) //nolint:gosec // user-picked workspace
	if err != nil {
		return nil, fmt.Errorf("read agent.yaml: %w", err)
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse agent.yaml: %w", err)
	}
	return m, nil
}

// ExistsPrivate says whether ns/name exists on the registry as a private
// agent, which a Submit leaves private (agentry-desktop#425).
func ExistsPrivate(ctx context.Context, regURL, token, ns, name string) (bool, error) {
	client, err := registry.New(regURL, token)
	if err != nil {
		return false, err
	}
	resp, err := client.GetAgentWithResponse(ctx, ns, name)
	if err != nil {
		return false, fmt.Errorf("check agent: %w", err)
	}
	switch resp.StatusCode() {
	case http.StatusOK:
		return resp.JSON200 != nil && resp.JSON200.Visibility == apiclient.AgentVisibilityPrivate, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, registry.Problem("check agent", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
}

// Publish submits the QuickAgent publicly (agentry-desktop#425, reversing
// builder-simplify D3's private default): it creates the agent public if
// it does not exist, uploads the packed archive (cli-upload source), and
// follows the pipeline to a verdict — the port of `agentry publish`
// without --tag. An agent that exists private stays private: making it
// public is its namespace administrators' act, which a desktop sign-in
// does not carry (DEV-078, D19; the operator's decision of 2026-09-30), so
// the result says so and names its settings page on the site. A registry
// that stores nothing and says why (credits, the namespace, the
// private-agent cap, an unverified email) is a refused result in words,
// not an error. Progress lines go to logw. privateTold is what the
// person's warning said of the agent: one told it stays private that is
// now public or gone would take the version public, so that is refused
// before anything is sent.
func Publish(ctx context.Context, regURL, token, dir string, privateTold bool, logw io.Writer) (*PublishResult, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("publishing needs a signed-in registry — sign in under Settings → Registry settings")
	}
	client, err := registry.New(regURL, token)
	if err != nil {
		return nil, err
	}

	// Ensure the agent record exists (the registry refuses versions for
	// agents nobody created — the smoke flow's explicit POST /agents).
	aresp, err := client.GetAgentWithResponse(ctx, m.Namespace, m.Name)
	if err != nil {
		return nil, fmt.Errorf("check agent: %w", err)
	}
	private := aresp.StatusCode() == http.StatusOK && aresp.JSON200 != nil && aresp.JSON200.Visibility == apiclient.AgentVisibilityPrivate
	if privateTold && !private {
		msg := fmt.Sprintf("%s/%s is no longer private, and this version would be public, which the warning did not say; submit again to be asked", m.Namespace, m.Name)
		_, _ = fmt.Fprintf(logw, "refused: %s\n", msg)
		return &PublishResult{Status: StatusRefused, Refusal: "visibility-changed", Detail: msg}, nil
	}
	switch aresp.StatusCode() {
	case http.StatusOK:
		if private {
			_, _ = fmt.Fprintf(logw, "agent %s/%s exists and is private: this version stays private with it\n", m.Namespace, m.Name)
		} else {
			_, _ = fmt.Fprintf(logw, "agent %s/%s exists\n", m.Namespace, m.Name)
		}
	case http.StatusNotFound:
		vis := apiclient.CreateAgentRequestVisibilityPublic
		summary := m.Summary
		modes := make(apiclient.AgentModes, 0, len(m.SupportedModes()))
		for _, mode := range m.SupportedModes() {
			modes = append(modes, apiclient.AgentMode(mode))
		}
		body := apiclient.CreateAgentJSONRequestBody{
			Namespace: m.Namespace, Name: m.Name,
			Modes: modes, Summary: &summary, Visibility: &vis,
		}
		cresp, cerr := client.CreateAgentWithResponse(ctx, body)
		if cerr != nil {
			return nil, fmt.Errorf("create agent: %w", cerr)
		}
		if cresp.StatusCode() != http.StatusCreated && cresp.StatusCode() != http.StatusOK {
			if refused := refusedWithReason(cresp.StatusCode(), cresp.Body, logw); refused != nil {
				return refused, nil
			}
			return nil, registry.Problem("create agent", cresp.StatusCode(), cresp.ApplicationproblemJSONDefault, cresp.Body)
		}
		_, _ = fmt.Fprintf(logw, "created agent %s/%s (public)\n", m.Namespace, m.Name)
	default:
		return nil, registry.Problem("check agent", aresp.StatusCode(), aresp.ApplicationproblemJSONDefault, aresp.Body)
	}

	// Pack the deterministic archive from publish.include (§6.5).
	files, err := pkgarchive.WalkWorkspace(dir)
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}
	include := manifest.ResolveInclude(m.Publish.Include, files)
	if len(include) == 0 {
		return nil, fmt.Errorf("publish.include matches no files (V4)")
	}
	var archive bytes.Buffer
	info, err := pkgarchive.BuildArchive(dir, include, &archive)
	if err != nil {
		return nil, fmt.Errorf("build archive: %w", err)
	}
	_, _ = fmt.Fprintf(logw, "submitting %s/%s %s: %d file(s), %d bytes, %s\n",
		m.Namespace, m.Name, m.Version, len(info.Files), info.Size, info.Digest)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("archive", "agent.tar.zst")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		return nil, err
	}
	if err := mw.WriteField("digest", info.Digest); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	sresp, err := client.SubmitVersionWithBodyWithResponse(ctx, m.Namespace, m.Name, mw.FormDataContentType(), &body)
	if err != nil {
		return nil, fmt.Errorf("upload version: %w", err)
	}
	if sresp.JSON202 == nil {
		if refused := refusedWithReason(sresp.StatusCode(), sresp.Body, logw); refused != nil {
			return refused, nil
		}
		if sresp.StatusCode() == http.StatusPaymentRequired {
			// The allowance's refusal is a result the page renders with the
			// way onward, not a failure: the registry stored nothing and
			// says why.
			slug, msg, account := refusalOf(sresp.Body)
			_, _ = fmt.Fprintf(logw, "refused: %s\n", msg)
			return &PublishResult{Status: StatusRefused, Refusal: slug, Detail: msg, Account: account}, nil
		}
		return nil, registry.Problem("submit version", sresp.StatusCode(), sresp.ApplicationproblemJSONDefault, sresp.Body)
	}
	v := sresp.JSON202
	_, _ = fmt.Fprintf(logw, "version %s accepted (status %s) — waiting for the pipeline verdict\n", v.Version, v.Status)

	// Poll to a terminal state (SSE deferred, like the CLI).
	deadline := time.Now().Add(5 * time.Minute)
	status := string(v.Status)
	for status == "submitted" || status == "scanning" {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("version %s still %s after 5m — check the agent page later", m.Version, status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
		iresp, ierr := client.GetVersionInfoWithResponse(ctx, m.Namespace, m.Name, m.Version)
		if ierr != nil {
			return nil, fmt.Errorf("poll version: %w", ierr)
		}
		if iresp.JSON200 == nil {
			return nil, registry.Problem("poll version", iresp.StatusCode(), iresp.ApplicationproblemJSONDefault, iresp.Body)
		}
		if next := string(iresp.JSON200.Status); next != status {
			status = next
			_, _ = fmt.Fprintf(logw, "status: %s\n", status)
		}
	}

	res := &PublishResult{Version: m.Version, Status: status, Digest: info.Digest, Visibility: "public"}
	if private {
		res.Visibility, res.VisibilityNote = "private", PrivateStaysNote
	}
	switch status {
	case "published":
		res.Detail = fmt.Sprintf("published %s/%s@%s", m.Namespace, m.Name, m.Version)
	case "needs_review":
		res.Detail = "findings exceed the auto-publish threshold — a reviewer decides whether it publishes"
	case "rejected":
		res.Detail = "rejected by the security pipeline"
	}
	if rresp, rerr := client.GetVersionReportWithResponse(ctx, m.Namespace, m.Name, m.Version); rerr == nil && rresp.JSON200 != nil && rresp.JSON200.FindingCounts != nil {
		res.FindingCounts = *rresp.JSON200.FindingCounts
	}
	return res, nil
}

// mitLicense is the full MIT text — a real license file, not a placeholder
// the pipeline would flag on public publication.
func mitLicense(holder string) string {
	return fmt.Sprintf(`MIT License

Copyright (c) %d %s

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`, time.Now().Year(), holder)
}

// PrivateStaysNote is what a submission to an agent that exists private
// says: it stays private, and why.
const PrivateStaysNote = "it stays private: making a QuickAgent public is its namespace administrators' act, which a desktop sign-in does not carry"

// refusedWithReason turns the registry's refusal to create the agent or
// take its version into a refused result in words, for the refusals the
// person can act on: a namespace they may not publish to (403), and the
// private-agent cap (422 private-agent-cap). Anything else is the
// caller's.
func refusedWithReason(status int, body []byte, logw io.Writer) *PublishResult {
	slug, msg, account := refusalOf(body)
	if status != http.StatusForbidden && slug != "private-agent-cap" {
		return nil
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	_, _ = fmt.Fprintf(logw, "refused: %s\n", msg)
	return &PublishResult{Status: StatusRefused, Refusal: slug, Detail: msg, Account: account}
}

// refusalOf reads a problem body's slug and sentence.
func refusalOf(body []byte) (slug, msg, account string) {
	var p struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
		// Account is the billing extension's own member on a 402: the
		// subscription the refusal is about.
		Account string `json:"account"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", strings.TrimSpace(string(body)), ""
	}
	slug = p.Type
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		slug = slug[i+1:]
	}
	msg = p.Title
	if p.Detail != "" {
		msg += ": " + p.Detail
	}
	return slug, msg, p.Account
}
