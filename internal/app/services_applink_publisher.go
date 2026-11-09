// The Assistant's publisher tools: what it may read of the QuickAgents the
// person publishes, and the copy of one it may change. They are answered
// with the person's own sign-in, which never reaches the agent: a tool's
// answer carries names, states, versions and the files fetched, never a
// token or an address that holds one. Only the namespaces the person
// publishes in are reached: their own, and each organization they are an
// active member of; the registry decides the rest.
//
// quickagent_fetch writes into the agent's own share folder, which the
// agent can change, so it writes through an os.Root of that folder: a link
// the agent planted there cannot carry the files elsewhere. What it fetched
// is kept beside the settings, so a later submission can show the person
// what changed since.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"

	"github.com/blaktron/agentry-cli/runnerimage/applink"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

const (
	// maxMineShown bounds the QuickAgents one quickagent_mine answers.
	maxMineShown = 25
	// maxVersionsShown bounds the versions one quickagent_versions answers.
	maxVersionsShown = 30
	// maxFetchedNamed bounds the files a fetch's answer names.
	maxFetchedNamed = 40
	// maxReasonText bounds a version's reason in quickagent_versions.
	maxReasonText = 300
)

// publisherOps are the registry operations the publisher tools reach: the
// app's in appPublisherOps, a test's its own.
type publisherOps interface {
	// account is who is signed in to reg; nil when no one is.
	account(reg string) (*publisherAccount, error)
	list(reg, ns, query string) ([]PublisherAgent, error)
	versions(reg, ns, name string) ([]apiclient.AgentVersion, error)
	// fetch downloads ref's archive, holds it to the registry's digest and
	// unpacks it into a folder of its own, which the caller removes.
	fetch(ctx context.Context, reg, ref string) (*fetchedRelease, error)
	// fork forks ns/name into target, as newName ("" keeps the name),
	// once the person agreed: it asks nothing itself.
	fork(reg, ns, name, target, newName string) (*ForkView, error)
	// forkState is a fork's copy: copying, ready or failed; "" for a
	// QuickAgent that is not a fork.
	forkState(reg, ns, name string) (string, error)
	// standing is whether ns/name exists, and whether it is private.
	standing(reg, ns, name string) (exists, private bool, err error)
	// cost is what a submission spends, in a sentence.
	cost(reg string) string
	// publish submits the folder dir, once the person agreed: privateTold
	// is what the dialog said of an existing QuickAgent, createPrivate
	// creates a missing one private.
	publish(ctx context.Context, reg, dir string, privateTold, createPrivate bool) (*build.PublishResult, error)
}

// publisherAccount is the signed-in person: their handle, the handles of
// the organizations they are an active member of, and of those the ones
// they publish for (as an admin or a member; a security reviewer
// publishes nothing).
type publisherAccount struct {
	handle      string
	orgs        []string
	publishOrgs []string
}

// namespaces are the person's own namespace, then their organizations'.
func (a *publisherAccount) namespaces() []string { return append([]string{a.handle}, a.orgs...) }

// publishes are the namespaces a fork or a submission may go into: the
// person's own, then the organizations they publish for.
func (a *publisherAccount) publishes() []string { return append([]string{a.handle}, a.publishOrgs...) }

// fetchedRelease is a release as fetch unpacked it.
type fetchedRelease struct {
	ns, name, version, digest string
	dir                       string
}

// errSignedOut says the person's QuickAgents cannot be read without them.
var errSignedOut = errors.New("the person is not signed in to Agentry in Agentry Desktop, and their QuickAgents are read with their sign-in: ask them to sign in first")

// answerPublisher answers one of the publisher tools. A link with no
// publisher operations (Agentry Helper's) answers none of them. An error
// reaches the agent with every address in it cut to its origin, since a
// download's address may carry a signature or a sign-in.
func (m *mainLink) answerPublisher(ctx context.Context, tool string, raw json.RawMessage) (string, error) {
	if m.publisher == nil {
		return "", fmt.Errorf("this app does not answer %s", tool)
	}
	text, err := m.publisherCall(ctx, tool, raw)
	if err != nil && !errors.Is(err, errNotConfirmed) {
		return "", errors.New(originsOnly(err.Error()))
	}
	return text, err
}

// addressInText is an http(s) address as it appears in a sentence.
var addressInText = regexp.MustCompile(`https?://[^\s"'<>()]+`)

