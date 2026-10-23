package servericon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"
)

var (
	png  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...)
	jpeg = append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 40)...)
	webp = append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 40)...)
	svg  = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
)

// TestSniffTakesPNGJPEGAndWebPAlone: an icon's type is read from its first
// bytes; SVG, HTML and a PNG's name on other bytes are refused.
func TestSniffTakesPNGJPEGAndWebPAlone(t *testing.T) {
	for _, c := range []struct {
		b    []byte
		want string
	}{{png, "image/png"}, {jpeg, "image/jpeg"}, {webp, "image/webp"}} {
		if got, ok := Sniff(c.b); !ok || got != c.want {
			t.Errorf("Sniff(%q…) = %q %v, want %q", c.b[:4], got, ok, c.want)
		}
	}
	for _, b := range [][]byte{svg, []byte("<?xml version=\"1.0\"?><svg/>"), []byte("<html>"), []byte("GIF89a"), nil, []byte("RIFF\x00\x00\x00\x00WAVE")} {
		if got, ok := Sniff(b); ok {
			t.Errorf("Sniff(%q) = %q, want refused", b, got)
		}
	}
}

// TestChooseRefusesSVGAndPlainHTTP: an SVG, an http address and an
// unknown type are never chosen; among the rest the largest size up to 256
// pixels wins.
func TestChooseRefusesSVGAndPlainHTTP(t *testing.T) {
	icons := []Icon{
		{Src: "https://example.com/logo.svg", MimeType: "image/svg+xml", Sizes: []string{"any"}},
		{Src: "https://example.com/icon.svg"},
		{Src: "http://example.com/icon.png", MimeType: "image/png", Sizes: []string{"128x128"}},
		{Src: "https://example.com/icon.gif", MimeType: "image/gif", Sizes: []string{"128x128"}},
		{Src: "https://example.com/icon-512.png", MimeType: "image/png", Sizes: []string{"512x512"}},
		{Src: "https://example.com/icon-180.png", MimeType: "image/png", Sizes: []string{"180x180"}},
		{Src: "https://example.com/icon-64.webp", Sizes: []string{"64x64"}},
	}
	if got, ok := Choose(icons); !ok || got.Src != "https://example.com/icon-180.png" {
		t.Errorf("Choose = %+v %v, want the 180 PNG", got, ok)
	}
	if got, ok := Choose(icons[:4]); ok {
		t.Errorf("Choose chose %+v from SVGs, http and GIF", got)
	}
	if got, ok := Choose([]Icon{{Src: "https://example.com/i.png", MimeType: "image/svg+xml"}}); ok {
		t.Errorf("a declared SVG with a .png name was chosen: %+v", got)
	}
}

// TestFetchHoldsTheRules: an icon over 256 KiB, an SVG served as a PNG and
// a redirect to http are refused; a PNG within the bound is kept.
func TestFetchHoldsTheRules(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(png) }))
	defer plain.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.png":
			_, _ = w.Write(png)
		case "/big.png":
			_, _ = w.Write(append(png, bytes.Repeat([]byte{0}, MaxBytes)...))
		case "/svg.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(svg)
		case "/away.png":
			http.Redirect(w, r, plain.URL+"/x.png", http.StatusFound)
		}
	}))
	defer srv.Close()
	c := srv.Client()
	ctx := context.Background()
	if b, mt, err := Fetch(ctx, c, srv.URL+"/ok.png"); err != nil || mt != "image/png" || !bytes.Equal(b, png) {
		t.Fatalf("ok.png: %v %q", err, mt)
	}
	for _, p := range []string{"/big.png", "/svg.png"} {
		if _, _, err := Fetch(ctx, c, srv.URL+p); !errors.Is(err, ErrNotAnImage) {
			t.Errorf("%s: %v, want refused as not an image", p, err)
		}
	}
	if _, _, err := Fetch(ctx, c, srv.URL+"/away.png"); err == nil || !strings.Contains(err.Error(), "https only") {
		t.Errorf("a redirect to http: %v", err)
	}
	if _, _, err := Fetch(ctx, c, plain.URL+"/x.png"); err == nil {
		t.Error("an http address was fetched")
	}
}

