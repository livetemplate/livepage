//go:build !ci

package tinkerdown_test

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
)

// TestLiterateShowSource_PerBlock verifies that an `lvt` block with the
// `show-source` flag emits BOTH a syntax-highlighted source listing AND
// the live interactive widget, wrapped in the literate-demo container.
func TestLiterateShowSource_PerBlock(t *testing.T) {
	htmlContent, consoleLogs := loadLiterateFixturePage(t, "/")

	if !strings.Contains(htmlContent, `class="tinkerdown-lvt-demo`) {
		t.Logf("HTML (first 4000 chars): %s", firstChars(htmlContent, 4000))
		t.Logf("Console: %v", consoleLogs)
		t.Fatal("expected tinkerdown-lvt-demo wrapper for show-source block")
	}
	if !strings.Contains(htmlContent, `class="language-html"`) {
		// Capture context around the demo wrapper for debugging.
		idx := strings.Index(htmlContent, `tinkerdown-lvt-demo`)
		start := idx - 40
		if start < 0 {
			start = 0
		}
		end := idx + 800
		if end > len(htmlContent) {
			end = len(htmlContent)
		}
		t.Logf("Demo region: %s", htmlContent[start:end])
		t.Fatal("expected source view re-tagged with language-html for syntax highlighting")
	}
	if !strings.Contains(htmlContent, `tinkerdown-interactive-block`) {
		idx := strings.Index(htmlContent, `tinkerdown-lvt-demo`)
		end := idx + 1500
		if end > len(htmlContent) {
			end = len(htmlContent)
		}
		t.Logf("Demo region: %s", htmlContent[idx:end])
		t.Fatal("expected live tinkerdown-interactive-block alongside source view")
	}
	// Source view contains the literal template syntax
	if !strings.Contains(htmlContent, `{{range .Data}}`) {
		t.Fatal("source view should contain the raw template body")
	}
}

// TestLiterateShowSource_FrontmatterDefault verifies that
// `lvt_show_source: true` in frontmatter activates source display for a
// block that has no per-block flag.
func TestLiterateShowSource_FrontmatterDefault(t *testing.T) {
	htmlContent, _ := loadLiterateFixturePage(t, "/frontmatter")

	if !strings.Contains(htmlContent, `class="tinkerdown-lvt-demo`) {
		t.Logf("HTML (first 4000 chars): %s", firstChars(htmlContent, 4000))
		t.Fatal("frontmatter lvt_show_source: true should activate source display")
	}
}

// TestLiterateShowSource_HideOverride verifies that a per-block
// `hide-source` flag suppresses source display even when the page-level
// `lvt_show_source: true` default is set.
func TestLiterateShowSource_HideOverride(t *testing.T) {
	htmlContent, _ := loadLiterateFixturePage(t, "/hide-override")

	if strings.Contains(htmlContent, `class="tinkerdown-lvt-demo`) {
		t.Logf("HTML (first 4000 chars): %s", firstChars(htmlContent, 4000))
		t.Fatal("per-block hide-source should suppress source even when frontmatter enables it")
	}
	// The widget should still be there.
	if !strings.Contains(htmlContent, `tinkerdown-interactive-block`) {
		t.Fatal("live widget should still render under hide-source")
	}
}

// loadLiterateFixturePage stands up the literate-show-source-test
// fixture, navigates to the given path, and returns rendered HTML plus
// captured console logs. Browser-level checks (event flow, widget
// updates) belong here too — extend with chromedp.Click / .Sleep as
// needed for follow-up tests that exercise the action round-trip.
func loadLiterateFixturePage(t *testing.T, path string) (string, []string) {
	t.Helper()

	cfg, err := config.LoadFromDir("examples/literate-show-source-test")
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	srv := server.NewWithConfig("examples/literate-show-source-test", cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	handler := server.WithCompression(srv)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

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

	url := ConvertURLForDockerChrome(ts.URL)
	t.Logf("Test server URL: %s (Docker: %s) path=%s", ts.URL, url, path)

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

func firstChars(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
