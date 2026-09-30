package dockerimages

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// The images a machine holds, as `docker image ls` lists them: Agentry's
// pulled and built ones among a person's own.
var listed = []Image{
	{Repository: "ghcr.io/blaktron/agentry-mcp-gateway", Tag: "v0.43.3", ID: "sha256:a1"},
	{Repository: "ghcr.io/blaktron/agentry-docker-agent", Tag: "1.128.0", ID: "sha256:a2"},
	{Repository: "ghcr.io/blaktron/agentry-golang-alpine", Tag: "<none>", Digest: "sha256:8a59", ID: "sha256:a3"},
	{Repository: "agentry-runner", Tag: "0.6.25", ID: "sha256:a4"},
	{Repository: "agentry-runner", Tag: "0.6.24", ID: "sha256:a5"},
	{Repository: "agentry-runner-exec", Tag: "0.6.25-0123456789ab", ID: "sha256:a6"},
	{Repository: "registry.example.test/my-gateway", Tag: "1", ID: "sha256:a7"},
	{Repository: "postgres", Tag: "16", ID: "sha256:b1"},
	{Repository: "ghcr.io/blaktron/other-project", Tag: "latest", ID: "sha256:b2"},
	{Repository: "agentry-runner-extra", Tag: "1", ID: "sha256:b3"},
	{Repository: "<none>", Tag: "<none>", ID: "sha256:b4"},
	{Repository: "myorg/agentry-runner", Tag: "1", ID: "sha256:b5"},
}

// Select keeps Agentry's images and nothing else: the ones under Agentry's
// registry prefix, the locally built runner and exec images, and whatever
// the runner policy names (an operator's own gateway). A person's
// postgres, another project under the same GitHub owner, a look-alike
// name and an untagged dangling image are never touched.
func TestSelectKeepsOnlyAgentrysImages(t *testing.T) {
	got := refs(Select(listed, Policy{Refs: []string{"registry.example.test/my-gateway:1", "ghcr.io/blaktron/agentry-mcp-gateway:v0.43.3"}}))
	want := []string{
		"ghcr.io/blaktron/agentry-mcp-gateway:v0.43.3",
		"ghcr.io/blaktron/agentry-docker-agent:1.128.0",
		"ghcr.io/blaktron/agentry-golang-alpine@sha256:8a59",
		"agentry-runner:0.6.25",
		"agentry-runner:0.6.24",
		"agentry-runner-exec:0.6.25-0123456789ab",
		"registry.example.test/my-gateway:1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected\n %v\nwant\n %v", got, want)
	}
	for _, not := range []string{"postgres:16", "ghcr.io/blaktron/other-project:latest", "agentry-runner-extra:1", "myorg/agentry-runner:1"} {
		for _, g := range got {
			if g == not {
				t.Fatalf("%s is not Agentry's and must be left alone", not)
			}
		}
	}
}

// A policy that renames the runner image (images.runner) has its exec
// images named from it; both are Agentry's.
func TestSelectFollowsARenamedRunnerImage(t *testing.T) {
	imgs := []Image{
		{Repository: "local/runner", Tag: "7", ID: "sha256:c1"},
		{Repository: "local/runner-exec", Tag: "7-0123456789ab", ID: "sha256:c2"},
		{Repository: "local/other", Tag: "7", ID: "sha256:c3"},
	}
	got := refs(Select(imgs, Policy{Runner: "local/runner:7"}))
	want := []string{"local/runner:7", "local/runner-exec:7-0123456789ab"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected %v, want %v", got, want)
	}
}

type fakeDocker struct {
	images  []Image
	listErr error
	refuse  map[string]string
	removed []string
}

func (f *fakeDocker) List(context.Context) ([]Image, error) { return f.images, f.listErr }
func (f *fakeDocker) Remove(_ context.Context, ref string) error {
	if why, ok := f.refuse[ref]; ok {
		return errors.New(why)
	}
	f.removed = append(f.removed, ref)
	return nil
}

