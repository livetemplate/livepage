package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/source"
)

// mockActionHandler tracks action executions for testing
type mockActionHandler struct {
	calls []mockActionCall
	err   error
}

type mockActionCall struct {
	actionName string
	params     map[string]interface{}
}

func (m *mockActionHandler) handle(actionName string, params map[string]interface{}) error {
	m.calls = append(m.calls, mockActionCall{
		actionName: actionName,
		params:     params,
	})
	return m.err
}

func TestWebhookHandler_BasicTrigger(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"notify-slack": {
				Kind:   "http",
				URL:    "https://hooks.slack.com/test",
				Method: "POST",
			},
		},
		Webhooks: map[string]*config.Webhook{
			"deploy": {
				Action: "notify-slack",
			},
		},
	}

	mock := &mockActionHandler{}
	handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)

	// Test POST request to webhook
	body := strings.NewReader(`{"message": "deployment complete"}`)
	req := httptest.NewRequest("POST", "/webhook/deploy", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response WebhookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success=true, got error: %s", response.Error)
	}

	// Verify action was called
	if len(mock.calls) != 1 {
		t.Fatalf("Expected 1 action call, got %d", len(mock.calls))
	}
	if mock.calls[0].actionName != "notify-slack" {
		t.Errorf("Expected action 'notify-slack', got %s", mock.calls[0].actionName)
	}
	if mock.calls[0].params["message"] != "deployment complete" {
		t.Errorf("Expected message param, got %v", mock.calls[0].params)
	}
}

func TestWebhookHandler_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"test-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"test": {Action: "test-action"},
		},
	}

	handler := NewWebhookHandler(cfg, t.TempDir(), nil)

	methods := []string{"GET", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/webhook/test", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s, got %d", method, w.Code)
			}

			var response WebhookResponse
			json.Unmarshal(w.Body.Bytes(), &response)
			if response.Success {
				t.Error("Expected success=false for non-POST method")
			}
		})
	}
}

func TestWebhookHandler_WebhookNotFound(t *testing.T) {
	cfg := &config.Config{
		Webhooks: map[string]*config.Webhook{},
	}

	handler := NewWebhookHandler(cfg, t.TempDir(), nil)

	req := httptest.NewRequest("POST", "/webhook/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestWebhookHandler_SecretValidation(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"secure-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"secure": {
				Action: "secure-action",
				Secret: "my-secret-key",
			},
		},
	}

	mock := &mockActionHandler{}
	handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)

	t.Run("valid secret in header", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/secure", nil)
		req.Header.Set("X-Webhook-Secret", "my-secret-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("valid secret in query param", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/secure?secret=my-secret-key", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/secure", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}

		var response WebhookResponse
		json.Unmarshal(w.Body.Bytes(), &response)
		if !strings.Contains(response.Error, "secret") {
			t.Errorf("Expected error about secret, got: %s", response.Error)
		}
	})

	t.Run("invalid secret", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/secure", nil)
		req.Header.Set("X-Webhook-Secret", "wrong-secret")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestWebhookHandler_ActionNotFound(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{},
		Webhooks: map[string]*config.Webhook{
			"test": {Action: "nonexistent-action"},
		},
	}

	handler := NewWebhookHandler(cfg, t.TempDir(), nil)

	req := httptest.NewRequest("POST", "/webhook/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var response WebhookResponse
	json.Unmarshal(w.Body.Bytes(), &response)
	if !strings.Contains(response.Error, "action not found") {
		t.Errorf("Expected action not found error, got: %s", response.Error)
	}
}

func TestWebhookHandler_RequestBodyParsing(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"test-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"test": {Action: "test-action"},
		},
	}

	mock := &mockActionHandler{}
	handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)

	t.Run("params wrapper format", func(t *testing.T) {
		mock.calls = nil
		body := strings.NewReader(`{"params": {"key": "value"}}`)
		req := httptest.NewRequest("POST", "/webhook/test", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		if len(mock.calls) != 1 {
			t.Fatalf("Expected 1 call, got %d", len(mock.calls))
		}
		if mock.calls[0].params["key"] != "value" {
			t.Errorf("Expected key=value, got %v", mock.calls[0].params)
		}
	})

	t.Run("direct params format", func(t *testing.T) {
		mock.calls = nil
		body := strings.NewReader(`{"foo": "bar", "count": 42}`)
		req := httptest.NewRequest("POST", "/webhook/test", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if mock.calls[0].params["foo"] != "bar" {
			t.Errorf("Expected foo=bar, got %v", mock.calls[0].params)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if len(mock.calls[0].params) != 0 {
			t.Errorf("Expected empty params, got %v", mock.calls[0].params)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mock.calls = nil
		body := strings.NewReader(`{invalid json}`)
		req := httptest.NewRequest("POST", "/webhook/test", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})
}

func TestWebhookHandler_MissingWebhookName(t *testing.T) {
	cfg := &config.Config{
		Webhooks: map[string]*config.Webhook{},
	}

	handler := NewWebhookHandler(cfg, t.TempDir(), nil)

	req := httptest.NewRequest("POST", "/webhook/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestWebhookHandler_NilConfig(t *testing.T) {
	handler := NewWebhookHandler(nil, t.TempDir(), nil)

	req := httptest.NewRequest("POST", "/webhook/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for nil config, got %d", w.Code)
	}
}

func TestSanitizeParams(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "no sensitive keys",
			input:    map[string]interface{}{"name": "test", "count": 42},
			expected: map[string]interface{}{"name": "test", "count": 42},
		},
		{
			name:     "redacts password",
			input:    map[string]interface{}{"username": "test", "password": "secret123"},
			expected: map[string]interface{}{"username": "test", "password": "[REDACTED]"},
		},
		{
			name:     "redacts api_key",
			input:    map[string]interface{}{"api_key": "sk-123456"},
			expected: map[string]interface{}{"api_key": "[REDACTED]"},
		},
		{
			name:     "redacts token",
			input:    map[string]interface{}{"auth_token": "bearer-xyz"},
			expected: map[string]interface{}{"auth_token": "[REDACTED]"},
		},
		{
			name:     "case insensitive",
			input:    map[string]interface{}{"PASSWORD": "secret", "API_KEY": "key123"},
			expected: map[string]interface{}{"PASSWORD": "[REDACTED]", "API_KEY": "[REDACTED]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeParams(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("Key %q: expected %v, got %v", k, v, result[k])
				}
			}
		})
	}
}

