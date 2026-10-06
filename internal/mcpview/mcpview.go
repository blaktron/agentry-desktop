// Package mcpview is how the desktop shows a release's declared MCP
// servers: each server's pin, its tool allowlist and its plain env
// (agentry-desktop#62). The agent page's "MCP servers & tools" section and
// the install review both read it, so the two cannot describe a server
// differently.
//
// env is configuration, not a secret: the manifest carries it so it is in
// the diff, the digest and the review (core DEV-098), and the core refuses
// an env name that is also a declared secret (V5). A secret's value is
// never in a manifest at all — secrets are names, collected into the MCP
// profile by the runner's guided setup. As a second line, an env entry
// whose name is declared as a secret anyway is left out here, never shown.
package mcpview

import (
	"slices"
	"sort"

	"github.com/blaktron/agentry/pkg/manifest"
)

// EnvVar is one plain env entry.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Server is one declared MCP server.
type Server struct {
	// ID is "source:package@version", the key the CLI's installed record
	// uses for the server (installed.CapSnapshot.MCPServers).
	ID string `json:"id"`
	// Tools is the allowlist; empty grants the server's full set.
	Tools []string `json:"tools"`
	// Env is the server's plain env, sorted by name; empty when it
	// declares none.
	Env []EnvVar `json:"env"`
	// Remote says it is a remote server, reached at its address
	// (agentry-cli#663); ID is then that address.
	Remote bool `json:"remote,omitempty"`
	// SignsIn says it signs in by OAuth: a remote server's auth: oauth, or
	// a package server's oauth block (plan mcp-oauth-connections.md). Its
	// connection is made on the run setup's Connect tools step.
	SignsIn bool `json:"signsIn,omitempty"`
}

// Of lists the declared servers of a capability block, in declaration
// order. Slices are never nil, so the page can map over them.
func Of(c manifest.Capabilities) []Server {
	out := []Server{}
	for _, srv := range c.MCPServers {
		s := Server{
			ID:      srv.SourceID() + "@" + srv.Version,
			Tools:   append([]string{}, srv.Tools...),
			Env:     []EnvVar{},
			SignsIn: srv.NeedsConnection(),
		}
		if srv.IsRemote() {
			// A remote server has no package version: its address is it.
			s.ID, s.Remote = srv.URL, true
		}
		for name, value := range srv.Env {
			if slices.Contains(srv.Secrets, name) || slices.Contains(c.Secrets, name) {
				continue
			}
			s.Env = append(s.Env, EnvVar{Name: name, Value: value})
		}
		sort.Slice(s.Env, func(i, j int) bool { return s.Env[i].Name < s.Env[j].Name })
		out = append(out, s)
	}
	return out
}
