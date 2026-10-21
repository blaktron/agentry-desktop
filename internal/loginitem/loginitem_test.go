package loginitem

import (
	"encoding/xml"
	"strings"
	"testing"
)

// Each platform's entry launches the app it names with HiddenFlag, and a
// path with spaces or quotes stays one argument.
func TestEachEntryLaunchesTheAppHidden(t *testing.T) {
	exe := `/opt/Agentry Desktop/agentry "desktop"`
	d := desktopEntry(exe)
	if !strings.Contains(d, `Exec="/opt/Agentry Desktop/agentry \"desktop\"" --hidden`) || !strings.HasPrefix(d, "[Desktop Entry]\n") {
		t.Fatalf("desktop entry:\n%s", d)
	}
	p := launchAgent("net.example.agent", `/Applications/Agentry & Co.app/Contents/MacOS/agentry`)
	var plist struct {
		Strings []string `xml:"dict>string"`
		Args    []string `xml:"dict>array>string"`
	}
	if err := xml.Unmarshal([]byte(p), &plist); err != nil {
		t.Fatalf("the LaunchAgent is not XML: %v\n%s", err, p)
	}
	if len(plist.Args) != 2 || plist.Args[0] != `/Applications/Agentry & Co.app/Contents/MacOS/agentry` || plist.Args[1] != HiddenFlag || !strings.Contains(p, "<key>RunAtLoad</key>\n\t<true/>") {
		t.Fatalf("LaunchAgent:\n%s", p)
	}
	if v := runValue(`C:\Program Files\Agentry\agentry-desktop.exe`); v != `"C:\\Program Files\\Agentry\\agentry-desktop.exe" --hidden` {
		t.Fatalf("Run value %s", v)
	}
}
