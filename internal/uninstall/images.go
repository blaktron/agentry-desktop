package uninstall

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-desktop/internal/dockerimages"
)

// ImagesFlag is the form the Windows uninstaller runs the app with to
// remove Agentry's Docker images, unless the person ticked "Keep my Docker
// images". It runs before the settings box, whose removal deletes the
// runner policy that may name them.
const ImagesFlag = "--remove-docker-images"

// Exit codes of ImagesMain, which the uninstaller turns into a message.
const (
	ImagesRemoved    = 0 // every Agentry image removed, or there were none
	ImagesSomeStayed = 1 // the daemon refused some (a running container uses them)
	ImagesNoDocker   = 2 // Docker did not answer: nothing was removed
)

// imagesRunTimeout bounds the uninstaller's wait on Docker.
const imagesRunTimeout = 3 * time.Minute

// ImagesMain removes Agentry's images through d and reports each to w.
func ImagesMain(w io.Writer, d dockerimages.Docker) int {
	var pol dockerimages.Policy
	if home, err := policy.Home(); err == nil {
		pol = dockerimages.ReadPolicy(home)
	}
	ctx, cancel := context.WithTimeout(context.Background(), imagesRunTimeout)
	defer cancel()
	rep, err := dockerimages.RemoveAll(ctx, d, pol)
	if err != nil {
		_, _ = fmt.Fprintf(w, "Docker images not removed: %v\n", err)
		return ImagesNoDocker
	}
	for _, r := range rep.Removed {
		_, _ = fmt.Fprintf(w, "removed %s\n", r)
	}
	for _, f := range rep.Failed {
		_, _ = fmt.Fprintf(w, "not removed: %s\n", f)
	}
	if len(rep.Failed) > 0 {
		return ImagesSomeStayed
	}
	return ImagesRemoved
}
