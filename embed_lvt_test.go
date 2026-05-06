package tinkerdown

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEmbedLvt_UpstreamAutoRoute(t *testing.T) {
	dir := t.TempDir()
	mdPath := dir + "/index.md"
	content := "```embed-lvt path=\"/apps/counter/\" upstream=\"http://localhost:9090\"\n```\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := ParseFile(mdPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(page.EmbedRoutes) != 1 {
		t.Fatalf("got %d embed routes, want 1", len(page.EmbedRoutes))
	}
	got := page.EmbedRoutes[0]
	if got.Path != "/apps/counter/" {
		t.Errorf("Path = %q, want /apps/counter/", got.Path)
	}
	if got.Upstream != "http://localhost:9090" {
		t.Errorf("Upstream = %q, want http://localhost:9090", got.Upstream)
	}
}

func TestEmbedLvt_NoUpstream_NoRoute(t *testing.T) {
	dir := t.TempDir()
	mdPath := dir + "/index.md"
	content := "```embed-lvt path=\"/apps/counter/\"\n```\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := ParseFile(mdPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(page.EmbedRoutes) != 0 {
		t.Errorf("expected no auto-routes without upstream=, got %v", page.EmbedRoutes)
	}
}

func TestEmbedLvt_UpstreamFetchSkipsSelfProxy(t *testing.T) {
	// Direct upstream — tinkerdown should fetch from here, not from
	// the docs request's host.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<div data-lvt-id="x"><p>direct</p></div>`))
	}))
	defer upstream.Close()

	placeholder := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-id="b1" data-block-type="embed-lvt" data-embed-path="/anything/" data-embed-upstream="` + upstream.URL + `"></div>`
	// Docs request points somewhere that has no /anything/ proxy
	// configured — only `upstream` should determine the fetch target.
	req := httptest.NewRequest("GET", "http://docs-host.invalid/", nil)
	got := ProcessEmbedLvt(placeholder, req)

	if !strings.Contains(got, `<p>direct</p>`) {
		t.Errorf("expected upstream content fetched directly; got:\n%s", got)
	}
}

func TestParseEmbedLvtBlock_RecognizedAsBlockType(t *testing.T) {
	content := "```embed-lvt server=\"https://app.example.com\" path=\"/foo\"\n```\n"

	_, blocks, _, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Type != "embed-lvt" {
		t.Errorf("Type = %q, want embed-lvt", blocks[0].Type)
	}
	if blocks[0].Metadata["server"] != "https://app.example.com" {
		t.Errorf("server metadata = %q", blocks[0].Metadata["server"])
	}
	if blocks[0].Metadata["path"] != "/foo" {
		t.Errorf("path metadata = %q", blocks[0].Metadata["path"])
	}
}

func TestEmbedLvt_PlaceholderEmittedInHTML(t *testing.T) {
	content := "```embed-lvt server=\"https://app.example.com\" path=\"/\"\n```\n"

	_, _, html, err := ParseMarkdown([]byte(content))
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}
	if !strings.Contains(html, `class="tinkerdown-embed-lvt"`) {
		t.Errorf("expected embed placeholder; got:\n%s", html)
	}
	if !strings.Contains(html, `data-block-type="embed-lvt"`) {
		t.Errorf("expected data-block-type; got:\n%s", html)
	}
	if !strings.Contains(html, `data-embed-server="https://app.example.com"`) {
		t.Errorf("expected data-embed-server attribute; got:\n%s", html)
	}
}

