// Package dockerimages finds and removes the Docker images Agentry put on
// this machine (agentry-desktop#424): the ones the runner pulls from
// Agentry's registry, the runner and per-QuickAgent exec images it builds,
// and whatever the runner policy names in their place. The Windows
// uninstaller removes them unless "Keep my Docker images" is ticked, and
// Run defaults offers the same on every platform, because the deb, rpm and
// dmg have no uninstaller that can ask.
//
// Images are chosen by the label every build of the runner carries
// (imagelabel.Key, agentry-cli#892), untagged leftovers included, and by
// name, and removed one by one by image ID (agentry-desktop#846): Docker
// Desktop's containerd store names an image pulled as repo:tag@digest by
// that whole reference, so the repository:tag the listing shows is not a
// name it can be removed by. Nothing here runs `docker image prune` or
// removes an image it did not select: a person's own images share the
// daemon, and some share Agentry's GitHub owner.
package dockerimages

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/imagelabel"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"
)

// RegistryPrefix is the name every image Agentry publishes starts with
// (agentry-dockerimages; the runner policy's defaults).
const RegistryPrefix = "ghcr.io/blaktron/agentry-"

// runnerRepo is the runner image built on this machine; its per-QuickAgent
// exec images are named "<runner repo>-exec" (spec.ExecImageTag).
const runnerRepo = "agentry-runner"

// builtRepos are the other images the runner builds on this machine, by
// repository: the decode step's, the host link's and the two harness
// images. A build made before the runner labelled its images
// (agentry-cli#892) is found by these names.
var builtRepos = []string{"agentry-decode", "agentry-hostlink-local", "agentry-claude-code", "agentry-copilot"}

// Image is one line of `docker image ls`.
type Image struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Digest     string `json:"digest,omitempty"`
	ID         string `json:"id"`
	Size       string `json:"size,omitempty"`
	// Labelled says the image carries the runner's build label
	// (imagelabel.Key): one of Agentry's builds whatever its name.
	Labelled bool `json:"labelled,omitempty"`
}

// named reports whether the image has a repository.
func (i Image) named() bool { return i.Repository != "" && i.Repository != "<none>" }

// Ref is the name the image is shown by: repository:tag, repository@digest
// for an image pulled by digest and never tagged, or, for an untagged
// build, the words and the short ID. It is never what the image is
// removed by: that is its ID.
func (i Image) Ref() string {
	if !i.named() {
		return "untagged Agentry build " + shortID(i.ID)
	}
	if i.Tag != "" && i.Tag != "<none>" {
		return i.Repository + ":" + i.Tag
	}
	if i.Digest != "" && i.Digest != "<none>" {
		return i.Repository + "@" + i.Digest
	}
	return i.ID
}

// fullRef is the name Docker Desktop's containerd store gives an image
// pulled as repo:tag@digest, "" when the image has no tag or digest.
func (i Image) fullRef() string {
	if !i.named() || i.Tag == "" || i.Tag == "<none>" || i.Digest == "" || i.Digest == "<none>" {
		return ""
	}
	return i.Repository + ":" + i.Tag + "@" + i.Digest
}

func shortID(id string) string {
	h := strings.TrimPrefix(id, "sha256:")
	if len(h) > 12 {
		h = h[:12]
	}
	return h
}

// Docker lists and removes images; CLI is the real one. Remove takes an
// image ID or a name.
type Docker interface {
	List(ctx context.Context) ([]Image, error)
	Remove(ctx context.Context, ref string) error
}

// splitRef is a ref's repository, tag and digest: "repo:tag@sha256:…".
func splitRef(ref string) (repo, tag, digest string) {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref, digest = ref[:i], ref[i+1:]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref, tag = ref[:i], ref[i+1:]
	}
	return ref, tag, digest
}

// Policy is what the runner policy names: the image refs it pins, matched
// exactly, and the runner image it renames, if any, whose builds and exec
// images are Agentry's by repository.
type Policy struct {
	Refs   []string
	Runner string
}

