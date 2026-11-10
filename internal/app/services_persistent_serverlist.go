package app

// The server Add card's search: the registry site's MCP server list, which
// records what a survey saw of each npm package, PyPI package and remote
// address it reached. Only the servers it saw working are offered, each at
// the version it tested, so one click adds a server that installs and
// lists tools. A server that signs in by OAuth is left out (a persistent
// agent cannot sign in), and so is one that refused an anonymous request.
// The list is advice: what is added still goes through AddServer and the
// consent dialog.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

const (
	serverListPath = "/build/mcp-servers/lookup"
	// serverListLimit is how many matches one search shows.
	serverListLimit = 12
	// serverListQueryMax bounds what a search sends.
	serverListQueryMax = 200
	// serverListBodyMax bounds what is read of an answer: twelve records
	// and the list's own facts fit in far less.
	serverListBodyMax = 1 << 20
	serverListWait    = 15 * time.Second
	// serverListTextMax bounds a record's title and description as drawn.
	serverListTextMax = 200
)

// ErrNoServerList is a registry that serves no MCP server list (a
// self-hosted registry has none).
var ErrNoServerList = errors.New("this registry has no MCP server list")

// ServerListItem is one working server the list knows, with the row that
// adds it.
type ServerListItem struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Name is the list's title for it, or its package or host.
	Name string `json:"name"`
	// Ref is what is added: the package at the tested version, or the
	// address.
	Ref         string `json:"ref"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	ToolCount   int    `json:"toolCount"`
	// Secrets are the key names its catalogue declared; it worked without
	// them, so they are shown and not declared.
	Secrets []string           `json:"secrets,omitempty"`
	Tested  string             `json:"tested,omitempty"`
	Row     build.MCPServerRow `json:"row"`
}

// ServerListView is a search's answer: the matches, how many the list
// holds in all, whether they match only some of the words, and the page
// that shows the whole list.
type ServerListView struct {
	Registry string           `json:"registry"`
	Query    string           `json:"query"`
	Items    []ServerListItem `json:"items"`
	Total    int              `json:"total"`
	Partial  bool             `json:"partial"`
	Page     string           `json:"page"`
}

// SearchServers searches the default registry's MCP server list for
// working servers matching q. An empty q answers no items.
func (p *PersistentAPI) SearchServers(q string) (*ServerListView, error) {
	reg := settings.SecureAgentryRegistry()
	if s, err := settings.Load(); err == nil && s.DefaultRegistry != "" {
		reg = s.DefaultRegistry
	}
	ctx, cancel := context.WithTimeout(context.Background(), serverListWait)
	defer cancel()
	return searchServerList(ctx, registry.HTTPClient, settings.Normalize(reg), q)
}

func searchServerList(ctx context.Context, client *http.Client, reg, q string) (*ServerListView, error) {
	q = strings.TrimSpace(q)
	if len(q) > serverListQueryMax {
		q = q[:serverListQueryMax]
	}
	reg = strings.TrimRight(reg, "/")
	view := &ServerListView{Registry: reg, Query: q, Items: []ServerListItem{}, Page: reg + "/build/mcp-servers"}
	if q == "" {
		return view, nil
	}
	v := url.Values{"q": {q}, "status": {"works"}, "limit": {fmt.Sprint(serverListLimit)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reg+serverListPath+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the MCP server list could not be reached: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNoServerList
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("the MCP server list answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, serverListBodyMax+1))
	if err != nil {
		return nil, fmt.Errorf("the MCP server list could not be read: %w", err)
	}
	if len(body) > serverListBodyMax {
		return nil, errors.New("the MCP server list's answer was too large")
	}
	var r struct {
		List struct {
			Page string `json:"page"`
		} `json:"list"`
		Total int    `json:"total"`
		Match string `json:"match"`
		Items []struct {
			ID          string   `json:"id"`
			Kind        string   `json:"kind"`
			Status      string   `json:"status"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			ToolCount   *int     `json:"toolCount"`
			Version     string   `json:"version"`
			Secrets     []string `json:"secrets"`
			Tested      string   `json:"tested"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		// A registry without the list may answer its own page, not JSON.
		return nil, ErrNoServerList
	}
	view.Total, view.Partial = r.Total, r.Match == "partial"
	if page := strings.TrimSpace(r.List.Page); strings.HasPrefix(page, "/") {
		view.Page = reg + page
	}
	for _, it := range r.Items {
		if it.Status != "works" {
			continue
		}
		row, name, ok := serverListRow(it.Kind, it.ID, it.Version)
		if !ok {
			continue
		}
		item := ServerListItem{
			ID: it.ID, Kind: it.Kind, Name: name, Ref: name, Version: row.Version, Secrets: it.Secrets, Tested: it.Tested, Row: row,
			Description: clipText(it.Description, serverListTextMax),
		}
		if row.Version != "" {
			item.Ref = name + "@" + row.Version
		} else if row.URL != "" {
			item.Ref = row.URL
		}
		if t := clipText(it.Title, serverListTextMax); t != "" {
			item.Name = t
		}
		if it.ToolCount != nil {
			item.ToolCount = *it.ToolCount
		}
		view.Items = append(view.Items, item)
	}
	return view, nil
}

// serverListRow is the Builder row that adds the list's record id at the
// version the list tested, and the name it is shown by. A package with no
// tested version cannot be pinned, so it is not offered.
func serverListRow(kind, id, version string) (build.MCPServerRow, string, bool) {
	prefix, rest, found := strings.Cut(id, ":")
	rest = strings.TrimSpace(rest)
	if !found || rest == "" || prefix != kind {
		return build.MCPServerRow{}, "", false
	}
	switch kind {
	case "npm", "pypi":
		version = strings.TrimSpace(version)
		if version == "" {
			return build.MCPServerRow{}, "", false
		}
		return build.MCPServerRow{Source: kind, Package: rest, Version: version, Args: []string{}, Tools: []string{}}, rest, true
	case "remote":
		if !strings.HasPrefix(rest, "https://") {
			return build.MCPServerRow{}, "", false
		}
		host := rest
		if u, err := url.Parse(rest); err == nil && u.Host != "" {
			host = u.Host
		}
		return build.MCPServerRow{Source: "url", URL: rest, Auth: "none", Tools: []string{}}, host, true
	}
	return build.MCPServerRow{}, "", false
}

// clipText folds s's white space and trims it to at most n bytes on a
// rune boundary, with an ellipsis.
func clipText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimRight(s[:cut], " ") + "…"
}
