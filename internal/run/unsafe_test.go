package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/runner/unsafeexec"
)

// Unsafe Mode's desktop half (plan unsafe-mode.md M3; agentry-desktop#418).
// The runner's half, and invariants 1–3, 8 and 9, are the CLI's tests
// (DR-53); these hold what the desktop adds: a run carries Unsafe Mode
// only when its request says so (invariant 4), a plan compiled one way is
// never reused the other way, and the consent line names what the image
// cannot provide (D8).

// testUnsafeBase is a well-formed base pinned by digest, as the policy's
// default is.
var testUnsafeBase = policy.Default().Images.UnsafeImage()

// unsafeFiles are the base's three data files as a fake docker serves
// them: echo is in the index, decompyle3 in the extras, and cat is
// ambiguous, so a release declaring it meets an unplaced command.
func unsafeFiles(base string) map[string]string {
	return map[string]string{
		base + ":" + unsafeexec.CommandsFile:  "echo\tcoreutils\nstrings\tbinutils\n",
		base + ":" + unsafeexec.ExtrasFile:    "pip\tdecompyle3\tdecompyle3==3.9.3\n",
		base + ":" + unsafeexec.AmbiguousFile: "cat\tcoreutils,busybox\n",
	}
}

func testUnsafeData(t *testing.T, base string) *unsafeexec.Data {
	t.Helper()
	f := unsafeFiles(base)
	d, err := unsafeexec.Parse(base, []byte(f[base+":"+unsafeexec.CommandsFile]), []byte(f[base+":"+unsafeexec.ExtrasFile]), []byte(f[base+":"+unsafeexec.AmbiguousFile]))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// A plan compiled with Unsafe Mode on is never reused by a run with it
// off, and the other way round; the hand-off of agentry-cli#490 asked for
// exactly this, and the CLI's own reuse refuses such a plan too.
func TestEnsurePlanRecompilesWhenUnsafeModeDiffersFromTheSwitch(t *testing.T) {
	home := isolateRunEnv(t)
	m, meta, installDir, workspaceDir := planFixture(t)
	pol := cataloguedPolicy()
	data := testUnsafeData(t, testUnsafeBase)
	planDir := filepath.Join(installDir, "plan", "autonomous")
	canary := filepath.Join(planDir, "canary")
	plant := func() {
		t.Helper()
		if err := os.WriteFile(canary, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reused := func() bool { _, err := os.Stat(canary); return err == nil }
	ensure := func(pol *policy.Policy, unsafe *unsafeexec.Data) (*spec.Plan, string, error) {
		t.Helper()
		var log strings.Builder
		plan, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha", adapter.DefaultName, nil, manifest.ModeAutonomous, unsafe)
		return plan, log.String(), err
	}

	// On: the catalogue is not consulted, the plan says so.
	plan, _, err := ensure(pol, data)
	if err != nil {
		t.Fatal(err)
	}
	if !planIsUnsafe(plan) || plan.ExecImage.Unsafe.Base != testUnsafeBase || !strings.HasPrefix(plan.Enforcement["exec"], "UNSAFE MODE:") {
		t.Fatalf("a compile with Unsafe Mode on: exec image %+v, exec line %q", plan.ExecImage, plan.Enforcement["exec"])
	}
	plant()
	if _, _, err := ensure(pol, data); err != nil || !reused() {
		t.Fatalf("the same switch and base recompiled (err %v)", err)
	}

	// Off: the Unsafe Mode plan is not reused, and the catalogue decides.
	plant()
	plan, log, err := ensure(pol, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reused() || planIsUnsafe(plan) || !strings.Contains(log, "compiled in Unsafe Mode, which is off") {
		t.Fatalf("with the switch off an Unsafe Mode plan was reused: reused=%v unsafe=%v log=%q", reused(), planIsUnsafe(plan), log)
	}
	if strings.Contains(plan.Enforcement["exec"], "UNSAFE MODE") {
		t.Fatalf("the default posture's exec line names Unsafe Mode: %q", plan.Enforcement["exec"])
	}
	plant()
	if _, _, err := ensure(pol, nil); err != nil || !reused() {
		t.Fatalf("the default posture's plan recompiled with the switch still off (err %v)", err)
	}

	// On again: the catalogue's plan is not reused either.
	plant()
	plan, log, err = ensure(pol, data)
	if err != nil {
		t.Fatal(err)
	}
	if reused() || !planIsUnsafe(plan) || !strings.Contains(log, "compiled without Unsafe Mode, which is on") {
		t.Fatalf("with the switch on the catalogue's plan was reused: reused=%v log=%q", reused(), log)
	}

	// A re-pinned base is another image, so another plan.
	other := strings.Replace(testUnsafeBase, "@sha256:", "@sha256:0000", 1)
	other = other[:len(other)-4]
	plant()
	plan, log, err = ensure(pol, testUnsafeData(t, other))
	if err != nil {
		t.Fatal(err)
	}
	if reused() || plan.ExecImage.Unsafe.Base != other || !strings.Contains(log, "names the Unsafe Mode image") {
		t.Fatalf("a re-pinned base reused the old plan: reused=%v log=%q", reused(), log)
	}

	// Off, on a machine whose catalogue does not carry the command: the
	// installed Unsafe Mode plan changes nothing, the run is refused as it
	// always was (invariant 4).
	_, _, err = ensure(policy.Default(), nil)
	if err == nil || !strings.Contains(err.Error(), "does not catalogue") {
		t.Fatalf("with the switch off and an Unsafe Mode plan installed: err = %v, want the exec catalogue refusal", err)
	}
}

// A release that declares no commands compiles the same plan either way,
// so the switch does not make it recompile on every run.
func TestUnsafeModeDoesNotRecompileAReleaseWithNoCommands(t *testing.T) {
	home := isolateRunEnv(t)
	_, meta, installDir, workspaceDir := planFixture(t)
	m, err := manifest.Parse([]byte(strings.Replace(runManifestYAML, "  exec: { commands: [echo] }\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	data := testUnsafeData(t, testUnsafeBase)
	var log strings.Builder
	plan, planDir, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, policy.Default(), "pol-sha", adapter.DefaultName, nil, manifest.ModeAutonomous, data)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ExecImage != nil {
		t.Fatalf("a release with no commands got an exec image: %+v", plan.ExecImage)
	}
	canary := filepath.Join(planDir, "canary")
	if err := os.WriteFile(canary, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []*unsafeexec.Data{data, nil, data} {
		if _, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, policy.Default(), "pol-sha", adapter.DefaultName, nil, manifest.ModeAutonomous, unsafe); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(canary); err != nil {
			t.Fatalf("the plan was recompiled (Unsafe Mode %v): %s", unsafe != nil, log.String())
		}
	}
}

// unsafeRunFixture is runFixture on a machine whose catalogue carries
// nothing (the default policy), with the Unsafe Mode base on its docker.
func unsafeRunFixture(t *testing.T) (string, *dockerx.Fake) {
	t.Helper()
	isolateRunEnv(t)
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunRegistry{}
	regURL := startRunRegistry(t, fr)
	fake := &dockerx.Fake{
		ExistingImages: map[string]bool{
			pol.Images.Gateway:            true,
			pol.Images.HarnessDockerAgent: true,
			runnerImageTag(pol):           true,
			testUnsafeBase:                true,
		},
		ImageFiles: unsafeFiles(testUnsafeBase),
	}
	scriptHappyAgent(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	return regURL, fake
}

// The end-to-end shape, against a fake registry and docker: the same
// release on the same machine is refused with the switch off and runs with
// it on, on an exec image built from the Unsafe Mode base, and a package
// that did not install is named in the result without stopping the run.
func TestARunIsUnsafeOnlyWhenItsRequestSaysSo(t *testing.T) {
	regURL, fake := unsafeRunFixture(t)

	// Off (the request's zero value, which is what a run has when
	// desktop.yaml says nothing): the catalogue refuses, and nothing of
	// Unsafe Mode was read or built.
	var log strings.Builder
	_, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log)
	if err == nil || !strings.Contains(err.Error(), "does not catalogue") {
		t.Fatalf("with Unsafe Mode off: err = %v, want the exec catalogue refusal\nlog:\n%s", err, log.String())
	}
	for _, op := range fake.Ops() {
		if strings.Contains(op, testUnsafeBase) {
			t.Fatalf("a run with Unsafe Mode off touched the Unsafe Mode image: %s", op)
		}
	}

	// On: the exec image the plan names is built from the base, and its
	// records come back, one install having failed.
	pol := policy.Default()
	data := testUnsafeData(t, testUnsafeBase)
	pkgs, u := data.Place([]string{"echo"})
	tag := spec.UnsafeExecImageTag(runnerImageTag(pol), u, []string{"echo"}, pkgs)
	fake.ImageFiles[tag+":"+gateway.ExecPackagesFile] = "libc6=2.41-1\n"
	fake.ImageFiles[tag+":"+gateway.ExecFailedFile] = "coreutils\tE: Unable to locate package coreutils\n"

	log.Reset()
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.Unsafe = true
	res, err := Run(context.Background(), req, &log)
	if err != nil {
		t.Fatalf("with Unsafe Mode on: %v\nlog:\n%s", err, log.String())
	}
	if res.Status != "ok" {
		t.Fatalf("with Unsafe Mode on the run ended %q: %s\nlog:\n%s", res.Status, res.Error, log.String())
	}
	if !slices.ContainsFunc(fake.Ops(), func(op string) bool { return strings.HasPrefix(op, "Build:"+tag+"<-") }) {
		t.Fatalf("the Unsafe Mode exec image %s was not built: %v", tag, fake.Ops())
	}
	for _, want := range []string{"UNSAFE MODE:", testUnsafeBase, "this machine's exec catalogue was not consulted",
		"NOT installed at image build", "coreutils (E: Unable to locate package coreutils) for echo"} {
		if !strings.Contains(res.Exec, want) {
			t.Errorf("the receipt's exec line lacks %q:\n%s", want, res.Exec)
		}
	}
	if !strings.Contains(log.String(), "UNSAFE MODE:") {
		t.Errorf("the run log does not name Unsafe Mode:\n%s", log.String())
	}

	// Off again, the Unsafe Mode plan still installed: refused.
	log.Reset()
	_, err = Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log)
	if err == nil || !strings.Contains(err.Error(), "does not catalogue") {
		t.Fatalf("with Unsafe Mode off again: err = %v, want the exec catalogue refusal\nlog:\n%s", err, log.String())
	}
}

func TestUnsafeNoticeNamesWhatTheImageCannotProvide(t *testing.T) {
	isolateRunEnv(t)
	fr := &fakeRunRegistry{manifestDoc: manifestDocFromYAML(t, strings.Replace(runManifestYAML,
		"commands: [echo]", "commands: [strings, cat, decompyle3, zzz-nothing]", 1))}
	regURL := startRunRegistry(t, fr)
	fake := &dockerx.Fake{ExistingImages: map[string]bool{testUnsafeBase: true}, ImageFiles: unsafeFiles(testUnsafeBase)}
	withFakeDocker(t, fake)

	n, err := UnsafeNoticeFor(context.Background(), regURL, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(n.Commands, ","); got != "cat,decompyle3,strings,zzz-nothing" {
		t.Fatalf("commands = %s", got)
	}
	if len(n.Unplaced) != 2 || n.Unplaced[0] != "cat (several packages ship it: busybox, coreutils)" || n.Unplaced[1] != "zzz-nothing" {
		t.Fatalf("unplaced = %q", n.Unplaced)
	}
	line := n.Line()
	for _, want := range []string{"Unsafe Mode is on", "cat, decompyle3, strings, zzz-nothing", "image's own PATH",
		"Not available in Unsafe Mode's image: cat (several packages ship it: busybox, coreutils); zzz-nothing", "goes on without them"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line lacks %q:\n%s", want, line)
		}
	}
	// It reads the image that is here and pulls nothing.
	for _, op := range fake.Ops() {
		if strings.HasPrefix(op, "Pull:") {
			t.Fatalf("reading the notice pulled an image: %s", op)
		}
	}

	// The base is not on this computer: said, not pulled.
	absent := &dockerx.Fake{}
	withFakeDocker(t, absent)
	n, err = UnsafeNoticeFor(context.Background(), regURL, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if n.Unknown == "" || len(n.Unplaced) != 0 || !strings.Contains(n.Line(), "could not be read") {
		t.Fatalf("with no base here: %+v / %s", n, n.Line())
	}
	if slices.ContainsFunc(absent.Ops(), func(op string) bool { return strings.HasPrefix(op, "Pull:") }) {
		t.Fatalf("reading the notice pulled the base: %v", absent.Ops())
	}

	// A release with no commands: the line says Unsafe Mode changes nothing.
	fr.manifestDoc = manifestDocFromYAML(t, strings.Replace(runManifestYAML, "  exec: { commands: [echo] }\n", "", 1))
	n, err = UnsafeNoticeFor(context.Background(), regURL, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Commands) != 0 || !strings.Contains(n.Line(), "changes nothing about this run") {
		t.Fatalf("with no commands: %+v / %s", n, n.Line())
	}
}

// Turning Unsafe Mode on pulls the base the policy names and reads its
// data back before the switch may read on (plan D2, D6).
func TestPullUnsafeBase(t *testing.T) {
	isolateRunEnv(t)

	// Absent: pulled, then read.
	fake := &dockerx.Fake{ImageFiles: unsafeFiles(testUnsafeBase)}
	withFakeDocker(t, fake)
	var log strings.Builder
	base, err := PullUnsafeBase(context.Background(), &log)
	if err != nil {
		t.Fatal(err)
	}
	if base != testUnsafeBase || !slices.Contains(fake.Ops(), "Pull:"+testUnsafeBase) {
		t.Fatalf("base = %q, ops = %v", base, fake.Ops())
	}
	if !strings.Contains(log.String(), "downloading the Unsafe Mode image") || !strings.Contains(log.String(), "is ready") {
		t.Fatalf("progress lines: %q", log.String())
	}

	// Here already: not pulled again.
	here := &dockerx.Fake{ExistingImages: map[string]bool{testUnsafeBase: true}, ImageFiles: unsafeFiles(testUnsafeBase)}
	withFakeDocker(t, here)
	if _, err := PullUnsafeBase(context.Background(), &log); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(here.Ops(), "Pull:"+testUnsafeBase) {
		t.Fatalf("a base already here was pulled again: %v", here.Ops())
	}

	// A failed pull, a base with no index, and no docker are each a
	// refusal: the caller leaves the switch off.
	pullFails := errors.New("manifest unknown")
	withFakeDocker(t, &dockerx.Fake{FailPull: map[string]error{testUnsafeBase: pullFails}})
	if _, err := PullUnsafeBase(context.Background(), &log); !errors.Is(err, pullFails) {
		t.Fatalf("a failed pull: err = %v", err)
	}
	withFakeDocker(t, &dockerx.Fake{})
	if _, err := PullUnsafeBase(context.Background(), &log); err == nil || !strings.Contains(err.Error(), unsafeexec.CommandsFile) {
		t.Fatalf("a base with no command index: err = %v", err)
	}
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	if _, err := PullUnsafeBase(context.Background(), &log); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("no docker: err = %v", err)
	}

	// The policy's own base wins, and one not pinned by digest is refused
	// before docker is asked for anything.
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	pol.Images.Unsafe = "registry.example/kali:latest"
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	unpinned := &dockerx.Fake{}
	withFakeDocker(t, unpinned)
	if _, err := PullUnsafeBase(context.Background(), &log); err == nil || !strings.Contains(err.Error(), "pinned by digest") {
		t.Fatalf("an unpinned base: err = %v", err)
	}
	if len(unpinned.Ops()) != 0 {
		t.Fatalf("an unpinned base reached docker: %v", unpinned.Ops())
	}
}
