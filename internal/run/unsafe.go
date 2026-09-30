package run

// Unsafe Mode's desktop half (agentry-notes plans/unsafe-mode.md M3;
// agentry-desktop#418): the base's pull when the switch is turned on, the
// data a run on this computer compiles with while it is on, and the line
// the run's consent carries. The runner's half is the CLI's
// (pkg/runner/unsafeexec, compile.Inputs.Unsafe; DR-53); nothing here
// decides that Unsafe Mode is on — the caller reads desktop.yaml, which
// only Go writes after a native confirmation (D-030, D-050).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/runner/unsafeexec"
)

// pullImage pulls ref, streaming docker's own progress lines to logw when
// the client is the docker CLI: the Unsafe Mode base is a few hundred
// megabytes, and the card shows the pull as it goes. A package variable as
// a test seam; any other client (a test's fake) pulls through the client.
var pullImage = func(ctx context.Context, docker dockerx.Client, ref string, logw io.Writer) error {
	c, ok := docker.(*dockerx.CLI)
	if !ok {
		return docker.Pull(ctx, ref)
	}
	cmd := exec.CommandContext(ctx, c.Bin, "pull", ref) //nolint:gosec // the runner's docker binary, a ref held to unsafeexec.ValidBase
	cmd.Env = os.Environ()
	procx.NoWindow(cmd)
	var errb strings.Builder
	cmd.Stdout = logw
	cmd.Stderr = io.MultiWriter(logw, &errb)
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return fmt.Errorf("docker pull: %w: %s", err, msg)
		}
		return fmt.Errorf("docker pull: %w", err)
	}
	return nil
}

// unsafeBase is the Unsafe Mode base the runner policy names
// (images.unsafe, else the CLI's pinned default), read without creating the
// policy. A policy that cannot be read is said: the run would refuse on it.
func unsafeBase() (string, error) {
	path, err := policy.DefaultPath()
	if err != nil {
		return "", err
	}
	pol, _, err := policy.Load(path)
	switch {
	case err == nil:
		return pol.Images.UnsafeImage(), nil
	case errors.Is(err, os.ErrNotExist):
		return policy.Default().Images.UnsafeImage(), nil
	}
	return "", fmt.Errorf("operator policy: %w", err)
}

// UnsafeBase is the image Unsafe Mode builds exec images on, for the Run
// defaults card to name.
func UnsafeBase() (string, error) { return unsafeBase() }

// PullUnsafeBase is what turning Unsafe Mode on does before the switch
// reads on (plan D2, D6): pull the base the policy names, with docker's
// progress on logw, then read its command index and extras list back, so a
// base that is on this machine is also one a run can compile with. It
// returns the base it pulled.
func PullUnsafeBase(ctx context.Context, logw io.Writer) (string, error) {
	base, err := unsafeBase()
	if err != nil {
		return "", err
	}
	if err := unsafeexec.ValidBase(base); err != nil {
		return "", err
	}
	docker := newDockerClient()
	if err := docker.Available(ctx); err != nil {
		return "", err
	}
	ok, err := docker.ImageExists(ctx, base)
	if err != nil {
		return "", err
	}
	if ok {
		_, _ = fmt.Fprintf(logw, "the Unsafe Mode image is already on this computer: %s\n", base)
	} else {
		_, _ = fmt.Fprintf(logw, "downloading the Unsafe Mode image %s\n", base)
		if err := pullImage(ctx, docker, base, logw); err != nil {
			return "", fmt.Errorf("download the Unsafe Mode image: %w", err)
		}
	}
	if _, err := unsafeexec.Load(ctx, docker, base, logw); err != nil {
		return "", err
	}
	_, _ = fmt.Fprintln(logw, "the Unsafe Mode image is ready")
	return base, nil
}