// TestLookupMatchesThePackageExactly: only the entry whose package (or
// remote address) is the server's own gives its icons; a name that merely
// contains the search term does not.
func TestLookupMatchesThePackageExactly(t *testing.T) {
	var asked []string
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("search"))
		_ = json.NewEncoder(w).Encode(map[string]any{"servers": []any{
			map[string]any{"server": map[string]any{"name": "io.example/other", "icons": []Icon{{Src: "https://other.example/i.png"}},
				"packages": []any{map[string]any{"registryType": "npm", "identifier": "@other/server-github"}}}},
			map[string]any{"server": map[string]any{"name": "io.example/github", "icons": []Icon{{Src: "https://example.com/i.png"}},
				"packages": []any{map[string]any{"registryType": "npm", "identifier": "@acme/server-github"}}}},
			map[string]any{"server": map[string]any{"name": "io.example/remote", "icons": []Icon{{Src: "https://remote.example/i.png"}},
				"remotes": []any{map[string]any{"url": "https://mcp.example.com/mcp/"}}}},
		}})
	}))
	defer reg.Close()
	ctx := context.Background()
	got, err := Lookup(ctx, reg.Client(), reg.URL, manifest.MCPServer{NPM: "@acme/server-github", Version: "1.0.0"})
	if err != nil || len(got) != 1 || got[0].Src != "https://example.com/i.png" {
		t.Fatalf("npm: %+v %v", got, err)
	}
	if asked[0] != "server-github" {
		t.Errorf("searched for %q", asked[0])
	}
	if got, _ := Lookup(ctx, reg.Client(), reg.URL, manifest.MCPServer{URL: "https://mcp.example.com/mcp"}); len(got) != 1 || got[0].Src != "https://remote.example/i.png" {
		t.Errorf("remote: %+v", got)
	}
	if got, _ := Lookup(ctx, reg.Client(), reg.URL, manifest.MCPServer{NPM: "server-github", Version: "1.0.0"}); got != nil {
		t.Errorf("a package of another name took an icon: %+v", got)
	}
}

// TestStoreKeepsOnceAndRereads: an icon is kept and read back as a data:
// URL; none is remembered as none; a kept file that is no longer an image
// draws nothing.
func TestStoreKeepsOnceAndRereads(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	a := manifest.MCPServer{NPM: "a", Version: "1.0.0"}
	b := manifest.MCPServer{PyPI: "b", Version: "1.0.0"}
	if _, known := s.Get(a); known {
		t.Fatal("an unfetched server is known")
	}
	if err := s.Put(a, png, "image/png"); err != nil {
		t.Fatal(err)
	}
	if u, known := s.Get(a); !known || !strings.HasPrefix(u, "data:image/png;base64,") {
		t.Errorf("a: %q %v", u, known)
	}
	if err := s.Put(b, nil, ""); err != nil {
		t.Fatal(err)
	}
	if u, known := s.Get(b); !known || u != "" {
		t.Errorf("b: %q %v", u, known)
	}
	if err := s.Put(b, svg, "image/png"); !errors.Is(err, ErrNotAnImage) {
		t.Errorf("an SVG was kept as a PNG: %v", err)
	}
}

// TestClientReachesPublicAddressesAlone: the client's connections are
// refused at a loopback, private, link-local or shared address, whatever
// name or redirect led there.
func TestClientReachesPublicAddressesAlone(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(png) }))
	defer srv.Close()
	if _, _, err := Fetch(context.Background(), Client(5*time.Second), srv.URL+"/i.png"); !errors.Is(err, errNotPublic) {
		t.Fatalf("a loopback address: %v", err)
	}
	for _, a := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.20", "172.16.0.9", "169.254.169.254", "100.64.0.7", "::1", "fd7a:115c:a1e0::1", "0.0.0.0"} {
		if publicIP(net.ParseIP(a)) {
			t.Errorf("%s reads as public", a)
		}
	}
	for _, a := range []string{"140.82.112.3", "2606:4700::6810:22"} {
		if !publicIP(net.ParseIP(a)) {
			t.Errorf("%s reads as not public", a)
		}
	}
}

// TestAPrivateHostIsNeverSearchedFor: a remote server on an address, a
// single label or a private network's name is not sent to the registry.
func TestAPrivateHostIsNeverSearchedFor(t *testing.T) {
	for _, u := range []string{"https://10.0.0.5/mcp", "https://mcp/mcp", "https://mcp.corp.internal/mcp", "https://nas.local/mcp", "https://box.lan/mcp"} {
		if got := searchTerm(manifest.MCPServer{URL: u}); got != "" {
			t.Errorf("%s searched as %q", u, got)
		}
	}
	if got := searchTerm(manifest.MCPServer{URL: "https://mcp.linear.app/mcp"}); got != "mcp.linear.app" {
		t.Errorf("a public host searched as %q", got)
	}
}
