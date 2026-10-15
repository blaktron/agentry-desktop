package knowledge

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

const cliModule = "github.com/blaktron/agentry-cli"

// cliBinary is the agentry CLI this app pins: AGENTRY_CLI_BIN when set (an
// installed release, for a check by hand), else the pinned version built
// with the go tool — from the module cache, or from the directory a
// workspace or a replace resolves it to.
func cliBinary(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("AGENTRY_CLI_BIN"); bin != "" {
		return bin
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("no build info in the test binary")
	}
	var dep *debug.Module
	for _, m := range info.Deps {
		if m.Path == cliModule {
			dep = m
			if m.Replace != nil {
				dep = m.Replace
			}
		}
	}
	if dep == nil {
		t.Fatalf("%s is not a dependency of this build", cliModule)
	}
	out := t.TempDir()
	var cmd *exec.Cmd
	if strings.HasPrefix(dep.Path, "/") || strings.HasPrefix(dep.Path, ".") || dep.Version == "" || dep.Version == "(devel)" {
		dir := dep.Path
		if dir == cliModule {
			t.Skipf("%s resolves to a workspace directory this test cannot find; set AGENTRY_CLI_BIN", cliModule)
		}
		cmd = exec.Command("go", "build", "-o", filepath.Join(out, "agentry"), "./cmd/agentry")
		cmd.Dir = dir
	} else {
		cmd = exec.Command("go", "install", cliModule+"/cmd/agentry@"+dep.Version)
		cmd.Env = append(os.Environ(), "GOBIN="+out, "GOWORK=off", "GOFLAGS=")
	}
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the pinned CLI (%s): %v\n%s", strings.Join(cmd.Args, " "), err, b)
	}
	return filepath.Join(out, "agentry")
}

// The CLI and the desktop open each other's packs on one runner home: a
// pack this app keeps is listed and exported by `agentry knowledge`, and
// one the CLI keeps is listed, previewed and attached here. Both are
// signed in to the same (fake core) registry, with the keychain off, as a
// machine without a Secret Service is. `agentry run --knowledge NAME`
// needs docker, and is checked live.
func TestTwoClientsOpenEachOthersPacks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the pinned CLI")
	}
	cli := cliBinary(t)
	ctx := context.Background()
	reg := knowledgetest.NewRegistry(t)
	tok := reg.Token("ana")
	home, userHome, config := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	t.Setenv(modelkey.EnvNoKeyring, "1")
	desk := Env{
		Home:     policy.Home,
		Keys:     store.OSKeychain{}, // AGENTRY_NO_KEYRING: no cache, as for the CLI
		TokenFor: func(u string) string { return map[bool]string{true: tok}[strings.TrimRight(u, "/") == reg.URL] },
		HTTP:     reg.Client(),
	}
	agentry := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(cli, append(args, "--registry", reg.URL)...)
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"), "HOME=" + userHome, "AGENTRY_CONFIG_DIR=" + config,
			// AGENTRY_TOKEN authenticates only to the self-hosted registry
			// AGENTRY_REGISTRY names.
			policy.HomeEnv + "=" + home, modelkey.EnvNoKeyring + "=1", "AGENTRY_TOKEN=" + tok,
			"AGENTRY_REGISTRY=" + reg.URL, "AGENTRY_SKIP_VERSION_CHECK=1",
		}
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("agentry %s: %v\n%s", strings.Join(args, " "), err, b)
		}
		return string(b)
	}

	// The desktop keeps a pack; the CLI lists it and writes it out.
	mine := keep(t, desk, reg.URL, "from-desktop")
	var listed []struct {
		Name, Wrapper, SHA256 string
	}
	if err := json.Unmarshal([]byte(agentry("knowledge", "list", "--json")), &listed); err != nil {
		t.Fatalf("agentry knowledge list --json: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "from-desktop" || listed[0].Wrapper != "registry" || listed[0].SHA256 != mine.Manifest.SHA256 {
		t.Fatalf("the CLI lists %+v, want the desktop's pack", listed)
	}
	out := filepath.Join(t.TempDir(), "out")
	agentry("knowledge", "export", "from-desktop", "--to", out, "--yes")
	if p, err := kn.Load(out); err != nil || p.Manifest.SHA256 != mine.Manifest.SHA256 {
		t.Fatalf("the CLI's export of the desktop's pack: %v", err)
	}

	// The CLI keeps a pack; the panel lists it, previews it, and a run
	// here attaches it.
	agentry("knowledge", "import", knowledgetest.Make(t, "from-cli", nil).Dir)
	l, err := desk.List(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	var theirs *Pack
	for i := range l.Packs {
		if l.Packs[i].Name == "from-cli" {
			theirs = &l.Packs[i]
		}
	}
	if theirs == nil || !theirs.Local || theirs.Wrapper != "registry" {
		t.Fatalf("the panel lists %+v, want the CLI's pack kept here", l.Packs)
	}
	if pv, err := desk.Preview(ctx, reg.URL, "from-cli"); err != nil || len(pv.Pages) != 1 {
		t.Fatalf("the panel's preview of the CLI's pack: %+v, %v", pv, err)
	}
	packs, done, err := desk.Attach(ctx, reg.URL, []string{"from-cli"})
	if err != nil {
		t.Fatalf("attach the CLI's pack here: %v", err)
	}
	done()
	if len(packs) != 1 || packs[0].Digest != theirs.sha(t, desk, reg.URL) {
		t.Fatalf("attached %+v", packs)
	}
	noPlaintextLeft(t, desk)
}

// sha is the kept pack's digest, from the store.
func (p *Pack) sha(t *testing.T, e Env, reg string) string {
	t.Helper()
	en, err := e.Find(reg, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	return en.Manifest.SHA256
}
