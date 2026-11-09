// The Assistant's publisher tools that change something in the registry:
// quickagent_fork and quickagent_submit. Each is put to the person in the
// app's native dialog, which names the agent asking, every time it is
// asked: no earlier Yes stands for a later call, and the setting that lets
// the Assistant start runs without asking does not reach these. A No sends
// nothing.
//
// A submission is of a copy: the folder in the share is copied out through
// an os.Root before anything is compared or asked, and the copy the person
// agreed to is the one sent, so a change the agent makes to the share
// after the question is not what goes. The dialog says whether the
// QuickAgent is new (and so created private), private or public, and for a
// public one that everyone who installs or updates it receives the version
// once the pipeline publishes it; the files added, changed and removed
// since the version the agent fetched (else the latest); every capability
// the manifest adds or widens, in the registry's own words; and what the
// submission spends. A version that is not newer than every version the
// QuickAgent has is refused, naming the newest. A QuickAgent that ships
// knowledge packs is refused: shipping a pack is confirmed pack by pack in
// the Build tab.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Masterminds/semver/v3"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/build"
)

const (
	// forkWait bounds how long quickagent_fork follows a fork's copy.
	forkWait = 60 * time.Second
	// forkPoll is how often it reads the fork's state meanwhile.
	forkPoll = 2 * time.Second
	// maxSubmitFiles and maxSubmitBytes bound the folder a submission
	// copies out of the share.
	maxSubmitFiles = 2000
	maxSubmitBytes = 64 << 20
	// maxChangesNamed bounds the files the dialog names in each list.
	maxChangesNamed = 30
)

func (m *mainLink) quickAgentFork(ref, into, name string) (string, error) {
	ns, origin, version, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	if version != "" {
		return "", errors.New("name the QuickAgent to fork as namespace/name: a fork starts from its latest published version")
	}
	name = strings.TrimSpace(name)
	if name != "" && !manifest.ValidName(name) {
		return "", fmt.Errorf("%s is not a name a QuickAgent can have: lowercase letters, digits and hyphens, starting and ending with a letter or digit", quoted(name))
	}
	reg := m.ops.registry()
	acct, err := m.publisher.account(reg)
	if err != nil {
		return "", err
	}
	if acct == nil {
		return "", errSignedOut
	}
	target := strings.TrimSpace(into)
	if target == "" {
		target = acct.handle
	}
	if !slices.Contains(acct.publishes(), target) {
		return "", fmt.Errorf("%s is neither the person's namespace nor one of the organizations they publish for (%s)", quoted(target), strings.Join(acct.publishes(), ", "))
	}
	forkName := cmpOr(name, origin)
	title := m.who() + " asks to fork " + ns + "/" + origin + " into " + target + "/" + forkName
	if err := m.app.askConsent(title, strings.Join(forkTerms, "\n\n")); err != nil {
		return "", err
	}
	v, err := m.publisher.fork(reg, ns, origin, target, name)
	if err != nil {
		return "", err
	}
	switch {
	case v.Refusal != "":
		return "", errors.New(v.Refusal)
	case v.Namespace == "" || v.Name == "":
		return "", errors.New("the registry did not say what the fork is called")
	}
	made := v.Namespace + "/" + v.Name
	deadline := time.Now().Add(m.forkWait())
	for {
		state, detail, err := m.publisher.forkState(reg, v.Namespace, v.Name)
		switch {
		case err != nil:
			return "Forked " + ns + "/" + origin + " into " + made + "; its state could not be read now, so ask quickagent_mine for it before you fetch it.", nil
		case state == "ready" || state == "":
			return "Forked " + ns + "/" + origin + " into " + made + ", and it is ready: quickagent_fetch " + made + " to change it. It stays private for good.", nil
		case state == "failed" && detail != "":
			return "", fmt.Errorf("the registry made %s, but copying %s/%s into it failed. The registry says why: %s. Tell the person, who can delete it in the Publisher section", made, ns, origin, quoted(detail))
		case state == "failed":
			return "", fmt.Errorf("the registry made %s, but copying %s/%s into it failed; tell the person, who can delete it in the Publisher section", made, ns, origin)
		case time.Now().After(deadline):
			return "Forked " + ns + "/" + origin + " into " + made + "; it is still being copied: ask quickagent_mine for it again in a minute before you fetch it.", nil
		}
		time.Sleep(min(forkPoll, m.forkWait()))
	}
}

