// Package build is the Build surface's engine: author a BriefAgent as a
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
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
)

// Model is the Build form's shape: flat fields that map one-to-one onto
// the manifest's YAML. The YAML is the contract (a closed schema, V-rules
// at validate/publish); this struct is only a convenient editor for it.
type Model struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"` // skill | prompt-agent
	Version   string   `json:"version"`
	Summary   string   `json:"summary"`
	License   string   `json:"license"`
	Homepage  string   `json:"homepage,omitempty"`
	Tags      []string `json:"tags"`

	// Instructions is the entrypoint file's content (SKILL.md / AGENT.md).
	Instructions string `json:"instructions"`

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
}

// entrypointFile maps kind → the instruction file the manifest names.
func entrypointFile(kind string) (string, error) {
	switch kind {
	case "skill":
		return "SKILL.md", nil
	case "prompt-agent":
		return "AGENT.md", nil
	default:
		return "", fmt.Errorf("the Build surface authors skill and prompt-agent BriefAgents; %q needs code and belongs in an editor", kind)
	}
}

// Default returns a fresh form model with the scaffold's defaults.
func Default(namespace string) *Model {
	return &Model{
		Kind: "skill", Version: "0.1.0", License: "MIT", Namespace: namespace,
		EgressMode: "none", FSRead: "workspace", FSWrite: "none",
		Tags: []string{}, EgressHosts: []string{}, FSReadPaths: []string{}, FSWritePaths: []string{},
		ExecCommands: []string{}, Secrets: []string{}, DataAccess: []string{}, MCPServers: []MCPServerRow{},
		Instructions: "# New BriefAgent\n\nDescribe the one job this agent does: task in, result out.\nWrite the report with the write_report tool.\n",
	}
}

// Validate applies the form-level rules before anything touches disk.
func (m *Model) Validate() error {
	if !manifest.ValidName(m.Name) {
		return fmt.Errorf("name %q is invalid (lowercase [a-z0-9-], 1-64 chars)", m.Name)
	}
	if !manifest.ValidNamespace(m.Namespace) {
		return fmt.Errorf("namespace %q is invalid (lowercase [a-z0-9-])", m.Namespace)
	}
	if _, err := entrypointFile(m.Kind); err != nil {
		return err
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required (strict semver, e.g. 0.1.0)")
	}
	if strings.TrimSpace(m.Summary) == "" {
		return fmt.Errorf("summary is required (one line, ≤140 chars)")
	}
	if m.EgressMode == "hosts" && len(compact(m.EgressHosts)) == 0 {
		return fmt.Errorf("egress mode is hosts but no hosts are listed")
	}
	if m.FSRead == "paths" && len(compact(m.FSReadPaths)) == 0 {
		return fmt.Errorf("fs read mode is paths but no paths are listed")
	}
	if m.FSWrite == "paths" && len(compact(m.FSWritePaths)) == 0 {
		return fmt.Errorf("fs write mode is paths but no paths are listed")
	}
	seen := map[string]bool{}
	for i, srv := range m.MCPServers {
		if srv.Source != "npm" && srv.Source != "pypi" {
			return fmt.Errorf("mcp server %d: source must be npm or pypi", i+1)
		}
		pkg := strings.TrimSpace(srv.Package)
		if pkg == "" {
			return fmt.Errorf("mcp server %d: package name is required", i+1)
		}
		if strings.TrimSpace(srv.Version) == "" {
			return fmt.Errorf("mcp server %d (%s): an exact version is required (no ranges)", i+1, pkg)
		}
		if id := srv.Source + ":" + pkg; seen[id] {
			return fmt.Errorf("mcp server %s is declared twice", id)
		} else {
			seen[id] = true
		}
	}
	return nil
}

// manifestYAML renders the model as agent.yaml. Rendered by hand rather
// than yaml.Marshal of manifest.Manifest: the manifest type's union fields
// have custom decode shapes, and the file should read like the template
// repo — stable key order, the contract last.
func (m *Model) manifestYAML() string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	q := func(s string) string {
		out, _ := yaml.Marshal(s)
		return strings.TrimSpace(string(out))
	}
	entry, _ := entrypointFile(m.Kind)

	w("schemaVersion: \"1\"")
	w("name: %s", m.Name)
	w("namespace: %s", m.Namespace)
	w("kind: %s", m.Kind)
	w("version: %s", m.Version)
	w("summary: %s", q(m.Summary))
	if m.License != "" {
		w("license: %s", m.License)
	}
	if m.Homepage != "" {
		w("homepage: %s", q(m.Homepage))
	}
	if tags := compact(m.Tags); len(tags) > 0 {
		w("tags: [%s]", strings.Join(tags, ", "))
	}
	w("")
	w("entrypoints:")
	w("  %s: %s", map[string]string{"skill": "skill", "prompt-agent": "prompt"}[m.Kind], entry)
	w("")
	w("capabilities:")
	w("  network:")
	switch m.EgressMode {
	case "all":
		w("    egress: \"*\"")
	case "hosts":
		w("    egress: [%s]", strings.Join(compact(m.EgressHosts), ", "))
	default:
		w("    egress: none")
	}
	w("  filesystem:")
	w("    read: %s", fsValue(m.FSRead, m.FSReadPaths))
	w("    write: %s", fsValue(m.FSWrite, m.FSWritePaths))
	if cmds := compact(m.ExecCommands); len(cmds) > 0 {
		w("  exec: { commands: [%s] }", strings.Join(cmds, ", "))
	} else {
		w("  exec: none")
	}
	if secs := compact(m.Secrets); len(secs) > 0 {
		w("  secrets: [%s]", strings.Join(secs, ", "))
	}
	if da := compact(m.DataAccess); len(da) > 0 {
		w("  dataAccess: [%s]", strings.Join(da, ", "))
	}
	if len(m.MCPServers) > 0 {
		w("  mcpServers:")
		for _, srv := range m.MCPServers {
			w("    - %s: %s", srv.Source, q(strings.TrimSpace(srv.Package)))
			w("      version: %s", q(strings.TrimSpace(srv.Version)))
			if tools := compact(srv.Tools); len(tools) > 0 {
				w("      tools: [%s]", strings.Join(tools, ", "))
			}
			if secs := compact(srv.Secrets); len(secs) > 0 {
				w("      secrets: [%s]", strings.Join(secs, ", "))
			}
			if eg := compact(srv.Egress); len(eg) > 0 {
				w("      egress: [%s]", strings.Join(quoteAll(eg), ", "))
			}
		}
	}
	w("")
	w("publish:")
	w("  include: [agent.yaml, %s, README.md, LICENSE]", entry)
	return b.String()
}

