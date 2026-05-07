//go:build !ci

package tinkerdown_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
)

// TestEmbedLvt_ServerSideFetchAndInline brings up a fake upstream
// LiveTemplate-style HTTP server, points a tinkerdown page at it via
// an `embed-lvt` block, and verifies that the docs HTML served to the
// browser already contains the upstream wrapper inlined — the
// server-side fetch pre-paint is the design's whole point.
func TestEmbedLvt_ServerSideFetchAndInline(t *testing.T) {
	const upstreamWrapperID = "upstream-wrap-xyz"
	const upstreamMarker = "UPSTREAM_CONTENT_MARKER_42"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><html><body><div data-lvt-id=%q><h2>%s</h2></div></body></html>`,
			upstreamWrapperID, upstreamMarker)
	}))
	t.Cleanup(upstream.Close)

	tdDir := authorEmbedFixture(t, upstream.URL)
	tdServer, tdURL := startTinkerdown(t, tdDir)
	_ = tdServer

	htmlContent, consoleLogs := navigateAndCapture(t, tdURL, "/")

	if !strings.Contains(htmlContent, upstreamMarker) {
		t.Logf("HTML (first 4000 chars): %s", firstChars(htmlContent, 4000))
		t.Logf("Console: %v", consoleLogs)
		t.Fatal("expected upstream content inlined in docs HTML")
	}
	// EmbedLvtBlock renames data-lvt-id-pending back to data-lvt-id
	// client-side before connecting, suffixing the original upstream id
	// with the block's unique id so multiple embeds of the same upstream
	// don't collide on LiveTemplate's per-id event delegator.
	if !strings.Contains(htmlContent, fmt.Sprintf(`data-lvt-id="%s-`, upstreamWrapperID)) {
		t.Fatal("expected upstream wrapper id (suffixed with block id) restored client-side")
	}
	if !strings.Contains(htmlContent, `data-block-type="embed-lvt"`) {
		t.Fatal("expected client-discovery container")
	}
}

// TestEmbedLvt_UpstreamUnavailable verifies the unavailable badge
// renders when the upstream cannot be reached, and the rest of the
// page still renders normally.
func TestEmbedLvt_UpstreamUnavailable(t *testing.T) {
	// A server that immediately closes its listener — embed fetch will
	// fail with a connection error.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	upstreamURL := upstream.URL
	upstream.Close()

	tdDir := authorEmbedFixture(t, upstreamURL)
	_, tdURL := startTinkerdown(t, tdDir)

	htmlContent, _ := navigateAndCapture(t, tdURL, "/")

	if !strings.Contains(htmlContent, "live demo unavailable") {
		t.Logf("HTML (first 3000 chars): %s", firstChars(htmlContent, 3000))
		t.Fatal("expected unavailable badge for unreachable upstream")
	}
	// Page chrome should still render
	if !strings.Contains(htmlContent, "Embedded counter") {
		t.Fatal("expected page title to render despite embed failure")
	}
}

// TestEmbedLvt_PerVisitorFetch verifies each docs request triggers a
// fresh GET to the upstream (so per-visitor session cookies and auth
// flow through). One visitor → one upstream request.
func TestEmbedLvt_PerVisitorFetch(t *testing.T) {
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`<div data-lvt-id="x"><p>visit</p></div>`))
	}))
	t.Cleanup(upstream.Close)

	tdDir := authorEmbedFixture(t, upstream.URL)
	_, tdURL := startTinkerdown(t, tdDir)

	// Two HTTP GETs to the docs page → two upstream fetches.
	for i := 0; i < 2; i++ {
		resp, err := http.Get(tdURL + "/")
		if err != nil {
			t.Fatalf("docs GET %d: %v", i, err)
		}
		_ = resp.Body.Close()
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("upstream hit count = %d, want 2", got)
	}
}

// authorEmbedFixture writes a tinkerdown markdown page that embeds the
// given upstream URL. Returns the temp dir tinkerdown should serve from.
func authorEmbedFixture(t *testing.T, upstreamURL string) string {
	t.Helper()
	dir := t.TempDir()
	body := "---\n" +
		"title: \"Embedded counter\"\n" +
		"---\n\n" +
		"# Embedded counter\n\n" +
		"```embed-lvt server=\"" + upstreamURL + "\"\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// startTinkerdown boots a tinkerdown server from the given content
// directory and returns it plus the test server URL.
func startTinkerdown(t *testing.T, dir string) (*httptest.Server, string) {
	t.Helper()
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	srv := server.NewWithConfig(dir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	handler := server.WithCompression(srv)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, ts.URL
}

// navigateAndCapture drives chromedp to the given path and returns
// the rendered HTML plus any console logs. Brings up Docker Chrome
// per project convention.
func navigateAndCapture(t *testing.T, baseURL, path string) (string, []string) {
	t.Helper()
	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	t.Cleanup(cleanup)

	ctx := chromeCtx.Context
	var consoleLogs []string
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if ev, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			for _, arg := range ev.Args {
				consoleLogs = append(consoleLogs, fmt.Sprintf("[Console] %s", arg.Value))
			}
		}
	})

	url := ConvertURLForDockerChrome(baseURL)
	t.Logf("Test server URL: %s (Docker: %s) path=%s", baseURL, url, path)

	var htmlContent string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url+path),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &htmlContent),
	); err != nil {
		t.Fatalf("chromedp.Run: %v", err)
	}
	return htmlContent, consoleLogs
}
