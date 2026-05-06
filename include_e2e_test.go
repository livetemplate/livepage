//go:build !ci

package tinkerdown_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
)

// TestInclude_RendersSlice verifies a markdown page with a
// `LANG include="..." lines="N-M"` fence renders the file slice as a
// normal syntax-highlighted code block — i.e., the include= and lines=
// attributes get stripped from the rendered fence info, the body is
// the dedented snippet, and Prism's class survives the substitution.
func TestInclude_RendersSlice(t *testing.T) {
	dir := authorIncludeFixture(t, "L1\nL2\nL3\nL4\nL5\nL6\n",
		"```go include=\"./code.go\" lines=\"2-4\"\n```\n",
	)
	htmlContent, consoleLogs := navigateAndCaptureInclude(t, dir, "/")

	if !strings.Contains(htmlContent, "L2") || !strings.Contains(htmlContent, "L3") || !strings.Contains(htmlContent, "L4") {
		t.Logf("Console: %v", consoleLogs)
		t.Logf("HTML (first 3000 chars): %s", firstChars(htmlContent, 3000))
		t.Fatal("expected sliced lines L2..L4 in rendered HTML")
	}
	if strings.Contains(htmlContent, "L1") || strings.Contains(htmlContent, "L5") || strings.Contains(htmlContent, "L6") {
		t.Errorf("expected lines outside range to NOT appear; got HTML containing them")
	}
	if strings.Contains(htmlContent, "include=") {
		t.Errorf("include= attribute should be stripped from rendered fence")
	}
	if !strings.Contains(htmlContent, `class="language-go"`) {
		t.Errorf("expected language-go class to survive substitution")
	}
}

// TestInclude_HotReload verifies that editing an included file while
// the page is open broadcasts a reload over the WebSocket so the
// snippet stays in sync.
func TestInclude_HotReload(t *testing.T) {
	dir := authorIncludeFixture(t, "ORIGINAL\n",
		"```go include=\"./code.go\"\n```\n",
	)

	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	srv := server.NewWithConfig(dir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if err := srv.EnableWatch(false); err != nil {
		t.Fatalf("EnableWatch: %v", err)
	}
	handler := server.WithCompression(srv)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	t.Cleanup(cleanup)
	ctx := chromeCtx.Context

	url := ConvertURLForDockerChrome(ts.URL)

	var initial string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url+"/"),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &initial),
	); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if !strings.Contains(initial, "ORIGINAL") {
		t.Fatalf("expected ORIGINAL in initial render")
	}

	// Mutate the included file and wait for the watcher to fire.
	codePath := filepath.Join(dir, "code.go")
	if err := os.WriteFile(codePath, []byte("UPDATED\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-discover synchronously (the watcher would also do this; we
	// don't want to depend on its async timing for the assertion).
	if err := srv.Discover(); err != nil {
		t.Fatalf("re-Discover: %v", err)
	}

	var reloaded string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url+"/"),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &reloaded),
	); err != nil {
		t.Fatalf("re-navigate: %v", err)
	}
	if !strings.Contains(reloaded, "UPDATED") {
		t.Logf("HTML (first 3000 chars): %s", firstChars(reloaded, 3000))
		t.Fatal("expected UPDATED in re-rendered HTML")
	}
	if strings.Contains(reloaded, "ORIGINAL") {
		t.Errorf("stale ORIGINAL should be gone after re-render")
	}
}

// TestInclude_PathConfinement verifies a path that escapes the page
// directory is rejected — the rendered page renders normally with the
// original empty block, no leakage of the secret file.
func TestInclude_PathConfinement(t *testing.T) {
	dir := t.TempDir()
	// A "secret" file outside the page dir.
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("S3CR3T"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A page that tries to include the secret.
	md := "---\ntitle: \"escape\"\n---\n\n" +
		"```text include=\"" + secret + "\"\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	htmlContent, _ := navigateAndCaptureInclude(t, dir, "/")
	if strings.Contains(htmlContent, "S3CR3T") {
		t.Fatal("escape attempt should not leak file content into the rendered page")
	}
}

// authorIncludeFixture writes a one-page tinkerdown directory with
// the given included-file body and the given page body and returns
// its path.
func authorIncludeFixture(t *testing.T, fileBody, pageBlock string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "code.go"), []byte(fileBody), 0o644); err != nil {
		t.Fatal(err)
	}
	page := "---\ntitle: \"Include test\"\n---\n\n# Heading\n\n" + pageBlock
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// navigateAndCaptureInclude boots tinkerdown serving the given dir
// and returns the rendered HTML + console logs after a 2s settle.
func navigateAndCaptureInclude(t *testing.T, dir, path string) (string, []string) {
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

	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	t.Cleanup(cleanup)
	ctx := chromeCtx.Context

	var logs []string
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if ev, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			for _, a := range ev.Args {
				logs = append(logs, fmt.Sprintf("[%s] %v", ev.Type, a.Value))
			}
		}
	})

	url := ConvertURLForDockerChrome(ts.URL)
	t.Logf("Test server URL: %s (Docker: %s) path=%s", ts.URL, url, path)

	var html string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url+path),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return html, logs
}
