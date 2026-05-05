package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the tinkerdown configuration
type Config struct {
	Title       string                  `yaml:"title"`
	Description string                  `yaml:"description"`
	Type        string                  `yaml:"type"` // "tutorial" or "site"
	Site        *SiteConfig             `yaml:"site,omitempty"`
	Navigation  []NavSection            `yaml:"navigation,omitempty"`
	Server      ServerConfig            `yaml:"server"`
	Styling     StylingConfig           `yaml:"styling"`
	Blocks      BlocksConfig            `yaml:"blocks"`
	Features    FeaturesConfig          `yaml:"features"`
	Ignore      []string                `yaml:"ignore"`
	Sources     map[string]SourceConfig `yaml:"sources,omitempty"`
	Actions     map[string]*Action      `yaml:"actions,omitempty"`
	API         *APIConfig              `yaml:"api,omitempty"`
	Webhooks    map[string]*Webhook     `yaml:"webhooks,omitempty"`
	Outputs     map[string]*OutputConfig `yaml:"outputs,omitempty"`
	Security    SecurityConfig          `yaml:"security,omitempty"`

	// VersionPrefix, when non-empty, becomes a URL path segment that is
	// stripped from incoming requests before route resolution. Routes serve
	// equivalently with or without the prefix; the prefix exists so that a
	// future multi-version deployment can mount several site builds under
	// distinct prefixes (e.g. /v0/, /latest/) without collision.
	VersionPrefix string `yaml:"version_prefix,omitempty"`

	// Routes declares custom URL routes that bypass the markdown page
	// resolver. Each entry binds a request-path pattern to an upstream
	// destination. Currently only `type: proxy` is supported, which
	// reverse-proxies the request (including WebSocket upgrades) to the
	// configured upstream host. The full request path is forwarded
	// unchanged — the upstream is responsible for its own URL structure.
	Routes []RouteEntry `yaml:"routes,omitempty"`
}

// RouteEntry binds a URL pattern to a destination. A pattern ending in
// "/" matches that subtree; otherwise it must match exactly. Routes are
// evaluated in declaration order; first match wins.
type RouteEntry struct {
	Pattern  string `yaml:"pattern"`
	Type     string `yaml:"type"`
	Upstream string `yaml:"upstream"`
}

// OutputConfig defines an output destination for notifications.
// Outputs receive messages from Notify imperatives in markdown content.
//
// # Example Configuration
//
//	outputs:
//	  team-slack:
//	    type: slack
//	    channel: "#team-updates"
//
//	  alerts-email:
//	    type: email
//	    to: "alerts@company.com"
//	    subject: "Tinkerdown Alert"
type OutputConfig struct {
	// Type is the output type: "slack" or "email"
	Type string `yaml:"type"`

	// Channel is the Slack channel (for slack type), e.g., "#team-updates"
	// Environment variable expansion is supported (e.g., "${SLACK_CHANNEL}")
	Channel string `yaml:"channel,omitempty"`

	// To is the email recipient address (for email type)
	// Environment variable expansion is supported (e.g., "${ALERT_EMAIL}")
	To string `yaml:"to,omitempty"`

	// Subject is the email subject line (for email type)
	// Defaults to "Notification from Tinkerdown"
	Subject string `yaml:"subject,omitempty"`
}

// GetChannel returns the channel with environment variable expansion
func (o *OutputConfig) GetChannel() string {
	if o == nil || o.Channel == "" {
		return ""
	}
	return os.ExpandEnv(o.Channel)
}

// GetTo returns the recipient address with environment variable expansion
func (o *OutputConfig) GetTo() string {
	if o == nil || o.To == "" {
		return ""
	}
	return os.ExpandEnv(o.To)
}

// Validate checks that the output configuration is valid.
func (o *OutputConfig) Validate(name string) error {
	if o == nil {
		return fmt.Errorf("output %q is nil", name)
	}

	switch o.Type {
	case "slack":
		if o.GetChannel() == "" {
			return fmt.Errorf("output %q: channel is required for slack type", name)
		}
	case "email":
		if o.GetTo() == "" {
			return fmt.Errorf("output %q: to is required for email type", name)
		}
	default:
		return fmt.Errorf("output %q: unsupported type %q (must be 'slack' or 'email')", name, o.Type)
	}

	return nil
}

