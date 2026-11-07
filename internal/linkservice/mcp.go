package linkservice

// The link service's MCP client: what speaks to an installed server. It
// speaks the current specification, 2026-07-28, where every request carries
// its protocol version and the client's capabilities in _meta and there is
// no handshake, and falls back to the initialize handshake of 2025-11-25 for
// a server that does not answer the modern probe, as the specification's
// backward-compatibility rules say: on stdio, probe with server/discover and
// fall back on any error that is not a recognised modern one, or on
// silence; over Streamable HTTP, try a modern request and fall back on a
// 4xx whose body is not a recognised modern error.
//
// The client declares no optional capability (no sampling, no elicitation,
// no roots), answers a ping and refuses every other request a server sends,
// and keeps only what a tool call needs: the tools list and a call's
// content items. What crosses back to the run is a tool call's answer,
// never an MCP message.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
)

// The protocol versions the client speaks.
const (
	// ModernVersion is the current specification's version.
	ModernVersion = "2026-07-28"
	// LegacyVersion is the handshake version the client falls back to.
	LegacyVersion = "2025-11-25"
)

// The eras an installed server is found to speak.
const (
	EraModern = "modern"
	EraLegacy = "legacy"
)

// The JSON-RPC error codes the client reads.
const (
	codeMethodNotFound             = -32601
	codeUnsupportedProtocolVersion = -32022
	codeHeaderMismatch             = -32020
	codeMissingClientCapability    = -32021
)

// probeTimeout bounds the modern probe: a legacy server may not answer an
// unknown request before its handshake at all.
var probeTimeout = 10 * time.Second

// maxMessage bounds one message from an installed server; a screenshot of
// a viewport is a few megabytes of base64.
const maxMessage = 32 << 20

// clientName is how the client names itself to an installed server.
const clientName = "Agentry Desktop"

// Session is one MCP session with an installed server, whatever carries it.
type Session interface {
	// Tools lists the server's tools.
	Tools(ctx context.Context) ([]desktoplink.Tool, error)
	// Call calls one tool.
	Call(ctx context.Context, name string, args json.RawMessage) (desktoplink.Answer, error)
	// Era is the era the server was found to speak.
	Era() string
	// Close ends the session and whatever process carries it.
	Close() error
}

// rpcError is a JSON-RPC error from a server.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// modern says the error is one only a server of the current specification
// sends, which identifies its era.
func (e *rpcError) modern() bool {
	switch e.Code {
	case codeUnsupportedProtocolVersion, codeHeaderMismatch, codeMissingClientCapability:
		return true
	}
	return false
}

// supported is an UnsupportedProtocolVersionError's list of versions.
func (e *rpcError) supported() []string {
	var d struct {
		Supported []string `json:"supported"`
	}
	_ = json.Unmarshal(e.Data, &d)
	return d.Supported
}

// message is one JSON-RPC message either way.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// requestMeta is the _meta every modern request carries.
func requestMeta(version string) map[string]any {
	return map[string]any{
		"io.modelcontextprotocol/protocolVersion":    version,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": clientName, "version": clientVersion},
	}
}

// clientVersion is the version the client reports; the app sets it.
var clientVersion = "dev"

// SetClientVersion sets the version the client reports to installed
// servers.
func SetClientVersion(v string) {
	if v != "" {
		clientVersion = v
	}
}

// caller sends one request and reads its result: the transport's half.
type caller interface {
	call(ctx context.Context, method string, params any) (json.RawMessage, error)
	notify(ctx context.Context, method string, params any) error
}

// mcp is the protocol over a transport: the era, the version, and the
// requests a link service makes.
type mcp struct {
	t       caller
	era     string
	version string
}