func TestParseEmbedAttrs(t *testing.T) {
	in := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-id="embed-lvt-0" data-block-type="embed-lvt" data-embed-server="https://x.example.com" data-embed-path="/foo" data-embed-session="tour" data-embed-timeout="500ms" style="min-height:120px"></div>`
	got := parseEmbedAttrs(in)
	if got.blockID != "embed-lvt-0" {
		t.Errorf("blockID=%q", got.blockID)
	}
	if got.server != "https://x.example.com" {
		t.Errorf("server=%q", got.server)
	}
	if got.path != "/foo" {
		t.Errorf("path=%q", got.path)
	}
	if got.session != "tour" {
		t.Errorf("session=%q", got.session)
	}
	if got.timeout != 500*time.Millisecond {
		t.Errorf("timeout=%v", got.timeout)
	}
	if got.style != "min-height:120px" {
		t.Errorf("style=%q", got.style)
	}
}

func TestExtractLvtWrapper(t *testing.T) {
	body := `<!doctype html><html><head><title>x</title></head><body>` +
		`<div data-lvt-id="abc123"><h1>Hello {{.Name}}</h1></div>` +
		`<script>console.log('not extracted')</script></body></html>`
	got := extractLvtWrapper(body)
	// data-lvt-id is renamed to data-lvt-id-pending server-side to keep
	// LiveTemplateClient.autoInit out of the way; EmbedLvtBlock renames
	// it back client-side before invoking connect().
	if !strings.Contains(got, `data-lvt-id-pending="abc123"`) {
		t.Errorf("expected data-lvt-id renamed to data-lvt-id-pending: %q", got)
	}
	if strings.Contains(got, ` data-lvt-id="abc123"`) {
		t.Errorf("data-lvt-id should be renamed before reaching the page: %q", got)
	}
	if !strings.Contains(got, `<h1>Hello {{.Name}}</h1>`) {
		t.Errorf("missing inner content: %q", got)
	}
	if strings.Contains(got, "console.log") {
		t.Errorf("script tag leaked into wrapper: %q", got)
	}
}

func TestExtractLvtWrapper_NoneFound(t *testing.T) {
	body := `<!doctype html><html><body><p>nothing here</p></body></html>`
	if got := extractLvtWrapper(body); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestProcessEmbedLvt_HappyPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body><div data-lvt-id="xyz"><p>upstream content</p></div></body></html>`))
	}))
	defer upstream.Close()

	placeholder := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-id="b1" data-block-type="embed-lvt" data-embed-server="` + upstream.URL + `" data-embed-path="/"></div>`
	page := `<h1>Page</h1>` + placeholder + `<p>after</p>`

	req := httptest.NewRequest("GET", "/", nil)
	got := ProcessEmbedLvt(page, req)

	if !strings.Contains(got, `<p>upstream content</p>`) {
		t.Errorf("expected upstream content inlined; got:\n%s", got)
	}
	if !strings.Contains(got, `data-lvt-id-pending="xyz"`) {
		t.Errorf("expected upstream wrapper preserved (with renamed id): got:\n%s", got)
	}
	if !strings.Contains(got, `class="tinkerdown-embed-lvt"`) {
		t.Errorf("expected outer embed container; got:\n%s", got)
	}
	if strings.Contains(got, `data-tinkerdown-block`) == false {
		t.Errorf("client discovery attribute missing")
	}
}

func TestProcessEmbedLvt_UpstreamUnavailable(t *testing.T) {
	// Upstream that closes connection immediately
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	placeholder := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-id="b1" data-block-type="embed-lvt" data-embed-server="` + upstream.URL + `" data-embed-path="/"></div>`
	req := httptest.NewRequest("GET", "/", nil)
	got := ProcessEmbedLvt(placeholder, req)

	if !strings.Contains(got, "live demo unavailable") {
		t.Errorf("expected unavailable badge; got:\n%s", got)
	}
}

func TestProcessEmbedLvt_NoWrapperInResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><html><body><p>no wrapper</p></body></html>`))
	}))
	defer upstream.Close()

	placeholder := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-id="b1" data-block-type="embed-lvt" data-embed-server="` + upstream.URL + `" data-embed-path="/"></div>`
	req := httptest.NewRequest("GET", "/", nil)
	got := ProcessEmbedLvt(placeholder, req)

	if !strings.Contains(got, "live demo unavailable") {
		t.Errorf("expected unavailable badge when upstream lacks data-lvt-id; got:\n%s", got)
	}
}

func TestProcessEmbedLvt_ForwardsCookies(t *testing.T) {
	var receivedCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte(`<div data-lvt-id="x"><p>ok</p></div>`))
	}))
	defer upstream.Close()

	placeholder := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-id="b1" data-block-type="embed-lvt" data-embed-server="` + upstream.URL + `" data-embed-path="/"></div>`
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", "session=opaque")
	_ = ProcessEmbedLvt(placeholder, req)

	if receivedCookie != "session=opaque" {
		t.Errorf("upstream got cookie %q, want %q", receivedCookie, "session=opaque")
	}
}

func TestProcessEmbedLvt_SameOriginDefaults(t *testing.T) {
	// When server attribute is empty, the request's host is used.
	var receivedURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		_, _ = w.Write([]byte(`<div data-lvt-id="x"></div>`))
	}))
	defer upstream.Close()

	// Build a placeholder with no server but a relative path.
	placeholder := `<div class="tinkerdown-embed-lvt" data-tinkerdown-block data-block-type="embed-lvt" data-embed-path="/sub"></div>`
	// The request points at the test upstream so the same-origin
	// resolution targets it.
	req := httptest.NewRequest("GET", upstream.URL+"/docs", nil)
	req.Host = strings.TrimPrefix(upstream.URL, "http://")
	_ = ProcessEmbedLvt(placeholder, req)

	if !strings.HasSuffix(receivedURL, "/sub") {
		t.Errorf("expected upstream path /sub, got %q", receivedURL)
	}
}

func TestEmbedLvt_RejectsNonEmptyBody(t *testing.T) {
	dir := t.TempDir()
	mdPath := dir + "/test.md"
	content := "```embed-lvt server=\"https://app.example.com\"\n<p>oops</p>\n```\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := ParseFile(mdPath)
	if page != nil {
		t.Errorf("expected error, got page")
	}
	if err == nil {
		t.Fatal("expected error for non-empty embed-lvt body")
	}
	if !strings.Contains(err.Error(), "pointer-only") {
		t.Errorf("expected 'pointer-only' in error: %v", err)
	}
}