// ValidateOutputs validates all output configurations in the config.
func (c *Config) ValidateOutputs() error {
	if c.Outputs == nil {
		return nil
	}

	for name, output := range c.Outputs {
		if err := output.Validate(name); err != nil {
			return err
		}
	}

	return nil
}

// SourceConfig defines a data source for lvt-source blocks
type SourceConfig struct {
	Type        string                 `yaml:"type"`                   // "exec", "pg", "rest", "csv", "json", "markdown", "sqlite", "wasm", "graphql"
	Cmd         string                 `yaml:"cmd,omitempty"`          // For exec: command to run
	Query       string                 `yaml:"query,omitempty"`        // For pg: SQL query
	From        string                 `yaml:"from,omitempty"`         // For rest/graphql: API endpoint URL
	File        string                 `yaml:"file,omitempty"`         // For csv/json/markdown: file path
	Anchor      string                 `yaml:"anchor,omitempty"`       // For markdown: section anchor (e.g., "#todos")
	DB          string                 `yaml:"db,omitempty"`           // For sqlite: database file path (default: ./tinkerdown.db)
	Table       string                 `yaml:"table,omitempty"`        // For sqlite: table name
	Path        string                 `yaml:"path,omitempty"`         // For wasm: path to .wasm file
	QueryFile   string                 `yaml:"query_file,omitempty"`   // For graphql: path to .graphql file
	Variables   map[string]interface{} `yaml:"variables,omitempty"`    // For graphql: query variables
	Headers     map[string]string      `yaml:"headers,omitempty"`      // For rest/graphql: HTTP headers (env vars expanded)
	QueryParams map[string]string      `yaml:"query_params,omitempty"` // For rest: URL query parameters (env vars expanded)
	ResultPath  string                 `yaml:"result_path,omitempty"`  // For rest/graphql: dot-path to extract array (e.g., "data.items")
	Readonly    *bool                  `yaml:"readonly,omitempty"`     // For markdown/sqlite: read-only mode (default: true, set to false for writes)
	Options     map[string]string      `yaml:"options,omitempty"`      // Type-specific options (also used for wasm init config)
	Manual      bool                   `yaml:"manual,omitempty"`       // For exec: require Run button click
	Format      string                 `yaml:"format,omitempty"`       // For exec: output format (json, lines, csv). Default: json
	Delimiter   string                 `yaml:"delimiter,omitempty"`    // For exec CSV: field delimiter. Default: ","
	Env         map[string]string      `yaml:"env,omitempty"`          // For exec: environment variables (env vars expanded)
	Timeout     string                 `yaml:"timeout,omitempty"`      // Request timeout (e.g., "30s", "1m"). Default: 10s
	Retry       *RetryConfig           `yaml:"retry,omitempty"`        // Retry configuration
	Cache       *CacheConfig           `yaml:"cache,omitempty"`        // Cache configuration
	AutoBind    *bool                  `yaml:"auto_bind,omitempty"`    // Set to false to exclude from auto-table matching

	// For computed sources: derive data from another source
	GroupBy   string            `yaml:"group_by,omitempty"`   // Field to group by (e.g., "category")
	Aggregate map[string]string `yaml:"aggregate,omitempty"`  // Field → aggregation expression (e.g., "total": "sum(amount)")
	Filter    string            `yaml:"filter,omitempty"`     // Optional filter expression (e.g., "status = active")
}

// RetryConfig configures retry behavior for a source
type RetryConfig struct {
	MaxRetries int    `yaml:"max_retries,omitempty"` // Maximum retry attempts (default: 3)
	BaseDelay  string `yaml:"base_delay,omitempty"`  // Initial delay (e.g., "100ms"). Default: 100ms
	MaxDelay   string `yaml:"max_delay,omitempty"`   // Maximum delay (e.g., "5s"). Default: 5s
}