// originsOnly is s with each address in it cut to its scheme and host: no
// sign-in, path or query.
func originsOnly(s string) string {
	return addressInText.ReplaceAllStringFunc(s, func(a string) string {
		// The sentence's own punctuation after an address is kept.
		trimmed := strings.TrimRight(a, ".,;:!?")
		tail := a[len(trimmed):]
		u, err := url.Parse(trimmed)
		if err != nil || u.Host == "" {
			return "an address" + tail
		}
		return u.Scheme + "://" + u.Host + tail
	})
}

func (m *mainLink) publisherCall(ctx context.Context, tool string, raw json.RawMessage) (string, error) {
	switch tool {
	case applink.QuickAgentMine:
		var a struct {
			Namespace string `json:"namespace"`
			Query     string `json:"query"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.quickAgentMine(strings.TrimSpace(a.Namespace), strings.TrimSpace(a.Query))
	case applink.QuickAgentVersions:
		var a struct {
			Ref string `json:"ref"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.quickAgentVersions(a.Ref)
	case applink.QuickAgentFetch:
		var a struct {
			Ref string `json:"ref"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.quickAgentFetch(ctx, a.Ref)
	case applink.QuickAgentFork:
		var a struct {
			Ref  string `json:"ref"`
			Into string `json:"into"`
			Name string `json:"name"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.quickAgentFork(a.Ref, a.Into, a.Name)
	case applink.QuickAgentSubmit:
		var a struct {
			Folder string `json:"folder"`
		}
		if err := args(raw, &a); err != nil {
			return "", err
		}
		return m.quickAgentSubmit(ctx, a.Folder)
	}
	return "", fmt.Errorf("this version of Agentry Desktop does not answer %s yet", tool)
}

// publisherNamespaces are the namespaces the person publishes in on reg.
func (m *mainLink) publisherNamespaces(reg string) ([]string, error) {
	acct, err := m.publisher.account(reg)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errSignedOut
	}
	return acct.namespaces(), nil
}

// ownNamespace refuses a namespace the person does not publish in.
func ownNamespace(ns string, nss []string) error {
	if slices.Contains(nss, ns) {
		return nil
	}
	return fmt.Errorf("%s is neither the person's namespace nor one of their organizations' (%s): only their own QuickAgents are reached", quoted(ns), strings.Join(nss, ", "))
}

func (m *mainLink) quickAgentMine(namespace, query string) (string, error) {
	reg := m.ops.registry()
	nss, err := m.publisherNamespaces(reg)
	if err != nil {
		return "", err
	}
	if namespace != "" {
		if err := ownNamespace(namespace, nss); err != nil {
			return "", err
		}
		nss = []string{namespace}
	}
	var b strings.Builder
	shown, more := 0, 0
	for _, ns := range nss {
		agents, err := m.publisher.list(reg, ns, query)
		if err != nil {
			return "", err
		}
		if len(agents) == 0 {
			fmt.Fprintf(&b, "%s: none.\n", ns)
			continue
		}
		fmt.Fprintf(&b, "%s:\n", ns)
		for _, a := range agents {
			if shown == maxMineShown {
				more++
				continue
			}
			shown++
			b.WriteString(m.mineLine(reg, a))
		}
	}
	if more > 0 {
		fmt.Fprintf(&b, "And %d more: name a namespace or a query to narrow the list.\n", more)
	}
	b.WriteString("quickagent_versions says where each version of one stands.")
	return b.String(), nil
}

// mineLine is one QuickAgent of quickagent_mine: its reference, visibility,
// retirement, origin and latest version with where it stands.
func (m *mainLink) mineLine(reg string, a PublisherAgent) string {
	parts := []string{a.Visibility}
	if a.RetiredAt != "" {
		parts = append(parts, "retired")
	}
	if f := a.ForkedFrom; f != nil {
		parts = append(parts, "a fork of "+f.Namespace+"/"+f.Name+"@"+f.Version)
	}
	if a.ForkState != "" && a.ForkState != "ready" {
		parts = append(parts, "fork "+a.ForkState)
	}
	line := "- " + a.Namespace + "/" + a.Name + ": " + strings.Join(parts, ", ")
	switch vs, err := m.publisher.versions(reg, a.Namespace, a.Name); {
	case err != nil:
		line += "; its versions could not be read now"
	case len(vs) == 0:
		line += "; no version yet"
	default:
		newestFirst(vs)
		line += "; latest version " + vs[0].Version + ", " + versionState(vs[0].Status)
	}
	if s := strings.TrimSpace(a.Summary); s != "" {
		line += "; its summary: " + quoted(s)
	}
	return line + "\n"
}

func (m *mainLink) quickAgentVersions(ref string) (string, error) {
	ns, name, version, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	if version != "" {
		return "", errors.New("name the QuickAgent as namespace/name: its versions are listed whole")
	}
	reg := m.ops.registry()
	nss, err := m.publisherNamespaces(reg)
	if err != nil {
		return "", err
	}
	if err := ownNamespace(ns, nss); err != nil {
		return "", err
	}
	vs, err := m.publisher.versions(reg, ns, name)
	if err != nil {
		return "", err
	}
	if len(vs) == 0 {
		return ns + "/" + name + " has no version yet.", nil
	}
	newestFirst(vs)
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s's versions, newest first:\n", ns, name)
	for i, v := range vs {
		if i == maxVersionsShown {
			fmt.Fprintf(&b, "And %d older.\n", len(vs)-i)
			break
		}
		line := "- " + v.Version + ": " + versionState(v.Status)
		if r := v.StatusReason; r != nil && strings.TrimSpace(*r) != "" {
			line += ", " + quoted(cut(*r, maxReasonText))
		}
		if r := v.YankReason; v.Status == apiclient.VersionStatusYanked && r != nil && strings.TrimSpace(*r) != "" {
			line += ", " + quoted(cut(*r, maxReasonText))
		}
		if v.Status != apiclient.VersionStatusWithdrawn {
			line += "; its report: " + reportURL(reg, ns, name, v.Version)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// newestFirst orders versions by when they were submitted, newest first.
func newestFirst(vs []apiclient.AgentVersion) {
	sort.SliceStable(vs, func(i, j int) bool { return vs[i].CreatedAt.After(vs[j].CreatedAt) })
}

// versionState is where a version stands, in the person's words.
func versionState(s apiclient.VersionStatus) string {
	switch s {
	case apiclient.VersionStatusPublished, apiclient.VersionStatusApproved:
		return "published"
	case apiclient.VersionStatusNeedsReview:
		return "held for review"
	case apiclient.VersionStatusSubmitted, apiclient.VersionStatusScanning:
		return "being checked"
	case apiclient.VersionStatusRejected:
		return "rejected"
	case apiclient.VersionStatusQuarantined:
		return "quarantined"
	case apiclient.VersionStatusBlocked:
		return "blocked"
	case apiclient.VersionStatusYanked:
		return "yanked"
	case apiclient.VersionStatusWithdrawn:
		return "withdrawn"
	}
	return quoted(string(s))
}

// reportURL is where a version's security report is read: the site's page
// on SecureAgentry, the registry's own report elsewhere.
func reportURL(reg, ns, name, version string) string {
	base := strings.TrimRight(reg, "/")
	p := "/" + url.PathEscape(ns) + "/" + url.PathEscape(name) + "/versions/" + url.PathEscape(version)
	if u, err := url.Parse(base); err == nil && (u.Hostname() == "secureagentry.net" || strings.HasSuffix(u.Hostname(), ".secureagentry.net")) {
		return base + p
	}
	return base + "/api/v1/agents" + p + "/report"
}

func (m *mainLink) quickAgentFetch(ctx context.Context, ref string) (string, error) {
	ns, name, version, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	reg := m.ops.registry()
	nss, err := m.publisherNamespaces(reg)
	if err != nil {
		return "", err
	}
	if err := ownNamespace(ns, nss); err != nil {
		return "", err
	}
	share, err := m.share()
	if err != nil {
		return "", err
	}
	rel, err := m.publisher.fetch(ctx, reg, strings.TrimSpace(ref))
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(rel.dir) }()
	if rel.ns != ns || rel.name != name || (version != "" && rel.version != version) {
		return "", fmt.Errorf("the registry answered %s/%s@%s for %s; nothing was fetched", rel.ns, rel.name, rel.version, quoted(ref))
	}
	files, err := placeInShare(share, ns, name, rel.dir)
	if err != nil {
		return "", err
	}
	kept := false
	if m.fetchedPath != nil {
		p, err := m.fetchedPath()
		if err != nil {
			return "", err
		}
		rec := fetchRecord{Ref: ns + "/" + name + "@" + rel.version, Digest: rel.digest, At: time.Now().UTC(), Files: files}
		if err := keepFetched(p, ns+"/"+name, rec); err != nil {
			return "", fmt.Errorf("the files are in the share, but what was fetched could not be kept: %w", err)
		}
		kept = true
	}
	names := slices.Sorted(maps.Keys(files))
	for i, n := range names {
		names[i] = quoted(n)
	}
	dest := path.Join("share", applink.PublisherFolder, ns, name)
	var b strings.Builder
	fmt.Fprintf(&b, "Fetched %s/%s@%s into %s/, replacing what was there: %d files", ns, name, rel.version, dest, len(names))
	if len(names) > maxFetchedNamed {
		fmt.Fprintf(&b, ", among them %s", strings.Join(names[:maxFetchedNamed], ", "))
	} else if len(names) > 0 {
		b.WriteString(", " + strings.Join(names, ", "))
	}
	b.WriteString(".")
	if kept {
		b.WriteString(" Agentry keeps what it fetched, to show the person what changed when you submit.")
	}
	return b.String(), nil
}

// placeInShare puts the regular files under src at
// quickagents/<ns>/<name>/ in the share, replacing what was there, through
// an os.Root of the share: a path or a link that leaves it is refused. It
// answers each file's path beneath that folder with its sha256.
func placeInShare(share, ns, name, src string) (map[string]string, error) {
	if !refPart.MatchString(ns) || !refPart.MatchString(name) {
		return nil, fmt.Errorf("%s/%s is not a QuickAgent's reference", ns, name)
	}
	root, err := os.OpenRoot(share)
	if err != nil {
		return nil, fmt.Errorf("open the share folder: %w", err)
	}
	defer func() { _ = root.Close() }()
	dest := path.Join(applink.PublisherFolder, ns, name)
	if err := root.RemoveAll(dest); err != nil {
		return nil, fmt.Errorf("clear %s in the share folder: %w", dest, err)
	}
	if err := root.MkdirAll(dest, 0o755); err != nil {
		return nil, fmt.Errorf("make %s in the share folder: %w", dest, err)
	}
	files := map[string]string{}
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir():
			return root.MkdirAll(path.Join(dest, rel), 0o755)
		case d.Type().IsRegular():
			data, err := os.ReadFile(p) //nolint:gosec // a file of the release this app unpacked itself
			if err != nil {
				return err
			}
			if err := root.WriteFile(path.Join(dest, rel), data, 0o644); err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			files[rel] = "sha256:" + hex.EncodeToString(sum[:])
			return nil
		}
		return fmt.Errorf("%s is not a regular file", rel)
	})
	if err != nil {
		// Half a release is not left for the agent to take as the whole.
		_ = root.RemoveAll(dest)
		return nil, fmt.Errorf("copy the release into the share folder (nothing of it was kept): %w", err)
	}
	return files, nil
}