// RemoveAll removes each selected image by its own ref, never by a broad
// prune, and reports what it could not remove without stopping at it.
func TestRemoveAllRemovesEachAndReportsWhatStayed(t *testing.T) {
	d := &fakeDocker{images: listed, refuse: map[string]string{"agentry-runner:0.6.25": "image is being used by running container"}}
	rep, err := RemoveAll(context.Background(), d, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 5 || len(rep.Failed) != 1 || !strings.Contains(rep.Failed[0], "agentry-runner:0.6.25") {
		t.Fatalf("report = %+v", rep)
	}
	for _, r := range d.removed {
		if r == "postgres:16" {
			t.Fatal("removed a person's own image")
		}
	}
}

// A daemon that does not answer removes nothing and says so.
func TestRemoveAllWithoutDocker(t *testing.T) {
	d := &fakeDocker{listErr: errors.New("docker daemon not reachable")}
	if _, err := RemoveAll(context.Background(), d, Policy{}); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("want the daemon's error, got %v", err)
	}
	if len(d.removed) != 0 {
		t.Fatal("removed something without a list")
	}
}

// The CLI's list output is parsed as `docker image ls --format` prints it.
func TestParseList(t *testing.T) {
	out := "agentry-runner\t0.6.25\t<none>\tsha256:a4\t1.2GB\nghcr.io/blaktron/agentry-alpine\t<none>\tsha256:5291\tsha256:a9\t8MB\n\n"
	got := parseList(out)
	if len(got) != 2 || got[0].Ref() != "agentry-runner:0.6.25" || got[1].Ref() != "ghcr.io/blaktron/agentry-alpine@sha256:5291" || got[0].Size != "1.2GB" {
		t.Fatalf("parsed %+v", got)
	}
}

func refs(imgs []Image) []string {
	out := []string{}
	for _, i := range imgs {
		out = append(out, i.Ref())
	}
	return out
}

// A ref the policy pins takes that image only: an operator whose node
// runtime is the public node:22-alpine loses that image and keeps every
// other node tag, and no "node-exec" repository is invented for it
// (agentry-desktop#424's review).
func TestSelectTakesAPolicyRefExactly(t *testing.T) {
	imgs := []Image{
		{Repository: "node", Tag: "22-alpine", ID: "sha256:d1"},
		{Repository: "node", Tag: "20", ID: "sha256:d2"},
		{Repository: "node-exec", Tag: "1", ID: "sha256:d3"},
		{Repository: "golang", Tag: "<none>", Digest: "sha256:abc", ID: "sha256:d4"},
		{Repository: "golang", Tag: "1.23", ID: "sha256:d5"},
	}
	got := refs(Select(imgs, Policy{Refs: []string{"node:22-alpine", "golang:1.27@sha256:abc"}}))
	want := []string{"node:22-alpine", "golang@sha256:abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected %v, want %v", got, want)
	}
}

// Remove takes the list it is given, the one a person confirmed, and
// nothing a later listing would add.
func TestRemoveTakesTheConfirmedList(t *testing.T) {
	d := &fakeDocker{images: listed}
	rep := Remove(context.Background(), d, []Image{{Repository: "agentry-runner", Tag: "0.6.25"}})
	if len(rep.Removed) != 1 || len(d.removed) != 1 || d.removed[0] != "agentry-runner:0.6.25" {
		t.Fatalf("removed %v", d.removed)
	}
}

// The policy's refs are read from the file the runner uses; an operator's
// own gateway is among them, and a home with no policy names nothing.
func TestReadPolicyReadsTheRunnerPolicy(t *testing.T) {
	home := t.TempDir()
	if got := ReadPolicy(home); got.Refs != nil || got.Runner != "" {
		t.Fatalf("no policy: %v, want none", got)
	}
	if _, _, err := policy.EnsureDefault(policy.PathIn(home)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(policy.PathIn(home))
	if err != nil {
		t.Fatal(err)
	}
	custom := strings.Replace(string(b), policy.DefaultGatewayImage, "registry.example.test/my-gateway:1", 1)
	if custom == string(b) {
		t.Fatal("the default policy did not name the default gateway")
	}
	if err := os.WriteFile(policy.PathIn(home), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ReadPolicy(home)
	found := false
	for _, r := range got.Refs {
		if r == "registry.example.test/my-gateway:1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("refs %v do not name the policy's gateway", got)
	}
}
