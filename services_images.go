package main

import (
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-desktop/internal/dockerimages"
)

// Run defaults → Docker images (agentry-desktop#424): the Docker images
// Agentry put on this machine, and a button that removes them on every
// platform. The Windows uninstaller removes them unless "Keep my Docker
// images" is ticked; the deb, rpm and dmg have no uninstaller that can ask,
// so this is where those installs remove them.

// ImageView is one of Agentry's images as the card lists it: the ref it is
// removed by, and its size.
type ImageView struct {
	Ref  string `json:"ref"`
	Size string `json:"size,omitempty"`
}

// ImagesView is Agentry's images on this daemon.
type ImagesView struct {
	Images []ImageView `json:"images"`
	// Unavailable says why the list could not be read: Docker is not
	// running, or not installed.
	Unavailable string `json:"unavailable,omitempty"`
}

// imageClient is the image client; a test passes its own.
func (r *RunAPI) imageClient() dockerimages.Docker {
	if r.images != nil {
		return r.images
	}
	return dockerimages.NewCLI()
}

func runnerPolicyImages() dockerimages.Policy {
	home, err := policy.Home()
	if err != nil {
		return dockerimages.Policy{}
	}
	return dockerimages.ReadPolicy(home)
}

// AgentryImages lists Agentry's Docker images on this machine.
func (r *RunAPI) AgentryImages() (*ImagesView, error) {
	imgs, err := dockerimages.Selected(r.app.ctx, r.imageClient(), runnerPolicyImages())
	if err != nil {
		return &ImagesView{Images: []ImageView{}, Unavailable: err.Error()}, nil
	}
	out := make([]ImageView, 0, len(imgs))
	for _, i := range imgs {
		out = append(out, ImageView{Ref: i.Ref(), Size: i.Size})
	}
	return &ImagesView{Images: out}, nil
}

// RemoveAgentryImages removes Agentry's Docker images after a native
// confirmation that names how many and says the CLI uses them too. It
// refuses while a run, an image setup or a knowledge build holds the
// docker lock, and removes nothing a running container still uses.
func (r *RunAPI) RemoveAgentryImages() (*dockerimages.Report, error) {
	if err := r.acquire(jobSetup, "removing Agentry's Docker images"); err != nil {
		return nil, err
	}
	defer r.release()
	d := r.imageClient()
	imgs, err := dockerimages.Selected(r.app.ctx, d, runnerPolicyImages())
	if err != nil {
		return nil, err
	}
	if len(imgs) == 0 {
		return &dockerimages.Report{Removed: []string{}, Failed: []string{}}, nil
	}
	names := make([]string, 0, len(imgs))
	for _, i := range imgs {
		names = append(names, i.Ref())
	}
	s := "s"
	if len(imgs) == 1 {
		s = ""
	}
	msg := fmt.Sprintf("Remove %d Docker image%s Agentry put on this computer? The agentry CLI uses the same images. The next run downloads or builds what it needs again.\n\n%s",
		len(imgs), s, strings.Join(names, "\n"))
	if err := r.app.askConsent("Remove Agentry's Docker images?", msg); err != nil {
		return nil, err
	}
	// Exactly the images the dialog named, not a fresh listing.
	return dockerimages.Remove(r.app.ctx, d, imgs), nil
}