// fetchRecord is one fetch as it is kept: the release, its digest, when,
// and each file's sha256 by its path beneath the QuickAgent's folder.
type fetchRecord struct {
	Ref    string            `json:"ref"`
	Digest string            `json:"digest"`
	At     time.Time         `json:"at"`
	Files  map[string]string `json:"files"`
}

type fetchRecords struct {
	Version int                    `json:"version"`
	Fetched map[string]fetchRecord `json:"fetched"`
}

// keepFetched records rec under ns/name in the file at p, replacing an
// earlier fetch of the same QuickAgent.
func keepFetched(p, key string, rec fetchRecord) error {
	recs := fetchRecords{Version: 1, Fetched: map[string]fetchRecord{}}
	if raw, err := os.ReadFile(p); err == nil { //nolint:gosec // the app's own record beside its settings
		if err := json.Unmarshal(raw, &recs); err != nil || recs.Fetched == nil {
			recs = fetchRecords{Version: 1, Fetched: map[string]fetchRecord{}}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	recs.Fetched[key] = rec
	raw, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// fetchedRecordPath is where an agent's fetches are kept: beside the
// settings, as <agent>-fetched.json.
func fetchedRecordPath(agent string) func() (string, error) {
	return func() (string, error) {
		p, err := settings.Path()
		if err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(p), agent+"-fetched.json"), nil
	}
}

// appPublisherOps are the registry operations through the Publisher
// section's own reads and the person's stored sign-in.
type appPublisherOps struct {
	pub *PublisherAPI
}

func (o appPublisherOps) account(reg string) (*publisherAccount, error) {
	acct, err := o.pub.signedInAccount(reg)
	if err != nil || acct == nil {
		return nil, err
	}
	out := &publisherAccount{handle: acct.handle}
	for _, mb := range acct.orgs {
		out.orgs = append(out.orgs, mb.handle)
		if mb.role == "admin" || mb.role == "member" {
			out.publishOrgs = append(out.publishOrgs, mb.handle)
		}
	}
	return out, nil
}

func (o appPublisherOps) fork(reg, ns, name, target, newName string) (*ForkView, error) {
	// The person was asked by the agent's own dialog, which named it.
	return o.pub.fork(reg, ns, name, target, newName, func(string, string) error { return nil })
}

func (o appPublisherOps) forkState(reg, ns, name string) (string, error) {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return "", err
	}
	res, err := client.GetAgentWithResponse(o.pub.ctx(), ns, name)
	if err != nil {
		return "", withoutToken(fmt.Errorf("reach %s: %w", originOf(regURL), err), token)
	}
	if res.JSON200 == nil {
		return "", withoutToken(registry.Problem("read "+ns+"/"+name, res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body), token)
	}
	if res.JSON200.ForkState == nil {
		return "", nil
	}
	return string(*res.JSON200.ForkState), nil
}

func (o appPublisherOps) standing(reg, ns, name string) (bool, bool, error) {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return false, false, err
	}
	res, err := client.GetAgentWithResponse(o.pub.ctx(), ns, name)
	if err != nil {
		return false, false, withoutToken(fmt.Errorf("reach %s: %w", originOf(regURL), err), token)
	}
	switch {
	case res.StatusCode() == http.StatusNotFound:
		return false, false, nil
	case res.JSON200 == nil:
		return false, false, withoutToken(registry.Problem("read "+ns+"/"+name, res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body), token)
	}
	return true, res.JSON200.Visibility == apiclient.AgentVisibilityPrivate, nil
}

