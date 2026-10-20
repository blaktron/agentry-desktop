package voice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/sources"
	"github.com/blaktron/agentry-cli/pkg/voice/build"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"
)

// Web addresses of the person's writing: a blog post, a newsletter issue,
// an article. The Voice Writer's build fetches each before the run and has
// AI Review read its text before the Voice Writer does; a page drawn by
// scripts is rendered in a headless browser and read by AI Review there.
// On this computer AI Review takes the person's own TypeSafe key, so
// addresses without one are refused, and the step says so before the run.

// AddressLine is the step's view of one address: the host its fetch
// reaches, or why the build would refuse it.
type AddressLine struct {
	Address string `json:"address"`
	Problem string `json:"problem,omitempty"`
	Shown   string `json:"shown,omitempty"`
	Host    string `json:"host,omitempty"`
}

// AddressCheck is the step's view of a list of addresses: each line, the
// first problem with the list as a whole (the count), and the most a build
// takes.
type AddressCheck struct {
	Lines   []AddressLine `json:"lines"`
	Problem string        `json:"problem,omitempty"`
	Max     int           `json:"max"`
}

// NoReviewKey is why addresses are refused on this computer without the
// person's own TypeSafe key, in the words the step and the build use.
const NoReviewKey = "AI Review reads every page from a web address before the Voice Writer does, and that takes your own TypeSafe key: " +
	"add one under Settings → Provider settings → AI Review key, or give the samples as files"

// trimmedURLs drops blank lines and trims the rest.
func trimmedURLs(urls []string) []string {
	var out []string
	for _, a := range urls {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// CheckAddresses reads the addresses as the Voice Writer's build would,
// with the CLI's own parse, so the step refuses what the build would.
func CheckAddresses(urls []string) *AddressCheck {
	urls = trimmedURLs(urls)
	out := &AddressCheck{Max: build.MaxURLs}
	if len(urls) > build.MaxURLs {
		out.Problem = fmt.Sprintf("a voice takes at most %d web addresses", build.MaxURLs)
	}
	for _, a := range urls {
		l := AddressLine{Address: a}
		if srcs, err := build.ParseURLs([]string{a}); err != nil {
			l.Problem = strings.TrimPrefix(err.Error(), "--url "+a+": ")
		} else {
			l.Shown, l.Host = srcs[0].Shown(), srcs[0].Host
		}
		out.Lines = append(out.Lines, l)
	}
	return out
}

// First is the first thing wrong with the list, as a sentence, or "".
func (c *AddressCheck) First() string {
	if c.Problem != "" {
		return c.Problem
	}
	for _, l := range c.Lines {
		if l.Problem != "" {
			return fmt.Sprintf("%s: %s", l.Address, l.Problem)
		}
	}
	return ""
}

// jevKey is pkg/drafter's JevKey, the person's own TypeSafe key as work on
// this computer reads it; a test substitutes one.
var jevKey = drafter.JevKey

// pageReview parses a build's addresses and answers AI Review and the
// web_read configuration that reads them, with the person's own TypeSafe
// key: nil for a build of files alone, and a refusal without the key.
func pageReview(pol *policy.Policy, urls []string) ([]*sources.Source, webscan.Scanner, *engine.WebRun, error) {
	urls = trimmedURLs(urls)
	if len(urls) == 0 {
		return nil, nil, nil, nil
	}
	if p := CheckAddresses(urls).First(); p != "" {
		return nil, nil, nil, fmt.Errorf("the web addresses: %s", p)
	}
	srcs, err := build.ParseURLs(urls)
	if err != nil {
		return nil, nil, nil, err
	}
	jev, err := jevKey(pol)
	if err != nil && !errors.Is(err, drafter.ErrNoJevKey) {
		return nil, nil, nil, err
	}
	if jev == "" {
		return nil, nil, nil, errors.New(NoReviewKey)
	}
	sc, err := drafter.CoreJev(jev)
	if err != nil {
		return nil, nil, nil, err
	}
	return srcs, sc, &engine.WebRun{Scan: toolsrv.WebScanJev, JevKey: jev}, nil
}
