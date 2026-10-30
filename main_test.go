package main

import (
	"strings"
	"testing"
)

// The notices embedded in the binary are the generated file, and name what
// the build contains that a licence requires naming.
func TestTheEmbeddedNoticesNameWhatTheAppShips(t *testing.T) {
	if !strings.HasPrefix(notices, "Third-party notices for Agentry Desktop\n") {
		t.Fatalf("the embedded notices start with %q", notices[:min(len(notices), 60)])
	}
	for _, want := range []string{
		"pdfjs-dist",                   // Apache-2.0, in the bundle
		"@ungap/structured-clone",      // ISC, in the bundle
		"SIL OPEN FONT LICENSE",        // the fonts
		"@lobehub/icons-static-svg",    // the provider logos
		"MicrosoftEdgeWebview2Setup",   // the Windows installer's bootstrapper
		"OpenWebView2Loader",           // the Windows loader's licence
		"github.com/wailsapp/wails/v3", // a Go module
		"Apache License",               // the Apache text, once for its modules
	} {
		if !strings.Contains(notices, want) {
			t.Errorf("the embedded notices do not mention %q", want)
		}
	}
}
