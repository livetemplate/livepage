//go:build !ci

package tinkerdown_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/websocket"
)

// TestProxyRoute_WebSocketUpgradeE2E is the real-browser regression for #257.
//
// It stands up a strict same-origin WebSocket upstream, fronts it with a
// tinkerdown `routes: type: proxy` route, drives a real browser to the
// proxied page, and asserts via CDP network events that the WS handshake
// returns 101 (not 403) and that a frame actually flows. The acceptance
// signal is deliberately the handshake status + frame delivery — NOT
// liveTemplateClient.isReady(), which returns true under the silent HTTP
// fallback that masked this bug in production.
//
// Per project convention this captures all four diagnostic channels:
// (1) browser console logs, (2) server logs (both the upstream's view of
// the forwarded request and tinkerdown's own log output), (3) WebSocket
// handshake status + frames via CDP, (4) rendered HTML.
func TestProxyRoute_WebSocketUpgradeE2E(t *testing.T) {
	const wsMarker = "WS_PUSH_MARKER_257"

	// --- Capture tinkerdown's package-level log output (server logs, hop 1).
	// These E2E tests run serially (single Docker Chrome container, no
	// t.Parallel), so a scoped global redirect is safe; restored on cleanup.
	var tdLog syncBuffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&tdLog)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	// --- Upstream: strict same-origin WS check (gorilla default / livetemplate
	// prod default). Serves an HTML page that opens a WS back to the same host,
	// and on the WS path upgrades + pushes one frame. Logs every request so the
	// upstream's view of the forwarded Origin/Upgrade headers is visible (server
	// logs, hop 2 — the decisive diagnostic for this bug).
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://"+r.Host
	}}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("[upstream] %s %s Host=%q Origin=%q Upgrade=%q", r.Method, r.URL.Path, r.Host, r.Header.Get("Origin"), r.Header.Get("Upgrade"))
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Logf("[upstream] WS upgrade rejected: %v", err)
				return // upgrader already wrote the 403
			}
			defer conn.Close()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(wsMarker)); err != nil {
				t.Logf("[upstream] WS write: %v", err)
				return
			}
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, _, _ = conn.ReadMessage() // drain the client's "hi" / wait for close
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><html><body>
<div id="status">connecting</div>
<script>
  var proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
  var ws = new WebSocket(proto + location.host + '/proxy/ws');
  ws.onopen = function () { console.log('ws open'); ws.send('hi'); };
  ws.onmessage = function (e) { document.getElementById('status').textContent = 'WS_FRAME:' + e.data; };
  ws.onerror = function () { document.getElementById('status').textContent = 'WS_ERROR'; console.log('ws error'); };
  ws.onclose = function (e) { console.log('ws close code=' + e.code); };
</script>
</body></html>`)
	}))
	t.Cleanup(upstream.Close)

	// --- tinkerdown fixture: a proxy route fronting the upstream.
	tdDir := authorProxyFixture(t, upstream.URL)
	_, tdURL := startTinkerdown(t, tdDir)

	// --- Drive a real browser, capturing all four channels.
	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	t.Cleanup(cleanup)
	ctx := chromeCtx.Context

	var (
		mu              sync.Mutex
		consoleLogs     []string
		handshakeStatus []int64
		framesReceived  []string
		frameErrors     []string
	)
	chromedp.ListenTarget(ctx, func(ev any) {
		mu.Lock()
		defer mu.Unlock()
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			for _, arg := range e.Args {
				consoleLogs = append(consoleLogs, string(arg.Value))
			}
		case *network.EventWebSocketHandshakeResponseReceived:
			if e.Response != nil {
				handshakeStatus = append(handshakeStatus, e.Response.Status)
			}
		case *network.EventWebSocketFrameReceived:
			if e.Response != nil {
				framesReceived = append(framesReceived, e.Response.PayloadData)
			}
		case *network.EventWebSocketFrameError:
			frameErrors = append(frameErrors, e.ErrorMessage)
		}
	})

	url := ConvertURLForDockerChrome(tdURL)
	t.Logf("Test server URL: %s (Docker: %s)", tdURL, url)

	var status, htmlContent string
	runErr := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(url+"/proxy/page"),
		// Wait for the WS frame to land and update the DOM.
		chromedp.WaitVisible(`#status`, chromedp.ByID),
		waitForStatus(`#status`, "WS_FRAME:", 5*time.Second),
		chromedp.Text(`#status`, &status, chromedp.ByID),
		chromedp.OuterHTML("html", &htmlContent),
	)

	// Hold the lock across the diagnostic dump and the assertions below —
	// the CDP listener goroutine may still append events (e.g. ws close).
	mu.Lock()
	defer mu.Unlock()

	// Dump all four diagnostic channels unconditionally — cheap, and
	// invaluable on any failure below.
	t.Logf("[console] %v", consoleLogs)
	t.Logf("[handshake statuses] %v", handshakeStatus)
	t.Logf("[frames] %v", framesReceived)
	t.Logf("[frame errors] %v", frameErrors)
	t.Logf("[tinkerdown log]\n%s", tdLog.String())
	t.Logf("[html] %s", firstChars(htmlContent, 2000))
	if runErr != nil {
		t.Fatalf("chromedp.Run: %v", runErr)
	}

	// (1) No 403, and a 101 Switching Protocols was observed.
	saw101 := false
	for _, s := range handshakeStatus {
		if s == 403 {
			t.Errorf("WS handshake returned 403 — proxy did not rewrite Origin (the #257 bug)")
		}
		if s == 101 {
			saw101 = true
		}
	}
	if !saw101 {
		t.Errorf("expected a 101 Switching Protocols handshake, got statuses %v", handshakeStatus)
	}

	// (2) A WS frame carrying the upstream marker actually flowed.
	frameSeen := false
	for _, f := range framesReceived {
		if strings.Contains(f, wsMarker) {
			frameSeen = true
		}
	}
	if !frameSeen {
		t.Errorf("expected a WS frame containing %q, got frames %v", wsMarker, framesReceived)
	}

	// (3) Rendered HTML reflects the pushed frame (not the HTTP fallback).
	if !strings.Contains(status, wsMarker) {
		t.Errorf("#status = %q, want it to contain the pushed marker %q", status, wsMarker)
	}
}

// authorProxyFixture writes a tinkerdown site with a `routes: type: proxy`
// route at /proxy/ pointing at the given upstream, plus a home page so
// Discover succeeds. Returns the temp dir tinkerdown should serve from.
func authorProxyFixture(t *testing.T, upstreamURL string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := "title: \"Proxy WS E2E\"\n" +
		"routes:\n" +
		"  - pattern: \"/proxy/\"\n" +
		"    type: proxy\n" +
		"    upstream: \"" + upstreamURL + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "tinkerdown.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "---\ntitle: \"Home\"\n---\n\n# Home\n"
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// waitForStatus polls an element's textContent until it has the wanted
// prefix or the timeout elapses. Avoids a fixed Sleep race on the WS frame.
func waitForStatus(sel, wantPrefix string, timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			var txt string
			if err := chromedp.Text(sel, &txt, chromedp.ByID).Do(ctx); err == nil {
				if strings.HasPrefix(txt, wantPrefix) || strings.HasPrefix(txt, "WS_ERROR") {
					return nil
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		return nil // let the assertions report the actual state
	})
}

// syncBuffer is a goroutine-safe bytes buffer for capturing log output that
// the http server writes from its handler goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
