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
// so the only version statement covered by the signing key is this comment.
// An update check that took the version from the manifest would let whoever
// serves the manifest name any version it liked; taking it from here means
// the claim is one the signing key made.
//
// The comment names its product first. The same key signs more than this
// application's releases — the CLI's release workflow runs this module's
// signrelease — and a signed statement must say what it covers:
// `agentry-cli 0.1.0 (commit …)` for a CLI bundle, `agentry-desktop 0.2.0
// (commit …)` for ours. The update check reads ONLY this product's
// (ParseTrustedComment); a CLI release's signature therefore never reads as
// a desktop update, whatever manifest offers it.
const ThisProduct = "agentry-desktop"

// A product is lower-case letters, digits and hyphens, as the repositories
// are named; the comment's shape admits nothing else, so a product name
// outside it is refused by the reader rather than accepted and mis-read.
var trustedCommentRE = regexp.MustCompile(`^([a-z][a-z0-9-]*) ([^\s(]+) \(commit ([0-9a-fA-F]{7,64})\)$`)

// TrustedComment is the signed comment for a release of this application at
// this version built from this commit.
func TrustedComment(version, commit string) string {
	return TrustedCommentFor(ThisProduct, version, commit)
}

// TrustedCommentFor is the signed comment for a release of any of
// Agentry's products. The product is lower-case letters, digits and hyphens,
// as the repositories are named; the signer checks that before it signs.
func TrustedCommentFor(product, version, commit string) string {
	return fmt.Sprintf("%s %s (commit %s)", product, version, commit)
}

// ParseTrustedComment reads the version and commit back out of a signed
// comment of THIS product, refusing anything else — another product's
// comment included. Strict on purpose: this is the one version statement the
// key covers, so "close enough" would be a version somebody else chose.
func ParseTrustedComment(s string) (version, commit string, err error) {
	product, version, commit, err := ParseAnyTrustedComment(s)
	if err != nil {
		return "", "", err
	}
	if product != ThisProduct {
		return "", "", fmt.Errorf("the signature's trusted comment is %q, a release of %s, not of %s", s, product, ThisProduct)
	}
	return version, commit, nil
}

// ParseAnyTrustedComment reads the product, version and commit out of a
// signed comment in the shape TrustedCommentFor writes, for whichever product
// it names. The signer's round-trip check is its caller; the application's
// update check does not use this — it reads its own product's comment with
// ParseTrustedComment, and the site's importer has its own reading.
func ParseAnyTrustedComment(s string) (product, version, commit string, err error) {
	m := trustedCommentRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", "", "", fmt.Errorf("the signature's trusted comment is %q, which is not %q", s, TrustedCommentFor("<product>", "<version>", "<commit>"))
	}
	return m[1], m[2], m[3], nil
}

// release is the X.Y.Z of a version string, and its N when it is a dev
// prerelease (vX.Y.Z-dev.N): dev is 0 for a production release. What
// follows them — the describe suffix, -dirty — is deliberately not kept: see
// parseVersion.
type release struct {
	major, minor, patch int
	dev                 int
}

// unstampedVersion is internal/version's default, what a build without the
// Makefile's -X reports.
const unstampedVersion = "0.0.0-dev"

var (
	versionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(.*)$`)
	devRE     = regexp.MustCompile(`^-dev\.([1-9]\d*)(.*)$`)
	// pastRE is what may follow a tag: git describe's -K-g<sha>, -dirty,
	// both, or nothing. Anything else is a version this product does not
	// produce, and would otherwise read as a production release.
	pastRE = regexp.MustCompile(`^(-\d+-g[0-9a-f]{4,40})?(-dirty)?$`)
)

// parseVersion reads the version strings this product actually produces.
//
// The Makefile's version is `git describe --tags --always --dirty` without the
// leading v, so it is a tag — a production release (`0.6.0`) or a dev
// prerelease (`0.6.1-dev.2`) — or a development build
// past one (`0.6.0-11-g5ac8c0d`, `0.6.1-dev.2-4-gabc1234`, optionally
// `-dirty`), or — on a checkout with no tag at all — a bare short sha, which
// carries no ordering and is refused here. A `-dev.` that is not `-dev.N` with
// N from 1 is refused too: it is not a tag this product cuts.
//
// The describe suffix is read as "past this tag", NOT as a semver prerelease.
// Semver would order `0.1.0-11-g5ac8c0d` BEFORE `0.1.0`; git describe means
// eleven commits after it. Treating the suffix the semver way would have a
// development build offer itself the tag it is already past, as an upgrade.
func parseVersion(s string) (release, error) {
	// An unstamped `go build` reports 0.0.0-dev (internal/version): a build
	// at no release at all, on the production side, so every release is
	// newer. It is the one bare -dev this product produces.
	if strings.TrimPrefix(strings.TrimSpace(s), "v") == unstampedVersion {
		return release{}, nil
	}
	m := versionRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return release{}, fmt.Errorf("%q is not a version this product produces (expected 0.1.0, 0.1.0-dev.2, or 0.1.0-11-g<sha> for a build past a tag)", s)
	}
	var r release
	for i, into := range []*int{&r.major, &r.minor, &r.patch} {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return release{}, fmt.Errorf("%q has a number this product does not produce: %w", s, err)
		}
		*into = n
	}
	rest := m[4]
	if strings.HasPrefix(rest, "-dev") {
		d := devRE.FindStringSubmatch(rest)
		if d == nil {
			return release{}, fmt.Errorf("%q is not a dev prerelease this product cuts (expected X.Y.Z-dev.N, N from 1)", s)
		}
		n, err := strconv.Atoi(d[1])
		if err != nil {
			return release{}, fmt.Errorf("%q has a number this product does not produce: %w", s, err)
		}
		r.dev, rest = n, d[2]
	}
	if !pastRE.MatchString(rest) {
		return release{}, fmt.Errorf("%q is not a version this product produces: after the tag only git describe's -K-g<sha> and -dirty may follow", s)
	}
	return r, nil
}

// IsNewer says whether `offered` is a release this build should be told
// about, given it is running `current`.
//
// Each channel keeps to itself: a dev build (its version a -dev.N prerelease,
// or a build past one) is only ever offered a newer dev prerelease, and a
// production build only a newer production release. A dev build bakes dev's
// registry and a production build production's, so the manifests they read
// already keep them apart; this is the same rule checked against the version
// the signature covers, which the manifest cannot name.
//
// Then the X.Y.Z triple decides, and within one triple a dev prerelease's N.
// A build past a tag is treated as being at that tag and a little beyond:
// offered 0.1.0 while running 0.1.0-11-g… is not an upgrade, and neither is
// 0.1.0 while running 0.2.0 — the app never offers a downgrade, which is also
// what stops a rolled-back manifest walking someone backwards onto an older
// release whose signature is still perfectly valid.
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
	if (o.dev > 0) != (c.dev > 0) {
		return false, nil
	}
	switch {
	case o.major != c.major:
		return o.major > c.major, nil
	case o.minor != c.minor:
		return o.minor > c.minor, nil
	case o.patch != c.patch:
		return o.patch > c.patch, nil
	default:
		// The same triple is never an upgrade for a production release. That
		// covers the case worth naming: a build at 0.1.0-11-g5ac8c0d offered
		// 0.1.0 is being offered the tag it is already past, and says no. For a
		// dev prerelease, only a higher N is.
		return o.dev > c.dev, nil
	}
}
