package run

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// Sweep removes exactly the run's labelled containers, then its volumes,
// then every network a run can make — and nothing it was not asked about.
func TestSweepRemovesTheRunsLabelledObjectsInOrder(t *testing.T) {
	isolateRunEnv(t)
	fake := &dockerx.Fake{
		ByLabel:         map[string][]string{"agentry.run=ab12cd34": {"agentry-ab12cd34-agent", "agentry-ab12cd34-gateway"}},
		LeftoverVolumes: []string{"agentry-ab12cd34-session"},
	}
	withFakeDocker(t, fake)

	if err := Sweep(context.Background(), "ab12cd34"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range fake.Calls {
		got = append(got, c.Op+" "+c.Arg)
	}
	want := []string{
		"PSByLabel agentry.run=ab12cd34",
		"RemoveForce agentry-ab12cd34-agent",
		"RemoveForce agentry-ab12cd34-gateway",
		"VolumesByLabel agentry.run=ab12cd34",
		"VolumeRemove agentry-ab12cd34-session",
		"NetworkRemove agentry-ab12cd34-net",
		"NetworkRemove agentry-ab12cd34-egress",
		"NetworkRemove agentry-ab12cd34-web",
		"NetworkRemove agentry-ab12cd34-relays",
		"NetworkRemove agentry-ab12cd34-relays-out",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// With no run id there is no label to sweep by; an empty label would match
// every run on the machine, the CLI's included. And an id that is not the
// engine's is refused before it becomes a label, an object name or a path.
func TestSweepRefusesARunIDThatIsNotTheEngines(t *testing.T) {
	home := isolateRunEnv(t)
	outside := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-abc", "workspace")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &dockerx.Fake{}
	withFakeDocker(t, fake)
	for _, id := range []string{"", "../workspace", "AB12CD34", "ab12", "ab12cd34/..", "*"} {
		if err := Sweep(context.Background(), id); err == nil {
			t.Fatalf("swept the run id %q", id)
		}
		if err := SweepFiles(home, id); err == nil {
			t.Fatalf("swept the files of the run id %q", id)
		}
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("touched docker: %v", fake.Calls)
	}
	if !exists(outside) {
		t.Fatal("a refused run id removed a directory")
	}
}

// The engine's names for a run's networks are its own, and the sweep has
// fallen behind them before: it named one network while the engine had come
// to make five (agentry-desktop#463). So the engine's source is read, as
// internal/applog reads Wails': every name the engine builds from a run id,
// and every place it makes a network, is one this test knows, and a pin that
// adds either fails here until runNetworks follows.
func TestSweepNamesEveryNetworkTheEngineMakes(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/blaktron/agentry-cli").Output()
	if err != nil {
		t.Fatalf("locate the agentry-cli module: %v", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "pkg", "runner", "engine")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("read the engine's source in %s: %v", dir, err)
	}
	named := regexp.MustCompile(`"agentry-" \+ \w+(?:\.\w+)? \+ "(-[a-z][a-z-]*)"`)
	suffixes := map[string]bool{}
	creates := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range named.FindAllStringSubmatch(string(src), -1) {
			suffixes[m[1]] = true
		}
		creates += strings.Count(string(src), ".NetworkCreate(")
	}
	var got []string
	for s := range suffixes {
		got = append(got, s)
	}
	sort.Strings(got)
	// The five networks, and the session store's volume, which the sweep
	// finds by its label.
	want := []string{"-egress", "-net", "-relays", "-relays-out", "-session", "-web"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the engine names %v from a run id, and the sweep knows %v: revisit runNetworks (internal/run/sweep.go) before taking this agentry-cli", got, want)
	}
	// One for the run network, one for the proxy's egress, one in the
	// relays' loop, one for the web tools.
	if creates != 4 {
		t.Fatalf("the engine makes a network in %d places, and the sweep knows 4: revisit runNetworks before taking this agentry-cli", creates)
	}
}