// unsafeData is what a run compiles with while Unsafe Mode is on: the
// base's placement data, loaded only when the release declares commands
// (nothing else is lifted, so a release with none compiles as it always
// did and pulls nothing). Nil means the default posture.
func unsafeData(ctx context.Context, docker dockerx.Client, pol *policy.Policy, m *manifest.Manifest, on bool, logw io.Writer) (*unsafeexec.Data, error) {
	if !on || len(execCommands(m)) == 0 {
		return nil, nil
	}
	data, err := unsafeexec.Load(ctx, docker, pol.Images.UnsafeImage(), logw)
	if err != nil {
		return nil, fmt.Errorf("the Unsafe Mode image: %w", err)
	}
	return data, nil
}

// execCommands are the commands a release declares for run_command.
func execCommands(m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}
	return m.Capabilities.Exec.Commands
}

// planIsUnsafe says whether a plan was compiled in Unsafe Mode.
func planIsUnsafe(plan *spec.Plan) bool {
	return plan.ExecImage != nil && plan.ExecImage.Unsafe != nil
}

// UnsafeNotice is what Unsafe Mode means for one release's run on this
// computer: the commands it declares and the ones the image cannot place
// (D8), for the run's consent and its Review step.
type UnsafeNotice struct {
	// Commands are the declared exec commands, sorted; none means Unsafe
	// Mode changes nothing about this run.
	Commands []string
	// Unplaced are the declared commands the image cannot provide, each
	// with the packages that ship it when there are several.
	Unplaced []string
	// Unknown says why the placement could not be read (the image is not
	// on this computer, docker is not running), "" when it was.
	Unknown string
}

// Line is the one sentence a local run's consent carries while Unsafe Mode
// is on (plan D6, D8, and DR-53's PATH): it names Unsafe Mode, says what is
// not checked, and lists what the image cannot provide.
func (n *UnsafeNotice) Line() string {
	if len(n.Commands) == 0 {
		return "Unsafe Mode is on. This QuickAgent declares no commands, so Unsafe Mode changes nothing about this run."
	}
	line := "Unsafe Mode is on: the commands this QuickAgent declares (" + strings.Join(n.Commands, ", ") +
		") are installed from Kali Linux's repository or PyPI without asking this computer's approved list, and each runs with the image's own PATH, so it can reach every program that image holds. Only the declared commands can be called."
	switch {
	case n.Unknown != "":
		line += " Which of them the image can provide could not be read (" + n.Unknown + ")."
	case len(n.Unplaced) > 0:
		line += " Not available in Unsafe Mode's image: " + strings.Join(n.Unplaced, "; ") + ". The run goes on without " + thatOrThose(len(n.Unplaced)) + "."
	}
	return line
}

func thatOrThose(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// UnsafeNoticeFor resolves ref as a run on this computer would and places
// its declared commands through the Unsafe Mode base this machine holds. It
// pulls nothing: a base that is not here yet is said, and the run downloads
// it. The caller asks only while Unsafe Mode is on.
func UnsafeNoticeFor(ctx context.Context, regURL, ref string) (*UnsafeNotice, error) {
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	n := &UnsafeNotice{Commands: append([]string(nil), execCommands(sig.Manifest)...)}
	if len(n.Commands) == 0 {
		return n, nil
	}
	base, err := unsafeBase()
	if err != nil {
		n.Unknown = err.Error()
		return n, nil
	}
	docker := newDockerClient()
	if ok, err := docker.ImageExists(ctx, base); err != nil || !ok {
		n.Unknown = "the Unsafe Mode image is not on this computer; the run downloads it first"
		if err != nil {
			n.Unknown = err.Error()
		}
		return n, nil
	}
	data, err := unsafeexec.Load(ctx, docker, base, io.Discard)
	if err != nil {
		n.Unknown = err.Error()
		return n, nil
	}
	_, u := data.Place(n.Commands)
	n.Commands = nil
	for _, a := range u.Apt {
		n.Commands = append(n.Commands, a.Command)
	}
	for _, e := range u.Extras {
		n.Commands = append(n.Commands, e.Command)
	}
	for _, c := range u.Unplaced {
		n.Commands = append(n.Commands, c.Command)
		s := c.Command
		if len(c.Candidates) > 0 {
			s += " (several packages ship it: " + strings.Join(c.Candidates, ", ") + ")"
		}
		n.Unplaced = append(n.Unplaced, s)
	}
	sort.Strings(n.Commands)
	return n, nil
}
