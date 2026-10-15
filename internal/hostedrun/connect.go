package hostedrun

// A run In Agentry whose release signs in to its MCP servers. Agentry
// keeps the connection in its vault, apart from any this computer keeps,
// and a hosted consent starts only from a browser signed in to the site
// with a session: the vault's connect routes take no token. So this app
// never makes a hosted connection itself and never sees one: it opens the
// site's run setup at Connect tools, pinned to the version this computer
// runs, and learns that the vault holds the connection when run create
// stops answering 409 connection-missing.

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/blaktron/agentry/pkg/manifest"
)

// SlugConnectionMissing is run create's refusal for a release whose
// servers sign in, when the account keeps no working connection to one of
// them for this version: Missing names each server still to connect.
const SlugConnectionMissing = "connection-missing"

// pinnedVersion is an exact version, major.minor.patch with any pre-release
// and build: never a range such as "1" or "1.x", which the site's setup would
// not find among the published versions.
var pinnedVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// ConnectPage is the site's run setup for ref, open on Connect tools and
// pinned to ref's exact version, on the registry's own origin: the site and
// the API share it (the edge sends /api/ to the server and the rest to the
// site). A connection is kept for one version alone, so a ref without an
// exact version has no page.
func ConnectPage(registryURL, ref string) (string, error) {
	r, err := manifest.ParseRef(ref)
	if err != nil {
		return "", fmt.Errorf("%s is not a release reference: %w", ref, err)
	}
	if !pinnedVersion.MatchString(r.Range) {
		return "", fmt.Errorf("%s names no exact version, and Agentry keeps a connection for one version alone", ref)
	}
	u, err := url.Parse(registryURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("%s is not a registry address", registryURL)
	}
	page := url.URL{
		Scheme:   u.Scheme,
		Host:     u.Host,
		Path:     "/" + r.Namespace + "/" + r.Name + "/run/setup",
		RawQuery: "version=" + url.QueryEscape(r.Range) + "&step=tools",
	}
	return page.String(), nil
}
