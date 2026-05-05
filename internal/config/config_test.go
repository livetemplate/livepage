package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceConfigIsCacheEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cache    *CacheConfig
		expected bool
	}{
		{"nil cache", nil, false},
		{"empty TTL", &CacheConfig{TTL: ""}, false},
		{"with TTL", &CacheConfig{TTL: "5m"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Cache: tt.cache}
			if got := cfg.IsCacheEnabled(); got != tt.expected {
				t.Errorf("IsCacheEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourceConfigGetTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{"empty defaults to 10s", "", 10 * time.Second},
		{"valid duration", "30s", 30 * time.Second},
		{"invalid duration defaults to 10s", "notaduration", 10 * time.Second},
		{"1 minute", "1m", time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Timeout: tt.timeout}
			if got := cfg.GetTimeout(); got != tt.expected {
				t.Errorf("GetTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourceConfigGetRetryBaseDelay(t *testing.T) {
	tests := []struct {
		name     string
		retry    *RetryConfig
		expected time.Duration
	}{
		{"nil retry defaults to 100ms", nil, 100 * time.Millisecond},
		{"empty base_delay defaults to 100ms", &RetryConfig{BaseDelay: ""}, 100 * time.Millisecond},
		{"invalid base_delay defaults to 100ms", &RetryConfig{BaseDelay: "bad"}, 100 * time.Millisecond},
		{"valid base_delay", &RetryConfig{BaseDelay: "200ms"}, 200 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Retry: tt.retry}
			if got := cfg.GetRetryBaseDelay(); got != tt.expected {
				t.Errorf("GetRetryBaseDelay() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourceConfigGetRetryMaxDelay(t *testing.T) {
	tests := []struct {
		name     string
		retry    *RetryConfig
		expected time.Duration
	}{
		{"nil retry defaults to 5s", nil, 5 * time.Second},
		{"empty max_delay defaults to 5s", &RetryConfig{MaxDelay: ""}, 5 * time.Second},
		{"invalid max_delay defaults to 5s", &RetryConfig{MaxDelay: "bad"}, 5 * time.Second},
		{"valid max_delay", &RetryConfig{MaxDelay: "10s"}, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Retry: tt.retry}
			if got := cfg.GetRetryMaxDelay(); got != tt.expected {
				t.Errorf("GetRetryMaxDelay() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourceConfigGetCacheTTL(t *testing.T) {
	tests := []struct {
		name     string
		cache    *CacheConfig
		expected time.Duration
	}{
		{"nil cache", nil, 0},
		{"empty TTL", &CacheConfig{TTL: ""}, 0},
		{"invalid TTL", &CacheConfig{TTL: "invalid"}, 0},
		{"5 minutes", &CacheConfig{TTL: "5m"}, 5 * time.Minute},
		{"1 hour", &CacheConfig{TTL: "1h"}, time.Hour},
		{"30 seconds", &CacheConfig{TTL: "30s"}, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Cache: tt.cache}
			if got := cfg.GetCacheTTL(); got != tt.expected {
				t.Errorf("GetCacheTTL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourceConfigGetCacheStrategy(t *testing.T) {
	tests := []struct {
		name     string
		cache    *CacheConfig
		expected string
	}{
		{"nil cache", nil, "simple"},
		{"empty strategy", &CacheConfig{Strategy: ""}, "simple"},
		{"simple", &CacheConfig{Strategy: "simple"}, "simple"},
		{"stale-while-revalidate", &CacheConfig{Strategy: "stale-while-revalidate"}, "stale-while-revalidate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Cache: tt.cache}
			if got := cfg.GetCacheStrategy(); got != tt.expected {
				t.Errorf("GetCacheStrategy() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourceConfigIsStaleWhileRevalidate(t *testing.T) {
	tests := []struct {
		name     string
		cache    *CacheConfig
		expected bool
	}{
		{"nil cache", nil, false},
		{"empty strategy", &CacheConfig{Strategy: ""}, false},
		{"simple", &CacheConfig{Strategy: "simple"}, false},
		{"stale-while-revalidate", &CacheConfig{Strategy: "stale-while-revalidate"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SourceConfig{Cache: tt.cache}
			if got := cfg.IsStaleWhileRevalidate(); got != tt.expected {
				t.Errorf("IsStaleWhileRevalidate() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWebhookValidate(t *testing.T) {
	actions := map[string]*Action{
		"test-action": {Kind: "http", URL: "http://example.com"},
	}

	tests := []struct {
		name      string
		webhook   *Webhook
		wantError bool
		errorMsg  string
	}{
		{
			name:      "nil webhook",
			webhook:   nil,
			wantError: true,
			errorMsg:  "is nil",
		},
		{
			name:      "empty action",
			webhook:   &Webhook{Action: ""},
			wantError: true,
			errorMsg:  "action is required",
		},
		{
			name:      "non-existent action",
			webhook:   &Webhook{Action: "missing-action"},
			wantError: true,
			errorMsg:  "non-existent action",
		},
		{
			name:      "negative timestamp tolerance",
			webhook:   &Webhook{Action: "test-action", ValidateTimestamp: true, TimestampTolerance: -1},
			wantError: true,
			errorMsg:  "cannot be negative",
		},
		{
			name:      "valid webhook with secret",
			webhook:   &Webhook{Action: "test-action", Secret: "mysecret"},
			wantError: false,
		},
		{
			name:      "valid webhook with signature secret",
			webhook:   &Webhook{Action: "test-action", SignatureSecret: "mysignaturesecret"},
			wantError: false,
		},
		{
			name:      "valid webhook with timestamp validation",
			webhook:   &Webhook{Action: "test-action", ValidateTimestamp: true, TimestampTolerance: 300},
			wantError: false,
		},
		{
			name:      "valid webhook with default timestamp tolerance",
			webhook:   &Webhook{Action: "test-action", ValidateTimestamp: true},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.webhook.Validate("test-webhook", actions)
			if tt.wantError {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestConfigValidateWebhooks(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name:      "nil webhooks",
			config:    &Config{},
			wantError: false,
		},
		{
			name: "valid webhooks",
			config: &Config{
				Actions: map[string]*Action{
					"action1": {Kind: "http", URL: "http://example.com"},
					"action2": {Kind: "sql", Statement: "SELECT 1"},
				},
				Webhooks: map[string]*Webhook{
					"webhook1": {Action: "action1"},
					"webhook2": {Action: "action2", Secret: "secret"},
				},
			},
			wantError: false,
		},
		{
			name: "invalid webhook - missing action reference",
			config: &Config{
				Actions: map[string]*Action{
					"action1": {Kind: "http", URL: "http://example.com"},
				},
				Webhooks: map[string]*Webhook{
					"webhook1": {Action: "nonexistent"},
				},
			},
			wantError: true,
		},
		{
			name: "invalid webhook - empty action",
			config: &Config{
				Webhooks: map[string]*Webhook{
					"webhook1": {Action: ""},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateWebhooks()
			if tt.wantError && err == nil {
				t.Errorf("ValidateWebhooks() expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateWebhooks() unexpected error = %v", err)
			}
		})
	}
}

func TestOutputConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		output    *OutputConfig
		wantError bool
		errorMsg  string
	}{
		{
			name:      "nil output",
			output:    nil,
			wantError: true,
			errorMsg:  "is nil",
		},
		{
			name:      "unsupported type",
			output:    &OutputConfig{Type: "discord"},
			wantError: true,
			errorMsg:  "unsupported type",
		},
		{
			name:      "slack without channel",
			output:    &OutputConfig{Type: "slack", Channel: ""},
			wantError: true,
			errorMsg:  "channel is required",
		},
		{
			name:      "email without to",
			output:    &OutputConfig{Type: "email", To: ""},
			wantError: true,
			errorMsg:  "to is required",
		},
		{
			name:      "valid slack",
			output:    &OutputConfig{Type: "slack", Channel: "#test"},
			wantError: false,
		},
		{
			name:      "valid email",
			output:    &OutputConfig{Type: "email", To: "user@example.com"},
			wantError: false,
		},
		{
			name:      "valid email with subject",
			output:    &OutputConfig{Type: "email", To: "user@example.com", Subject: "Alert"},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.output.Validate("test-output")
			if tt.wantError {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestOutputConfigGetChannel(t *testing.T) {
	// Test with nil
	var nilOutput *OutputConfig
	if got := nilOutput.GetChannel(); got != "" {
		t.Errorf("GetChannel() on nil = %q, want empty string", got)
	}

	// Test with empty
	output := &OutputConfig{Channel: ""}
	if got := output.GetChannel(); got != "" {
		t.Errorf("GetChannel() on empty = %q, want empty string", got)
	}

	// Test with value
	output = &OutputConfig{Channel: "#test-channel"}
	if got := output.GetChannel(); got != "#test-channel" {
		t.Errorf("GetChannel() = %q, want #test-channel", got)
	}

	// Test with env var expansion
	t.Setenv("TEST_CHANNEL", "#from-env")
	output = &OutputConfig{Channel: "${TEST_CHANNEL}"}
	if got := output.GetChannel(); got != "#from-env" {
		t.Errorf("GetChannel() with env var = %q, want #from-env", got)
	}
}

func TestOutputConfigGetTo(t *testing.T) {
	// Test with nil
	var nilOutput *OutputConfig
	if got := nilOutput.GetTo(); got != "" {
		t.Errorf("GetTo() on nil = %q, want empty string", got)
	}

	// Test with empty
	output := &OutputConfig{To: ""}
	if got := output.GetTo(); got != "" {
		t.Errorf("GetTo() on empty = %q, want empty string", got)
	}

	// Test with value
	output = &OutputConfig{To: "user@example.com"}
	if got := output.GetTo(); got != "user@example.com" {
		t.Errorf("GetTo() = %q, want user@example.com", got)
	}

	// Test with env var expansion
	t.Setenv("TEST_EMAIL", "env@example.com")
	output = &OutputConfig{To: "${TEST_EMAIL}"}
	if got := output.GetTo(); got != "env@example.com" {
		t.Errorf("GetTo() with env var = %q, want env@example.com", got)
	}
}

func TestConfigValidateOutputs(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name:      "nil outputs",
			config:    &Config{},
			wantError: false,
		},
		{
			name: "valid outputs",
			config: &Config{
				Outputs: map[string]*OutputConfig{
					"slack": {Type: "slack", Channel: "#alerts"},
					"email": {Type: "email", To: "alerts@example.com"},
				},
			},
			wantError: false,
		},
		{
			name: "invalid output - unsupported type",
			config: &Config{
				Outputs: map[string]*OutputConfig{
					"invalid": {Type: "discord"},
				},
			},
			wantError: true,
		},
		{
			name: "invalid output - slack missing channel",
			config: &Config{
				Outputs: map[string]*OutputConfig{
					"slack": {Type: "slack"},
				},
			},
			wantError: true,
		},
		{
			name: "invalid output - email missing to",
			config: &Config{
				Outputs: map[string]*OutputConfig{
					"email": {Type: "email"},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateOutputs()
			if tt.wantError && err == nil {
				t.Errorf("ValidateOutputs() expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateOutputs() unexpected error = %v", err)
			}
		})
	}
}

// TestLoadRejectsInjectedFrameSrc closes the loop on the startup-failure
// guarantee in the SecurityConfig.Validate godoc: a config file containing
// an injection-style frame_src must surface as an error from Load(),
// not as a silently-broken CSP at runtime.
func TestLoadRejectsInjectedFrameSrc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinkerdown.yaml")
	yaml := []byte(`title: bad
type: tutorial
security:
  frame_src:
    - "https://evil.com; script-src *"
`)
	if err := os.WriteFile(path, yaml, 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted a frame_src entry containing a semicolon; want error")
	}
}

func TestSecurityConfigValidate(t *testing.T) {
	cases := []struct {
		name      string
		frameSrc  []string
		wantError bool
	}{
		{name: "empty list", frameSrc: nil, wantError: false},
		{name: "single valid origin", frameSrc: []string{"https://lt-landing-demo.fly.dev"}, wantError: false},
		{name: "multiple valid origins", frameSrc: []string{"https://a.example.com", "https://b.example.com:8443"}, wantError: false},
		{name: "rejects semicolon (CSP injection)", frameSrc: []string{"https://evil.com; script-src *"}, wantError: true},
		{name: "rejects comma (defensive)", frameSrc: []string{"https://evil.com,foo"}, wantError: true},
		{name: "rejects newline (header split)", frameSrc: []string{"https://evil.com\nX-XSS: 0"}, wantError: true},
		{name: "rejects carriage return", frameSrc: []string{"https://evil.com\r"}, wantError: true},
		{name: "rejects bare space", frameSrc: []string{"https://evil.com script-src *"}, wantError: true},
		{name: "rejects empty entry", frameSrc: []string{""}, wantError: true},
		{name: "rejects schemeless host", frameSrc: []string{"lt-landing-demo.fly.dev"}, wantError: true},
		{name: "rejects scheme-only", frameSrc: []string{"https://"}, wantError: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := SecurityConfig{FrameSrc: tt.frameSrc}.Validate()
			if tt.wantError && err == nil {
				t.Errorf("Validate() expected error for %v, got nil", tt.frameSrc)
			}
			if !tt.wantError && err != nil {
				t.Errorf("Validate() unexpected error for %v: %v", tt.frameSrc, err)
			}
		})
	}
}