// ReadPolicy reads the runner policy under home. It is read before anything
// is removed, since the uninstaller's settings box deletes the policy. A
// missing or unreadable policy names nothing, and the defaults are under
// RegistryPrefix anyway.
func ReadPolicy(home string) Policy {
	pol, _, err := policy.Load(policy.PathIn(home))
	if err != nil {
		return Policy{}
	}
	i := pol.Images
	p := Policy{Runner: i.Runner}
	// images.unsafe is Unsafe Mode's base where an operator repointed it
	// (plan unsafe-mode.md D12); the files and host link images and the
	// harness images likewise. Their defaults are under RegistryPrefix or
	// built here.
	refs := []string{i.Gateway, i.HarnessDockerAgent, i.GoBuilder, i.NodeRuntime, i.PythonRuntime, i.Unsafe, i.Files, i.Hostlink}
	for _, h := range i.HarnessImages {
		refs = append(refs, h.Ref)
	}
	for _, r := range refs {
		if r != "" {
			p.Refs = append(p.Refs, r)
		}
	}
	return p
}

// Select keeps the images that are Agentry's, and nothing else:
//   - every image carrying the runner's build label (imagelabel.Key),
//     untagged ones included: a build left behind by a newer one, or a
//     stage the classic builder left (agentry-cli#892);
//   - every image under Agentry's registry prefix;
//   - the decode, host link and harness images the runner builds, by
//     repository, for builds made before the label;
//   - the runner image this app builds and the per-QuickAgent exec images
//     built on it, by repository (agentry-runner, agentry-runner-exec, or
//     the policy's renamed runner and its "-exec"); an Unsafe Mode exec
//     image is built on the Unsafe Mode base but named in the same "-exec"
//     repository (spec.UnsafeExecImageTag), so it is taken here too;
//   - each image the policy pins, matched by its exact tag or digest, so
//     an operator's `node:22-alpine` takes that image and never the rest of
//     `node`.
//
// Images with no repository and no label are left alone. A label is a
// claim anyone can build into an image: an image the person labels
// org.agentry.image themselves is taken as Agentry's, which the page and
// the uninstaller say. A labelled image the list shows under several
// names (one the person tagged it with besides Agentry's) has only the
// names the rules above take selected, so Remove keeps the image for the
// person's name.
func Select(all []Image, p Policy) []Image {
	builds := map[string]bool{runnerRepo: true, runnerRepo + "-exec": true}
	for _, r := range builtRepos {
		builds[r] = true
	}
	names := map[string]int{}
	for _, i := range all {
		if i.named() {
			names[i.ID]++
		}
	}
	if p.Runner != "" {
		repo, _, _ := splitRef(p.Runner)
		builds[repo] = true
		builds[repo+"-exec"] = true
	}
	var out []Image
	for _, i := range all {
		byName := i.named() && (strings.HasPrefix(i.Repository, RegistryPrefix) || builds[i.Repository] || pinned(i, p.Refs))
		if byName || (i.Labelled && names[i.ID] <= 1) {
			out = append(out, i)
		}
	}
	return out
}

// pinned reports whether the image is one of the refs, by the same
// repository and the same tag or digest.
func pinned(i Image, refs []string) bool {
	for _, r := range refs {
		repo, tag, digest := splitRef(strings.TrimSpace(r))
		if repo != i.Repository {
			continue
		}
		if (tag != "" && tag == i.Tag) || (digest != "" && digest == i.Digest) {
			return true
		}
	}
	return false
}

// Report is what a removal did.
type Report struct {
	Removed []string `json:"removed"`
	Failed  []string `json:"failed"`
}

// Selected lists Agentry's images on this daemon.
func Selected(ctx context.Context, d Docker, p Policy) ([]Image, error) {
	all, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	return Select(all, p), nil
}

// RemoveAll lists Agentry's images and removes them: the uninstaller's
// step, which shows no list. A daemon that cannot list removes nothing.
func RemoveAll(ctx context.Context, d Docker, p Policy) (*Report, error) {
	imgs, err := Selected(ctx, d, p)
	if err != nil {
		return nil, err
	}
	return Remove(ctx, d, imgs), nil
}

