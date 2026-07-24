package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// TestWebhookHandler_ApprovedSurfaceGate proves the M3 runtime gate on the
// webhook path (which reaches RunSQLAction directly, bypassing WS dispatch): an
// external trigger for an action outside the approved set is rejected 403 before
// it executes, and approving the action lets it through.
func TestWebhookHandler_ApprovedSurfaceGate(t *testing.T) {
	newCfg := func(approved []string) *config.Config {
		return &config.Config{
			Actions: map[string]*config.Action{
				"notify-slack": {Kind: "http", URL: "https://hooks.slack.com/test", Method: "POST"},
			},
			Webhooks:   map[string]*config.Webhook{"deploy": {Action: "notify-slack"}},
			Generation: &config.GenerationConfig{Actions: approved},
		}
	}
	post := func(cfg *config.Config, mock *mockActionHandler) *httptest.ResponseRecorder {
		handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)
		req := httptest.NewRequest("POST", "/webhook/deploy", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	// notify-slack NOT approved → 403, action never runs.
	mock := &mockActionHandler{}
	if w := post(newCfg([]string{"some-other-action"}), mock); w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for an unapproved action, got %d: %s", w.Code, w.Body.String())
	}
	if len(mock.calls) != 0 {
		t.Errorf("an unapproved action must not execute; got %d calls", len(mock.calls))
	}

	// Approve it → the webhook passes the gate.
	mock2 := &mockActionHandler{}
	if w := post(newCfg([]string{"notify-slack"}), mock2); w.Code != http.StatusOK {
		t.Errorf("an approved action should pass the gate, got %d: %s", w.Code, w.Body.String())
	}
}
