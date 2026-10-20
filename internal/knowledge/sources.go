package knowledge

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/sources"
	"github.com/blaktron/agentry-cli/runnerimage/netaddr"
)

// Sources are the git repositories and the files at web addresses a build
// fetches beside its documents: each repository an https address with an
// optional @ref (a branch, a tag or a commit), each address one file or
// page. On this computer a repository or a file that needs an access token
// uses the key the run setup saved for its host in the system keychain
// (source:<host>, the entry the CLI reads too); In Agentry, the key saved
// in Agentry for that host.
type Sources struct {
	Repos []string `json:"repos"`
	URLs  []string `json:"urls"`
}

// The kinds of a SourceLine.
const (
	KindRepository = "repository"
	KindURL        = "url"
)

// Count is how many sources there are, each counted as a document by the
// documents-or-a-brief rule.
func (s Sources) Count() int { return len(s.Repos) + len(s.URLs) }

// Trimmed drops blank entries and trims the rest, as the step's fields hold
// one address a line.
func (s Sources) Trimmed() Sources {
	return Sources{Repos: TrimmedLines(s.Repos), URLs: TrimmedLines(s.URLs)}
}

// TrimmedLines is the non-blank lines of a field that holds one address a
// line, each trimmed.
func TrimmedLines(in []string) []string {
	var out []string
	for _, a := range in {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// Redacted is an address as a refusal may name it: no name or password,
// and a query as "?…", since either can carry a token.
func Redacted(a string) string {
	u, err := url.Parse(a)
	if err != nil || u.Host == "" {
		if i := strings.IndexAny(a, "@?"); i >= 0 {
			return a[:i] + "…"
		}
		return a
	}
	u.User = nil
	if u.RawQuery != "" || u.ForceQuery {
		u.RawQuery, u.ForceQuery = "", false
		return u.String() + "?…"
	}
	return u.String()
}

// SourceLine is the Documents step's view of one repository or address:
// what the build would fetch, or why it cannot. It never carries a token.
type SourceLine struct {
	// Address is the line as given.
	Address string `json:"address"`
	// Kind is repository or url.
	Kind string `json:"kind"`
	// Problem is why the build would refuse it, in the CLI's words; the
	// rest is empty when there is one.
	Problem string `json:"problem,omitempty"`
	// Shown is the address as a person reads it (a query is "?…"); Host
	// the one host its fetch reaches.
	Shown string `json:"shown,omitempty"`
	Host  string `json:"host,omitempty"`
	// Saved says this computer's keychain holds a key for Host, which a
	// build here uses for a repository or a file on that host (looked up,
	// never returned).
	Saved bool `json:"saved,omitempty"`
}

// SourcesCheck is the step's view of a build's sources: each line, and the
// first problem with the list as a whole (the count), "" when it has none.
type SourcesCheck struct {
	Lines   []SourceLine `json:"lines"`
	Problem string       `json:"problem,omitempty"`
	// MaxRepos and MaxURLs are the most a build takes of each.
	MaxRepos int `json:"maxRepos"`
	MaxURLs  int `json:"maxUrls"`
}

// CheckSourceList reads the sources as a build would, with the CLI's own
// parse, so the step refuses what the build would refuse, and says which
// hosts have a key saved on this computer.
func CheckSourceList(s Sources) *SourcesCheck { return checkSourceList(s, true) }

// checkSourceList is CheckSourceList; keys says whether to look up the
// keychain, which a build In Agentry, sending no key, does not.
func checkSourceList(s Sources, keys bool) *SourcesCheck {
	s = s.Trimmed()
	out := &SourcesCheck{MaxRepos: build.MaxRepos, MaxURLs: build.MaxURLs}
	switch {
	case len(s.Repos) > build.MaxRepos:
		out.Problem = fmt.Sprintf("a build takes at most %d repositories", build.MaxRepos)
	case len(s.URLs) > build.MaxURLs:
		out.Problem = fmt.Sprintf("a build takes at most %d web addresses", build.MaxURLs)
	}
	line := func(kind, a string, parse func() ([]*sources.Source, error)) {
		l := SourceLine{Address: a, Kind: kind}
		srcs, err := parse()
		switch {
		case err != nil:
			l.Problem = strings.TrimPrefix(strings.TrimPrefix(err.Error(), "--repo "+a+": "), "--url "+a+": ")
		case len(srcs) == 1:
			if p := hostProblem(srcs[0].Host); p != "" {
				l.Problem = p
				break
			}
			l.Shown, l.Host = srcs[0].Shown(), srcs[0].Host
			if keys && !modelkey.Disabled() {
				saved, _ := sources.SavedToken(l.Host)
				l.Saved = saved != ""
			}
		}
		out.Lines = append(out.Lines, l)
	}
	for _, a := range s.Repos {
		line(KindRepository, a, func() ([]*sources.Source, error) { return build.ParseSources([]string{a}, nil) })
	}
	for _, a := range s.URLs {
		line(KindURL, a, func() ([]*sources.Source, error) { return build.ParseSources(nil, []string{a}) })
	}
	return out
}

// CheckHostedSources is the list a build In Agentry sends, held to the
// step's rules, with no keychain lookup: the registry uses the key saved in
// Agentry for each host, and no key from this computer is sent.
func CheckHostedSources(s Sources) (Sources, error) {
	s = s.Trimmed()
	if p := checkSourceList(s, false).First(); p != "" {
		return Sources{}, fmt.Errorf("the build's sources: %s", p)
	}
	return s, nil
}

// First is the first thing wrong with the list, as a sentence naming the
// address redacted, or "".
func (c *SourcesCheck) First() string {
	if c.Problem != "" {
		return c.Problem
	}
	for _, l := range c.Lines {
		if l.Problem != "" {
			return fmt.Sprintf("%s: %s", Redacted(l.Address), l.Problem)
		}
	}
	return ""
}

// hostProblem refuses a host that is an address literal not on the public
// internet: a build fetches only from public addresses, and the fetch
// step's proxy holds a name to the same rule when it resolves it.
func hostProblem(host string) string {
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil && !netaddr.Public(a) {
		return host + " is not a public address: a build fetches only from the public internet"
	}
	return ""
}

// parseSources reads a build's sources for its run, held to the step's
// rules, each repository's or address's token the key saved on this
// computer for its host when there is one. A keychain that cannot be read
// is said through warn and the source is fetched without it.
func parseSources(s Sources, warn func(string)) ([]*sources.Source, error) {
	s = s.Trimmed()
	if s.Count() == 0 {
		return nil, nil
	}
	if p := CheckSourceList(s).First(); p != "" {
		return nil, fmt.Errorf("the build's sources: %s", p)
	}
	srcs, err := build.ParseSources(s.Repos, s.URLs)
	if err != nil {
		return nil, err
	}
	if err := sources.ResolveBuildTokens(srcs, nil, func(string) string { return "" }, nil, warn); err != nil {
		return nil, err
	}
	return srcs, nil
}