// Remove removes exactly these images, the list a person confirmed and
// nothing listed after it, each by its image ID, once however many of the
// listed names it carries. An image the daemon refuses (one a running
// container uses) is reported and the rest are still removed. Each is
// reported by the name the list showed.
//
// An ID carrying several names is refused by `docker image rm <id>`
// without --force, which is never passed; its names are then removed one
// by one, so a name of the person's own on the same image keeps it.
func Remove(ctx context.Context, d Docker, imgs []Image) *Report {
	rep := &Report{Removed: []string{}, Failed: []string{}}
	var order []string
	byID := map[string][]Image{}
	for _, i := range imgs {
		if _, ok := byID[i.ID]; !ok {
			order = append(order, i.ID)
		}
		byID[i.ID] = append(byID[i.ID], i)
	}
	for _, id := range order {
		group := byID[id]
		if id == "" {
			for _, i := range group {
				rep.Failed = append(rep.Failed, i.Ref()+": no image ID in the list")
			}
			continue
		}
		err := d.Remove(ctx, id)
		switch {
		case err == nil:
			for _, i := range group {
				rep.Removed = append(rep.Removed, i.Ref())
			}
		case gone(err):
			// The ID is the full sha256 the list gave, so the daemon not
			// knowing it means the image is not there: removed with
			// another, or by someone else since the list.
			for _, i := range group {
				rep.Removed = append(rep.Removed, i.Ref()+" (already gone)")
			}
		case manyNames(err):
			for _, i := range group {
				if err := removeName(ctx, d, i); err != nil {
					rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", i.Ref(), err))
					continue
				}
				rep.Removed = append(rep.Removed, i.Ref())
			}
		default:
			for _, i := range group {
				rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", i.Ref(), err))
			}
		}
	}
	return rep
}

// removeName removes one name of an image by the name the store knows it
// by: repository:tag, or the whole repo:tag@digest the containerd store
// names a pinned pull by.
func removeName(ctx context.Context, d Docker, i Image) error {
	err := d.Remove(ctx, i.Ref())
	if err != nil && gone(err) {
		if full := i.fullRef(); full != "" {
			err = d.Remove(ctx, full)
		}
	}
	return err
}

// gone reports the daemon's answer for an image that is not there. The
// wording, like manyNames', was read from both stores' daemons
// (agentry-desktop#846).
func gone(err error) bool { return strings.Contains(err.Error(), "No such image") }

// manyNames reports the daemon's refusal of an ID several names point at
// (and not its refusal of an image a container uses, which also says
// "must be forced").
func manyNames(err error) bool {
	return strings.Contains(err.Error(), "referenced in multiple repositories")
}

// CLI drives the docker binary the runner uses.
type CLI struct{ Bin string }

// NewCLI returns the docker CLI the runner would use: dockerx.New's rule,
// AGENTRY_DOCKER or else "docker" on PATH.
func NewCLI() *CLI { return &CLI{Bin: dockerx.New().Bin} }

func (c *CLI) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Bin, args...) //nolint:gosec // fixed binary, structured args
	cmd.Env = os.Environ()
	procx.NoWindow(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return out.String(), nil
}

// listFormat is the `docker image ls` line parseList reads.
const listFormat = "{{.Repository}}\t{{.Tag}}\t{{.Digest}}\t{{.ID}}\t{{.Size}}"

// List implements Docker: every image `docker image ls` shows, each marked
// when it carries the runner's build label, and the untagged images that
// carry it. Those are listed only under the dangling filter: the plain
// list leaves them out, on both the classic and the containerd store.
func (c *CLI) List(ctx context.Context) ([]Image, error) {
	out, err := c.run(ctx, "image", "ls", "--digests", "--no-trunc", "--format", listFormat)
	if err != nil {
		return nil, err
	}
	all := parseList(out)
	ids, err := c.run(ctx, "image", "ls", "-q", "--no-trunc", "--filter", imagelabel.Filter)
	if err != nil {
		return nil, err
	}
	labelled := map[string]bool{}
	for _, id := range strings.Fields(ids) {
		labelled[id] = true
	}
	seen := map[string]bool{}
	for k := range all {
		all[k].Labelled = labelled[all[k].ID]
		seen[all[k].ID] = true
	}
	dangling, err := c.run(ctx, "image", "ls", "--digests", "--no-trunc", "--filter", "dangling=true", "--filter", imagelabel.Filter, "--format", listFormat)
	if err != nil {
		return nil, err
	}
	for _, i := range parseList(dangling) {
		if !seen[i.ID] {
			i.Labelled = true
			all = append(all, i)
			seen[i.ID] = true
		}
	}
	return all, nil
}

// Remove implements Docker: one image by its ID or a name, without
// --force, so an image a running container uses stays and says why.
func (c *CLI) Remove(ctx context.Context, ref string) error {
	_, err := c.run(ctx, "image", "rm", "--", ref)
	return err
}

func parseList(out string) []Image {
	var imgs []Image
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 4 {
			continue
		}
		img := Image{Repository: f[0], Tag: f[1], Digest: f[2], ID: f[3]}
		if len(f) > 4 {
			img.Size = f[4]
		}
		imgs = append(imgs, img)
	}
	return imgs
}
