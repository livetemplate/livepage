package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/livetemplate/tinkerdown/internal/config"
)

func TestNewProxyRoute_RejectsUnsupportedType(t *testing.T) {
	_, err := newProxyRoute(config.RouteEntry{Pattern: "/x", Type: "markdown", Upstream: "http://up"})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestNewProxyRoute_RejectsBadPattern(t *testing.T) {
	cases := []string{"", "x", "no-leading-slash"}
	for _, p := range cases {
		_, err := newProxyRoute(config.RouteEntry{Pattern: p, Type: "proxy", Upstream: "http://up"})
		if err == nil {
			t.Errorf("expected error for pattern %q", p)
		}
	}
}

func TestNewProxyRoute_RejectsBadUpstream(t *testing.T) {
	cases := []string{"", "://bad", "ftp://example.com"}
	for _, u := range cases {
		_, err := newProxyRoute(config.RouteEntry{Pattern: "/x", Type: "proxy", Upstream: u})
		if err == nil {
			t.Errorf("expected error for upstream %q", u)
		}
	}
}

func TestProxyRouteMatches(t *testing.T) {
	// Prefix route
	prefix, err := newProxyRoute(config.RouteEntry{Pattern: "/patterns/", Type: "proxy", Upstream: "http://up"})
	if err != nil {
		t.Fatal(err)
	}
	prefixCases := []struct {
		path string
		want bool
	}{
		{"/patterns", true},     // bare match (no trailing slash)
		{"/patterns/", true},    // exact pattern
		{"/patterns/foo", true}, // sub-path
		{"/patternsfoo", false}, // not a prefix-segment
		{"/other", false},       // unrelated
		{"/", false},            // root
	}
	for _, c := range prefixCases {
		if got := prefix.matches(c.path); got != c.want {
			t.Errorf("prefix /patterns/ matches %q = %v, want %v", c.path, got, c.want)
		}
	}

	// Exact route
	exact, err := newProxyRoute(config.RouteEntry{Pattern: "/exact", Type: "proxy", Upstream: "http://up"})
	if err != nil {
		t.Fatal(err)
	}
	if !exact.matches("/exact") {
		t.Error("exact /exact should match /exact")
	}
	if exact.matches("/exact/sub") {
		t.Error("exact /exact should NOT match /exact/sub")
	}
	if exact.matches("/exactly") {
		t.Error("exact /exact should NOT match /exactly")
	}
}

func TestBuildProxyRoutes_SkipsInvalidEntries(t *testing.T) {
	entries := []config.RouteEntry{
		{Pattern: "/good", Type: "proxy", Upstream: "http://example.com"},
		{Pattern: "bad", Type: "proxy", Upstream: "http://example.com"},
		{Pattern: "/also-good/", Type: "proxy", Upstream: "https://example.com"},
	}
	routes, errs := buildProxyRoutes(entries)
	if len(routes) != 2 {
		t.Errorf("expected 2 valid routes, got %d", len(routes))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

// Integration: real upstream + real reverse proxy + path forwarded unchanged.
func TestProxyRoute_HTTPIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Saw", r.URL.Path)
		_, _ = io.WriteString(w, "hello "+r.URL.Path)
	}))
	defer upstream.Close()

	pr, err := newProxyRoute(config.RouteEntry{Pattern: "/patterns/", Type: "proxy", Upstream: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/patterns/", pr.handler)
	front := httptest.NewServer(mux)
	defer front.Close()

	resp, err := http.Get(front.URL + "/patterns/click-to-edit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello /patterns/click-to-edit") {
		t.Errorf("body: %q", body)
	}
	if got := resp.Header.Get("X-Upstream-Saw"); got != "/patterns/click-to-edit" {
		t.Errorf("upstream saw %q, want full path passthrough", got)
	}
}

// Integration: WebSocket upgrade survives the reverse proxy. Critical for
// livetemplate apps which use WS for real-time updates.
func TestProxyRoute_WebSocketIntegration(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upstream upgrade: %v", err)
			return
		}
		defer conn.Close()
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(mt, append([]byte("echo:"), msg...))
	}))
	defer upstream.Close()

	pr, err := newProxyRoute(config.RouteEntry{Pattern: "/proxy/", Type: "proxy", Upstream: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/proxy/", pr.handler)
	front := httptest.NewServer(mux)
	defer front.Close()

	wsURL := "ws" + strings.TrimPrefix(front.URL, "http") + "/proxy/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial via proxy: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != "echo:hi" {
		t.Errorf("ws echo: got %q", msg)
	}
}