func (o appPublisherOps) cost(reg string) string {
	regURL, token, err := storedCredential(reg)
	if err != nil {
		return submissionCostLine(nil, errSignedOut)
	}
	sub, subErr := fetchSubscriptionStrictly(o.pub.ctx(), regURL, token)
	return submissionCostLine(sub, withoutToken(subErr, token))
}

func (o appPublisherOps) publish(ctx context.Context, reg, dir string, privateTold, createPrivate bool) (*build.PublishResult, error) {
	regURL, token, err := storedCredential(reg)
	if err != nil {
		return nil, errSignedOut
	}
	res, err := build.PublishWith(ctx, regURL, token, dir, privateTold, build.PublishOptions{CreatePrivate: createPrivate}, io.Discard)
	return res, withoutToken(err, token)
}

func (o appPublisherOps) list(reg, ns, query string) ([]PublisherAgent, error) {
	return o.pub.List(reg, ns, PublisherQuery{Q: query, Sort: "submitted"})
}

// client is a registry client for reg with the person's stored sign-in,
// and that sign-in's token, which an error is cleared of before it is
// answered.
func (o appPublisherOps) client(reg string) (*apiclient.ClientWithResponses, string, string, error) {
	regURL, token, err := storedCredential(reg)
	if err != nil {
		return nil, "", "", errSignedOut
	}
	client, err := registry.New(regURL, token)
	if err != nil {
		return nil, "", "", fmt.Errorf("%s is not a registry address this app can use", originOf(regURL))
	}
	return client, regURL, token, nil
}

