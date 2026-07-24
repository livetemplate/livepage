//go:build !ci

package tinkerdown_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/server"
)

// serveStyled writes a minimal one-page site with the given tinkerdown.yaml and
// serves it exactly as `tinkerdown serve` does (LoadFromDir + NewWithConfig), so
// the styling.tokens override runs through the real page-render path.
func serveStyled(t *testing.T, manifest string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tinkerdown.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Styled\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	srv := server.NewWithConfig(dir, cfg)
	if err := srv.Discover(); err != nil {
		t.Fatalf("discover: %v", err)
	}
	return httptest.NewServer(server.WithCompression(srv))
}

// cssVar reads a CSS custom property off the document root as the browser
// actually computes it — the cascade-resolved value, not the authored text.
// Dumps the four-channel capture before failing so a broken read is diagnosable.
func cssVar(t *testing.T, ctx context.Context, ch *channels, name string) string {
	t.Helper()
	var v string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(
			`getComputedStyle(document.documentElement).getPropertyValue('`+name+`').trim()`,
			&v,
		),
	); err != nil {
		ch.dump(t)
		t.Fatalf("read %s: %v", name, err)
	}
	return v
}

// TestStylingTokensComputedStyle proves the styling.tokens design-token override
// reaches the browser's computed style: a distinctive accent (#ff00ff) config
// produces that computed --accent, while an unconfigured site keeps the default
// accent (#007bff, from DefaultConfig.PrimaryColor). This is the M4 Phase 1
// acceptance bar — token-in-config to pixels, through the real cascade, not just
// emitted <style> text. That the token wins over the default primary_color also
// verifies emission order: styling.go emits primary_color first, tokens second,
// so an explicit tokens.accent takes precedence.
//
// Four-channel capture (CLAUDE.md): browser console + exceptions and WebSocket
// frames via listen(); server log via captureServerLog(); rendered HTML/screenshot
// via saveScreenshot(). All dumped on any failure.
func TestStylingTokensComputedStyle(t *testing.T) {
	serverLog := captureServerLog(t)

	const styled = "title: Styled\n" +
		"type: site\n" +
		"styling:\n" +
		"  theme: light\n" +
		"  tokens:\n" +
		"    accent: \"#ff00ff\"\n" +
		"    card_bg: \"#00ff00\"\n"
	const plain = "title: Plain\n" +
		"type: site\n" +
		"styling:\n" +
		"  theme: light\n"

	styledTS := serveStyled(t, styled)
	defer styledTS.Close()
	plainTS := serveStyled(t, plain)
	defer plainTS.Close()

	chromeCtx, cleanup := SetupDockerChrome(t, 60*time.Second)
	defer cleanup()
	ctx := chromeCtx.Context

	ch := &channels{serverLog: serverLog}
	listen(ctx, ch)

	// Configured site: the override wins the cascade, so computed --accent and
	// --card-bg are exactly the token values.
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(ConvertURLForDockerChrome(styledTS.URL)),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	); err != nil {
		ch.dump(t)
		t.Fatalf("navigate styled: %v", err)
	}
	saveScreenshot(t, ctx, "styling-tokens-styled")
	if got := cssVar(t, ctx, ch, "--accent"); !strings.EqualFold(got, "#ff00ff") {
		ch.dump(t)
		t.Errorf("styled --accent: got %q, want #ff00ff (token override did not reach computed style)", got)
	}
	if got := cssVar(t, ctx, ch, "--card-bg"); !strings.EqualFold(got, "#00ff00") {
		ch.dump(t)
		t.Errorf("styled --card-bg: got %q, want #00ff00", got)
	}

	// Site with no tokens: the token override machinery emits nothing, so the
	// accent falls back to DefaultConfig.PrimaryColor — proving tokens don't leak
	// across sites and the default stands unchanged when unconfigured.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(ConvertURLForDockerChrome(plainTS.URL)),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	); err != nil {
		ch.dump(t)
		t.Fatalf("navigate plain: %v", err)
	}
	if got := cssVar(t, ctx, ch, "--accent"); !strings.EqualFold(got, "#007bff") {
		ch.dump(t)
		t.Errorf("plain --accent: got %q, want #007bff (DefaultConfig.PrimaryColor)", got)
	}
}
