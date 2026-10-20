package run

import (
	"context"
	"errors"
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
// then every network a run can make and every other network carrying the
// run's label, such as an input source's — and nothing it was not asked
// about: another run's network stays.
func TestSweepRemovesTheRunsLabelledObjectsInOrder(t *testing.T) {
	isolateRunEnv(t)
	fake := &dockerx.Fake{
		ByLabel:         map[string][]string{"agentry.run=ab12cd34": {"agentry-ab12cd34-agent", "agentry-ab12cd34-gateway"}},
		LeftoverVolumes: []string{"agentry-ab12cd34-session"},
		Networks: []dockerx.Labeled{
			{Name: "agentry-ab12cd34-net", Labels: map[string]string{"agentry.run": "ab12cd34"}},
			{Name: "agentry-ab12cd34-src-code", Labels: map[string]string{"agentry.run": "ab12cd34"}},
			{Name: "agentry-ef56ab78-src-code", Labels: map[string]string{"agentry.run": "ef56ab78"}},
		},
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
		"NetworksByLabel agentry.run=ab12cd34",
		"NetworkRemove agentry-ab12cd34-net",
		"NetworkRemove agentry-ab12cd34-egress",
		"NetworkRemove agentry-ab12cd34-web",
		"NetworkRemove agentry-ab12cd34-relays",
		"NetworkRemove agentry-ab12cd34-relays-out",
		"NetworkRemove agentry-ab12cd34-browser",
		"NetworkRemove agentry-ab12cd34-render",
		"NetworkRemove agentry-ab12cd34-src-code",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(fake.Networks) != 1 || fake.Networks[0].Name != "agentry-ef56ab78-src-code" {
		t.Fatalf("networks left: %v; want only the other run's", fake.Networks)
	}
}

// unlistedNetworks is a daemon that answers everything but a list of
// networks.
type unlistedNetworks struct{ *dockerx.Fake }

func (unlistedNetworks) NetworksByLabel(context.Context, string) ([]string, error) {
	return nil, errors.New("network ls refused")
}

// A docker that cannot list networks still has every network the engine
// names asked for, and the failure is reported, so the run's record stays
// for the next start to sweep again.
func TestSweepRemovesTheNamedNetworksWhenTheListFails(t *testing.T) {
	isolateRunEnv(t)
	fake := &dockerx.Fake{}
	withFakeDocker(t, unlistedNetworks{fake})

	err := Sweep(context.Background(), "ab12cd34")
	if err == nil || !strings.Contains(err.Error(), "list networks: network ls refused") {
		t.Fatalf("err %v; want the list's failure reported", err)
	}
	var removed []string
	for _, c := range fake.Calls {
		if c.Op == "NetworkRemove" {
			removed = append(removed, c.Arg)
		}
	}
	want := "agentry-ab12cd34-net agentry-ab12cd34-egress agentry-ab12cd34-web agentry-ab12cd34-relays agentry-ab12cd34-relays-out agentry-ab12cd34-browser agentry-ab12cd34-render"
	if strings.Join(removed, " ") != want {
		t.Fatalf("removed %v; want %s", removed, want)
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
// to make five. So the engine's source is read, as internal/applog reads
// Wails': every name the engine builds from a run id, and every place it
// makes a network, is one this test knows, and a pin that adds either fails
// here until runNetworks follows.
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
	// The five networks, and the session store's volume, which the sweep finds by
	// its label. "-src-" is an input source's network, named by the source's id as
	// well as the run's, so no name from a run id alone finds it: runNetworks
	// finds it, and any other the engine labels with the run, by the run's label.
	// "-browser" is the browser's own network and "-render" the page
	// renderer's, both named above; "-docker" is the
	// volume of the docker filter's container on Docker Desktop, labelled with the
	// run and so removed with the session's by label; "-applink" is a
	// persistent agent's app link relay volume where the daemon runs in a VM,
	// labelled with the run in the same way.
	// "-dlv" is a delivery's network, named by an id of the delivery's own and not
	// the run's (a delivery runs after the run's record is gone, and a later one
	// for a finished run's folder has no run): it is removed by the step and, from
	// its containers' owner labels, by engine.SweepAbandoned.
	want := []string{"-applink", "-browser", "-dlv", "-docker", "-egress", "-net", "-relays", "-relays-out", "-render", "-session", "-src-", "-web"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the engine names %v from a run id, and the sweep knows %v: revisit runNetworks (internal/run/sweep.go) before taking this agentry-cli", got, want)
	}
	// One for the run network, one for the proxy's egress, one in the relays'
	// loop, one for the web tools, an input source's own (above), and a Copilot
	// model list's: named as a run's "-net" (and "-egress"), under a run id no run
	// record holds, so a list killed midway is removed by engine.SweepAbandoned,
	// which the engine runs before each run, from its containers' owner labels;
	// the browser's own; the page renderer's; and a delivery's.
	if creates != 9 {
		t.Fatalf("the engine makes a network in %d places, and the sweep knows 9: revisit runNetworks before taking this agentry-cli", creates)
	}
}
