package run

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/export"
)

// fullCaps is a contract declaring every capability class.
func fullCaps() manifest.Capabilities {
	return manifest.Capabilities{
		Network:    manifest.Network{Egress: manifest.Egress{Hosts: []string{"api.example.com"}}},
		Filesystem: manifest.Filesystem{Read: manifest.FSAccess{Mode: manifest.FSModeWorkspace}, Write: manifest.FSAccess{Mode: manifest.FSModePaths, Paths: []string{"out/"}}},
		Exec:       manifest.ExecCapability{Commands: []string{"jq", "git"}},
		Secrets:    []string{"EXAMPLE_TOKEN"},
		DataAccess: []manifest.DataAccess{"code"},
	}
}

// toolNames projects rows to their tool names for order assertions.
func toolNames(rows []ToolRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Tool)
	}
	return out
}

func TestGatewayToolRowsPureText(t *testing.T) {
	rows := gatewayToolRows(manifest.Capabilities{})
	// The operator channels exist for every contract (§P4); nothing else.
	want := []string{"write_report", "list_inputs", "read_input"}
	if got := toolNames(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("pure-text tools = %v, want %v", got, want)
	}
}

func TestGatewayToolRowsFullContract(t *testing.T) {
	rows := gatewayToolRows(fullCaps())
	want := []string{"write_report", "list_inputs", "read_input", "fs_read", "fs_list", "fs_write", "fetch", "run_command"}
	if got := toolNames(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("full-contract tools = %v, want %v", got, want)
	}
	for _, r := range rows {
		switch r.Tool {
		case "fetch":
			if !strings.Contains(r.GrantedBy, "api.example.com") {
				t.Errorf("fetch grantedBy %q does not name the pinned hosts", r.GrantedBy)
			}
		case "run_command":
			if !strings.Contains(r.GrantedBy, "jq, git") {
				t.Errorf("run_command grantedBy %q does not name the commands", r.GrantedBy)
			}
		case "fs_write":
			if !strings.Contains(r.GrantedBy, "only out/") {
				t.Errorf("fs_write grantedBy %q does not name the declared paths", r.GrantedBy)
			}
		}
	}
}

func TestGatewayToolRowsWildcardEgress(t *testing.T) {
	c := manifest.Capabilities{Network: manifest.Network{Egress: manifest.Egress{Hosts: []string{"*"}}}}
	rows := gatewayToolRows(c)
	var fetch *ToolRow
	for i := range rows {
		if rows[i].Tool == "fetch" {
			fetch = &rows[i]
		}
	}
	if fetch == nil {
		t.Fatal("wildcard egress should still grant fetch (policy-gated at run time)")
	}
	if !strings.Contains(fetch.GrantedBy, "refused at run time unless") {
		t.Errorf("wildcard grantedBy %q must say it is policy-gated", fetch.GrantedBy)
	}
}

func TestGatewayToolRowsEgressNoneScalar(t *testing.T) {
	c := manifest.Capabilities{Network: manifest.Network{Egress: manifest.Egress{None: true}}}
	for _, r := range gatewayToolRows(c) {
		if r.Tool == "fetch" {
			t.Fatal("egress: none must not grant fetch")
		}
	}
}

// TestEmbeddedMatchesExporter pins req-4 parity: the section's embedded
// list IS the exporter's mapping, not a re-implementation of it.
func TestEmbeddedMatchesExporter(t *testing.T) {
	c := fullCaps()
	got := export.HostToolGrants(c)
	want := []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash(git:*)", "Bash(jq:*)", "WebFetch(domain:api.example.com)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HostToolGrants = %v, want %v (the exporter mapping changed — update the section copy)", got, want)
	}
}

// TestRequirementsFromRegistry pins the §P6 req-3/4 section end to end:
// the resolved release's declared contract drives every field, and the
// embedded list is the exporter's own mapping.
func TestRequirementsFromRegistry(t *testing.T) {
	isolateRunEnv(t)
	fr := &fakeRunRegistry{}
	regURL := startRunRegistry(t, fr)

	info, err := Requirements(context.Background(), regURL, "acme", "reporter")
	if err != nil {
		t.Fatal(err)
	}
	if info.Ref != "acme/reporter@1.0.0" || info.Version != "1.0.0" {
		t.Fatalf("identity = %+v", info)
	}
	want := []string{"write_report", "list_inputs", "read_input", "fs_read", "fs_list", "fs_write", "fetch", "run_command"}
	if got := toolNames(info.ToolRows); !reflect.DeepEqual(got, want) {
		t.Fatalf("tool rows = %v, want %v", got, want)
	}
	if len(info.Servers) != 1 || !strings.Contains(info.Servers[0], "BriefAgent Tools") {
		t.Errorf("servers = %v", info.Servers)
	}
	if !reflect.DeepEqual(info.SecretNames, []string{"RUN_TEST_TOKEN"}) {
		t.Errorf("secret names = %v", info.SecretNames)
	}
	if !reflect.DeepEqual(info.DataAccess, []string{"code"}) {
		t.Errorf("dataAccess = %v", info.DataAccess)
	}
	if len(info.EmbeddedTools) == 0 || len(info.CapabilityLines) != 6 {
		t.Errorf("embedded/capability sections = %v / %v", info.EmbeddedTools, info.CapabilityLines)
	}
}

func TestCapabilityLines(t *testing.T) {
	if got := capabilityLines(manifest.Capabilities{}); len(got) != 1 || got[0] != "none declared (pure text agent)" {
		t.Fatalf("zero contract lines = %v", got)
	}
	lines := strings.Join(capabilityLines(fullCaps()), "\n")
	for _, want := range []string{
		"network.egress: api.example.com",
		"filesystem.read: workspace",
		"filesystem.write: paths: out/",
		"exec: commands: jq, git",
		"secrets: EXAMPLE_TOKEN",
		"dataAccess: code",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("capability lines missing %q in:\n%s", want, lines)
		}
	}
	wild := strings.Join(capabilityLines(manifest.Capabilities{
		Network: manifest.Network{Egress: manifest.Egress{Hosts: []string{"*"}}},
	}), "\n")
	if !strings.Contains(wild, "unrestricted — policy-gated") {
		t.Errorf("wildcard egress must render as policy-gated, got:\n%s", wild)
	}
}