func TestWebhookActionExecutor_ValidateParams(t *testing.T) {
	executor := newWebhookActionExecutor(nil, "")

	action := &config.Action{
		Params: map[string]config.ParamDef{
			"required_param": {Required: true, Type: "string"},
			"optional_param": {Required: false, Type: "string"},
		},
	}

	t.Run("all required params present", func(t *testing.T) {
		data := map[string]interface{}{
			"required_param": "value",
		}
		err := executor.validateParams(action, data)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("missing required param", func(t *testing.T) {
		data := map[string]interface{}{
			"optional_param": "value",
		}
		err := executor.validateParams(action, data)
		if err == nil {
			t.Error("Expected error for missing required param")
		}
		if !strings.Contains(err.Error(), "required_param") {
			t.Errorf("Expected error about required_param, got: %v", err)
		}
	})

	t.Run("empty string for required param", func(t *testing.T) {
		data := map[string]interface{}{
			"required_param": "",
		}
		err := executor.validateParams(action, data)
		if err == nil {
			t.Error("Expected error for empty required param")
		}
	})

	t.Run("nil value for required param", func(t *testing.T) {
		data := map[string]interface{}{
			"required_param": nil,
		}
		err := executor.validateParams(action, data)
		if err == nil {
			t.Error("Expected error for nil required param")
		}
	})
}

func TestWebhookActionExecutor_SubstituteParams(t *testing.T) {
	tests := []struct {
		name        string
		stmt        string
		data        map[string]interface{}
		expectedSQL string
		expectError bool
	}{
		{
			name:        "simple substitution",
			stmt:        "DELETE FROM tasks WHERE id = :id",
			data:        map[string]interface{}{"id": "123"},
			expectedSQL: "DELETE FROM tasks WHERE id = ?",
		},
		{
			name:        "multiple params",
			stmt:        "UPDATE tasks SET status = :status WHERE id = :id",
			data:        map[string]interface{}{"status": "done", "id": "456"},
			expectedSQL: "UPDATE tasks SET status = ? WHERE id = ?",
		},
		{
			name:        "preserve double colon",
			stmt:        "SELECT value::text FROM data WHERE id = :id",
			data:        map[string]interface{}{"id": "1"},
			expectedSQL: "SELECT value::text FROM data WHERE id = ?",
		},
		{
			name:        "preserve time literals",
			stmt:        "SELECT * FROM events WHERE time > '12:30:00' AND id = :id",
			data:        map[string]interface{}{"id": "1"},
			expectedSQL: "SELECT * FROM events WHERE time > '12:30:00' AND id = ?",
		},
		{
			name:        "undefined param error",
			stmt:        "SELECT * FROM t WHERE id = :unknown",
			data:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, args, err := source.SubstituteParams(tt.stmt, tt.data)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expectedSQL {
				t.Errorf("Expected SQL: %q, got: %q", tt.expectedSQL, result)
			}

			// Verify args count matches placeholder count
			placeholderCount := strings.Count(result, "?")
			if len(args) != placeholderCount {
				t.Errorf("Expected %d args, got %d", placeholderCount, len(args))
			}
		})
	}
}

func TestWebhookActionExecutor_ExpandTemplate(t *testing.T) {
	executor := newWebhookActionExecutor(nil, "")

	tests := []struct {
		name     string
		text     string
		data     map[string]interface{}
		expected string
	}{
		{
			name:     "no templates",
			text:     "plain text",
			data:     map[string]interface{}{},
			expected: "plain text",
		},
		{
			name:     "simple replacement",
			text:     "Hello {{.name}}!",
			data:     map[string]interface{}{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "multiple replacements",
			text:     "{{.action}} by {{.user}}",
			data:     map[string]interface{}{"action": "Deploy", "user": "admin"},
			expected: "Deploy by admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.expandTemplate(tt.text, tt.data)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestWebhookActionExecutor_SanitizeExecCommand(t *testing.T) {
	executor := newWebhookActionExecutor(nil, "")

	tests := []struct {
		name        string
		cmd         string
		expectError bool
	}{
		{"valid command", "echo hello", false},
		{"command with args", "ls -la /tmp", false},
		{"empty command", "", true},
		{"whitespace only", "   ", true},
		{"command injection semicolon", "echo hello; rm -rf /", true},
		{"command injection pipe", "echo hello | cat", true},
		{"command injection ampersand", "echo hello && rm file", true},
		{"command injection backtick", "echo `whoami`", true},
		{"command injection dollar", "echo $HOME", true},
		{"command injection redirect", "echo hello > file", true},
		{"command injection newline", "echo hello\nrm file", true},
		{"null byte", "echo\x00hello", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.sanitizeExecCommand(tt.cmd)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestValidateHMACSignature(t *testing.T) {
	cfg := &config.Config{}
	handler := NewWebhookHandler(cfg, "", nil)

	secret := "my-secret"
	body := []byte(`{"test": "data"}`)

	t.Run("missing signature header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		result := handler.validateHMACSignature(req, body, secret)
		if result {
			t.Error("Expected false for missing signature")
		}
	})

	t.Run("invalid signature format", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		req.Header.Set("X-Webhook-Signature", "invalid-format")
		result := handler.validateHMACSignature(req, body, secret)
		if result {
			t.Error("Expected false for invalid format")
		}
	})

	t.Run("wrong prefix", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		req.Header.Set("X-Webhook-Signature", "md5=abc123")
		result := handler.validateHMACSignature(req, body, secret)
		if result {
			t.Error("Expected false for wrong prefix")
		}
	})

	t.Run("valid HMAC signature", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		// Compute correct HMAC-SHA256 signature for body with secret
		// sha256 HMAC of '{"test": "data"}' with secret 'my-secret'
		// = 7a0c8c2d9c2f8e8a8f5e9d6c7b3a2e1d0f8e7c6b5a4d3c2b1a0f9e8d7c6b5a4d (example)
		// We compute it properly:
		h := computeHMAC(body, secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+h)
		result := handler.validateHMACSignature(req, body, secret)
		if !result {
			t.Error("Expected true for valid HMAC signature")
		}
	})

	t.Run("invalid HMAC signature", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		req.Header.Set("X-Webhook-Signature", "sha256=invalidsignature")
		result := handler.validateHMACSignature(req, body, secret)
		if result {
			t.Error("Expected false for invalid HMAC signature")
		}
	})

	t.Run("wrong secret produces different signature", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		wrongSecretSig := computeHMAC(body, "wrong-secret")
		req.Header.Set("X-Webhook-Signature", "sha256="+wrongSecretSig)
		result := handler.validateHMACSignature(req, body, secret)
		if result {
			t.Error("Expected false when signature computed with wrong secret")
		}
	})
}

// computeHMAC computes HMAC-SHA256 signature for testing
func computeHMAC(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func TestWebhookHandler_RateLimiting(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"test-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"test": {Action: "test-action"},
		},
	}

	mock := &mockActionHandler{}
	handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)

	// Exhaust the rate limiter burst
	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 30; i++ {
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			successCount++
		} else if w.Code == http.StatusTooManyRequests {
			rateLimitedCount++
			// Check Retry-After header
			if w.Header().Get("Retry-After") == "" {
				t.Error("Expected Retry-After header on rate limit response")
			}
		}
	}

	// Should have some successes and some rate limited
	if successCount == 0 {
		t.Error("Expected some requests to succeed")
	}
	if rateLimitedCount == 0 {
		t.Error("Expected some requests to be rate limited")
	}
}