// withoutToken is err with token cut out of its text.
func withoutToken(err error, token string) error {
	if err == nil || token == "" || !strings.Contains(err.Error(), token) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "[the sign-in]"))
}

func (o appPublisherOps) versions(reg, ns, name string) ([]apiclient.AgentVersion, error) {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return nil, err
	}
	res, err := client.ListVersionsWithResponse(o.pub.ctx(), ns, name)
	if err != nil {
		return nil, withoutToken(fmt.Errorf("reach %s: %w", originOf(regURL), err), token)
	}
	if res.JSON200 == nil {
		return nil, withoutToken(registry.Problem("list "+ns+"/"+name+"'s versions", res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body), token)
	}
	return *res.JSON200, nil
}

func (o appPublisherOps) fetch(ctx context.Context, reg, ref string) (*fetchedRelease, error) {
	client, regURL, token, err := o.client(reg)
	if err != nil {
		return nil, err
	}
	res, err := registry.Resolve(ctx, client, ref, false)
	if err != nil {
		return nil, withoutToken(err, token)
	}
	archive, err := registry.Download(ctx, regURL, token, res)
	if err != nil {
		return nil, withoutToken(err, token)
	}
	if computed := registry.Digest(archive); computed != res.Digest {
		return nil, fmt.Errorf("digest mismatch: the registry says %s, the archive is %s; nothing was fetched", res.Digest, computed)
	}
	dir, err := os.MkdirTemp("", "agentry-fetch-*")
	if err != nil {
		return nil, err
	}
	if err := pkgarchive.Extract(bytes.NewReader(archive), dir, int64(len(archive)), registry.ExtractLimits()); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("unpack %s: %w", ref, err)
	}
	return &fetchedRelease{ns: res.Namespace, name: res.Name, version: res.Version, digest: res.Digest, dir: dir}, nil
}
