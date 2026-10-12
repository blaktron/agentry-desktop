package dockerimages

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/imagelabel"
)

// The live check of agentry-desktop#846, against a real daemon (Docker
// Desktop's containerd store is the case it was written for). Off unless
// AGENTRY_LIVE_IMAGES_REF names a pinned pull (repo:tag@sha256:…) under
// Agentry's prefix that is not on the daemon yet; the check pulls it and
// removes it, and nothing else. With AGENTRY_LIVE_IMAGES_BASE (an image
// on the daemon with a shell), it also builds a labelled two-stage image,
// removes its tag and expects the untagged stage the classic builder
// leaves to be listed and removed. It also tags the pull with a name of
// its own, which must keep the image while Agentry's names go.
func TestLiveRemovesAPinnedPullAndALabelledLeftover(t *testing.T) {
	ref := os.Getenv("AGENTRY_LIVE_IMAGES_REF")
	if ref == "" {
		t.Skip("AGENTRY_LIVE_IMAGES_REF not set")
	}
	ctx := context.Background()
	c := NewCLI()
	if _, err := c.run(ctx, "image", "inspect", ref); err == nil {
		t.Fatalf("%s is already on the daemon; the check removes only what it pulls", ref)
	}
	if _, err := c.run(ctx, "pull", ref); err != nil {
		t.Fatal(err)
	}
	id, err := c.run(ctx, "image", "inspect", "--format", "{{.Id}}", ref)
	if err != nil {
		t.Fatal(err)
	}
	id = strings.TrimSpace(id)
	repo, tag, _ := splitRef(ref)
	if _, err := c.run(ctx, "image", "inspect", repo+":"+tag); err != nil {
		t.Logf("as the listing names it, %s:%s: %v (the containerd store's naming)", repo, tag, err)
	}
	var want []Image
	if base := os.Getenv("AGENTRY_LIVE_IMAGES_BASE"); base != "" {
		dir := t.TempDir()
		df := "FROM " + base + " AS b\n" + imagelabel.Line("probe846") + "RUN echo hi > /x\nFROM " + base + "\n" + imagelabel.Line("probe846") + "COPY --from=b /x /x\n"
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(df), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.run(ctx, "build", "-t", "probe846:live", dir); err != nil {
			t.Fatal(err)
		}
		if _, err := c.run(ctx, "image", "rm", "probe846:live"); err != nil {
			t.Fatal(err)
		}
	}
	// A name of the person's own on the same image: the image must stay
	// for it, and only Agentry's name goes.
	mine := "probe846/mine:1"
	if _, err := c.run(ctx, "tag", ref, mine); err != nil {
		t.Fatal(err)
	}
	all, err := c.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var leftovers []string
	for _, i := range Select(all, Policy{Refs: []string{ref}}) {
		if i.ID == id {
			want = append(want, i)
		}
		if !i.named() {
			if v, _ := c.run(ctx, "image", "inspect", "--format", "{{index .Config.Labels \""+imagelabel.Key+"\"}}", i.ID); strings.TrimSpace(v) == "probe846" {
				want = append(want, i)
				leftovers = append(leftovers, i.ID)
			}
		}
	}
	if len(want) == 0 || want[0].ID != id {
		t.Fatalf("the pull %s (%s) was not selected", ref, id)
	}
	if os.Getenv("AGENTRY_LIVE_IMAGES_BASE") != "" && len(leftovers) == 0 {
		t.Error("the labelled build's untagged stage was not listed")
	}
	rep := Remove(ctx, c, want)
	t.Logf("removed %v, failed %v", rep.Removed, rep.Failed)
	if len(rep.Failed) != 0 {
		t.Fatalf("not removed: %v", rep.Failed)
	}
	if _, err := c.run(ctx, "image", "inspect", mine); err != nil {
		t.Errorf("the person's own name %s went with Agentry's: %v", mine, err)
	}
	if _, err := c.run(ctx, "image", "rm", mine); err != nil {
		t.Errorf("removing the check's own %s: %v", mine, err)
	}
	for _, left := range append([]string{id}, leftovers...) {
		if _, err := c.run(ctx, "image", "inspect", left); err == nil {
			t.Errorf("%s is still on the daemon", left)
		}
	}
}
