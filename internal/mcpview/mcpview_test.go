package mcpview

import (
	"reflect"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// A server's env is shown as NAME=value rows sorted by name, beside its pin
// and tools; a server without env has an empty list, not a missing one.
func TestOfListsEachServersPinToolsAndSortedEnv(t *testing.T) {
	c := manifest.Capabilities{MCPServers: []manifest.MCPServer{
		{NPM: "@acme/github-mcp", Version: "1.2.3", Tools: []string{"list_issues"},
			Env: map[string]string{"GITHUB_HOST": "github.example", "API_MODE": "read"}},
		{PyPI: "fetcher", Version: "0.4.0"},
	}}
	got := Of(c)
	want := []Server{
		{ID: "npm:@acme/github-mcp@1.2.3", Tools: []string{"list_issues"},
			Env: []EnvVar{{"API_MODE", "read"}, {"GITHUB_HOST", "github.example"}}},
		{ID: "pypi:fetcher@0.4.0", Tools: []string{}, Env: []EnvVar{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Of = %+v\nwant %+v", got, want)
	}
	if got := Of(manifest.Capabilities{}); got == nil || len(got) != 0 {
		t.Fatalf("no servers: %#v, want an empty list", got)
	}
}

// Plain env only: a name declared as a secret — on the server or at the
// top level — is never shown, whatever value a manifest put beside it.
// (The core refuses such a manifest; this is the second line.)
func TestOfNeverShowsAnEnvNameDeclaredAsASecret(t *testing.T) {
	c := manifest.Capabilities{
		Secrets: []string{"TOP_SECRET"},
		MCPServers: []manifest.MCPServer{{
			NPM: "x", Version: "1.0.0", Secrets: []string{"GITHUB_TOKEN"},
			Env: map[string]string{"GITHUB_TOKEN": "ghp_leak", "TOP_SECRET": "leak", "MODE": "ro"},
		}},
	}
	got := Of(c)[0].Env
	if !reflect.DeepEqual(got, []EnvVar{{"MODE", "ro"}}) {
		t.Fatalf("env = %+v, want only MODE", got)
	}
}
