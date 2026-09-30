// Package dockerimages finds and removes the Docker images Agentry put on
// this machine (agentry-desktop#424): the ones the runner pulls from
// Agentry's registry, the runner and per-QuickAgent exec images it builds,
// and whatever the runner policy names in their place. The Windows
// uninstaller removes them unless "Keep my Docker images" is ticked, and
// Run defaults offers the same on every platform, because the deb, rpm and
// dmg have no uninstaller that can ask.
//
// Images are chosen by name, one by one, and removed one by one. Nothing
// here runs `docker image prune` or removes an image it did not select: a
// person's own images share the daemon, and some share Agentry's GitHub
// owner.
package dockerimages

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"
)

// RegistryPrefix is the name every image Agentry publishes starts with
// (agentry-dockerimages; the runner policy's defaults).
const RegistryPrefix = "ghcr.io/blaktron/agentry-"

// runnerRepo is the runner image built on this machine; its per-QuickAgent
// exec images are named "<runner repo>-exec" (spec.ExecImageTag).
const runnerRepo = "agentry-runner"

// Image is one line of `docker image ls`.
type Image struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Digest     string `json:"digest,omitempty"`
	ID         string `json:"id"`
	Size       string `json:"size,omitempty"`
}

// Ref is the name the image is removed by: repository:tag, or
// repository@digest for an image pulled by digest and never tagged.
func (i Image) Ref() string {
	if i.Tag != "" && i.Tag != "<none>" {
		return i.Repository + ":" + i.Tag
	}
	if i.Digest != "" && i.Digest != "<none>" {
		return i.Repository + "@" + i.Digest
	}
	return i.ID
}

// Docker lists and removes images; CLI is the real one.
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
	// (plan unsafe-mode.md D12); the default is under RegistryPrefix.
	for _, r := range []string{i.Gateway, i.HarnessDockerAgent, i.GoBuilder, i.NodeRuntime, i.PythonRuntime, i.Unsafe} {
		if r != "" {
			p.Refs = append(p.Refs, r)
		}
	}
	return p
}

// Select keeps the images that are Agentry's, and nothing else:
//   - every image under Agentry's registry prefix;
//   - the runner image this app builds and the per-QuickAgent exec images
//     built on it, by repository (agentry-runner, agentry-runner-exec, or
//     the policy's renamed runner and its "-exec"); an Unsafe Mode exec
//     image is built on the Unsafe Mode base but named in the same "-exec"
//     repository (spec.UnsafeExecImageTag), so it is taken here too;
//   - each image the policy pins, matched by its exact tag or digest, so
//     an operator's `node:22-alpine` takes that image and never the rest of
//     `node`.
//
// Images with no repository are left alone.
func Select(all []Image, p Policy) []Image {
	builds := map[string]bool{runnerRepo: true, runnerRepo + "-exec": true}
	if p.Runner != "" {
		repo, _, _ := splitRef(p.Runner)
		builds[repo] = true
		builds[repo+"-exec"] = true
	}
	var out []Image
	for _, i := range all {
		if i.Repository == "" || i.Repository == "<none>" {
			continue
		}
		if strings.HasPrefix(i.Repository, RegistryPrefix) || builds[i.Repository] || pinned(i, p.Refs) {
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

// Remove removes exactly these images one by one, the list a person
// confirmed and nothing listed after it. An image the daemon refuses (one
// a running container uses) is reported and the rest are still removed.
func Remove(ctx context.Context, d Docker, imgs []Image) *Report {
	rep := &Report{Removed: []string{}, Failed: []string{}}
	for _, i := range imgs {
		if err := d.Remove(ctx, i.Ref()); err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", i.Ref(), err))
			continue
		}
		rep.Removed = append(rep.Removed, i.Ref())
	}
	return rep
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

// List implements Docker.
func (c *CLI) List(ctx context.Context) ([]Image, error) {
	out, err := c.run(ctx, "image", "ls", "--digests", "--no-trunc", "--format", "{{.Repository}}\t{{.Tag}}\t{{.Digest}}\t{{.ID}}\t{{.Size}}")
	if err != nil {
		return nil, err
	}
	return parseList(out), nil
}

// Remove implements Docker: one image by its ref, without --force, so an
// image a running container uses stays and says why.
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
