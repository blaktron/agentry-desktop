// Package deliverables is what the desktop does with the files a run was
// for: where they are copied by default, which of them one click opens, and
// how each is read back from the run folder. The run's envelope
// (result.json) names them by path and SHA-256; it is the runner's word, but
// the folder it names files in is what an agent wrote, so every read here
// goes through runfile (no link at any part) and every file is hashed
// against the envelope before it is listed, shown or opened.
package deliverables

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/blaktron/agentry-cli/pkg/runner/destination"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/runfile"
)

// Limits on what an envelope may list and a check reads.
const (
	MaxFiles         = 200
	maxEnvelopeBytes = 16 << 20
	// MaxFileBytes is the largest file the app hashes, shows or opens.
	MaxFileBytes  = 256 << 20
	maxTitleRunes = 200
)

// File is one deliverable the envelope names.
type File struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	// Marked says the agent named the file; false means the runner picked it.
	Marked bool `json:"marked"`
}

// Ext is the file's extension, taken from its path: the envelope's own word
// for the type is not relied on.
func (f File) Ext() string { return Ext(f.Path) }

// Delivery is one copy the envelope records.
type Delivery struct {
	Object string `json:"object"`
	SHA256 string `json:"sha256"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Kind is "folder" (or none, in a record from before destinations) for a
	// copy and "webhook" for a send; the rest belong to a send.
	Kind        string `json:"kind,omitempty"`
	Destination string `json:"destination,omitempty"`
	Host        string `json:"host,omitempty"`
	OnMyNetwork bool   `json:"onMyNetwork,omitempty"`
}

// Send is where a file, or a run's result alone (Object "result"), last
// went to a saved destination, and how that ended: the latest of the record's
// lines for the destination and the object, since a retry is a later line.
type Send struct {
	Destination string
	Host        string
	Object      string
	Status      string
	Error       string
	OnMyNetwork bool
}

// Record is the envelope's deliverables and the copies made of them.
type Record struct {
	// Ref is the release the run ran, as the envelope says.
	Ref        string
	Files      []File
	Truncated  bool
	Deliveries []Delivery
	// Sends are the sends to saved destinations, the latest outcome of each.
	Sends []Send
	// Withheld counts entries the envelope lists that are not well formed (a
	// path outside reports/, a link-like or climbing path, a digest that is
	// not a SHA-256) and so are not offered.
	Withheld int
}

// ReadRecord reads the deliverables block and the deliveries block of the run
// folder's result.json. A missing, linked, oversized or unreadable envelope
// lists nothing. The list is never nil.
func ReadRecord(runDir string) Record {
	rec := Record{Files: []File{}, Deliveries: []Delivery{}, Sends: []Send{}}
	b, ok := runfile.Read(runDir, "result.json", maxEnvelopeBytes)
	if !ok {
		return rec
	}
	var env struct {
		Ref          string `json:"ref"`
		Deliverables *struct {
			Files     []File `json:"files"`
			Truncated bool   `json:"truncated"`
		} `json:"deliverables"`
		Deliveries []Delivery `json:"deliveries"`
	}
	if json.Unmarshal(b, &env) != nil {
		return rec
	}
	rec.Ref = env.Ref
	if env.Deliverables != nil {
		rec.Truncated = env.Deliverables.Truncated
		seen := map[string]bool{}
		for _, f := range env.Deliverables.Files {
			f.SHA256 = strings.ToLower(f.SHA256)
			if !validFile(f) || seen[f.Path] {
				rec.Withheld++
				continue
			}
			seen[f.Path] = true
			if len(rec.Files) >= MaxFiles {
				rec.Truncated = true
				break
			}
			f.Title = tidyTitle(f.Title)
			rec.Files = append(rec.Files, f)
		}
	}
	// The copies: the envelope's block (a run on this computer wrote it as it
	// copied), and the folder's own record, which is the only one a run In
	// Agentry has, because its envelope was written before this app copied.
	all := env.Deliveries
	if led, ok := runfile.Read(runDir, "receipts/deliveries.ndjson", maxEnvelopeBytes); ok {
		for _, line := range bytes.Split(led, []byte("\n")) {
			var d Delivery
			if len(bytes.TrimSpace(line)) > 0 && json.Unmarshal(line, &d) == nil {
				all = append(all, d)
			}
		}
	}
	var sends []Delivery
	for _, d := range all {
		if d.Kind == "webhook" {
			sends = append(sends, d)
			continue
		}
		if d.Status == "delivered" {
			d.SHA256 = strings.ToLower(d.SHA256)
			rec.Deliveries = append(rec.Deliveries, d)
		}
	}
	// The ledger holds every line the run's own envelope block does, and a
	// retry's after them, so the last line for a destination and an object
	// is the outcome now.
	index := map[string]int{}
	names := map[string]bool{}
	for _, d := range sends {
		if !validSend(d) {
			continue
		}
		k := d.Destination + "\x00" + d.Object
		if _, seen := index[k]; !seen && (len(rec.Sends) >= maxSends || (!names[d.Destination] && len(names) >= destination.MaxDestinations)) {
			continue
		}
		names[d.Destination] = true
		s := Send{Destination: d.Destination, Host: d.Host, Object: d.Object, Status: d.Status, Error: closedError(d.Error), OnMyNetwork: d.OnMyNetwork}
		if i, ok := index[k]; ok {
			rec.Sends[i] = s
			continue
		}
		index[k] = len(rec.Sends)
		rec.Sends = append(rec.Sends, s)
	}
	return rec
}

// validFile is a deliverable the app can name: a clean "/"-separated path
// under reports/ (and not under reports/browser/, where a browsing run's
// captures go), and a SHA-256.
func validFile(f File) bool {
	if d, err := hex.DecodeString(f.SHA256); err != nil || len(d) != sha256.Size {
		return false
	}
	p := f.Path
	if p == "" || strings.ContainsAny(p, "\\\x00") || (runtime.GOOS == "windows" && strings.Contains(p, ":")) || path.Clean(p) != p || !filepath.IsLocal(filepath.FromSlash(p)) {
		return false
	}
	if !strings.HasPrefix(p, "reports/") || strings.HasPrefix(p, "reports/browser/") {
		return false
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return false
		}
	}
	return f.Bytes >= 0
}

// tidyTitle bounds a title to a length the page can show; what the page
// draws it with reveals any invisible character in it.
func tidyTitle(t string) string {
	t = strings.TrimSpace(t)
	if r := []rune(t); len(r) > maxTitleRunes {
		t = string(r[:maxTitleRunes])
	}
	return t
}

// validSend is a send the app can name: a saved destination's name, a host and
// port, and an object that is a report's path or "result". Anything else is the
// envelope's word about nothing the app holds.
func validSend(d Delivery) bool {
	if !destination.ValidName(d.Destination) || len(d.Host) > 300 {
		return false
	}
	for _, r := range d.Destination + d.Host + d.Object {
		if unicode.IsControl(r) {
			return false
		}
	}
	switch d.Status {
	case "delivered", "refused", "failed":
	default:
		return false
	}
	return d.Object == "result" || (strings.HasPrefix(d.Object, "reports/") && path.Clean(d.Object) == d.Object)
}

// maxSends bounds the sends the app reads from one record.
const maxSends = 2000

// closedError is a record's reason when it is one of the receipt's closed
// words, and "step" otherwise: the record's own text is never shown.
func closedError(e string) string {
	if e == "" {
		return ""
	}
	if slices.Contains(receipt.DeliveryErrors, e) {
		return e
	}
	return receipt.DeliveryErrStep
}