// forkWait is how long a fork's copy is followed.
func (m *mainLink) forkWait() time.Duration {
	if m.forkFollow > 0 {
		return m.forkFollow
	}
	return forkWait
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// fileKey is what a submission compares a file by: its content's sha256.
// Whether it is executable is kept in the copy, which the archive records,
// but not compared: a release is unpacked without it.
func fileKey(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// submission is a folder copied out of the share for a submission: its
// manifest, its files by path with their key (fileKey), and where the copy
// is.
type submission struct {
	dir   string
	man   *manifest.Manifest
	files map[string]string
}

func (m *mainLink) quickAgentSubmit(ctx context.Context, folder string) (string, error) {
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
	man := sub.man
	if man.Namespace != ns || man.Name != name {
		return "", fmt.Errorf("the folder is %s/%s, but its agent.yaml names %s/%s: they must agree", ns, name, man.Namespace, man.Name)
	}
	if ships := man.ShippedPacks(); len(ships) > 0 {
		return "", fmt.Errorf("%s/%s ships the knowledge packs %s, and shipping a pack is confirmed pack by pack: the person submits this version from Agentry's Build tab", ns, name, strings.Join(ships, ", "))
	}
	next, err := semver.StrictNewVersion(man.Version)
	if err != nil {
		return "", fmt.Errorf("%s is not a version: write it as major.minor.patch, such as 1.2.3", quoted(man.Version))
	}
	exists, private, err := m.publisher.standing(reg, ns, name)
	if err != nil {
		return "", err
	}
	var base *submission
	baseFrom := ""
	if exists {
		vs, err := m.publisher.versions(reg, ns, name)
		if err != nil {
			return "", err
		}
		newest, err := newestVersion(vs)
		if err != nil {
			return "", fmt.Errorf("%s/%s's versions cannot all be read as versions (%w), so whether %s is newer is not known: the person submits this one from Agentry's Build tab", ns, name, err, man.Version)
		}
		if newest != nil && !next.GreaterThan(newest) {
			return "", fmt.Errorf("%s/%s already has version %s, and %s is not newer: give agent.yaml a newer version", ns, name, newest.Original(), man.Version)
		}
		ref, from := m.comparedWith(ns, name, vs)
		if ref != "" {
			b, err := m.baseRelease(ctx, reg, ref)
			if err != nil {
				return "", fmt.Errorf("read %s to compare with: %w", ref, err)
			}
			defer func() { _ = os.RemoveAll(b.dir) }()
			base, baseFrom = b, from
		}
	}
	if exists && base == nil {
		baseFrom = "none: it has no published version to compare with"
	}
	text := submitDialogText(ns+"/"+name+"@"+man.Version, exists, private, sub, base, baseFrom, m.publisher.cost(reg))
	if err := m.app.askConsent(m.who()+" asks to submit "+ns+"/"+name+"@"+man.Version, text); err != nil {
		return "", err
	}
	res, err := m.publisher.publish(ctx, reg, sub.dir, private, !exists)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", errors.New("the registry did not answer the submission")
	}
	if res.Status == build.StatusRefused {
		return "", fmt.Errorf("not submitted: %s", res.Detail)
	}
	out := "Submitted " + ns + "/" + name + "@" + res.Version + ": " + submittedState(res.Status)
	if n := findingsText(res.FindingCounts); n != "" {
		out += ", with " + n
	}
	if d := strings.TrimSpace(res.Detail); d != "" {
		out += ": " + quoted(cut(d, maxReasonText))
	}
	out += ". Its report: " + reportURL(reg, ns, name, res.Version)
	if !exists {
		out += ". It was created private; making it public is the person's, in Agentry's Publisher section."
	}
	return out, nil
}

// submitFolder reads the folder a submission names: share/quickagents/<ns>/<name>.
func submitFolder(folder string) (ns, name string, err error) {
	f := strings.TrimSuffix(strings.TrimSpace(folder), "/")
	f = strings.TrimPrefix(strings.TrimPrefix(f, "/agent/"), "share/")
	parts := strings.Split(f, "/")
	if len(parts) != 3 || parts[0] != "quickagents" || !refPart.MatchString(parts[1]) || !refPart.MatchString(parts[2]) {
		return "", "", fmt.Errorf("%s is not a QuickAgent's folder: name it as share/quickagents/<namespace>/<name>", quoted(folder))
	}
	return parts[1], parts[2], nil
}

// copyOutOfShare copies quickagents/<ns>/<name> out of the share, through
// an os.Root of it, into a folder of its own: regular files alone, within
// the bounds; a link or anything else is refused.
func copyOutOfShare(share, ns, name string) (*submission, error) {
	root, err := os.OpenRoot(share)
	if err != nil {
		return nil, fmt.Errorf("open the share folder: %w", err)
	}
	defer func() { _ = root.Close() }()
	src := path.Join("quickagents", ns, name)
	if info, err := root.Lstat(src); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("there is no folder share/%s: fetch the QuickAgent or write its files there first", src)
	}
	dir, err := os.MkdirTemp("", "agentry-submit-*")
	if err != nil {
		return nil, err
	}
	sub := &submission{dir: dir, files: map[string]string{}}
	total := int64(0)
	err = fs.WalkDir(root.FS(), src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, src), "/")
		if rel == "" {
			return nil
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(filepath.Join(dir, filepath.FromSlash(rel)), 0o700)
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file", rel)
		}
		if len(sub.files) == maxSubmitFiles {
			return fmt.Errorf("the folder holds more than %d files", maxSubmitFiles)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		f, err := root.Open(p)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(f, maxSubmitBytes-total+1))
		_ = f.Close()
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > maxSubmitBytes {
			return fmt.Errorf("the folder holds more than %d MiB", maxSubmitBytes>>20)
		}
		// The archive keeps whether a file is executable, so the copy does.
		mode := os.FileMode(0o600)
		if info.Mode()&0o111 != 0 {
			mode = 0o700
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), data, mode); err != nil {
			return err
		}
		sub.files[rel] = fileKey(data)
		return nil
	})
	if err == nil {
		sub.man, err = build.ReadManifest(dir)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("read share/%s: %w", src, err)
	}
	return sub, nil
}

