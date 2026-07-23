package tinkerdown_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
	_ "modernc.org/sqlite"
)

// TestPIIConsoleFrameworkLatency measures the in-process framework leg for the
// PII console — the time from "the app.md and its data exist" to "a live,
// WebSocket-connected page": parse/discover, first server-side render, and the
// WebSocket upgrade with its initial tree.
//
// The point is order of magnitude, not a flattering millisecond figure. The
// plan's "~30s" budget is a generation-reliability target: the LLM authoring the
// document is the slow part, and this test exists to show the runtime that turns
// that document into a live page is three-plus orders of magnitude faster — so
// the budget really is the LLM. The `tinkerdown serve` *command* adds Go process
// startup on top of these numbers; this measures the runtime work itself.
func TestPIIConsoleFrameworkLatency(t *testing.T) {
	dir := stageLatencyConsole(t)

	// 1. Parse + discover the served directory.
	tParse := time.Now()
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	srv := server.NewWithConfig(dir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("discover: %v", err)
	}
	parseDur := time.Since(tParse)

	ts := httptest.NewServer(server.WithCompression(srv))
	defer ts.Close()

	// 2. First server-side render.
	tSSR := time.Now()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("SSR GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	ssrDur := time.Since(tSSR)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSR returned %d, want 200", resp.StatusCode)
	}
	// The SSR renders the page shell (title, filter tabs); the live block data
	// (the {{range .Data}} rows with Approve/Deny buttons) arrives over the WS
	// tree, which the next step measures.
	if !strings.Contains(string(body), "Data-Export Access Approval") {
		t.Fatalf("first render did not contain the console page (title missing)")
	}

	// 3. WebSocket upgrade + initial tree. The server pushes the initial state on
	// connect (initializeInstances runs before the read loop), so one ReadMessage
	// after the handshake is a real "page is live" signal — no client init needed.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?page=" + url.QueryEscape("/app")
	tWS := time.Now()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, firstFrame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws first frame: %v", err)
	}
	wsDur := time.Since(tWS)

	total := parseDur + ssrDur + wsDur
	t.Logf("framework leg for the PII console:")
	t.Logf("  parse/discover : %v", parseDur.Round(time.Microsecond))
	t.Logf("  first SSR      : %v", ssrDur.Round(time.Microsecond))
	t.Logf("  ws upgrade+tree: %v  (first frame %d bytes)", wsDur.Round(time.Microsecond), len(firstFrame))
	t.Logf("  total          : %v", total.Round(time.Microsecond))
	t.Logf("order of magnitude: framework leg %v vs the ~30s generation budget — the budget is the LLM, not the runtime", total.Round(time.Millisecond))

	// Sanity guard, not a precision claim: the framework leg must stay comfortably
	// sub-second so the order-of-magnitude argument holds. If this ever regresses
	// past a second, the "near-instant runtime" premise is no longer true.
	if total > time.Second {
		t.Errorf("framework leg %v exceeds 1s — the runtime is no longer near-instant relative to the generation budget", total)
	}
}

// stageLatencyConsole copies the reference manifest + app.md into a temp dir with
// a freshly seeded database, mirroring how the console is actually served.
func stageLatencyConsole(t *testing.T) string {
	t.Helper()
	const src = "examples/pii-access-approval"
	dir := t.TempDir()
	for _, name := range []string{"tinkerdown.yaml", "app.md"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(filepath.Join(src, "seed.sql"))
	if err != nil {
		t.Fatalf("read seed.sql: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "data", "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(string(seed)); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	return dir
}