// CacheConfig configures caching behavior for a source
type CacheConfig struct {
	TTL      string `yaml:"ttl,omitempty"`       // Cache TTL (e.g., "5m", "1h"). Default: disabled (empty)
	Strategy string `yaml:"strategy,omitempty"`  // Cache strategy: "simple" or "stale-while-revalidate". Default: "simple"
	MaxRows  int    `yaml:"max_rows,omitempty"`  // Maximum rows to cache (truncates if exceeded). Default: unlimited
	MaxBytes int    `yaml:"max_bytes,omitempty"` // Maximum bytes to cache (truncates if exceeded). Default: unlimited
}

// IsReadonly returns true if the source is read-only (default: true for markdown sources)
func (c SourceConfig) IsReadonly() bool {
	if c.Readonly == nil {
		return true // Default to read-only for safety
	}
	return *c.Readonly
}

// GetTimeout returns the parsed timeout duration (default: 10s)
func (c SourceConfig) GetTimeout() time.Duration {
	if c.Timeout == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		log.Printf("[Config] Warning: invalid timeout %q, using default 10s", c.Timeout)
		return 10 * time.Second
	}
	return d
}

// GetRetryMaxRetries returns the max retries (default: 3, set to 0 to disable retries)
func (c SourceConfig) GetRetryMaxRetries() int {
	if c.Retry == nil {
		return 3
	}
	if c.Retry.MaxRetries < 0 {
		return 3
	}
	return c.Retry.MaxRetries
}

// GetRetryBaseDelay returns the base delay (default: 100ms)
func (c SourceConfig) GetRetryBaseDelay() time.Duration {
	if c.Retry == nil || c.Retry.BaseDelay == "" {
		return 100 * time.Millisecond
	}
	d, err := time.ParseDuration(c.Retry.BaseDelay)
	if err != nil {
		log.Printf("[Config] Warning: invalid retry base_delay %q, using default 100ms", c.Retry.BaseDelay)
		return 100 * time.Millisecond
	}
	return d
}

// GetRetryMaxDelay returns the max delay (default: 5s)
func (c SourceConfig) GetRetryMaxDelay() time.Duration {
	if c.Retry == nil || c.Retry.MaxDelay == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(c.Retry.MaxDelay)
	if err != nil {
		log.Printf("[Config] Warning: invalid retry max_delay %q, using default 5s", c.Retry.MaxDelay)
		return 5 * time.Second
	}
	return d
}

// IsCacheEnabled returns true if caching is enabled for this source
func (c SourceConfig) IsCacheEnabled() bool {
	return c.Cache != nil && c.Cache.TTL != ""
}

// GetCacheTTL returns the cache TTL (0 if caching is disabled)
func (c SourceConfig) GetCacheTTL() time.Duration {
	if c.Cache == nil || c.Cache.TTL == "" {
		return 0
	}
	d, err := time.ParseDuration(c.Cache.TTL)
	if err != nil {
		log.Printf("[Config] Warning: invalid cache ttl %q, caching disabled", c.Cache.TTL)
		return 0
	}
	return d
}

// GetCacheStrategy returns the cache strategy (default: "simple")
func (c SourceConfig) GetCacheStrategy() string {
	if c.Cache == nil || c.Cache.Strategy == "" {
		return "simple"
	}
	return c.Cache.Strategy
}

// IsStaleWhileRevalidate returns true if using stale-while-revalidate strategy
func (c SourceConfig) IsStaleWhileRevalidate() bool {
	return c.GetCacheStrategy() == "stale-while-revalidate"
}

// GetCacheMaxRows returns the max rows limit (0 = unlimited)
func (c SourceConfig) GetCacheMaxRows() int {
	if c.Cache == nil {
		return 0
	}
	return c.Cache.MaxRows
}

// GetCacheMaxBytes returns the max bytes limit (0 = unlimited)
func (c SourceConfig) GetCacheMaxBytes() int {
	if c.Cache == nil {
		return 0
	}
	return c.Cache.MaxBytes
}