func TestWebhookHandler_TimestampValidation(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"test-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"test": {
				Action:             "test-action",
				ValidateTimestamp:  true,
				TimestampTolerance: 300, // 5 minutes
			},
		},
	}

	mock := &mockActionHandler{}
	handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)

	t.Run("valid timestamp", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing timestamp", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("expired timestamp", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		// Set timestamp 10 minutes ago (beyond 5 minute tolerance)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix()))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for expired timestamp, got %d", w.Code)
		}
	})

	t.Run("future timestamp beyond tolerance", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		// Set timestamp 2 minutes in the future (within 60s clock skew tolerance)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Add(2*time.Minute).Unix()))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for future timestamp, got %d", w.Code)
		}
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		mock.calls = nil
		req := httptest.NewRequest("POST", "/webhook/test", nil)
		req.Header.Set("X-Webhook-Timestamp", "not-a-number")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for invalid timestamp, got %d", w.Code)
		}
	})
}

func TestWebhookHandler_HMACSignatureIntegration(t *testing.T) {
	sigSecret := "hmac-secret-key"
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"test-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"test": {
				Action:          "test-action",
				SignatureSecret: sigSecret,
			},
		},
	}

	mock := &mockActionHandler{}
	handler := NewWebhookHandler(cfg, t.TempDir(), mock.handle)

	t.Run("valid HMAC signature triggers action", func(t *testing.T) {
		mock.calls = nil
		body := []byte(`{"message": "test"}`)
		signature := computeHMAC(body, sigSecret)

		req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		if len(mock.calls) != 1 {
			t.Errorf("Expected 1 action call, got %d", len(mock.calls))
		}
	})

	t.Run("invalid HMAC signature rejected", func(t *testing.T) {
		mock.calls = nil
		body := []byte(`{"message": "test"}`)

		req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Signature", "sha256=invalidsignature")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}

		if len(mock.calls) != 0 {
			t.Error("Expected no action calls for invalid signature")
		}
	})

	t.Run("missing signature rejected", func(t *testing.T) {
		mock.calls = nil
		body := []byte(`{"message": "test"}`)

		req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestWebhookHandler_ConcurrencyControl(t *testing.T) {
	cfg := &config.Config{
		Actions: map[string]*config.Action{
			"slow-action": {Kind: "http"},
		},
		Webhooks: map[string]*config.Webhook{
			"test": {Action: "slow-action"},
		},
	}

	// Create a slow action handler
	var activeCount int32
	var maxActive int32
	var mu sync.Mutex

	slowHandler := func(actionName string, params map[string]interface{}) error {
		mu.Lock()
		activeCount++
		if activeCount > maxActive {
			maxActive = activeCount
		}
		mu.Unlock()

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		activeCount--
		mu.Unlock()

		return nil
	}

	handler := NewWebhookHandler(cfg, t.TempDir(), slowHandler)

	// Fire many concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/webhook/test", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	// Verify concurrency was limited
	if maxActive > defaultMaxConcurrentActions {
		t.Errorf("Expected max concurrent actions <= %d, got %d", defaultMaxConcurrentActions, maxActive)
	}
}

func TestValidateTimestamp(t *testing.T) {
	handler := NewWebhookHandler(nil, "", nil)

	t.Run("valid current timestamp", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		err := handler.validateTimestamp(req, 300)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("timestamp within tolerance", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Add(-2*time.Minute).Unix()))

		err := handler.validateTimestamp(req, 300)
		if err != nil {
			t.Errorf("Expected no error for timestamp within tolerance, got: %v", err)
		}
	})

	t.Run("timestamp expired", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix()))

		err := handler.validateTimestamp(req, 300)
		if err == nil {
			t.Error("Expected error for expired timestamp")
		}
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)

		err := handler.validateTimestamp(req, 300)
		if err == nil {
			t.Error("Expected error for missing timestamp header")
		}
	})

	t.Run("slight clock skew allowed", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		// 30 seconds in future (within 60s tolerance)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Add(30*time.Second).Unix()))

		err := handler.validateTimestamp(req, 300)
		if err != nil {
			t.Errorf("Expected no error for slight future timestamp, got: %v", err)
		}
	})
}
