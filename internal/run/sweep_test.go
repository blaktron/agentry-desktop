package run

import (
	"context"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// Sweep removes exactly the run's labelled containers, then its volumes,
// then its network — and nothing it was not asked about.
func TestSweepRemovesTheRunsLabelledObjectsInOrder(t *testing.T) {
	fake := &dockerx.Fake{
		ByLabel:         map[string][]string{"agentry.run=ab12cd34": {"agentry-ab12cd34-agent", "agentry-ab12cd34-gateway"}},
		LeftoverVolumes: []string{"agentry-ab12cd34-session"},
	}
	prev := newDockerClient
	newDockerClient = func() dockerx.Client { return fake }
	t.Cleanup(func() { newDockerClient = prev })

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
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// With no run id there is no label to sweep by; an empty label would match
// every run on the machine, the CLI's included.
func TestSweepRefusesAnEmptyRunID(t *testing.T) {
	fake := &dockerx.Fake{}
	prev := newDockerClient
	newDockerClient = func() dockerx.Client { return fake }
	t.Cleanup(func() { newDockerClient = prev })
	if err := Sweep(context.Background(), ""); err == nil {
		t.Fatal("swept with no run id")
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("touched docker: %v", fake.Calls)
	}
}