// newestVersion is the newest of a QuickAgent's versions, by semver; nil
// for none. A version that does not read as one is an error, never skipped:
// skipped, it could be the newest.
func newestVersion(vs []apiclient.AgentVersion) (*semver.Version, error) {
	var newest *semver.Version
	for _, v := range vs {
		sv, err := semver.NewVersion(v.Version)
		if err != nil {
			return nil, fmt.Errorf("%s is not a version", quoted(v.Version))
		}
		if newest == nil || sv.GreaterThan(newest) {
			newest = sv
		}
	}
	return newest, nil
}

// comparedWith is the release a submission is compared with, and how it
// is named in the dialog: the version the agent fetched, when that is one
// of the QuickAgent's; else its newest published version; "" for none.
func (m *mainLink) comparedWith(ns, name string, vs []apiclient.AgentVersion) (ref, from string) {
	if m.fetchedPath != nil {
		if p, err := m.fetchedPath(); err == nil {
			if rec, ok := readFetched(p, ns+"/"+name); ok {
				_, version, _ := strings.Cut(rec.Ref, "@")
				for _, v := range vs {
					if v.Version == version {
						return rec.Ref, "the version it fetched, " + version
					}
				}
			}
		}
	}
	newestFirst(vs)
	for _, v := range vs {
		if versionState(v.Status) == "published" {
			return ns + "/" + name + "@" + v.Version, "its latest published version, " + v.Version
		}
	}
	return "", ""
}

// readFetched is the fetch kept for key in the file at p.
func readFetched(p, key string) (fetchRecord, bool) {
	raw, err := os.ReadFile(p) //nolint:gosec // the app's own record beside its settings
	if err != nil {
		return fetchRecord{}, false
	}
	var recs fetchRecords
	if json.Unmarshal(raw, &recs) != nil {
		return fetchRecord{}, false
	}
	rec, ok := recs.Fetched[key]
	return rec, ok
}

// baseRelease is ref fetched and read as a submission is: its included
// files with their sha256, and its manifest.
func (m *mainLink) baseRelease(ctx context.Context, reg, ref string) (*submission, error) {
	rel, err := m.publisher.fetch(ctx, reg, ref)
	if err != nil {
		return nil, err
	}
	b := &submission{dir: rel.dir, files: map[string]string{}}
	err = filepath.WalkDir(rel.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // a release this app unpacked itself
		if err != nil {
			return err
		}
		r, err := filepath.Rel(rel.dir, p)
		if err != nil {
			return err
		}
		b.files[filepath.ToSlash(r)] = fileKey(data)
		return nil
	})
	if err == nil {
		b.man, err = build.ReadManifest(rel.dir)
	}
	if err != nil {
		_ = os.RemoveAll(rel.dir)
		return nil, err
	}
	return b, nil
}

