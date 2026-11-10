package run

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// Several runs of this app can be under way at once, each in its own
// containers, networks and volumes. What they share on this computer is
// guarded here: the images and MCP server installs a run prepares, which
// are built under fixed names and from shared build folders; the consent
// record every run writes; and an MCP profile two runs of one release both
// read.

// prepareSlot is held while a run makes sure of its images and installs its
// MCP servers. A second run waits for the first and then finds what it
// built, rather than building it again beside it under the same names.
var prepareSlot = make(chan struct{}, 1)

// holdPrepare takes prepareSlot, saying so in the run's log when another
// run holds it, or returns ctx's error when the run is stopped first. Its
// release may be called more than once; only the first gives the slot back.
func holdPrepare(ctx context.Context, logw io.Writer) (release func(), err error) {
	var once sync.Once
	release = func() { once.Do(func() { <-prepareSlot }) }
	select {
	case prepareSlot <- struct{}{}:
		return release, nil
	default:
	}
	_, _ = fmt.Fprintln(logw, "waiting for another run to finish preparing its images and MCP servers")
	select {
	case prepareSlot <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// consentRecordMu orders this app's runs' reads and writes of the consent
// record, so two runs ending together do not lose each other's entry.
var consentRecordMu sync.Mutex

// profileUsers counts this app's runs holding each MCP profile folder, from
// the run's preparation to its finishProfile.
var profileUsers = struct {
	sync.Mutex
	n map[string]int
}{n: map[string]int{}}

// profileHold is one run's hold on a profile folder.
type profileHold struct {
	dir  string
	once sync.Once
	last bool
}

// useProfile records a run holding the profile in dir.
func useProfile(dir string) *profileHold {
	profileUsers.Lock()
	defer profileUsers.Unlock()
	profileUsers.n[dir]++
	return &profileHold{dir: dir}
}

// leave lets the run's hold go, and says whether no other run of this app
// holds the profile now. Only the first call counts; a later one says what
// the first said. A nil hold is a run that never held one: it is the last.
func (h *profileHold) leave() (last bool) {
	if h == nil {
		return true
	}
	h.once.Do(func() {
		profileUsers.Lock()
		defer profileUsers.Unlock()
		if profileUsers.n[h.dir] > 0 {
			profileUsers.n[h.dir]--
		}
		h.last = profileUsers.n[h.dir] == 0
		if h.last {
			delete(profileUsers.n, h.dir)
		}
	})
	return h.last
}