// Action defines a custom action that can be triggered via lvt-click
type Action struct {
	Kind      string              `yaml:"kind"`                // Action kind: "sql", "http", "exec"
	Source    string              `yaml:"source,omitempty"`    // For sql: source name to execute against
	Statement string              `yaml:"statement,omitempty"` // For sql: SQL statement with :param placeholders
	URL       string              `yaml:"url,omitempty"`       // For http: request URL (supports template expressions)
	Method    string              `yaml:"method,omitempty"`    // For http: HTTP method (default: POST)
	Body      string              `yaml:"body,omitempty"`      // For http: request body template
	Cmd       string              `yaml:"cmd,omitempty"`       // For exec: command to run
	Params    map[string]ParamDef `yaml:"params,omitempty"`    // Parameter definitions
	Confirm   string              `yaml:"confirm,omitempty"`   // Confirmation message (triggers dialog)
}

// ParamDef defines a parameter for an action
type ParamDef struct {
	Type     string `yaml:"type,omitempty"`     // Parameter type: "string", "number", "date", "bool"
	Required bool   `yaml:"required,omitempty"` // Whether the parameter is required
	Default  string `yaml:"default,omitempty"`  // Default value
}

// Webhook defines a webhook trigger that can receive HTTP POST requests.
//
// Webhooks allow external services (CI/CD, monitoring, etc.) to trigger actions
// by sending POST requests to /webhook/{name} endpoints.
//
// # Security Features
//
// Webhooks support multiple authentication methods:
//   - Simple secret: X-Webhook-Secret header or ?secret= query parameter
//   - HMAC signature: X-Webhook-Signature header with "sha256=<hex>" format
//   - Timestamp validation: X-Webhook-Timestamp header for replay attack prevention
//
// # Exec Action Restrictions
//
// When a webhook triggers an "exec" action, the command is validated for safety.
// The following shell metacharacters are blocked to prevent command injection:
//   - & ; | : command chaining/background execution
//   - $ : variable expansion (could leak environment variables)
//   - > < : redirection (could overwrite files)
//   - ` : command substitution
//   - \ : escape sequences
//   - \n \r : newlines (could inject additional commands)
//
// For complex commands, create an intermediate script and invoke it via the webhook.
//
// # Example Configuration
//
//	webhooks:
//	  deploy:
//	    action: deploy-app
//	    signature_secret: ${WEBHOOK_SECRET}
//	    validate_timestamp: true
//	    timestamp_tolerance: 300
//
//	  github-push:
//	    action: sync-repo
//	    secret: ${GITHUB_WEBHOOK_SECRET}
type Webhook struct {
	// Action is the name of the action to execute when this webhook is triggered
	Action string `yaml:"action"`
	// Secret is the shared secret for validation (supports env var expansion)
	// Used for X-Webhook-Secret header or ?secret= query param validation
	Secret string `yaml:"secret,omitempty"`
	// SignatureSecret is the secret used for HMAC signature validation (supports env var expansion)
	// When set, validates X-Webhook-Signature header in format "sha256=<hex>"
	// This provides stronger security than plain secret validation
	SignatureSecret string `yaml:"signature_secret,omitempty"`
	// ValidateTimestamp enables replay attack prevention by validating X-Webhook-Timestamp
	// Requests older than TimestampTolerance seconds are rejected
	ValidateTimestamp bool `yaml:"validate_timestamp,omitempty"`
	// TimestampTolerance is the maximum age in seconds for timestamp validation (default: 300 = 5 minutes)
	TimestampTolerance int `yaml:"timestamp_tolerance,omitempty"`
}

// GetSecret returns the webhook secret with environment variable expansion
func (w *Webhook) GetSecret() string {
	if w == nil || w.Secret == "" {
		return ""
	}
	return os.ExpandEnv(w.Secret)
}

// GetSignatureSecret returns the HMAC signature secret with environment variable expansion
func (w *Webhook) GetSignatureSecret() string {
	if w == nil || w.SignatureSecret == "" {
		return ""
	}
	return os.ExpandEnv(w.SignatureSecret)
}

// GetTimestampTolerance returns the timestamp tolerance in seconds (default 300)
func (w *Webhook) GetTimestampTolerance() int {
	if w == nil || w.TimestampTolerance <= 0 {
		return 300 // 5 minutes default
	}
	return w.TimestampTolerance
}