// Regression for #257: a browser dials the proxy with Origin = the proxy's
// own host. The proxy rewrites Host to the upstream, so an upstream using a
// strict same-origin check (gorilla's default, mirrored here) would 403 the
// handshake unless the proxy also rewrites Origin to the upstream origin.
// Without the Origin rewrite in newProxyRoute's Director this dial fails;
// with it the handshake echoes.
func TestProxyRoute_WebSocketCrossOrigin(t *testing.T) {
	// Strict same-origin upstream: Origin must equal scheme://Host. This is
	// equivalent to gorilla's default checkOrigin and to livetemplate's
	// createSecureOriginChecker (AllowedOrigins empty, prod mode).
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://"+r.Host
	}}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return // upgrader already wrote 403
		}
		defer conn.Close()
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(mt, append([]byte("echo:"), msg...))
	}))
	defer upstream.Close()

	pr, err := newProxyRoute(config.RouteEntry{Pattern: "/proxy/", Type: "proxy", Upstream: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/proxy/", pr.handler)
	front := httptest.NewServer(mux)
	defer front.Close()

	// Dial with an explicit cross-origin Origin (the proxy's own host) —
	// this is what a real browser sends.
	wsURL := "ws" + strings.TrimPrefix(front.URL, "http") + "/proxy/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {front.URL}})
	if err != nil {
		status := "n/a"
		if resp != nil {
			status = resp.Status
		}
		t.Fatalf("ws dial via proxy with cross-origin Origin failed (upstream status %s): %v", status, err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != "echo:hi" {
		t.Errorf("ws echo: got %q", msg)
	}
}

// Unit-level proof that the proxy rewrites Origin to the upstream origin only
// for WebSocket upgrades — guards the gating logic. Drives the handler via
// ServeHTTP against a recording upstream (which echoes the Origin it saw back
// in a response header) so the assertion is race-free and doesn't couple to
// the proxy's internal Director field.
func TestProxyRoute_RewritesOriginOnlyForWS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Saw-Origin", r.Header.Get("Origin"))
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	// httptest URLs carry no path, so upstream.URL is exactly the origin
	// (scheme://host) the Director should rewrite Origin to.
	wantUpstreamOrigin := upstream.URL

	pr, err := newProxyRoute(config.RouteEntry{Pattern: "/proxy/", Type: "proxy", Upstream: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}

	// WS upgrade: Origin rewritten to the upstream origin. (The upstream
	// returns 200 rather than 101, so the proxy relays normally and we read
	// the echoed header — we're asserting on the forwarded Origin, not on a
	// completed upgrade, which the cross-origin test already covers.)
	wsReq := httptest.NewRequest(http.MethodGet, "http://docs.example.com/proxy/ws", nil)
	wsReq.Header.Set("Upgrade", "websocket")
	wsReq.Header.Set("Connection", "keep-alive, Upgrade")
	wsReq.Header.Set("Origin", "http://docs.example.com")
	wsRec := httptest.NewRecorder()
	pr.handler.ServeHTTP(wsRec, wsReq)
	if got := wsRec.Header().Get("X-Saw-Origin"); got != wantUpstreamOrigin {
		t.Errorf("WS upgrade: upstream saw Origin %q, want rewritten to %q", got, wantUpstreamOrigin)
	}

	// Plain HTTP with an Origin: left untouched (non-WS CORS unaffected).
	httpReq := httptest.NewRequest(http.MethodPost, "http://docs.example.com/proxy/api", nil)
	httpReq.Header.Set("Origin", "http://docs.example.com")
	httpRec := httptest.NewRecorder()
	pr.handler.ServeHTTP(httpRec, httpReq)
	if got := httpRec.Header().Get("X-Saw-Origin"); got != "http://docs.example.com" {
		t.Errorf("non-WS: upstream saw Origin %q, want left unchanged", got)
	}

	// WS upgrade with no Origin header (some dialers omit it on same-host
	// connections): must not inject one — the != "" guard handles this.
	noOriginReq := httptest.NewRequest(http.MethodGet, "http://docs.example.com/proxy/ws", nil)
	noOriginReq.Header.Set("Upgrade", "websocket")
	noOriginReq.Header.Set("Connection", "Upgrade")
	noOriginRec := httptest.NewRecorder()
	pr.handler.ServeHTTP(noOriginRec, noOriginReq)
	if got := noOriginRec.Header().Get("X-Saw-Origin"); got != "" {
		t.Errorf("no-Origin WS: upstream saw Origin %q, want empty", got)
	}
}
