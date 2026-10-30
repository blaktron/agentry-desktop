package uninstall

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/dockerimages"
)

type fakeDocker struct {
	images  []dockerimages.Image
	listErr error
	refuse  string
	removed []string
}

func (f *fakeDocker) List(context.Context) ([]dockerimages.Image, error) { return f.images, f.listErr }
func (f *fakeDocker) Remove(_ context.Context, ref string) error {
	if ref == f.refuse {
		return errors.New("image is being used by running container")
	}
	f.removed = append(f.removed, ref)
	return nil
}

// The uninstaller's images step: Agentry's images go, a person's stay, and
// the exit code tells the uninstaller which message to
// show.
func TestImagesMainExitCodes(t *testing.T) {
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	imgs := []dockerimages.Image{
		{Repository: "agentry-runner", Tag: "0.6.25", ID: "sha256:1"},
		{Repository: "ghcr.io/blaktron/agentry-mcp-gateway", Tag: "v0.44.3", ID: "sha256:2"},
		{Repository: "postgres", Tag: "16", ID: "sha256:3"},
	}
	var out bytes.Buffer
	d := &fakeDocker{images: imgs}
	if code := ImagesMain(&out, d); code != ImagesRemoved {
		t.Fatalf("all removed: code %d\n%s", code, out.String())
	}
	if len(d.removed) != 2 || strings.Contains(strings.Join(d.removed, " "), "sha256:3") {
		t.Fatalf("removed %v", d.removed)
	}
	d = &fakeDocker{images: imgs, refuse: "sha256:1"}
	if code := ImagesMain(&out, d); code != ImagesSomeStayed {
		t.Fatalf("one refused: code %d", code)
	}
	d = &fakeDocker{listErr: errors.New("docker daemon not reachable")}
	if code := ImagesMain(&out, d); code != ImagesNoDocker || len(d.removed) != 0 {
		t.Fatalf("no docker: code %d, removed %v", code, d.removed)
	}
}
