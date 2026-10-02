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
// megabytes, and the card shows the pull as it goes. dockerx.Client's Pull
// returns only when it is over, so the CLI is driven here; any other client
// (a test's fake) pulls through the client.
func pullImage(ctx context.Context, docker dockerx.Client, ref string, logw io.Writer) error {
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
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
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

// PullUnsafeBase is what turning Unsafe Mode on does before the switch
// reads on (plan D2, D6): pull the base the policy names, with docker's
// progress on logw, then read its command index and extras list back, so a
// base that is on this machine is also one a run can compile with.
func PullUnsafeBase(ctx context.Context, logw io.Writer) error {
	base, err := unsafeBase()
	if err != nil {
		return err
	}
	if err := unsafeexec.ValidBase(base); err != nil {
		return err
	}
	docker := newDockerClient()
	if err := docker.Available(ctx); err != nil {
		return err
	}
	ok, err := docker.ImageExists(ctx, base)
	if err != nil {
		return err
	}
	if ok {
		_, _ = fmt.Fprintf(logw, "the Unsafe Mode image is already on this computer: %s\n", base)
	} else {
		_, _ = fmt.Fprintf(logw, "downloading the Unsafe Mode image %s\n", base)
		if err := pullImage(ctx, docker, base, logw); err != nil {
			return fmt.Errorf("download the Unsafe Mode image: %w", err)
		}
	}
	if _, err := unsafeexec.Load(ctx, docker, base, logw); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(logw, "the Unsafe Mode image is ready")
	return nil
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
// computer, for the run's consent dialog and its Review step.
//
// It is built from what the release resolves to, before anything is
// downloaded: a registry's answer, which this app has not verified, or a
// local install's own manifest. So the sentence about what Unsafe Mode does
// never depends on it, and what it lists is held to a grammar and a bound
// before it reaches a native dialog: only names unsafeexec.ValidCommand
// takes, at most noticeMax of them, with the rest counted. The run itself
// compiles the verified archive's manifest, and its plan's exec line, which
// the run log and the receipt carry, is the record of what was placed.
type UnsafeNotice struct {
	// Commands are the declared exec commands that are command names,
	// sorted.
	Commands []string
	// Malformed counts the declared names that are not command names: the
	// image cannot place them and run_command cannot call them.
	Malformed int
	// Unplaced are the declared commands the image cannot provide (D8),
	// each with the packages that ship it when there are several.
	Unplaced []string
	// Unknown says why the placement could not be read (the image is not
	// on this computer, docker is not running), "" when it was.
	Unknown string
	// Unread says the release itself could not be read here, so nothing
	// is listed at all.
	Unread bool
}

// noticeMax bounds each list a notice shows, and noticeWhy the length of a
// reason it quotes: the line goes into a native dialog, where a long one
// would push the question it sits above off the screen.
const (
	noticeMax = 40
	noticeWhy = 200
)

// UnsafeNoticeUnread is the notice for a release that could not be read
// here (the run would refuse on the same error a moment later): it still
// names Unsafe Mode, and lists nothing.
func UnsafeNoticeUnread() *UnsafeNotice { return &UnsafeNotice{Unread: true} }

// Line is the one paragraph a local run's consent carries while Unsafe Mode
// is on (plan D6, D8, DR-53's PATH, and D-055's servers): what Unsafe Mode
// does to any run,
// then what this release declares and what the image cannot provide.
func (n *UnsafeNotice) Line() string {
	line := "Unsafe Mode is on: every command this QuickAgent declares is installed from Kali Linux's repository or PyPI without asking this computer's approved list, and each runs with the image's own PATH, so it can start any program that image holds. run_command still takes only the declared commands. A declared MCP server the registry's gate rates high can run too, only with your consent for the run."
	switch {
	case n.Unread:
		return line
	case len(n.Commands) == 0 && n.Malformed == 0:
		return line + " As resolved, it declares none."
	}
	if len(n.Commands) > 0 {
		line += " As resolved, it declares: " + listed(n.Commands) + "."
	}
	if n.Malformed > 0 {
		line += fmt.Sprintf(" %d declared name%s not command names and cannot be called.", n.Malformed, isOrAre(n.Malformed))
	}
	switch {
	case n.Unknown != "":
		line += " Which of them the image can provide could not be read (" + clip(n.Unknown, noticeWhy) + ")."
	case len(n.Unplaced) > 0:
		line += " Not available in Unsafe Mode's image: " + listed(n.Unplaced) + ". The run goes on without " + itOrThem(len(n.Unplaced)) + "."
	}
	return line
}

// listed joins at most noticeMax items and counts the rest.
func listed(items []string) string {
	if len(items) <= noticeMax {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(items[:noticeMax], ", "), len(items)-noticeMax)
}

// clip is s on one line, cut to max bytes.
func clip(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func isOrAre(n int) string {
	if n == 1 {
		return " is"
	}
	return "s are"
}

func itOrThem(n int) string {
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
	n := &UnsafeNotice{}
	for _, c := range execCommands(sig.Manifest) {
		if unsafeexec.ValidCommand(c) {
			n.Commands = append(n.Commands, c)
		} else {
			n.Malformed++
		}
	}
	sort.Strings(n.Commands)
	if len(n.Commands) == 0 {
		return n, nil
	}
	base, err := unsafeBase()
	if err != nil {
		n.Unknown = err.Error()
		return n, nil
	}
	docker := newDockerClient()
	ok, err := docker.ImageExists(ctx, base)
	switch {
	case err != nil:
		n.Unknown = err.Error()
		return n, nil
	case !ok:
		n.Unknown = "the Unsafe Mode image is not on this computer; the run downloads it first"
		return n, nil
	}
	data, err := unsafeexec.Load(ctx, docker, base, io.Discard)
	if err != nil {
		n.Unknown = err.Error()
		return n, nil
	}
	// Every name placed here passed the command grammar, and the base's
	// own rows (the packages an ambiguous command names) passed the package
	// grammar when the data was parsed.
	_, u := data.Place(n.Commands)
	for _, c := range u.Unplaced {
		s := c.Command
		if len(c.Candidates) > 0 {
			s += " (several packages ship it: " + listed(c.Candidates) + ")"
		}
		n.Unplaced = append(n.Unplaced, s)
	}
	return n, nil
}
