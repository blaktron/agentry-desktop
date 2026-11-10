package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const serverListAnswer = `{
  "list": {"count": 3, "page": "/build/mcp-servers", "tested": {"first": "2026-10-01", "last": "2026-10-26"}},
  "query": {"q": "weather"},
  "total": 5,
  "match": "partial",
  "items": [
    {"id": "npm:@scope/weather-mcp", "kind": "npm", "status": "works", "title": "Weather", "description": "Forecasts  and\nalerts.", "toolCount": 3, "version": "1.2.3", "secrets": ["WEATHER_KEY"], "tested": "2026-10-20"},
    {"id": "pypi:weather_tool", "kind": "pypi", "status": "works", "toolCount": 2, "version": "0.4.0", "tested": "2026-10-21"},
    {"id": "remote:https://mcp.weather.example/mcp", "kind": "remote", "status": "works", "toolCount": 5, "tested": "2026-10-22"},
    {"id": "npm:no-version", "kind": "npm", "status": "works", "toolCount": 1},
    {"id": "npm:broken", "kind": "npm", "status": "fails", "version": "1.0.0"},
    {"id": "remote:https://signin.example/mcp", "kind": "remote", "status": "sign-in"}
  ]
}`

func TestSearchServerListOffersWorkingServersAsRows(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(serverListAnswer))
	}))
	defer srv.Close()

	v, err := searchServerList(context.Background(), srv.Client(), srv.URL+"/", "  weather  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != serverListPath {
		t.Fatalf("path %q", got.URL.Path)
	}
	if q := got.URL.Query(); q.Get("q") != "weather" || q.Get("status") != "works" || q.Get("limit") != "12" {
		t.Fatalf("query %v", q)
	}
	if v.Total != 5 || !v.Partial || v.Page != srv.URL+"/build/mcp-servers" || v.Query != "weather" {
		t.Fatalf("view %+v", v)
	}
	if len(v.Items) != 3 {
		t.Fatalf("want the three working servers with a pin, got %d: %+v", len(v.Items), v.Items)
	}
	npm, pypi, remote := v.Items[0], v.Items[1], v.Items[2]
	if npm.Name != "Weather" || npm.Ref != "@scope/weather-mcp@1.2.3" || npm.Description != "Forecasts and alerts." || npm.ToolCount != 3 {
		t.Fatalf("npm item %+v", npm)
	}
	if r := npm.Row; r.Source != "npm" || r.Package != "@scope/weather-mcp" || r.Version != "1.2.3" || len(r.Secrets) != 0 {
		t.Fatalf("npm row %+v", r)
	}
	if r := pypi.Row; r.Source != "pypi" || r.Package != "weather_tool" || r.Version != "0.4.0" || pypi.Name != "weather_tool" {
		t.Fatalf("pypi item %+v", pypi)
	}
	if r := remote.Row; r.Source != "url" || r.URL != "https://mcp.weather.example/mcp" || r.Auth != "none" || remote.Name != "mcp.weather.example" {
		t.Fatalf("remote item %+v", remote)
	}
}

func TestSearchServerListEmptyQueryAsksNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("an empty search reached the registry")
	}))
	defer srv.Close()
	v, err := searchServerList(context.Background(), srv.Client(), srv.URL, "   ")
	if err != nil || len(v.Items) != 0 {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestSearchServerListWithoutAList(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"404":  func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"page": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>not here</html>")) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			if _, err := searchServerList(context.Background(), srv.Client(), srv.URL, "x"); !errors.Is(err, ErrNoServerList) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestSearchServerListOtherFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := searchServerList(context.Background(), srv.Client(), srv.URL, "x")
	if err == nil || errors.Is(err, ErrNoServerList) || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err %v", err)
	}
}

func TestServerListRowRefusesWhatCannotBeAdded(t *testing.T) {
	for _, c := range []struct{ kind, id, version string }{
		{"npm", "npm:x", ""},
		{"npm", "pypi:x", "1.0.0"},
		{"remote", "remote:http://plain.example/mcp", ""},
		{"docker", "docker:x", "1"},
		{"npm", "npm:", "1.0.0"},
	} {
		if _, _, ok := serverListRow(c.kind, c.id, c.version); ok {
			t.Errorf("%+v was offered", c)
		}
	}
}

func TestClipText(t *testing.T) {
	if got := clipText("ab  cd", 10); got != "ab cd" {
		t.Fatalf("%q", got)
	}
	if got := clipText("héllo wörld", 2); got != "h…" {
		t.Fatalf("%q", got)
	}
}
