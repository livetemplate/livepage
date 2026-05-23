package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsTransientChromedpError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// Nil and arbitrary non-transient errors should not retry.
		{"nil error", nil, false},
		{"plain error", errors.New("boom"), false},
		{"chrome failed to launch", errors.New("chrome failed to start: exec: \"chrome\": not found"), false},
		{"file write error", errors.New("write /tmp/foo: no space left on device"), false},

		// The actual flake we observed in docs CI: chromedp's internal
		// "websocket url timeout reached" error surfaces verbatim in the
		// returned error's Error() string. Match on the substring so any
		// chromedp wrapping (e.g. "chrome: websocket url timeout reached")
		// still triggers a retry.
		{"chromedp websocket timeout", errors.New("websocket url timeout reached"), true},
		{"wrapped websocket error", fmt.Errorf("navigation failed: %w", errors.New("websocket dial failed")), true},

		// context.DeadlineExceeded is what fires when our per-attempt
		// 30s timeout elapses during Navigate or Evaluate. Detected via
		// errors.Is so wrapped instances still match.
		{"raw deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped deadline exceeded", fmt.Errorf("navigate: %w", context.DeadlineExceeded), true},

		// "timeout" substring catches chromedp's other timeout error
		// shapes that don't go through context.DeadlineExceeded (e.g.
		// internal cdproto deadlines).
		{"bare timeout string", errors.New("operation timeout: response not received"), true},

		// Case-insensitive substring match: "Timeout" with a capital T
		// should still be classified transient.
		{"capitalised Timeout", errors.New("Connection Timeout from Chrome"), true},

		// Negative case: a syntactic "websocket" reference inside a
		// non-timeout error (e.g. a configuration error) still matches
		// — this is intentional. We err on the side of retrying anything
		// websocket-shaped because the actual production flake comes
		// from the websocket layer and a wasted retry is cheap (~5s).
		{"websocket config error", errors.New("websocket origin not allowed"), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isTransientChromedpError(c.err)
			if got != c.want {
				t.Errorf("isTransientChromedpError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestValidateOneMermaidDiagramWithRetry_RespectsExpiredParent(t *testing.T) {
	// Pre-expired parent context — validateOneMermaidDiagramWithRetry
	// must bail out before opening any chromedp context (the function
	// would otherwise spend up to maxAttempts × attemptTimeout = 90s
	// burning through retries that have no chance of succeeding).
	//
	// Uses a real expired context.Context instead of mocking chromedp:
	// the parentCtx.Err() check happens BEFORE any chromedp call, so
	// no real Chrome is required for this test to exercise the path.
	parent, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	cancel() // expire immediately
	// Give the runtime a moment to actually mark the context as
	// expired — race-y on very fast machines without this.
	time.Sleep(time.Millisecond)

	hasError, err := validateOneMermaidDiagramWithRetry(parent, "/tmp/nonexistent-mermaid.html")
	if err == nil {
		t.Fatalf("expected error from expired parent context, got nil (hasError=%v)", hasError)
	}
	if hasError {
		t.Errorf("hasError should be false when parent is expired, got true")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected wrapped context.DeadlineExceeded or context.Canceled, got: %v", err)
	}
	// Sanity check the error message mentions the per-file budget so
	// operators reading CI logs know which timeout fired.
	if msg := err.Error(); !strings.Contains(msg, "per-file deadline") {
		t.Errorf("error should mention 'per-file deadline'; got: %v", err)
	}
}
