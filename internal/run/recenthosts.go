package run

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/targethost"
)

// Recent hosts: the target hosts this computer's runs reached, newest
// first, so the run setup can offer them again. Each is the answer as
// ssh takes it, user@host[:port]: an address and an account name, never
// a credential. They are kept beside desktop.yaml, on this computer
// alone.

// maxRecentHosts bounds the list.
const maxRecentHosts = 10

const recentHostsFile = "recent-hosts.json"

// recentHostsPath is the file; a variable so a test can move it.
var recentHostsPath = func() (string, error) {
	dir, err := configdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, recentHostsFile), nil
}

// recentMu orders the file's read-modify-write within the app.
var recentMu sync.Mutex

// RecentHosts are the hosts this computer's runs reached, newest first.
// A list that cannot be read is empty: it is a convenience, never a gate.
func RecentHosts() []string {
	recentMu.Lock()
	defer recentMu.Unlock()
	return readRecentHosts()
}

func readRecentHosts() []string {
	path, err := recentHostsPath()
	if err != nil {
		return []string{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	var doc struct {
		Hosts []string `json:"hosts"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return []string{}
	}
	// What the file holds is only what this app wrote, but it is a file:
	// each entry is offered again only if the run would take it.
	out := []string{}
	for _, h := range doc.Hosts {
		if a, err := targethost.ParseAnswer(h); err == nil && !slices.Contains(out, a.String()) {
			out = append(out, a.String())
		}
	}
	if len(out) > maxRecentHosts {
		out = out[:maxRecentHosts]
	}
	return out
}

// rememberHost puts a at the head of the list.
func rememberHost(a targethost.Answer) error {
	recentMu.Lock()
	defer recentMu.Unlock()
	return writeRecentHosts(append([]string{a.String()}, slices.DeleteFunc(readRecentHosts(), func(h string) bool { return h == a.String() })...))
}

// ForgetRecentHost takes answer off the list. One not on it is no error.
func ForgetRecentHost(answer string) error {
	recentMu.Lock()
	defer recentMu.Unlock()
	hosts := readRecentHosts()
	kept := slices.DeleteFunc(slices.Clone(hosts), func(h string) bool { return h == answer })
	if len(kept) == len(hosts) {
		return nil
	}
	return writeRecentHosts(kept)
}

func writeRecentHosts(hosts []string) error {
	if len(hosts) > maxRecentHosts {
		hosts = hosts[:maxRecentHosts]
	}
	path, err := recentHostsPath()
	if err != nil {
		return err
	}
	if path == "" {
		return errors.New("no folder to keep the recent hosts in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(struct {
		Hosts []string `json:"hosts"`
	}{hosts}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
