package main

// Edit on the QuickAgent page (plan drafter-revisions.md §4, D1, D5, D6, D7;
// agentry-desktop#500): the current release read into the Build tab under
// the same name at its next minor, as the site's Edit opens the Builder
// (agentry-web#861). Whom it is offered to is the registry's answer: the
// QuickAgent page shows Edit where GET /agents serves canPublish, the core's
// publish decision (agentry#875), because the desktop's sign-in token never
// carries the org:admin that the maintainer's settings need (DEV-199). The
// upload decides again.

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/registry"
)

// EditView is a release read into the form for Edit.
type EditView struct {
	Model *build.Model `json:"model"`
	// Namespace and Name are the QuickAgent the edit adds a version to,
	// which the form keeps fixed while it still names them.
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// From is the release the edit started from; Version is the one it
	// set, the next minor, which the person can change.
	From    string `json:"from"`
	Version string `json:"version"`
}

// errNotEditable is a QuickAgent this caller may not add a version to, or
// one the registry does not show them: one answer, as the registry gives.
var errNotEditable = errors.New("you cannot add a version to this QuickAgent from this sign-in")

// EditRelease reads ns/name's current release (its newest published,
// unyanked version) for Edit: its manifest and instruction file, through
// the API with the app's token, into the form (build.FromRelease), at the
// next minor above every version the QuickAgent has. It refuses a
// QuickAgent the registry does not say this caller may publish to, one with
// nothing published, and an instruction file the registry cut at its
// display cap, which would be edited short. Nothing is written.
func (b *BuildAPI) EditRelease(reg, ns, name string) (*EditView, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, fmt.Errorf("editing needs a signed-in registry (sign in under Settings → Registry settings): %w", err)
	}
	client, err := registry.New(url, token)
	if err != nil {
		return nil, err
	}
	ctx := b.app.ctx
	aresp, err := client.GetAgentWithResponse(ctx, ns, name)
	if err != nil {
		return nil, fmt.Errorf("fetch %s/%s: %w", ns, name, err)
	}
	if aresp.StatusCode() == http.StatusNotFound {
		return nil, errNotEditable
	}
	if aresp.JSON200 == nil {
		return nil, registry.Problem("agent", aresp.StatusCode(), aresp.ApplicationproblemJSONDefault, aresp.Body)
	}
	if aresp.JSON200.CanPublish == nil || !*aresp.JSON200.CanPublish {
		return nil, errNotEditable
	}
	vresp, err := client.ListVersionsWithResponse(ctx, ns, name)
	if err != nil {
		return nil, fmt.Errorf("fetch versions: %w", err)
	}
	if vresp.JSON200 == nil {
		return nil, registry.Problem("versions", vresp.StatusCode(), vresp.ApplicationproblemJSONDefault, vresp.Body)
	}
	all := make([]string, 0, len(*vresp.JSON200))
	current := ""
	for _, v := range *vresp.JSON200 {
		all = append(all, v.Version)
		// ListVersions is newest first, as the page reads it.
		if current == "" && v.Status == apiclient.VersionStatusPublished && v.YankedAt == nil {
			current = v.Version
		}
	}
	if current == "" {
		return nil, fmt.Errorf("%s/%s has no published version to edit yet", ns, name)
	}
	mresp, err := client.GetVersionManifestWithResponse(ctx, ns, name, current)
	if err != nil {
		return nil, fmt.Errorf("fetch the manifest of %s: %w", current, err)
	}
	if mresp.JSON200 == nil {
		return nil, registry.Problem("manifest", mresp.StatusCode(), mresp.ApplicationproblemJSONDefault, mresp.Body)
	}
	path := build.ReleaseInstructionPath(mresp.Body)
	fresp, err := client.GetVersionFileWithResponse(ctx, ns, name, current, &apiclient.GetVersionFileParams{Path: path})
	if err != nil {
		return nil, fmt.Errorf("fetch %s of %s: %w", path, current, err)
	}
	if fresp.JSON200 == nil {
		return nil, registry.Problem("file", fresp.StatusCode(), fresp.ApplicationproblemJSONDefault, fresp.Body)
	}
	f := fresp.JSON200
	switch {
	case f.Truncated:
		return nil, fmt.Errorf("%s of %s is too long for the Build tab to edit; edit it with agentry publish", path, current)
	case f.Binary || f.Content == nil:
		return nil, fmt.Errorf("%s of %s could not be read as text", path, current)
	}
	next := build.NextMinor(all)
	m, err := build.FromRelease(mresp.Body, *f.Content, ns, name, next)
	if err != nil {
		return nil, err
	}
	return &EditView{Model: m, Namespace: ns, Name: name, From: current, Version: next}, nil
}