// Validate checks that the webhook configuration is valid.
// Returns an error if any validation fails.
func (w *Webhook) Validate(name string, actions map[string]*Action) error {
	if w == nil {
		return fmt.Errorf("webhook %q is nil", name)
	}

	// Action is required
	if w.Action == "" {
		return fmt.Errorf("webhook %q: action is required", name)
	}

	// Validate that the referenced action exists
	if actions != nil {
		if _, exists := actions[w.Action]; !exists {
			return fmt.Errorf("webhook %q: references non-existent action %q", name, w.Action)
		}
	}

	// If both secret and signature_secret are set, warn (signature takes precedence)
	// This is not an error, just a note for clarity

	// Validate timestamp tolerance if timestamp validation is enabled
	if w.ValidateTimestamp && w.TimestampTolerance < 0 {
		return fmt.Errorf("webhook %q: timestamp_tolerance cannot be negative", name)
	}

	return nil
}

// ValidateWebhooks validates all webhook configurations in the config.
// Returns an error if any webhook has invalid configuration.
func (c *Config) ValidateWebhooks() error {
	if c.Webhooks == nil {
		return nil
	}

	for name, webhook := range c.Webhooks {
		if err := webhook.Validate(name, c.Actions); err != nil {
			return err
		}
	}

	return nil
}

// SiteConfig holds site-level configuration
type SiteConfig struct {
	Home       string `yaml:"home"`       // Homepage markdown file (e.g., "index.md")
	Logo       string `yaml:"logo"`       // Logo path (e.g., "/assets/logo.svg")
	Repository string `yaml:"repository"` // GitHub repository URL

	// URL is the canonical absolute base URL of the deployed site
	// (e.g., "https://livetemplate.fly.dev"), with NO trailing slash.
	// When set, sitemap.xml emits absolute URLs and OpenGraph meta tags
	// include og:url. When empty, sitemap entries use relative paths and
	// og:url is omitted.
	URL string `yaml:"url,omitempty"`
}

// NavSection represents a navigation section with pages
type NavSection struct {
	Title     string    `yaml:"title"`              // Section title (e.g., "Getting Started")
	Path      string    `yaml:"path"`               // Section path (e.g., "getting-started")
	Collapsed bool      `yaml:"collapsed"`          // Whether section is collapsed by default
	Pages     []NavPage `yaml:"pages,omitempty"`    // Pages in this section
}

// NavPage represents a single page in navigation
type NavPage struct {
	Title string `yaml:"title"` // Page title (e.g., "Installation")
	Path  string `yaml:"path"`  // Page path (e.g., "getting-started/installation.md")
}

// SecurityConfig exposes site-level security knobs that change emitted
// security response headers. Currently scoped to CSP overrides; further
// fields can be added without breaking existing configs because the
// struct is yaml-omitempty.
type SecurityConfig struct {
	// FrameSrc lists origins appended to the CSP frame-src directive,
	// in addition to the implicit 'self'. Set this when the site embeds
	// cross-origin iframes (e.g., a separately-deployed app loaded as a
	// live demo on a docs page). Each entry is emitted verbatim, so use
	// scheme + host (e.g., "https://lt-landing-demo.fly.dev"). When the
	// list is empty no frame-src directive is emitted, falling back to
	// default-src 'self' which blocks cross-origin frames.
	FrameSrc []string `yaml:"frame_src,omitempty"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port  int    `yaml:"port"`
	Host  string `yaml:"host"`
	Debug bool   `yaml:"debug"`
}

// StylingConfig holds styling-related configuration
type StylingConfig struct {
	Theme        string `yaml:"theme"`
	PrimaryColor string `yaml:"primary_color"`
	Font         string `yaml:"font"`
}

// BlocksConfig holds block-related configuration
type BlocksConfig struct {
	AutoID          bool   `yaml:"auto_id"`
	IDFormat        string `yaml:"id_format"`
	ShowLineNumbers bool   `yaml:"show_line_numbers"`
}

// FeaturesConfig holds feature flags
type FeaturesConfig struct {
	HotReload bool `yaml:"hot_reload"`
	Sidebar   bool `yaml:"sidebar"`  // Show navigation sidebar (default: false)
	Headless  bool `yaml:"headless"` // Run without web UI, only API/webhooks/schedules

	// PrerenderDiagrams enables server-side mermaid pre-rendering via
	// Kroki. When true, ```mermaid blocks are converted to inline SVG at
	// parse time and the ~3.3MB client-side mermaid runtime is skipped on
	// pages where ALL blocks pre-rendered successfully. Blocks that fail
	// to render fall back to the client-side runtime. Default: false.
	PrerenderDiagrams bool `yaml:"prerender_diagrams,omitempty"`

	// KrokiURL overrides the Kroki base URL used for diagram pre-render
	// (default: https://kroki.io public instance). Point this at a
	// self-hosted Kroki container for production deployments where the
	// public instance's rate limits or terms are not acceptable.
	KrokiURL string `yaml:"kroki_url,omitempty"`
}

