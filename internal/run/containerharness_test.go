package run

// Settings → Models' harness: the policy's containerHarness, read and
// written through the policy package the CLI reads, so `agentry harness`
// and the app agree.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func TestContainerHarnessIsDockerAgentWithNoPolicy(t *testing.T) {
	isolateRunEnv(t)
	got, err := ContainerHarness()
	if err != nil {
		t.Fatal(err)
	}
	if got.Harness != policy.DockerAgentAdapter || got.RunsOn != policy.DockerAgentAdapter {
		t.Fatalf("no policy reads as %+v, want docker-agent", got)
	}
	if strings.Join(got.Choices, ",") != "docker-agent,agentry-harness" {
		t.Fatalf("choices %v, want docker-agent then agentry-harness", got.Choices)
	}
	// Choosing the default with no policy file writes no file.
	if _, err := SetContainerHarness(policy.DockerAgentAdapter); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got.PolicyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("choosing the default wrote a policy file: %v", err)
	}
}

func TestSetContainerHarnessWritesTheKeyTheCLIReads(t *testing.T) {
	isolateRunEnv(t)
	got, err := SetContainerHarness(" Agentry-Harness ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Harness != adapter.AgentryHarnessAdapterName || got.RunsOn != adapter.AgentryHarnessAdapterName {
		t.Fatalf("after choosing the Agentry harness: %+v", got)
	}
	pol, _, err := policy.Load(got.PolicyPath)
	if err != nil {
		t.Fatalf("the CLI cannot load the policy the app wrote: %v", err)
	}
	if pol.ContainerHarness != adapter.AgentryHarnessAdapterName || adapter.DefaultFor(pol) != adapter.AgentryHarnessAdapterName {
		t.Fatalf("the policy holds containerHarness %q, a run takes %s", pol.ContainerHarness, adapter.DefaultFor(pol))
	}

	// The rest of the policy is kept, and the default takes the key out.
	pol.Budgets.MaxTurns = 33
	if err := policy.Save(got.PolicyPath, pol); err != nil {
		t.Fatal(err)
	}
	back, err := SetContainerHarness(policy.DockerAgentAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if back.Harness != policy.DockerAgentAdapter {
		t.Fatalf("after choosing Docker Agent: %+v", back)
	}
	b, err := os.ReadFile(got.PolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "\ncontainerHarness:") {
		t.Fatalf("the default left the key in the policy:\n%s", b)
	}
	after, _, _ := policy.Load(got.PolicyPath)
	if after.Budgets.MaxTurns != 33 {
		t.Fatalf("the harness write lost the budgets: %+v", after.Budgets)
	}
}

func TestSetContainerHarnessRefusesAnotherHarness(t *testing.T) {
	isolateRunEnv(t)
	for _, name := range []string{"", "claude-image", "codex"} {
		if _, err := SetContainerHarness(name); err == nil {
			t.Fatalf("%q was taken as a container harness", name)
		}
	}
	path, _ := policy.DefaultPath()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused choice wrote a policy file: %v", err)
	}
}

// A model paid by a model sign-in runs on its vendor's harness whatever the
// choice says, and the answer says so.
func TestContainerHarnessSaysASignInModelRunsOnItsVendorsHarness(t *testing.T) {
	isolateRunEnv(t)
	if _, err := SetContainerHarness(adapter.AgentryHarnessAdapterName); err != nil {
		t.Fatal(err)
	}
	path, _ := policy.DefaultPath()
	pol, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	pol.Model.Provider = "openai"
	pol.Model.APIKeyKind = policy.KeyKindChatGPTPlan
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	got, err := ContainerHarness()
	if err != nil {
		t.Fatal(err)
	}
	if got.Harness != adapter.AgentryHarnessAdapterName || got.RunsOn != adapter.CodexImageAdapterName {
		t.Fatalf("a ChatGPT plan's default model reads as %+v, want the choice kept and runs on codex-image", got)
	}
}