// negotiate finds the server's era: a modern probe first, and the legacy
// handshake when the probe fails as a legacy server fails it.
func (m *mcp) negotiate(ctx context.Context, probe func(ctx context.Context) error) error {
	err := probe(ctx)
	if err == nil {
		m.era, m.version = EraModern, ModernVersion
		return nil
	}
	var re *rpcError
	if errors.As(err, &re) && re.modern() {
		// A modern server that does not speak this client's modern version:
		// retrying with a version it lists is only possible for a version
		// this client also speaks, and it speaks one.
		return fmt.Errorf("the installed server speaks MCP %s and not %s: %w", strings.Join(re.supported(), ", "), ModernVersion, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	res, err := m.t.call(ctx, "initialize", map[string]any{
		"protocolVersion": LegacyVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": clientVersion},
	})
	if err != nil {
		return fmt.Errorf("the installed server answered neither the current MCP specification nor the %s handshake: %w", LegacyVersion, err)
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(res, &init); err != nil || init.ProtocolVersion == "" {
		return fmt.Errorf("the installed server's handshake answer is not one: %s", clip(string(res), 200))
	}
	m.era, m.version = EraLegacy, init.ProtocolVersion
	return m.t.notify(ctx, "notifications/initialized", nil)
}

// params are a request's parameters, with _meta for a modern server.
func (m *mcp) params(p map[string]any) map[string]any {
	if p == nil {
		p = map[string]any{}
	}
	if m.era == EraModern {
		p["_meta"] = requestMeta(m.version)
	}
	return p
}

// maxToolPages bounds a tools list's pages.
const maxToolPages = 50

func (m *mcp) tools(ctx context.Context) ([]desktoplink.Tool, error) {
	var all []desktoplink.Tool
	cursor := ""
	for range maxToolPages {
		p := map[string]any{}
		if cursor != "" {
			p["cursor"] = cursor
		}
		res, err := m.t.call(ctx, "tools/list", m.params(p))
		if err != nil {
			return nil, err
		}
		var page struct {
			Tools      []desktoplink.Tool `json:"tools"`
			NextCursor string             `json:"nextCursor"`
		}
		if err := json.Unmarshal(res, &page); err != nil {
			return nil, fmt.Errorf("the installed server's tools list is not one: %w", err)
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
	return nil, fmt.Errorf("the installed server's tools list ran past %d pages", maxToolPages)
}

func (m *mcp) callTool(ctx context.Context, name string, args json.RawMessage) (desktoplink.Answer, error) {
	p := map[string]any{"name": name}
	if len(args) > 0 && string(args) != "null" {
		p["arguments"] = args
	} else {
		p["arguments"] = map[string]any{}
	}
	res, err := m.t.call(ctx, "tools/call", m.params(p))
	if err != nil {
		var re *rpcError
		if errors.As(err, &re) {
			// A protocol error from the server is the call's answer, as an
			// error the agent reads.
			return desktoplink.TextAnswer("the installed server refused the call: "+re.Message, true), nil
		}
		return desktoplink.Answer{}, err
	}
	var out struct {
		Content           []json.RawMessage `json:"content"`
		IsError           bool              `json:"isError"`
		StructuredContent json.RawMessage   `json:"structuredContent"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return desktoplink.Answer{}, fmt.Errorf("the installed server's answer is not a tool result: %w", err)
	}
	a := desktoplink.Answer{IsError: out.IsError}
	for _, raw := range out.Content {
		a.Content = append(a.Content, contentItem(raw))
	}
	if len(a.Content) == 0 && len(out.StructuredContent) > 0 && string(out.StructuredContent) != "null" {
		a.Content = append(a.Content, desktoplink.Content{Type: "text", Text: string(out.StructuredContent)})
	}
	return a, nil
}

// contentItem is one content item as the desktop link carries it: text and
// images whole, a resource's text or a link's address as text, anything
// else named by its type.
func contentItem(raw json.RawMessage) desktoplink.Content {
	var c struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Data     string `json:"data"`
		MimeType string `json:"mimeType"`
		URI      string `json:"uri"`
		Resource struct {
			URI      string `json:"uri"`
			Text     string `json:"text"`
			MimeType string `json:"mimeType"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return desktoplink.Content{Type: "text", Text: "(a content item that is not one)"}
	}
	switch c.Type {
	case "text":
		return desktoplink.Content{Type: "text", Text: c.Text}
	case "image", "audio":
		return desktoplink.Content{Type: c.Type, Data: c.Data, MimeType: c.MimeType}
	case "resource":
		if c.Resource.Text != "" {
			return desktoplink.Content{Type: "text", Text: c.Resource.Text}
		}
		return desktoplink.Content{Type: "text", Text: "a resource: " + c.Resource.URI}
	case "resource_link":
		return desktoplink.Content{Type: "text", Text: "a resource: " + c.URI}
	}
	return desktoplink.Content{Type: "text", Text: "(a " + c.Type + " content item)"}
}

// clip cuts s to n bytes for a message.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// rpcConn is JSON-RPC over newline-delimited messages on a pair of
// streams: the stdio transport's framing.
type rpcConn struct {
	w      io.Writer
	wmu    sync.Mutex
	mu     sync.Mutex
	nextID int64
	wait   map[string]chan message
	closed chan struct{}
	err    error
}

func newRPCConn(r io.Reader, w io.Writer) *rpcConn {
	c := &rpcConn{w: w, wait: map[string]chan message{}, closed: make(chan struct{})}
	go c.read(r)
	return c
}

// read dispatches each message: an answer to its waiting request, a
// server's request answered (a ping) or refused, a notification dropped.
func (c *rpcConn) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxMessage)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m message
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		switch {
		case m.Method != "" && len(m.ID) > 0:
			c.answerServer(m)
		case m.Method != "":
			// A notification: nothing the link service keeps.
		case len(m.ID) > 0:
			c.mu.Lock()
			ch := c.wait[string(m.ID)]
			delete(c.wait, string(m.ID))
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
	}
	c.mu.Lock()
	c.err = sc.Err()
	if c.err == nil {
		c.err = io.EOF
	}
	c.mu.Unlock()
	close(c.closed)
}

// answerServer answers a request the server sent: a ping, and nothing
// else, since the client declared no capability a request could use.
func (c *rpcConn) answerServer(m message) {
	ans := message{JSONRPC: "2.0", ID: m.ID}
	if m.Method == "ping" {
		ans.Result = json.RawMessage(`{}`)
	} else {
		ans.Error = &rpcError{Code: codeMethodNotFound, Message: "Agentry Desktop does not answer " + m.Method}
	}
	_ = c.write(ans)
}

func (c *rpcConn) write(m message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.w.Write(append(b, '\n'))
	return err
}

func (c *rpcConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := json.RawMessage(strconv.FormatInt(c.nextID, 10))
	ch := make(chan message, 1)
	c.wait[string(id)] = ch
	c.mu.Unlock()
	drop := func() {
		c.mu.Lock()
		delete(c.wait, string(id))
		c.mu.Unlock()
	}
	m := message{JSONRPC: "2.0", ID: id, Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			drop()
			return nil, err
		}
		m.Params = b
	}
	if err := c.write(m); err != nil {
		drop()
		return nil, fmt.Errorf("write to the installed server: %w", err)
	}
	select {
	case ans := <-ch:
		if ans.Error != nil {
			return nil, ans.Error
		}
		return ans.Result, nil
	case <-c.closed:
		drop()
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		return nil, fmt.Errorf("the installed server ended: %w", err)
	case <-ctx.Done():
		drop()
		if method != "initialize" && method != "server/discover" {
			_ = c.notifyRaw("notifications/cancelled", map[string]any{"requestId": id, "reason": "the call was ended"})
		}
		return nil, ctx.Err()
	}
}

func (c *rpcConn) notify(_ context.Context, method string, params any) error {
	return c.notifyRaw(method, params)
}

func (c *rpcConn) notifyRaw(method string, params any) error {
	m := message{JSONRPC: "2.0", Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		m.Params = b
	}
	return c.write(m)
}

// stdioProbe is the stdio transport's modern probe: server/discover with
// the client's modern version, bounded by probeTimeout.
func stdioProbe(c caller) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		pctx, cancel := context.WithTimeout(ctx, probeTimeout)
		defer cancel()
		res, err := c.call(pctx, "server/discover", map[string]any{"_meta": requestMeta(ModernVersion)})
		if err != nil {
			return err
		}
		var d struct {
			SupportedVersions []string `json:"supportedVersions"`
		}
		if err := json.Unmarshal(res, &d); err != nil || len(d.SupportedVersions) == 0 {
			return errors.New("the discover answer named no version")
		}
		for _, v := range d.SupportedVersions {
			if v == ModernVersion {
				return nil
			}
		}
		return &rpcError{Code: codeUnsupportedProtocolVersion, Message: "unsupported protocol version", Data: mustJSON(map[string]any{"supported": d.SupportedVersions})}
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// httpConn is JSON-RPC over Streamable HTTP to a loopback endpoint: each
// request a POST whose answer is JSON or an event stream carrying it.
type httpConn struct {
	endpoint string
	client   *http.Client
	// version is the MCP-Protocol-Version header a modern request carries,
	// "" for a legacy session.
	version string
	// session is a legacy session's Mcp-Session-Id.
	session string
	mu      sync.Mutex
	nextID  int64
}

// httpStatusError is a non-2xx answer: a legacy server's 4xx to a modern
// request is how its era shows.
type httpStatusError struct {
	status int
	body   []byte
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("the installed server answered HTTP %d: %s", e.status, clip(strings.TrimSpace(string(e.body)), 200))
}

func (c *httpConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	version, session := c.version, c.session
	c.mu.Unlock()
	body := message{JSONRPC: "2.0", ID: json.RawMessage(strconv.FormatInt(id, 10)), Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		body.Params = b
	}
	ans, hdr, err := c.post(ctx, body, version, session)
	if err != nil {
		return nil, err
	}
	if method == "initialize" {
		if sid := hdr.Get("Mcp-Session-Id"); sid != "" {
			c.mu.Lock()
			c.session = sid
			c.mu.Unlock()
		}
	}
	if ans == nil {
		return nil, errors.New("the installed server's answer held no response")
	}
	if ans.Error != nil {
		return nil, ans.Error
	}
	return ans.Result, nil
}

func (c *httpConn) notify(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	version, session := c.version, c.session
	c.mu.Unlock()
	m := message{JSONRPC: "2.0", Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		m.Params = b
	}
	_, _, err := c.post(ctx, m, version, session)
	return err
}

// post sends one message and reads the response to it, from a JSON body or
// the first matching message of an event stream; nil for a notification's
// 202.
func (c *httpConn) post(ctx context.Context, m message, version, session string) (*message, http.Header, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if version != "" {
		req.Header.Set("MCP-Protocol-Version", version)
	}
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("reach the installed server at %s: %w", c.endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := io.LimitReader(resp.Body, maxMessage)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(body)
		return nil, resp.Header, &httpStatusError{status: resp.StatusCode, body: raw}
	}
	if len(m.ID) == 0 {
		return nil, resp.Header, nil
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		ans, err := readEventStream(body, m.ID)
		return ans, resp.Header, err
	}
	var ans message
	if err := json.NewDecoder(body).Decode(&ans); err != nil {
		return nil, resp.Header, fmt.Errorf("the installed server's answer is not JSON-RPC: %w", err)
	}
	return &ans, resp.Header, nil
}

// readEventStream reads an event stream until the response to id.
func readEventStream(r io.Reader, id json.RawMessage) (*message, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxMessage)
	var data strings.Builder
	flush := func() *message {
		defer data.Reset()
		if data.Len() == 0 {
			return nil
		}
		var m message
		if json.Unmarshal([]byte(data.String()), &m) != nil {
			return nil
		}
		if m.Method == "" && string(m.ID) == string(id) {
			return &m
		}
		return nil
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if m := flush(); m != nil {
				return m, nil
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if m := flush(); m != nil {
		return m, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("the installed server's event stream ended without the response")
}

// httpProbe is Streamable HTTP's modern probe: a modern tools/list. A 4xx
// whose body is not a recognised modern error identifies a legacy server;
// the client then drops the version header for the handshake.
func httpProbe(c *httpConn, m *mcp) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		c.mu.Lock()
		c.version = ModernVersion
		c.mu.Unlock()
		pctx, cancel := context.WithTimeout(ctx, probeTimeout)
		defer cancel()
		m.era, m.version = EraModern, ModernVersion
		_, err := c.call(pctx, "tools/list", m.params(nil))
		if err == nil {
			return nil
		}
		m.era, m.version = "", ""
		var se *httpStatusError
		if errors.As(err, &se) {
			var body message
			if json.Unmarshal(se.body, &body) == nil && body.Error != nil && body.Error.modern() {
				err = body.Error
			}
		}
		var re *rpcError
		if !errors.As(err, &re) || !re.modern() {
			c.mu.Lock()
			c.version = ""
			c.mu.Unlock()
		}
		return err
	}
}

// httpSession is an MCP session with a loopback HTTP server.
type httpSession struct {
	m *mcp
	c *httpConn
}

// dialHTTP opens a session with the loopback HTTP server at endpoint.
func dialHTTP(ctx context.Context, endpoint string) (*httpSession, error) {
	c := &httpConn{endpoint: endpoint, client: &http.Client{Transport: &http.Transport{Proxy: nil}}}
	m := &mcp{t: c}
	if err := m.negotiate(ctx, httpProbe(c, m)); err != nil {
		return nil, err
	}
	return &httpSession{m: m, c: c}, nil
}

func (s *httpSession) Tools(ctx context.Context) ([]desktoplink.Tool, error) { return s.m.tools(ctx) }
func (s *httpSession) Call(ctx context.Context, name string, args json.RawMessage) (desktoplink.Answer, error) {
	return s.m.callTool(ctx, name, args)
}
func (s *httpSession) Era() string { return s.m.era }
func (s *httpSession) Close() error {
	s.c.mu.Lock()
	session := s.c.session
	s.c.mu.Unlock()
	if session == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.c.endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Mcp-Session-Id", session)
	resp, err := s.c.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	return nil
}