// APIConfig holds REST API configuration
type APIConfig struct {
	Enabled   bool             `yaml:"enabled"` // Enable REST API endpoints (default: false)
	CORS      *CORSConfig      `yaml:"cors,omitempty"`
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
	Auth      *AuthConfig      `yaml:"auth,omitempty"`
}

// Permission represents an API operation permission.
type Permission string

const (
	PermRead   Permission = "read"
	PermWrite  Permission = "write"
	PermDelete Permission = "delete"
)

// APIKeyConfig represents a named API key with associated permissions.
type APIKeyConfig struct {
	Name        string       `yaml:"name"`
	Key         string       `yaml:"key"`
	Permissions []Permission `yaml:"permissions,omitempty"`
}

// HasPermission checks if this key has a specific permission.
func (k *APIKeyConfig) HasPermission(perm Permission) bool {
	for _, p := range k.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// AuthConfig holds authentication configuration for the API
type AuthConfig struct {
	// APIKey is the legacy single API key for authentication (backward compatible).
	// Supports environment variable expansion (e.g., "${API_KEY}" or "$API_KEY").
	// Gets full permissions (read, write, delete) when used.
	APIKey string `yaml:"api_key,omitempty"`
	// HeaderName is the HTTP header name for the API key (default: "X-API-Key")
	// Also supports "Authorization: Bearer <token>" format when set to "Authorization"
	HeaderName string `yaml:"header_name,omitempty"`
	// Keys is a list of named API keys with specific permissions.
	Keys []APIKeyConfig `yaml:"keys,omitempty"`
}

// GetAPIKeys returns all configured API keys, normalizing legacy and new formats.
// Legacy api_key gets full permissions for backward compatibility.
// Uses the raw (unexpanded) value to detect intent — if the user configured
// an api_key that expands to empty, auth is still enforced (rejecting all
// requests) rather than silently disabled.
func (c *AuthConfig) GetAPIKeys() []APIKeyConfig {
	if c == nil {
		return nil
	}
	var keys []APIKeyConfig
	// Add legacy key with full permissions (check raw config, not expanded,
	// so auth stays enforced even if the env var is missing/empty).
	if c.APIKey != "" {
		keys = append(keys, APIKeyConfig{
			Name:        "default",
			Key:         c.APIKey, // Keep unexpanded; middleware does ExpandEnv
			Permissions: []Permission{PermRead, PermWrite, PermDelete},
		})
	}
	keys = append(keys, c.Keys...)
	return keys
}

// CORSConfig holds CORS configuration for the API
type CORSConfig struct {
	Origins []string `yaml:"origins,omitempty"` // Allowed origins (e.g., ["http://localhost:3000", "*"])
}

// RateLimitConfig holds rate limiting configuration for the API
type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second,omitempty"` // Rate limit in requests per second (default: 10)
	Burst             int     `yaml:"burst,omitempty"`               // Burst size (default: 20)
	MaxTrackedIPs     int     `yaml:"max_tracked_ips,omitempty"`     // Maximum unique IPs to track (default: 10000)
}

// GetCORSOrigins returns the configured CORS origins, or nil if not configured
func (c *APIConfig) GetCORSOrigins() []string {
	if c == nil || c.CORS == nil {
		return nil
	}
	return c.CORS.Origins
}

