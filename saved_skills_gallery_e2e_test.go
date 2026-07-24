//go:build !ci

package tinkerdown_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// TestSavedSkillsGalleryRenders serves the dogfooded saved-skills gallery and confirms
// a captured workflow renders as a browsable entry — its name, the "what it stands up"
// description, and a reachable location path — through the real lvt-source render path
// (no custom JS). This is the M5 Phase 2 acceptance: the captured skills have a
// discoverable home.
//
// Four-channel capture (CLAUDE.md): browser console + exceptions and WebSocket frames
// via listen(); server log via captureServerLog(); rendered HTML + screenshot via
// saveScreenshot(). All dumped on any failure.
func TestSavedSkillsGalleryRenders(t *testing.T) {
	serverLog := captureServerLog(t)

	ts := serveConsole(t, "examples/gallery")
	defer ts.Close()

	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	defer cleanup()
	ctx := chromeCtx.Context

	ch := &channels{serverLog: serverLog}
	listen(ctx, ch)

	// Wait for the lvt-source container (present in SSR), then let the render path
	// populate the rows — the same shape as the pii console E2E. The data-bearing
	// <table> arrives through the live render, not necessarily the initial HTML.
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(ConvertURLForDockerChrome(ts.URL)),
		chromedp.WaitVisible(`[lvt-source="skills"]`, chromedp.ByQuery),
		chromedp.Sleep(1500*time.Millisecond),
	); err != nil {
		dumpGalleryHTML(t, ctx)
		ch.dump(t)
		t.Fatalf("navigate gallery: %v", err)
	}
	saveScreenshot(t, ctx, "saved-skills-gallery")

	var body string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.body.innerText`, &body),
	); err != nil {
		ch.dump(t)
		t.Fatalf("read gallery text: %v", err)
	}

	// The captured PII workflow renders with its name, a distinctive slice of its
	// description, and its reachable location — the browsable entry the gallery exists
	// to surface.
	var missing []string
	for _, want := range []string{
		"pii-access-approval",
		"access-approval console",    // distinctive slice of the description
		"skills/pii-access-approval", // the reachable location
	} {
		if !strings.Contains(body, want) {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		// Dump the four-channel capture once, then report each miss.
		dumpGalleryHTML(t, ctx)
		ch.dump(t)
		for _, want := range missing {
			t.Errorf("gallery is missing %q; rendered body text:\n%s", want, body)
		}
	}
}

// dumpGalleryHTML logs the rendered page HTML — the fourth capture channel — so a
// non-rendering source is diagnosable from the failure output alone.
func dumpGalleryHTML(t *testing.T, ctx context.Context) {
	t.Helper()
	var html string
	if err := chromedp.Run(ctx, chromedp.OuterHTML(`html`, &html)); err != nil {
		t.Logf("could not capture HTML: %v", err)
		return
	}
	t.Logf("---- rendered HTML ----\n%s", html)
}
