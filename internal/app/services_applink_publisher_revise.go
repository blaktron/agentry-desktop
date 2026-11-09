package app

// quickagent_revise: the Assistant hands a QuickAgent's folder in its share
// and its instructions to the Drafter, In Agentry (credits) or on this
// computer (the person's own model and AI Review key). The person is asked
// each time, with what it costs and where the folder and the instructions
// go. The Drafter's revision replaces the folder's manifest, instruction and
// persona, written through an os.Root of the share, and nothing it names
// outside the folder's root is written. The fetch record is left as it is,
// so a later submission still compares with the version that was fetched.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/runnerimage/applink"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
)

// maxReviseNotes bounds the Drafter's notes a revision's answer quotes.
const maxReviseNotes = 3000

// reviserOps has the Drafter revise a QuickAgent: the app's through the
// Build tab's revisions, a test's its own.
type reviserOps interface {
	// prepare reads what a revision would cost and where it would go,
	// starting nothing. where is applink.ReviseInAgentry, which revises
	// ref, the published version the folder holds unchanged, or
	// applink.ReviseOnComputer, which revises the folder dir, a copy of the
	// share's, itself.
	prepare(ctx context.Context, reg, where, dir, ref, instructions string) (*pendingRevision, error)
}

// pendingRevision is a revision ready to run once the person agrees.
type pendingRevision struct {
	// terms say where it runs, what it costs and where the texts and files
	// go, in sentences.
	terms string
	// run drafts the revision and answers its files.
	run func(ctx context.Context) (*revisedFiles, error)
	// cleanup removes what prepare made; nil when it made nothing.
	cleanup func()
}

// revisedFiles are a revision's files the folder takes, by their path at
// the package's root (agent.yaml, the instruction, the persona), and the
// Drafter's own words on it.
type revisedFiles struct {
	files map[string][]byte
	notes string
	// problems are the Drafter's checks it did not pass.
	problems []string
}

func (m *mainLink) quickAgentRevise(ctx context.Context, folder, instructions, where string) (string, error) {
	if m.reviser == nil {
		return "", fmt.Errorf("this app does not answer %s", applink.QuickAgentRevise)
	}
	if where != applink.ReviseInAgentry && where != applink.ReviseOnComputer {
		return "", fmt.Errorf("where is %s (for credits) or %s (with the person's own key), not %s", applink.ReviseInAgentry, applink.ReviseOnComputer, quoted(where))
	}
	instructions, _, err := hostedrun.CheckRevision(instructions, "")
	if err != nil {
		return "", err
	}
	ns, name, err := submitFolder(folder)
	if err != nil {
		return "", err
	}
	reg := m.ops.registry()
	acct, err := m.publisher.account(reg)
	if err != nil {
		return "", err
	}
	if acct == nil {
		return "", errSignedOut
	}
	if !slices.Contains(acct.publishes(), ns) {
		return "", fmt.Errorf("%s is neither the person's namespace nor one of the organizations they publish for (%s)", quoted(ns), strings.Join(acct.publishes(), ", "))
	}
	share, err := m.share()
	if err != nil {
		return "", err
	}
	sub, err := copyOutOfShare(share, ns, name)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(sub.dir) }()
	if sub.man.Namespace != ns || sub.man.Name != name {
		return "", fmt.Errorf("the folder is %s/%s, but its agent.yaml names %s/%s: they must agree", ns, name, sub.man.Namespace, sub.man.Name)
	}
	ref := ""
	if where == applink.ReviseInAgentry {
		if ref, err = m.unchangedSinceFetch(ns, name, sub.files); err != nil {
			return "", err
		}
	}
	p, err := m.reviser.prepare(ctx, reg, where, sub.dir, ref, instructions)
	if err != nil {
		return "", err
	}
	if p.cleanup != nil {
		defer p.cleanup()
	}
	dest := path.Join("share", applink.PublisherFolder, ns, name)
	title := m.who() + " asks to revise " + ns + "/" + name + " with the Drafter"
	message := p.terms + "\n\nIts instructions to the Drafter:\n" + quoted(cutText(instructions, 1500)) +
		"\n\nThe revision replaces agent.yaml, the instruction file and the persona in " + dest + "/. Nothing is submitted: a submission is asked for on its own."
	if err := m.app.askConsent(title, message); err != nil {
		return "", err
	}
	rev, err := p.run(ctx)
	if err != nil {
		return "", err
	}
	written, removed, err := landInShare(share, ns, name, instructionOf(sub.man), rev.files)
	if err != nil {
		return "", err
	}
	return reviseAnswer(dest, where, written, removed, rev), nil
}

