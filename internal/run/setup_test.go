package run

// Setup-surface tests (§P1 desktop shape): GetStatus must be a pure read,
// ApplySetup must write exactly the provider-table model block into the
// operator policy and only touch docker when asked to.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// TestGetStatusReadOnlyDefaults pins req 5's status read: no policy file
// means defaults are REPORTED, and looking must not create the file.
func TestGetStatusReadOnlyDefaults(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})

	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.PolicyExists {
		t.Fatal("no policy file exists yet")
	}
	def := policy.Default()
	if st.Provider != def.Model.Provider || st.Model != def.Model.Name || st.APIKeyEnv != def.Model.APIKeyEnv {
		t.Errorf("defaults not reported: %+v", st)
	}
	if st.TimeoutSeconds != def.Budgets.TimeoutSeconds || st.MaxTurns != def.Budgets.MaxTurns {
		t.Errorf("budget defaults not reported: %+v", st)
	}
	if !st.KeepProfilesByDefault {
		t.Error("shipped keep-profiles default is true (§P2)")
	}
	if st.DockerOK || st.DockerErr == "" {
		t.Errorf("docker failure must be surfaced: %+v", st)
	}
	if st.ImagesReady {
		t.Error("images cannot be ready without docker")
	}
	// The read must not have created the policy file.
	if _, statErr := os.Stat(st.PolicyPath); !os.IsNotExist(statErr) {
		t.Fatalf("GetStatus created %s", st.PolicyPath)
	}
}

// TestGetStatusImagesReady covers the docker-up half: images present ⇒
// ready; any one missing ⇒ not ready.
func TestGetStatusImagesReady(t *testing.T) {
	isolateRunEnv(t)
	fake := readyImagesFake()
	withFakeDocker(t, fake)

	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DockerOK || st.DockerErr != "" || !st.ImagesReady {
		t.Fatalf("status = %+v, want docker ok + images ready", st)
	}

	delete(fake.ExistingImages, policy.Default().Images.Gateway)
	st, err = GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DockerOK || st.ImagesReady {
		t.Fatalf("a missing image must report not-ready: %+v", st)
	}
}

// TestGetStatusBrokenPolicyIsAnError: a policy file that fails to load is
// a fact the settings page must show, not hide behind defaults.
func TestGetStatusBrokenPolicyIsAnError(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, readyImagesFake())
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GetStatus(context.Background()); err == nil || !strings.Contains(err.Error(), "operator policy") {
		t.Fatalf("err = %v, want the broken policy surfaced", err)
	}
}

// TestApplySetupWritesProviderTable pins §P1: choosing a provider fills
// the model block from the table, PullImages=false touches no docker, and
// everything outside the model block survives.
func TestApplySetupWritesProviderTable(t *testing.T) {
	isolateRunEnv(t)
	fake := readyImagesFake()
	withFakeDocker(t, fake)

	// Pre-write a policy with a custom budget so the preserve rule is
	// observable.
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pre := policy.Default()
	pre.Budgets.TimeoutSeconds = 123
	if err := policy.Save(path, pre); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai"}, &log); err != nil {
		t.Fatal(err)
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pol.Model.Provider != "openai" || pol.Model.Name != "gpt-5.2" || pol.Model.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("model block = %+v, want the openai table row", pol.Model)
	}
	if len(pol.Model.ProxyAllow) != 1 || pol.Model.ProxyAllow[0] != "api.openai.com:443" {
		t.Fatalf("proxyAllow = %v", pol.Model.ProxyAllow)
	}
	if pol.Budgets.TimeoutSeconds != 123 {
		t.Fatalf("budgets outside the model block must survive: %+v", pol.Budgets)
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("PullImages=false must not touch docker: %v", fake.Ops())
	}
	if !strings.Contains(log.String(), "policy written") {
		t.Errorf("log = %q", log.String())
	}

	// GetStatus now reflects the written preference.
	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.PolicyExists || st.Provider != "openai" || st.Model != "gpt-5.2" {
		t.Fatalf("status after setup = %+v", st)
	}
}

// TestApplySetupPullImagesBuildsRunner covers the PullImages half: with
// the pinned images present and the runner image absent, setup
// materializes the embedded context and builds the runner tag.
func TestApplySetupPullImagesBuildsRunner(t *testing.T) {
	home := isolateRunEnv(t)
	pol := policy.Default()
	fake := &dockerx.Fake{ExistingImages: map[string]bool{
		pol.Images.Gateway:            true,
		pol.Images.HarnessDockerAgent: true,
		// runner tag absent → EnsureInstalled must build it
	}}
	withFakeDocker(t, fake)

	var log strings.Builder
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", PullImages: true}, &log); err != nil {
		t.Fatal(err)
	}
	wantOp := "Build:" + runnerImageTag(pol) + "<-" + filepath.Join(home, "image")
	found := false
	for _, op := range fake.Ops() {
		if op == wantOp {
			found = true
		}
	}
	if !found {
		t.Fatalf("runner image not built: %v", fake.Ops())
	}
	if _, err := os.Stat(filepath.Join(home, "image", "Dockerfile")); err != nil {
		t.Errorf("embedded build context not materialized: %v", err)
	}
	if !strings.Contains(log.String(), "setup complete") {
		t.Errorf("log = %q", log.String())
	}
}

// TestApplySetupCustomProviderNeedsBaseURL: the form's Yes=true flow turns
// the missing-BaseURL question into a refusal instead of a hung window.
func TestApplySetupCustomProviderNeedsBaseURL(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, readyImagesFake())
	err := ApplySetup(context.Background(), SetupOpts{Provider: "custom"}, &strings.Builder{})
	if err == nil {
		t.Fatal("custom provider without a BaseURL must refuse")
	}
}

// TestApplySetupPullImagesDockerDown: the policy still lands, but the
// image half surfaces the docker failure with its class wording.
func TestApplySetupPullImagesDockerDown(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", PullImages: true}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "gateway/docker environment") {
		t.Fatalf("err = %v, want the docker-environment wrap", err)
	}
	path, perr := policy.DefaultPath()
	if perr != nil {
		t.Fatal(perr)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("the policy half must have been written before the image half failed: %v", statErr)
	}
}
