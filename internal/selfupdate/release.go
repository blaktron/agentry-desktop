package selfupdate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The trusted comment a release's signature carries, and the only place its
// format is written down. The signer formats it with TrustedComment and the
// update check reads it with ParseTrustedComment, so the two cannot drift;
// agentry-web's importer also looks for the version inside it.
//
// Why it matters more than a label: the downloads manifest is written by the
// site and is NOT signed, so nothing in it can be believed on its own. The
// signature is over SHA256SUMS, and SHA256SUMS lists digests, not a version —
// so the only version statement covered by the estate's key is this comment.
// An update check that took the version from the manifest would let whoever
// serves the manifest name any version it liked; taking it from here means
// the claim is one the signing key made.
const trustedCommentPrefix = "agentry-desktop "

var trustedCommentRE = regexp.MustCompile(`^agentry-desktop ([^\s(]+) \(commit ([0-9a-fA-F]{7,64})\)$`)

// TrustedComment is the signed comment for a release of this version built
// from this commit.
func TrustedComment(version, commit string) string {
	return fmt.Sprintf("%s%s (commit %s)", trustedCommentPrefix, version, commit)
}

// ParseTrustedComment reads the version and commit back out of a signed
// comment, refusing anything that is not exactly the shape TrustedComment
// writes. Strict on purpose: this is the one version statement the key
// covers, so "close enough" would be a version somebody else chose.
func ParseTrustedComment(s string) (version, commit string, err error) {
	m := trustedCommentRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", "", fmt.Errorf("the signature's trusted comment is %q, which is not %q", s, TrustedComment("<version>", "<commit>"))
	}
	return m[1], m[2], nil
}

// release is the X.Y.Z of a version string, and whether the build is past
// that tag rather than on it.
type release struct {
	major, minor, patch int
	past                bool
}

var versionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(.*)$`)

// parseVersion reads the version strings this product actually produces.
//
// The Makefile's version is `git describe --tags --always --dirty` without the
// leading v, so it is either a tag (`0.1.0`) or a development build past one
// (`0.1.0-11-g5ac8c0d`, optionally `-dirty`), or — on a checkout with no tag
// at all — a bare short sha, which carries no ordering and is refused here.
//
// The suffix is read as "past this tag", NOT as a semver prerelease. Semver
// would order `0.1.0-11-g5ac8c0d` BEFORE `0.1.0`; git describe means eleven
// commits after it. Treating the suffix the semver way would have a
// development build offer itself the tag it is already past, as an upgrade.
func parseVersion(s string) (release, error) {
	m := versionRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return release{}, fmt.Errorf("%q is not a version this product produces (expected 0.1.0, or 0.1.0-11-g<sha> for a build past a tag)", s)
	}
	var r release
	for i, into := range []*int{&r.major, &r.minor, &r.patch} {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return release{}, fmt.Errorf("%q has a number this product does not produce: %w", s, err)
		}
		*into = n
	}
	r.past = m[4] != ""
	return r, nil
}

// IsNewer says whether `offered` is a release this build should be told
// about, given it is running `current`.
//
// Only the X.Y.Z triple decides. A build past a tag is treated as being at
// that tag and a little beyond: offered 0.1.0 while running 0.1.0-11-g… is
// not an upgrade, and neither is 0.1.0 while running 0.2.0 — the app never
// offers a downgrade, which is also what stops a rolled-back manifest walking
// someone backwards onto an older release whose signature is still perfectly
// valid.
func IsNewer(offered, current string) (bool, error) {
	o, err := parseVersion(offered)
	if err != nil {
		return false, fmt.Errorf("the release offered: %w", err)
	}
	c, err := parseVersion(current)
	if err != nil {
		// A build with no tag in its history cannot be compared, and guessing
		// would mean offering an upgrade to something that might be newer.
		return false, fmt.Errorf("this build: %w", err)
	}
	switch {
	case o.major != c.major:
		return o.major > c.major, nil
	case o.minor != c.minor:
		return o.minor > c.minor, nil
	case o.patch != c.patch:
		return o.patch > c.patch, nil
	default:
		// Same triple: the offered release is only newer if THIS build is a
		// tagged one and the offered one is somehow past it, which the site
		// never publishes. In practice this is "no".
		return false, nil
	}
}