// included are the files of sub its manifest's publish.include sends.
func included(sub *submission) map[string]string {
	files, err := pkgarchive.WalkWorkspace(sub.dir)
	if err != nil {
		return sub.files
	}
	out := map[string]string{}
	for _, f := range manifest.ResolveInclude(sub.man.Publish.Include, files) {
		if h, ok := sub.files[f.Path]; ok {
			out[f.Path] = h
		}
	}
	return out
}

// submitDialogText is the submission's question: the QuickAgent's
// standing, the files and capabilities that changed, and the cost.
func submitDialogText(ref string, exists, private bool, sub, base *submission, baseFrom, cost string) string {
	var lines []string
	switch {
	case !exists:
		lines = append(lines, ref+" is new: Agentry creates it private, so only the person and those they share it with can find it. Making it public stays the person's, in the Publisher section.")
	case private:
		lines = append(lines, ref+" is private, and this version stays private with it.")
	default:
		lines = append(lines, ref+" is public.", "Everyone who installs or updates it receives this version once the pipeline publishes it.")
	}
	sent := included(sub)
	if base == nil {
		if baseFrom != "" {
			lines = append(lines, "", fmt.Sprintf("The %d files it sends (compared with %s):", len(sent), baseFrom))
		} else {
			lines = append(lines, "", fmt.Sprintf("All %d files are new:", len(sent)))
		}
		lines = append(lines, namedList(slices.Sorted(maps.Keys(sent)))...)
	} else {
		was := included(base)
		var added, changed, removed []string
		for f, h := range sent {
			switch old, ok := was[f]; {
			case !ok:
				added = append(added, f)
			case old != h:
				changed = append(changed, f)
			}
		}
		for f := range was {
			if _, ok := sent[f]; !ok {
				removed = append(removed, f)
			}
		}
		lines = append(lines, "", "Compared with "+baseFrom+":")
		for _, l := range []struct {
			what  string
			files []string
		}{{"Added", added}, {"Changed", changed}, {"Removed", removed}} {
			if len(l.files) == 0 {
				continue
			}
			slices.Sort(l.files)
			lines = append(lines, l.what+":")
			lines = append(lines, namedList(l.files)...)
		}
		if len(added)+len(changed)+len(removed) == 0 {
			lines = append(lines, "No file changed.")
		}
	}
	var widened []string
	if base == nil {
		widened = manifest.DiffCapabilities(manifest.Capabilities{}, sub.man.Capabilities).Widened
	} else {
		widened = manifest.DiffCapabilities(base.man.Capabilities, sub.man.Capabilities).Widened
	}
	if len(widened) == 0 {
		lines = append(lines, "", "It adds or widens no capability.")
	} else {
		lines = append(lines, "", "What it may reach that it could not before:")
		for _, w := range widened {
			lines = append(lines, "  • "+plainText(w))
		}
	}
	return strings.Join(append(lines, "", cost, "The pipeline then checks this version like any other, and decides whether it is published.", "", "Submit it?"), "\n")
}

// namedList is files as the dialog lists them, cut at maxChangesNamed.
func namedList(files []string) []string {
	var out []string
	for i, f := range files {
		if i == maxChangesNamed {
			out = append(out, fmt.Sprintf("  … and %d more", len(files)-i))
			break
		}
		out = append(out, "  • "+plainText(f))
	}
	return out
}

// plainText is text the agent chose (a file's name, a host its manifest
// names) as the dialog shows it: as it is when every character is a
// printable one, else quoted with its control and format characters
// escaped, so it cannot break a line or reorder what follows it.
func plainText(s string) string {
	for _, r := range s {
		if !unicode.IsGraphic(r) || unicode.Is(unicode.Cf, r) {
			return strconv.QuoteToASCII(s)
		}
	}
	return s
}

// submittedState is a submission's outcome in the person's words.
func submittedState(status string) string {
	switch status {
	case "published":
		return "published"
	case "needs_review":
		return "held for a person to review"
	case "rejected":
		return "rejected by the pipeline"
	}
	return "submitted, " + quoted(status)
}

// findingsText is a report's finding counts in words: "2 high and 1 low
// findings"; "" for none.
func findingsText(counts map[string]int) string {
	var parts []string
	total := 0
	for _, sev := range []string{"critical", "high", "medium", "low", "info"} {
		if n := counts[sev]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, sev))
			total += n
		}
	}
	if total == 0 {
		return ""
	}
	word := "findings"
	if total == 1 {
		word = "finding"
	}
	if len(parts) == 1 {
		return parts[0] + " " + word
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1] + " " + word
}