// GetRateLimitRPS returns the rate limit in requests per second (default: 10)
func (c *APIConfig) GetRateLimitRPS() float64 {
	if c == nil || c.RateLimit == nil || c.RateLimit.RequestsPerSecond <= 0 {
		return 10
	}
	return c.RateLimit.RequestsPerSecond
}

// GetRateLimitBurst returns the burst size (default: 20)
func (c *APIConfig) GetRateLimitBurst() int {
	if c == nil || c.RateLimit == nil || c.RateLimit.Burst <= 0 {
		return 20
	}
	return c.RateLimit.Burst
}

// GetMaxTrackedIPs returns the maximum number of unique IPs tracked by the rate limiter (default: 10000)
func (c *APIConfig) GetMaxTrackedIPs() int {
	if c == nil || c.RateLimit == nil || c.RateLimit.MaxTrackedIPs <= 0 {
		return 10000
	}
	return c.RateLimit.MaxTrackedIPs
}

// IsAuthEnabled returns true if API authentication is configured.
// It checks the raw config value (not env-expanded) so that a configured
// but missing env var is treated as "auth intended" rather than silently disabled.
func (c *APIConfig) IsAuthEnabled() bool {
	if c == nil || c.Auth == nil {
		return false
	}
	return c.Auth.APIKey != "" || len(c.Auth.Keys) > 0
}

// GetAPIKey returns the configured API key with environment variable expansion
func (c *AuthConfig) GetAPIKey() string {
	if c == nil || c.APIKey == "" {
		return ""
	}
	return os.ExpandEnv(c.APIKey)
}

// GetHeaderName returns the header name for authentication (default: "X-API-Key")
func (c *AuthConfig) GetHeaderName() string {
	if c == nil || c.HeaderName == "" {
		return "X-API-Key"
	}
	return c.HeaderName
}

// IsAPIEnabled returns whether the API is enabled
func (c *Config) IsAPIEnabled() bool {
	return c.API != nil && c.API.Enabled
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Title:       "LiveTemplate Tutorial",
		Description: "Interactive tutorial powered by LiveTemplate",
		Type:        "tutorial", // Default to tutorial mode
		Server: ServerConfig{
			Port:  8080,
			Host:  "localhost",
			Debug: false,
		},
		Styling: StylingConfig{
			Theme:        "clean",
			PrimaryColor: "#007bff",
			Font:         "system-ui",
		},
		Blocks: BlocksConfig{
			AutoID:          true,
			IDFormat:        "kebab-case",
			ShowLineNumbers: true,
		},
		Features: FeaturesConfig{
			HotReload: true,
		},
		Ignore: []string{
			"drafts/**",
			"_*.md",
		},
	}
}

// IsSiteMode returns true if the config is for a multi-page site
func (c *Config) IsSiteMode() bool {
	return c.Type == "site"
}

// IsTutorialMode returns true if the config is for a single tutorial
func (c *Config) IsTutorialMode() bool {
	return c.Type == "tutorial" || c.Type == ""
}

// Load loads configuration from a YAML file
// If the file doesn't exist, returns the default configuration
func Load(configPath string) (*Config, error) {
	// If no config path provided, use default
	if configPath == "" {
		return DefaultConfig(), nil
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	config := DefaultConfig() // Start with defaults
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// LoadFromDir looks for tinkerdown.yaml, lmt.yaml, or livemdtools.yaml in the given directory
// tinkerdown.yaml is checked first, then lmt.yaml (short form), then livemdtools.yaml (legacy)
// If none is found, returns the default configuration
func LoadFromDir(dir string) (*Config, error) {
	// Check for tinkerdown.yaml first (new name)
	tinkerdownPath := filepath.Join(dir, "tinkerdown.yaml")
	if _, err := os.Stat(tinkerdownPath); err == nil {
		return Load(tinkerdownPath)
	}

	// Check for lmt.yaml (short form)
	lmtPath := filepath.Join(dir, "lmt.yaml")
	if _, err := os.Stat(lmtPath); err == nil {
		return Load(lmtPath)
	}

	// Check for livemdtools.yaml (legacy, backwards compatibility)
	configPath := filepath.Join(dir, "livemdtools.yaml")
	return Load(configPath)
}

// Save writes the configuration to a YAML file
func (c *Config) Save(configPath string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