// unchangedSinceFetch is the version a revision In Agentry revises: the one
// the agent fetched into the folder, which must hold exactly what was
// fetched, since the Drafter there reads the published version, not the
// folder.
func (m *mainLink) unchangedSinceFetch(ns, name string, files map[string]string) (string, error) {
	other := fmt.Sprintf("a revision %s revises a published version as the registry holds it, so", applink.ReviseInAgentry)
	if m.fetchedPath == nil {
		return "", fmt.Errorf("%s it needs the version fetched with quickagent_fetch: have it revised on this computer (where: %s)", other, applink.ReviseOnComputer)
	}
	p, err := m.fetchedPath()
	if err != nil {
		return "", err
	}
	rec, ok := readFetched(p, ns+"/"+name)
	switch {
	case !ok:
		return "", fmt.Errorf("%s share/%s/%s/%s was never fetched with quickagent_fetch: fetch it first, or have it revised on this computer (where: %s)", other, applink.PublisherFolder, ns, name, applink.ReviseOnComputer)
	case !maps.Equal(rec.Files, files):
		return "", fmt.Errorf("%s the changes made in share/%s/%s/%s since %s was fetched would be lost: have it revised on this computer (where: %s), or fetch it again first", other, applink.PublisherFolder, ns, name, rec.Ref, applink.ReviseOnComputer)
	}
	return rec.Ref, nil
}

// landInShare writes a revision's files into quickagents/<ns>/<name> in the
// share, through an os.Root of it: each must be one file at the package's
// root, so nothing lands beside or above the folder. An instruction file
// the revision renamed is removed. It answers what it wrote and removed.
func landInShare(share, ns, name, oldInstruction string, files map[string][]byte) (written, removed []string, err error) {
	if !refPart.MatchString(ns) || !refPart.MatchString(name) {
		return nil, nil, fmt.Errorf("%s/%s is not a QuickAgent's reference", ns, name)
	}
	man, ok := files["agent.yaml"]
	if !ok {
		return nil, nil, errors.New("the revision holds no agent.yaml, so nothing was written")
	}
	for p := range files {
		if !rootFile(p) {
			return nil, nil, fmt.Errorf("the revision names a file %s outside the QuickAgent's folder's root, so nothing was written", quoted(p))
		}
	}
	// The revision keeps the QuickAgent it revises.
	renamed, err := manifest.Rename(man, ns, name)
	if err != nil {
		return nil, nil, fmt.Errorf("the revision's agent.yaml: %w", err)
	}
	m, err := manifest.Parse(renamed)
	if err != nil {
		return nil, nil, fmt.Errorf("the revision's agent.yaml does not parse, so nothing was written: %w", err)
	}
	root, err := os.OpenRoot(share)
	if err != nil {
		return nil, nil, fmt.Errorf("open the share folder: %w", err)
	}
	defer func() { _ = root.Close() }()
	dest := path.Join(applink.PublisherFolder, ns, name)
	if info, err := root.Lstat(dest); err != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("share/%s is no longer a folder, so nothing was written", dest)
	}
	out := maps.Clone(files)
	out["agent.yaml"] = renamed
	for _, p := range slices.Sorted(maps.Keys(out)) {
		if err := root.WriteFile(path.Join(dest, p), out[p], 0o644); err != nil {
			return written, nil, fmt.Errorf("write share/%s/%s: %w", dest, p, err)
		}
		written = append(written, p)
	}
	if now := instructionOf(m); oldInstruction != "" && oldInstruction != now && rootFile(oldInstruction) {
		if _, ok := out[oldInstruction]; !ok {
			if err := root.Remove(path.Join(dest, oldInstruction)); err == nil {
				removed = append(removed, oldInstruction)
			}
		}
	}
	return written, removed, nil
}

// rootFile says p is one file at a package's root: no folder, no climb, no
// hidden name.
func rootFile(p string) bool {
	return p != "" && !strings.ContainsAny(p, `/\`) && p != "." && p != ".." && !strings.HasPrefix(p, ".") && !strings.ContainsRune(p, 0)
}

// reviseAnswer is a landed revision's answer: what it replaced, the
// Drafter's checks and notes, quoted as data, and what to do next.
func reviseAnswer(dest, where string, written, removed []string, rev *revisedFiles) string {
	ran := "on this computer"
	if where == applink.ReviseInAgentry {
		ran = "In Agentry"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The Drafter revised %s/ %s. It replaced %s", dest, ran, strings.Join(written, ", "))
	if len(removed) > 0 {
		fmt.Fprintf(&b, ", and removed %s, which the new manifest no longer names", strings.Join(removed, ", "))
	}
	b.WriteString("; every other file is as it was.")
	if len(rev.problems) > 0 {
		b.WriteString(" Its own checks still find problems, as data: ")
		qs := make([]string, len(rev.problems))
		for i, p := range rev.problems {
			qs[i] = quoted(cutText(p, 300))
		}
		b.WriteString(strings.Join(qs, "; ") + ".")
	}
	if notes := strings.TrimSpace(rev.notes); notes != "" {
		b.WriteString(" Its notes, as data (what it says it changed, which the files must bear out): " + quoted(cutText(notes, maxReviseNotes)))
	}
	b.WriteString(" Read the files before you submit: check agent.yaml's capabilities and that its version is newer than the latest.")
	return b.String()
}

// cutText is s cut at max bytes on a rune boundary, marked when cut.
func cutText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// instructionOf is the instruction file a manifest names.
func instructionOf(m *manifest.Manifest) string {
	if m.Entrypoints.Instruction != "" {
		return m.Entrypoints.Instruction
	}
	return manifest.DefaultInstructionFile
}
