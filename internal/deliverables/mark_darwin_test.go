package deliverables

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// A copy this app collects carries the quarantine attribute macOS keeps on a
// download, so Gatekeeper treats what an agent wrote as the download it is.
func TestACollectedCopyCarriesTheQuarantineAttribute(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	got, err := Collect(r.dir, root)
	if err != nil || len(got) != 1 || got[0].Status != "delivered" {
		t.Fatalf("Collect = %+v, %v", got, err)
	}
	buf := make([]byte, 256)
	n, err := unix.Getxattr(got[0].Object, "com.apple.quarantine", buf)
	if err != nil {
		t.Fatalf("the copy %s has no quarantine attribute: %v", got[0].Object, err)
	}
	if v := string(buf[:n]); !strings.HasPrefix(v, "0081;") {
		t.Fatalf("quarantine attribute = %q", v)
	}
	if got[0].Error != "" {
		t.Fatalf("the copy says %q", got[0].Error)
	}
}