func fsValue(mode string, paths []string) string {
	switch mode {
	case "workspace":
		return "workspace"
	case "paths":
		return fmt.Sprintf("{ paths: [%s] }", strings.Join(compact(paths), ", "))
	default:
		return "none"
	}
}

// quoteAll YAML-quotes each item ("*" is a YAML alias marker unquoted).
func quoteAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		b, _ := yaml.Marshal(s)
		out = append(out, strings.TrimSpace(string(b)))
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entry, _ := entrypointFile(m.Kind)
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
// decode contract, and the closed schema means unknown keys already failed
// validate — tolerating only what the Build form itself writes.
func Load(dir string) (*Model, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "agent.yaml")) //nolint:gosec // user-picked workspace
	if err != nil {
		return nil, fmt.Errorf("read agent.yaml: %w", err)
	}
	var doc struct {
		Name         string   `yaml:"name"`
		Namespace    string   `yaml:"namespace"`
		Kind         string   `yaml:"kind"`
		Version      string   `yaml:"version"`
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
				NPM     string   `yaml:"npm"`
				PyPI    string   `yaml:"pypi"`
				Version string   `yaml:"version"`
				Tools   []string `yaml:"tools"`
				Secrets []string `yaml:"secrets"`
				Egress  []string `yaml:"egress"`
			} `yaml:"mcpServers"`
		} `yaml:"capabilities"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse agent.yaml: %w", err)
	}
	m := Default(doc.Namespace)
	m.Name, m.Kind, m.Version = doc.Name, doc.Kind, doc.Version
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
			Tools: orEmpty(srv.Tools), Secrets: orEmpty(srv.Secrets), Egress: orEmpty(srv.Egress)}
		if srv.PyPI != "" {
			row.Source, row.Package = "pypi", srv.PyPI
		}
		m.MCPServers = append(m.MCPServers, row)
	}

	if entry, err := entrypointFile(m.Kind); err == nil {
		if b, rerr := os.ReadFile(filepath.Join(dir, entry)); rerr == nil { //nolint:gosec
			m.Instructions = string(b)
		}
	}
	return m, nil
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
	Version       string         `json:"version"`
	Status        string         `json:"status"` // published | needs_review | rejected
	Digest        string         `json:"digest"`
	Detail        string         `json:"detail,omitempty"`
	FindingCounts map[string]int `json:"findingCounts,omitempty"`
}

// Publish creates the agent if it does not exist, uploads the packed
// archive (cli-upload source), and follows the pipeline to a verdict —
// the port of `agentry publish` without --tag. Progress lines go to logw.
func Publish(ctx context.Context, regURL, token, dir, visibility string, logw io.Writer) (*PublishResult, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "agent.yaml")) //nolint:gosec // user-picked workspace
	if err != nil {
		return nil, fmt.Errorf("read agent.yaml: %w", err)
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse agent.yaml: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("publishing needs a signed-in registry — Settings → Sign in")
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
	switch aresp.StatusCode() {
	case http.StatusOK:
		_, _ = fmt.Fprintf(logw, "agent %s/%s exists\n", m.Namespace, m.Name)
	case http.StatusNotFound:
		vis := apiclient.CreateAgentRequestVisibility(visibility)
		summary := m.Summary
		body := apiclient.CreateAgentJSONRequestBody{
			Namespace: m.Namespace, Name: m.Name,
			Kind: apiclient.AgentKind(string(m.Kind)), Summary: &summary, Visibility: &vis,
		}
		cresp, cerr := client.CreateAgentWithResponse(ctx, body)
		if cerr != nil {
			return nil, fmt.Errorf("create agent: %w", cerr)
		}
		if cresp.StatusCode() != http.StatusCreated && cresp.StatusCode() != http.StatusOK {
			return nil, registry.Problem("create agent", cresp.StatusCode(), cresp.ApplicationproblemJSONDefault, cresp.Body)
		}
		_, _ = fmt.Fprintf(logw, "created agent %s/%s (%s)\n", m.Namespace, m.Name, visibility)
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

	res := &PublishResult{Version: m.Version, Status: status, Digest: info.Digest}
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
