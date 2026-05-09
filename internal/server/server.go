package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/livetemplate/tinkerdown"
	"github.com/livetemplate/tinkerdown/internal/assets"
	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/diagrams"
	"github.com/livetemplate/tinkerdown/internal/schedule"
	"github.com/livetemplate/tinkerdown/internal/site"
)

// defaultStr returns s when non-empty, otherwise fallback. Local helper
// for log lines where an empty config value should display as the
// runtime-applied default.
func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// Route represents a discovered page route.
type Route struct {
	Pattern  string           // URL pattern (e.g., "/counter")
	FilePath string           // Relative file path (e.g., "counter.md")
	Page     *tinkerdown.Page // Parsed page
}

// Server is the tinkerdown development server.
type Server struct {
	rootDir            string
	config             *config.Config
	routes             []*Route
	siteManager        *site.Manager // For multi-page documentation sites
	mu                 sync.RWMutex
	connections        map[*websocket.Conn]*WebSocketHandler // Track connected WebSocket clients with their handlers
	connMu             sync.RWMutex                          // Separate mutex for connections
	watcher            *Watcher                              // File watcher for live reload
	playground         *PlaygroundHandler                    // Playground for testing AI-generated apps
	apiHandler         *APIHandler                           // REST API handler for sources
	apiRoutes          http.Handler                          // Wrapped API handler with middleware
	webhookHandler     *WebhookHandler                       // Webhook handler for external triggers
	scheduleRunner     *schedule.Runner                      // Schedule runner for timed jobs
	rateLimitCancel    context.CancelFunc                    // Stops the rate limiter cleanup goroutine
	rateLimitDone      <-chan struct{}                        // Closed when rate limiter goroutine exits
	recentSourceWrites map[string]time.Time                  // Files recently written by source actions
	sourceWriteMu      sync.Mutex                            // Protects recentSourceWrites
	proxyRoutes        []*proxyRoute                         // Custom routes that bypass markdown resolution and reverse-proxy to upstream
	csp                string                                // Pre-computed Content-Security-Policy header value (immutable for the server's lifetime)
}

// New creates a new server for the given root directory.
func New(rootDir string) *Server {
	cfg := config.DefaultConfig()
	srv := &Server{
		rootDir:            rootDir,
		config:             cfg,
		routes:             make([]*Route, 0),
		connections:        make(map[*websocket.Conn]*WebSocketHandler),
		recentSourceWrites: make(map[string]time.Time),
		csp:                buildCSP(cfg.Security.FrameSrc),
	}
	srv.playground = NewPlaygroundHandler(srv)
	return srv
}

// NewWithConfig creates a new server with a specific configuration.
func NewWithConfig(rootDir string, cfg *config.Config) *Server {
	srv := &Server{
		rootDir:            rootDir,
		config:             cfg,
		routes:             make([]*Route, 0),
		connections:        make(map[*websocket.Conn]*WebSocketHandler),
		recentSourceWrites: make(map[string]time.Time),
		csp:                buildCSP(cfg.Security.FrameSrc),
	}

	// Initialize site manager if in site mode
	if cfg.IsSiteMode() {
		srv.siteManager = site.New(rootDir, cfg)
	}

	// Wire up server-side diagram pre-rendering (G16) when enabled. Done
	// BEFORE Discover() so the parser sees the renderer on first parse.
	// Setter is idempotent — re-init or hot-reload paths can call again.
	if cfg.Features.PrerenderDiagrams {
		tinkerdown.SetMermaidRenderer(diagrams.NewKrokiMermaid(cfg.Features.KrokiURL))
		log.Printf("[Diagrams] mermaid pre-render enabled (Kroki=%s)",
			defaultStr(cfg.Features.KrokiURL, "https://kroki.io"))
	}

	// Build reverse-proxy routes from config. Bad entries are logged but
	// do not abort startup — operator-friendly behaviour for misconfig.
	if len(cfg.Routes) > 0 {
		routes, errs := buildProxyRoutes(cfg.Routes)
		srv.proxyRoutes = routes
		for _, err := range errs {
			log.Printf("[Routes] skipping invalid route: %v", err)
		}
		log.Printf("[Routes] loaded %d proxy route(s)", len(routes))
	}

	// Initialize playground handler
	srv.playground = NewPlaygroundHandler(srv)

	// Initialize API handler if enabled
	if cfg.IsAPIEnabled() {
		srv.apiHandler = NewAPIHandler(cfg, rootDir)

		// Wrap API handler with middleware
		// Order (outer to inner): SecurityHeaders → CORS → RateLimit → Auth → MethodPerm → Handler
		var handler http.Handler = srv.apiHandler

		// Apply auth + authorization middleware (innermost)
		if cfg.API.IsAuthEnabled() {
			handler = MethodPermissionMiddleware()(handler)
			handler = AuthMiddleware(cfg.API.Auth)(handler)
		}

		// Apply per-IP rate limiting (context controls cleanup goroutine lifetime)
		rateLimitCtx, rateLimitCancel := context.WithCancel(context.Background())
		srv.rateLimitCancel = rateLimitCancel
		rateLimitMW, rateLimitDone := RateLimitMiddleware(
			rateLimitCtx,
			cfg.API.GetRateLimitRPS(),
			cfg.API.GetRateLimitBurst(),
			cfg.API.GetMaxTrackedIPs(),
		)
		srv.rateLimitDone = rateLimitDone
		handler = rateLimitMW(handler)

		// Apply CORS middleware (pass auth header name for preflight)
		authHeader := ""
		if cfg.API.Auth != nil {
			authHeader = cfg.API.Auth.GetHeaderName()
		}
		handler = CORSMiddleware(cfg.API.GetCORSOrigins(), authHeader)(handler)

		// Apply security headers (outermost)
		handler = SecurityHeadersMiddleware(cfg.Security.FrameSrc)(handler)

		srv.apiRoutes = handler

		log.Printf("[API] Middleware: auth=%v cors=%v rate_limit=%.1f rps (burst %d)",
			cfg.API.IsAuthEnabled(),
			len(cfg.API.GetCORSOrigins()) > 0,
			cfg.API.GetRateLimitRPS(),
			cfg.API.GetRateLimitBurst())
	}

	// Initialize webhook handler if webhooks are configured
	if len(cfg.Webhooks) > 0 {
		srv.webhookHandler = NewWebhookHandler(cfg, rootDir, srv.executeWebhookAction)
	}

	// Initialize schedule runner for timed jobs
	srv.scheduleRunner = schedule.NewRunner(schedule.RunnerConfig{})
	srv.scheduleRunner.SetActionHandler(srv.executeScheduledAction)
	srv.scheduleRunner.SetNotificationHandler(srv.handleScheduledNotification)

	return srv
}

// executeScheduledAction handles action execution triggered by schedules.
func (s *Server) executeScheduledAction(pageID, actionName string, args []string, message string) error {
	if s.config == nil || s.config.Actions == nil {
		return fmt.Errorf("no actions configured")
	}

	action, exists := s.config.Actions[actionName]
	if !exists || action == nil {
		return fmt.Errorf("action %q not found", actionName)
	}

	log.Printf("[Schedule] Executing action %q from page %s", actionName, pageID)

	// Create an executor for the scheduled action
	executor := newWebhookActionExecutor(s.config, s.rootDir)
	params := make(map[string]interface{})
	// Parse args into params (key=value pairs)
	for _, arg := range args {
		if idx := strings.Index(arg, "="); idx > 0 {
			params[arg[:idx]] = arg[idx+1:]
		} else {
			log.Printf("[Schedule] Warning: skipping invalid arg format %q (expected key=value)", arg)
		}
	}
	// Add message to params for use in action templates (e.g., {{.Message}})
	if message != "" {
		params["Message"] = message
	}
	return executor.execute(action, params)
}

// handleScheduledNotification handles notifications triggered by schedules.
func (s *Server) handleScheduledNotification(pageID, message string) error {
	log.Printf("[Schedule] Notification from page %s: %s", pageID, message)
	// In headless mode, notifications are logged. In future, could send to webhook/API.
	return nil
}

// Discover scans the directory for .md files and creates routes.
func (s *Server) Discover() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.routes = make([]*Route, 0)

	// Use site manager for site mode
	if s.siteManager != nil {
		if err := s.siteManager.Discover(); err != nil {
			return err
		}

		// Convert PageNodes to Routes
		for _, pageNode := range s.siteManager.AllPages() {
			route := &Route{
				Pattern:  pageNode.Path,
				FilePath: pageNode.FilePath,
				Page:     pageNode.Page,
			}
			s.routes = append(s.routes, route)
		}

		// Sort routes
		sortRoutes(s.routes)

		// Parse schedules from all discovered pages
		s.parseSchedulesFromRoutes()

		// Merge auto-proxy routes from embed-lvt blocks (same as the
		// legacy tutorial-mode path below).
		s.registerAutoEmbedRoutes()
		return nil
	}

	// Legacy tutorial mode discovery
	err := filepath.WalkDir(s.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			// Skip directories starting with _ or .
			name := d.Name()
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .md files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(s.rootDir, path)
		if err != nil {
			return err
		}

		// Skip files in _ directories
		if strings.Contains(relPath, "/_") || strings.HasPrefix(relPath, "_") {
			return nil
		}

		// Generate route pattern
		pattern := mdToPattern(relPath)

		// Parse the page
		page, err := tinkerdown.ParseFileInSite(path, s.rootDir)
		if err != nil {
			log.Printf("Warning: Failed to parse %s: %v", relPath, err)
			return nil // Continue with other files
		}

		route := &Route{
			Pattern:  pattern,
			FilePath: relPath,
			Page:     page,
		}

		s.routes = append(s.routes, route)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// Sort routes (index routes first)
	sortRoutes(s.routes)

	// Merge auto-proxy routes declared by embed-lvt blocks into the
	// runtime proxy table. Authors can write `embed-lvt path="/x/"
	// upstream="http://..."` and tinkerdown wires up the reverse-proxy
	// without a separate `routes:` entry in tinkerdown.yaml.
	s.registerAutoEmbedRoutes()

	// Make the watcher fire on changes to files referenced by any
	// page's `LANG include="..."` fences, so docs stay in sync with
	// the cited source. Idempotent across re-discovery.
	s.refreshWatcherIncludes()

	// Parse schedules from all discovered pages
	s.parseSchedulesFromRoutes()

	return nil
}

// refreshWatcherIncludes pushes the union of every page's
// IncludedFiles into the watcher's extra-paths set. Caller must hold
// s.mu (Discover already does).
func (s *Server) refreshWatcherIncludes() {
	if s.watcher == nil {
		return
	}
	pages := s.collectPagesForAutoRoutes()
	seen := map[string]struct{}{}
	var paths []string
	for _, page := range pages {
		for _, f := range page.IncludedFiles {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			paths = append(paths, f)
		}
	}
	s.watcher.SetExtraFiles(paths)
}

// parseSchedulesFromRoutes registers schedules from all discovered pages.
// Pages already have their schedules parsed during page.go parsing phase.
func (s *Server) parseSchedulesFromRoutes() {
	if s.scheduleRunner == nil {
		return
	}

	totalSchedules := 0
	for _, route := range s.routes {
		if route.Page != nil && len(route.Page.Imperatives) > 0 {
			// Register each imperative with the schedule runner
			for i, imp := range route.Page.Imperatives {
				if imp.Token == nil {
					continue // No schedule token, skip
				}

				jobID := fmt.Sprintf("%s:%d", route.Pattern, i)
				job := &schedule.Job{
					ID:      jobID,
					PageID:  route.Pattern,
					Line:    imp.Raw,
					Token:   imp.Token,
					Handler: s.createScheduleHandler(route.Pattern, imp),
				}
				s.scheduleRunner.AddJob(job)
				totalSchedules++
			}
		}
	}

	if totalSchedules > 0 {
		log.Printf("[Schedule] Registered %d scheduled jobs from pages", totalSchedules)
	}
}

// createScheduleHandler creates a handler function for a scheduled job.
func (s *Server) createScheduleHandler(pageID string, imp *schedule.Imperative) schedule.JobHandler {
	return func(job *schedule.Job) error {
		switch imp.Type {
		case schedule.ImperativeNotify:
			return s.handleScheduledNotification(pageID, imp.Message)
		case schedule.ImperativeRunAction:
			return s.executeScheduledAction(pageID, imp.ActionName, imp.Args, imp.Message)
		}
		return nil
	}
}

// AddJob adds a job to the schedule runner (exposed for external registration).
func (s *Server) AddJob(job *schedule.Job) {
	if s.scheduleRunner != nil {
		s.scheduleRunner.AddJob(job)
	}
}

// Routes returns the discovered routes.
func (s *Server) Routes() []*Route {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.routes
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Custom proxy routes match BEFORE we set our own security headers,
	// so the upstream owns its full response surface (CSP, cookies, frame
	// policy). Without this, our X-Frame-Options/CSP would override the
	// upstream's intent — which breaks WebSocket apps that need their own
	// connect-src and frame settings.
	//
	// Reverse-proxied responses also bypass our gzip middleware: upstreams
	// own their own Content-Encoding/Content-Length, and double-encoding
	// breaks browsers (Chrome reports ERR_CONTENT_DECODING_FAILED).
	for _, pr := range s.proxyRoutes {
		if pr.matches(r.URL.Path) {
			rw := w
			if u, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
				rw = u.Unwrap()
				rw.Header().Del("Content-Encoding")
				rw.Header().Del("Vary")
			}
			pr.handler.ServeHTTP(rw, r)
			return
		}
	}

	// Security headers applied via SecurityHeadersMiddleware on API routes.
	// Apply same headers to all other routes for consistency.
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", s.csp)

	// Strip the configured version prefix (if any) so the rest of routing
	// can be prefix-agnostic. With prefix unset this is a no-op. Both
	// Path and RawPath must be stripped so downstream handlers (e.g. the
	// reverse-proxy Director, which preserves RawPath verbatim) don't
	// see an out-of-sync original-prefixed path.
	r.URL.Path = stripVersionPrefix(r.URL.Path, s.config.VersionPrefix)
	if r.URL.RawPath != "" {
		r.URL.RawPath = stripVersionPrefix(r.URL.RawPath, s.config.VersionPrefix)
	}

	// Health endpoint (always available, especially important in headless mode)
	if r.URL.Path == "/health" {
		s.serveHealth(w, r)
		return
	}

	// SEO endpoints (always available; sitemap returns 404 in headless mode
	// since there are no pages to enumerate, but robots.txt is still useful).
	if r.URL.Path == "/sitemap.xml" {
		s.serveSitemap(w, r)
		return
	}
	if r.URL.Path == "/robots.txt" {
		s.serveRobots(w, r)
		return
	}

	// Serve REST API endpoints
	if strings.HasPrefix(r.URL.Path, "/api/sources/") && s.apiRoutes != nil {
		s.apiRoutes.ServeHTTP(w, r)
		return
	}

	// Serve webhook endpoints
	if strings.HasPrefix(r.URL.Path, "/webhook/") && s.webhookHandler != nil {
		s.webhookHandler.ServeHTTP(w, r)
		return
	}

	// In headless mode, only API, webhooks, and health are available
	if s.config.Features.Headless {
		http.Error(w, "Not found (headless mode - only /health, /api/*, and /webhook/* are available)", http.StatusNotFound)
		return
	}

	// --- Web UI routes (not available in headless mode) ---

	// Serve WebSocket endpoint
	if r.URL.Path == "/ws" {
		s.serveWebSocket(w, r)
		return
	}

	// Serve search index for site mode
	if r.URL.Path == "/search-index.json" && s.siteManager != nil {
		s.serveSearchIndex(w, r)
		return
	}

	// Serve assets
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		s.serveAsset(w, r)
		return
	}

	// Serve playground routes
	if r.URL.Path == "/playground" {
		s.playground.ServePlaygroundPage(w, r)
		return
	}
	if r.URL.Path == "/playground/render" {
		s.playground.HandleRender(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/playground/preview/") {
		s.playground.HandlePreview(w, r)
		return
	}
	if r.URL.Path == "/playground/ws" {
		s.playground.HandlePreviewWS(w, r)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find matching route
	for _, route := range s.routes {
		if route.Pattern == r.URL.Path {
			s.servePage(w, r, route)
			return
		}
	}

	// No route found - redirect to first available route instead of 404
	if len(s.routes) > 0 {
		http.Redirect(w, r, s.routes[0].Pattern, http.StatusSeeOther)
	} else {
		http.Error(w, "No pages available", http.StatusNotFound)
	}
}

// serveHealth handles the /health endpoint.
func (s *Server) serveHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Include basic health info
	response := map[string]interface{}{
		"status":   "healthy",
		"headless": s.config.Features.Headless,
	}

	// Add schedule count if available
	if s.scheduleRunner != nil {
		jobs := s.scheduleRunner.GetAllJobs()
		response["schedules"] = len(jobs)
	}

	// Add webhook count
	if s.config.Webhooks != nil {
		response["webhooks"] = len(s.config.Webhooks)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[Health] Error encoding health response: %v", err)
	}
}

// serveWebSocket handles WebSocket connections for interactive blocks.
func (s *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get the page from query parameter
	pagePath := r.URL.Query().Get("page")
	if pagePath == "" {
		pagePath = "/" // Default to home page
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.routes) == 0 {
		http.Error(w, "No pages available", http.StatusNotFound)
		return
	}

	// Find the route matching the requested page
	var route *Route
	for _, rt := range s.routes {
		if rt.Pattern == pagePath {
			route = rt
			break
		}
	}

	// Fall back to first route if page not found
	if route == nil {
		log.Printf("[WS] Page %q not found, falling back to first route", pagePath)
		route = s.routes[0]
	}

	log.Printf("[WS] WebSocket connection for page: %s (pattern: %s)", pagePath, route.Pattern)

	// Create a new WebSocketHandler instance for this connection.
	// NOTE: Each WebSocket connection (e.g., each browser tab) gets its own
	// handler with isolated state. Interactive state is intentionally NOT
	// synchronized across multiple connections to the same page.
	wsHandler := NewWebSocketHandler(route.Page, s, true, s.rootDir, s.config)
	wsHandler.ServeHTTP(w, r)
}

// serveAsset serves embedded client assets.
func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/assets/")

	// Serve client JS
	if path == "tinkerdown-client.js" {
		js, err := assets.GetClientJS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(js)
		return
	}

	// Serve client CSS
	if path == "tinkerdown-client.css" {
		css, err := assets.GetClientCSS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Write(css)
		return
	}

	// Serve Prism.js core
	if path == "prism.js" {
		js, err := assets.GetPrismJS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(js)
		return
	}

	// Serve Prism.js CSS theme
	if path == "prism.css" {
		css, err := assets.GetPrismCSS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(css)
		return
	}

	// Serve Prism Line Highlight plugin (used by `LANG include="..."
	// highlight="3-5"` blocks). Must be registered BEFORE the
	// generic prism-LANG.js handler since "line-highlight" isn't a
	// language whitelisted by GetPrismLanguage.
	if path == "prism-line-highlight.js" {
		js, err := assets.GetPrismLineHighlightJS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(js)
		return
	}
	if path == "prism-line-highlight.css" {
		css, err := assets.GetPrismLineHighlightCSS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(css)
		return
	}
	if path == "prism-line-numbers.js" {
		js, err := assets.GetPrismLineNumbersJS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(js)
		return
	}
	if path == "prism-line-numbers.css" {
		css, err := assets.GetPrismLineNumbersCSS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(css)
		return
	}

	// Serve Prism language components
	if strings.HasPrefix(path, "prism-") && strings.HasSuffix(path, ".js") {
		lang := strings.TrimSuffix(strings.TrimPrefix(path, "prism-"), ".js")
		js, err := assets.GetPrismLanguage(lang)
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(js)
		return
	}

	// Serve Mermaid.js
	if path == "mermaid.js" {
		js, err := assets.GetMermaidJS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(js)
		return
	}

	// Serve Chart.js
	if path == "chart.js" {
		js, err := assets.GetChartJS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(js)
		return
	}

	// Serve Pico CSS
	if path == "pico.css" {
		css, err := assets.GetPicoCSS()
		if err != nil {
			http.Error(w, "Asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Write(css)
		return
	}

	http.NotFound(w, r)
}

// serveSearchIndex serves the search index JSON for site mode
func (s *Server) serveSearchIndex(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.siteManager == nil {
		http.NotFound(w, r)
		return
	}

	// Generate search index
	searchIndex := s.siteManager.GenerateSearchIndex()

	// Return as JSON
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache") // Don't cache during development

	if err := json.NewEncoder(w).Encode(searchIndex); err != nil {
		http.Error(w, "Failed to encode search index", http.StatusInternalServerError)
		return
	}
}

// servePage serves a page.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request, route *Route) {
	// For now, just serve the static HTML
	// TODO: Add WebSocket support for interactivity
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// HTML pages are dynamic by nature: site config, frontmatter, and
	// rendered partials change without a URL change, so a stale cached
	// HTML response paired with fresh fingerprinted assets produces
	// hard-to-explain "first paint missing styles" / "refresh fixes it"
	// breakage. Force every navigation to revalidate; assets keep their
	// long max-age=31536000 because their URLs change on rebuild.
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	html := s.renderPage(route.Page, r.URL.Path, r.Host, detectWSScheme(r))
	// Resolve embed-lvt placeholders against their upstream LiveTemplate
	// apps. Runs per-request so each visitor gets a fresh upstream session
	// and forwards their cookies/auth context.
	html = tinkerdown.ProcessEmbedLvt(html, r)
	w.Write([]byte(html))
}

// detectWSScheme returns the appropriate WebSocket URL scheme ("wss"
// or "ws") for the request. A page served over HTTPS must connect to
// a wss:// endpoint or browsers reject the upgrade as mixed-content,
// silently breaking every interactive block on the page.
//
// We trust X-Forwarded-Proto from the edge first (fly, cloudflare,
// generic reverse proxies all set it on TLS-terminated requests),
// then fall back to r.TLS for direct HTTPS servers, then "ws".
func detectWSScheme(r *http.Request) string {
	// When X-Forwarded-Proto is present, it is authoritative — the
	// proxy knows what scheme the client actually spoke. We must NOT
	// fall through to the r.TLS check because a TLS-terminated server
	// behind a proxy that talks plain http to its backend would then
	// emit wss:// for clients that connected over plain http.
	//
	// EqualFold defends against proxies that send "HTTPS" / "Https";
	// net/http canonicalises header *names* but not *values*.
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		if strings.EqualFold(proto, "https") {
			return "wss"
		}
		return "ws"
	}
	if r.TLS != nil {
		return "wss"
	}
	return "ws"
}

// renderPage renders a page to HTML. wsScheme should be "ws" for
// http-served pages and "wss" for https-served pages — see
// detectWSScheme. Passing the wrong scheme silently breaks every
// interactive block under HTTPS via mixed-content rejection.
func (s *Server) renderPage(page *tinkerdown.Page, currentPath string, host string, wsScheme string) string {
	// Render code blocks with metadata for client discovery
	content := s.renderContent(page)

	// User-config styling overrides (primary color, font) and the initial
	// theme selection for visitors with no localStorage value.
	stylingOverrideCSS := buildStylingOverrideCSS(s.config.Styling)
	defaultTheme := themeDefault(s.config.Styling)

	// Determine effective sidebar setting (page-level overrides site-level)
	showSidebar := s.config.Features.Sidebar
	if page.Sidebar != nil {
		showSidebar = *page.Sidebar
	}

	// Render navigation sidebar if enabled in config
	sidebar := ""
	if s.siteManager != nil && showSidebar {
		sidebar = s.renderSidebar(currentPath)
	}

	// Render breadcrumbs and prev/next for site mode
	breadcrumbsHTML := ""
	prevNextHTML := ""
	if s.siteManager != nil {
		breadcrumbsHTML = s.renderBreadcrumbs(currentPath)
		prevNextHTML = s.renderPrevNext(currentPath)
	}

	// Build the "Edit this page on GitHub" footer link, if configurable.
	// Page frontmatter (source_repo + source_path) wins over site repo.
	editLinkHTML := ""
	siteRepo := ""
	if s.config.Site != nil {
		siteRepo = s.config.Site.Repository
	}
	pageRelPath := ""
	if s.siteManager != nil {
		if node, ok := s.siteManager.GetPage(currentPath); ok && node != nil {
			pageRelPath = node.FilePath
		}
	}
	if editURL := buildEditURL(siteRepo, page.SourceRepo, page.SourcePath, pageRelPath); editURL != "" {
		editLinkHTML = fmt.Sprintf(
			`<div class="page-edit-link"><a href="%s" target="_blank" rel="noopener">Edit this page on GitHub</a></div>`,
			editURL,
		)
	}

	// Wrap content with breadcrumbs and prev/next
	contentWithNav := fmt.Sprintf(`
		%s
		<div class="content-wrapper">
			%s
		</div>
		%s
		%s
	`, breadcrumbsHTML, content, editLinkHTML, prevNextHTML)

	// Build WebSocket URL from host with page path for multi-page routing.
	// Scheme MUST match the page's request scheme — see detectWSScheme.
	wsURL := fmt.Sprintf("%s://%s/ws?page=%s", wsScheme, host, url.QueryEscape(currentPath))

	// Build per-page SEO meta tags (OG, Twitter card, description, canonical).
	// Falls back to site-level config when frontmatter fields are absent.
	seoTags := s.buildSEOTags(page, currentPath)

	// Conditionally include Mermaid (~3.3MB) only when the page has
	// ```mermaid fenced blocks. HasMermaid is set by the parser AST walk.
	mermaidScript := ""
	if page.HasMermaid {
		mermaidScript = `
    <!-- Mermaid.js for diagrams (embedded) -->
    <script defer src="/assets/mermaid.js"></script>
    <script>
        document.addEventListener('DOMContentLoaded', function() {
            mermaid.initialize({
                startOnLoad: false,
                theme: document.documentElement.classList.contains('theme-dark') ? 'dark' : 'default',
                flowchart: { useMaxWidth: true, htmlLabels: true, curve: 'basis' },
                sequence: {
                    diagramMarginX: 50, diagramMarginY: 10, actorMargin: 50,
                    width: 150, height: 65, boxMargin: 10, boxTextMargin: 5,
                    noteMargin: 10, messageMargin: 35, mirrorActors: true, useMaxWidth: true
                }
            });

            // Convert ` + "`" + `` + "`" + `` + "`" + `mermaid code blocks into rendered diagrams.
            document.querySelectorAll('code.language-mermaid').forEach(function(codeBlock) {
                const mermaidDiv = document.createElement('div');
                mermaidDiv.className = 'mermaid';
                mermaidDiv.textContent = codeBlock.textContent;
                const preBlock = codeBlock.parentElement;
                preBlock.parentNode.replaceChild(mermaidDiv, preBlock);
            });
            mermaid.run();
        });

        document.addEventListener('themeChanged', function(e) {
            mermaid.initialize({ theme: e.detail.theme === 'dark' ? 'dark' : 'default' });
            mermaid.run();
        });
    </script>`
	}

	// Conditionally include Chart.js for pages with chart annotations.
	// defer makes the external script non-render-blocking. The inline init
	// script registers a DOMContentLoaded listener, which fires after defer
	// scripts execute, so Chart will be defined when the listener runs.
	chartScript := ""
	if page.HasCharts {
		chartScript = `
    <!-- Chart.js for data visualization (embedded) -->
    <script defer src="/assets/chart.js"></script>
    <script>
        document.addEventListener('DOMContentLoaded', function() {
            var isDark = document.documentElement.classList.contains('theme-dark');
            var colors = [
                'rgba(54, 162, 235, 0.8)',
                'rgba(255, 99, 132, 0.8)',
                'rgba(75, 192, 192, 0.8)',
                'rgba(255, 205, 86, 0.8)',
                'rgba(153, 102, 255, 0.8)',
                'rgba(255, 159, 64, 0.8)',
                'rgba(201, 203, 207, 0.8)'
            ];
            var borderColors = colors.map(function(c) { return c.replace('0.8', '1'); });

            document.querySelectorAll('.tinkerdown-chart').forEach(function(container) {
                var canvas = container.querySelector('canvas');
                var chartType = container.dataset.chartType || 'bar';
                var chartTitle = container.dataset.chartTitle || '';
                var chartData, opts;
                try {
                    chartData = JSON.parse(container.dataset.chartData || '{}');
                    opts = container.dataset.chartOptions ? JSON.parse(container.dataset.chartOptions) : {};
                } catch(e) {
                    console.warn('[Tinkerdown] Failed to parse chart data:', e);
                    return;
                }

                // Custom colors from frontmatter or defaults
                var chartColors = (opts.colors && opts.colors.length) ? opts.colors : colors;
                var chartBorderColors = (opts.colors && opts.colors.length) ? opts.colors.map(function(c) {
                    return c.startsWith('rgba') ? c.replace(/[\d.]+\)$/, '1)') : c;
                }) : borderColors;

                chartData.datasets.forEach(function(dataset, i) {
                    if (chartType === 'pie' || chartType === 'doughnut') {
                        dataset.backgroundColor = chartColors.slice(0, dataset.data.length);
                        dataset.borderColor = chartBorderColors.slice(0, dataset.data.length);
                    } else {
                        dataset.backgroundColor = chartColors[i % chartColors.length];
                        dataset.borderColor = chartBorderColors[i % chartBorderColors.length];
                    }
                    dataset.borderWidth = 1;
                });

                // Horizontal bar: use indexAxis 'y'
                var indexAxis = (opts.horizontal && chartType === 'bar') ? 'y' : 'x';

                // Legend visibility (default: show)
                var showLegend = (opts.legend === false) ? false : true;

                // Stacking
                var stacked = !!opts.stacked;

                var isPie = (chartType === 'pie' || chartType === 'doughnut');
                var scalesConfig = isPie ? {} : {
                    x: {
                        stacked: stacked,
                        ticks: { color: isDark ? '#b0b0b0' : '#555' },
                        grid: { color: isDark ? '#404040' : '#e1e4e8' }
                    },
                    y: {
                        stacked: stacked,
                        ticks: { color: isDark ? '#b0b0b0' : '#555' },
                        grid: { color: isDark ? '#404040' : '#e1e4e8' }
                    }
                };

                new Chart(canvas, {
                    type: chartType,
                    data: chartData,
                    options: {
                        responsive: true,
                        maintainAspectRatio: true,
                        indexAxis: indexAxis,
                        plugins: {
                            title: {
                                display: !!chartTitle,
                                text: chartTitle,
                                color: isDark ? '#e0e0e0' : '#333'
                            },
                            legend: {
                                display: showLegend,
                                labels: { color: isDark ? '#e0e0e0' : '#333' }
                            }
                        },
                        scales: scalesConfig
                    }
                });
            });
        });

        document.addEventListener('themeChanged', function(e) {
            document.querySelectorAll('.tinkerdown-chart canvas').forEach(function(canvas) {
                var chart = Chart.getChart(canvas);
                if (chart) {
                    var isDark = e.detail.theme === 'dark';
                    chart.options.plugins.title.color = isDark ? '#e0e0e0' : '#333';
                    chart.options.plugins.legend.labels.color = isDark ? '#e0e0e0' : '#333';
                    if (chart.options.scales && chart.options.scales.x) {
                        chart.options.scales.x.ticks.color = isDark ? '#b0b0b0' : '#555';
                        chart.options.scales.x.grid.color = isDark ? '#404040' : '#e1e4e8';
                    }
                    if (chart.options.scales && chart.options.scales.y) {
                        chart.options.scales.y.ticks.color = isDark ? '#b0b0b0' : '#555';
                        chart.options.scales.y.grid.color = isDark ? '#404040' : '#e1e4e8';
                    }
                    chart.update();
                }
            });
        });
    </script>`
	}

	// Conditionally include Prism Line Numbers and Line Highlight plugins
	// (~30 KB combined) only on pages that have `include=` blocks.
	// Pages without source-include snippets pay no cost.
	prismIncludeCSS := ""
	prismIncludeJS := ""
	if len(page.IncludedFiles) > 0 {
		prismIncludeCSS = `<link href="/assets/prism-line-highlight.css" rel="stylesheet" />
    <link href="/assets/prism-line-numbers.css" rel="stylesheet" />`
		prismIncludeJS = `<script defer src="/assets/prism-line-highlight.js"></script>
    <script defer src="/assets/prism-line-numbers.js"></script>`
	}

	// Basic HTML wrapper with the static content
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="tinkerdown-ws-url" content="%s">
    <meta name="tinkerdown-debug" content="true">
    <meta name="tinkerdown-sidebar" content="%t">
    <title>%s</title>%s
    <!-- PicoCSS - Semantic/Classless CSS Framework (embedded) -->
    <link rel="stylesheet" href="/assets/pico.css">
    <link rel="stylesheet" href="/assets/tinkerdown-client.css">
    <style>
        /* Theme Variables */
        :root {
            --bg-primary: #ffffff;
            --bg-secondary: linear-gradient(135deg, #f5f7fa 0%%, #e8ecf1 100%%);
            --text-primary: #333;
            --text-secondary: #555;
            --text-heading: #2c3e50;
            --border-color: #e1e4e8;
            --code-bg: #f4f4f4;
            --code-border: #e1e4e8;
            --pre-bg: #282c34;
            --pre-text: #abb2bf;
            --card-bg: #ffffff;
            --card-border: rgba(0,0,0,0.06);
            --card-shadow: rgba(0,0,0,0.08);
            --accent: #0066cc;

            /* PicoCSS size overrides - reduce by ~25%% */
            --pico-font-size: 87.5%%;
            --pico-spacing: 0.75rem;
            --pico-form-element-spacing-vertical: 0.5rem;
            --pico-form-element-spacing-horizontal: 0.75rem;
        }

        [data-theme="dark"] {
            --bg-primary: #1a1a1a;
            --bg-secondary: linear-gradient(135deg, #1a1a1a 0%%, #2d2d2d 100%%);
            --text-primary: #e0e0e0;
            --text-secondary: #b0b0b0;
            --text-heading: #f0f0f0;
            --border-color: #404040;
            --code-bg: #2d2d2d;
            --code-border: #404040;
            --pre-bg: #1e1e1e;
            --pre-text: #d4d4d4;
            --card-bg: #242424;
            --card-border: rgba(255,255,255,0.1);
            --card-shadow: rgba(0,0,0,0.3);
            --accent: #4da6ff;
        }

        /* Theme transition */
        * {
            transition: background-color 0.3s ease, color 0.3s ease, border-color 0.3s ease;
        }

        /* Base styles */
        * {
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.7;
            max-width: 1200px;
            margin: 0 auto;
            padding: 2rem 1.5rem;
            color: var(--text-primary);
            background: var(--bg-secondary);
            min-height: 100vh;
        }

        /* Content wrapper for readable line lengths */
        .content-wrapper {
            max-width: 900px;
            margin: 0 auto;
            padding: 2rem 3rem;
        }

        /* Typography */
        h1, h2, h3 {
            color: var(--text-heading);
            letter-spacing: -0.02em;
            line-height: 1.4;
        }

        h1 {
            font-size: 2.25rem !important;
            font-weight: 900 !important;
            margin-bottom: 1.5rem !important;
            margin-top: 0 !important;
            padding-bottom: 0.75rem !important;
            border-bottom: 3px solid var(--accent) !important;
            color: var(--text-heading) !important;
            line-height: 1.2 !important;
            letter-spacing: -0.025em !important;
        }

        /* Subtitle/description that follows H1 */
        h1 + p {
            font-size: 1rem;
            margin-top: 0;
            padding-top: 1rem;
            margin-bottom: 2rem;
            color: var(--text-secondary);
            line-height: 1.6;
        }

        h2 {
            font-size: 1.4rem !important;
            font-weight: 700 !important;
            margin-top: 2.5rem !important;
            margin-bottom: 1rem !important;
            padding-bottom: 0.5rem !important;
            border-bottom: 2px solid var(--border-color) !important;
            color: var(--text-heading) !important;
            line-height: 1.3 !important;
            letter-spacing: -0.02em !important;
        }

        h3 {
            font-size: 1.125rem !important;
            font-weight: 700 !important;
            margin-top: 1.75rem !important;
            margin-bottom: 0.75rem !important;
            color: var(--text-heading) !important;
            line-height: 1.4 !important;
        }

        /* Space content after H2 to prevent sticking to border */
        h2 + p, h2 + ul, h2 + ol, h2 + pre, h2 + h3 {
            padding-top: 1rem;
        }

        /* Space content after H3 */
        h3 + p, h3 + ul, h3 + ol, h3 + pre {
            padding-top: 0.5rem;
        }

        p {
            margin-bottom: 1.25rem;
            margin-top: 0;
            color: var(--text-secondary);
            line-height: 1.7;
        }

        /* Better list styling */
        ul, ol {
            margin: 1rem 0 1.5rem 0;
            padding-left: 1.5rem;
        }

        li {
            margin-bottom: 0.5rem;
            line-height: 1.7;
            color: var(--text-secondary);
        }

        li:last-child {
            margin-bottom: 0;
        }

        /* Add spacing after lists before next heading */
        ul + h2, ol + h2,
        ul + h3, ol + h3 {
            margin-top: 3rem;
        }

        /* Content links */
        .content-wrapper a:not(.prev-next-link) {
            color: var(--accent);
            text-decoration: none;
            border-bottom: 1px solid rgba(0, 102, 204, 0.3);
            transition: all 0.2s ease;
            font-weight: 500;
        }

        .content-wrapper a:not(.prev-next-link):hover {
            border-bottom-color: var(--accent);
            background: rgba(0, 102, 204, 0.05);
            padding: 0 0.2rem;
            margin: 0 -0.2rem;
        }

        .content-wrapper a:not(.prev-next-link):visited {
            color: rgb(117, 55, 184);
            border-bottom-color: rgba(117, 55, 184, 0.3);
        }

        .content-wrapper a:not(.prev-next-link):visited:hover {
            border-bottom-color: rgb(117, 55, 184);
            background: rgba(117, 55, 184, 0.05);
        }

        [data-theme="dark"] .content-wrapper a:not(.prev-next-link):visited {
            color: rgb(196, 181, 253);
            border-bottom-color: rgba(196, 181, 253, 0.3);
        }

        /* Code blocks */
        code {
            background: var(--code-bg);
            padding: 0.2rem 0.4rem;
            border-radius: 4px;
            font-size: 0.9em;
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            border: 1px solid var(--code-border);
            color: var(--text-primary);
            /* PicoCSS forces inline code to display:inline-block with
             * its own padding, which prevents long inline code (CLI
             * invocations, package paths, long flags) from wrapping at
             * word boundaries — the unbreakable inline-block forces
             * horizontal page overflow on mobile.
             * Restore inline + allow soft-wrap inside long tokens. */
            display: inline;
            overflow-wrap: anywhere;
        }

        pre {
            background: var(--pre-bg);
            color: var(--pre-text);
            padding: 1.25rem 1rem;
            border-radius: 8px;
            overflow-x: auto;
            margin: 1.5rem 0;
            max-width: 100%%;
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
            border: 1px solid var(--border-color);
        }

        pre code {
            background: none;
            border: none;
            padding: 0;
            color: inherit;
        }

        /* Wide tables on narrow viewports get a horizontal scroll
         * container instead of overflowing the page. display: block on
         * the table preserves table-internal cell layout while letting
         * the table itself scroll within its column.
         *
         * Scoped to .content-wrapper (tinkerdown's content container)
         * rather than the semantic article element, which the renderer
         * does not emit — v0.1.10 used the article-prefixed selector
         * and matched nothing on every docs page. */
        .content-wrapper table {
            display: block;
            max-width: 100%%;
            overflow-x: auto;
        }

        /* Interactive blocks */
        .tinkerdown-wasm-block,
        .tinkerdown-interactive-block {
            margin: 2rem 0;
            max-width: 100%%;
            padding: 1.5rem;
            background: var(--card-bg);
            border-radius: 16px;
            box-shadow: 0 4px 16px var(--card-shadow);
            border: 1px solid var(--card-border);
            transition: transform 0.2s ease, box-shadow 0.2s ease;
        }

        /* Bleed pre + interactive blocks beyond the article column on
         * wide screens for visual emphasis. The article column maxes
         * at 800px, so this trick is safe only when the viewport
         * itself is wide enough that the negative margin doesn't pull
         * the box past the viewport edge — below this breakpoint the
         * blocks fit within the article column instead. */
        @media (min-width: 900px) {
            pre,
            .tinkerdown-wasm-block,
            .tinkerdown-interactive-block {
                margin-left: calc((800px - 100%%) / 2 * -1);
                margin-right: calc((800px - 100%%) / 2 * -1);
                max-width: 1000px;
            }
        }

        .tinkerdown-wasm-block:hover,
        .tinkerdown-interactive-block:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 24px var(--card-shadow);
        }

        /* Literate-docs demo wrapper: pairs a template source view with
         * its live interactive block. The wrapper itself is a transparent
         * grouping element; children keep their own card styling but
         * share margins so they read as one demo.
         *
         * We use descendant (not direct-child) selectors because Prism's
         * post-processor wraps the <pre> inside a <div class="code-block-
         * wrapper">, so the immediate child of .tinkerdown-lvt-demo is
         * that wrapper, not the <pre> we authored. */
        .tinkerdown-lvt-demo {
            margin: 2rem 0;
            border: 1px solid var(--card-border);
            border-radius: 16px;
            overflow: hidden;
            background: var(--card-bg);
            box-shadow: 0 4px 16px var(--card-shadow);
        }
        .tinkerdown-lvt-demo .code-block-wrapper,
        .tinkerdown-lvt-demo > pre {
            margin: 0;
            border-radius: 0;
            border: 0;
            border-bottom: 1px solid var(--card-border);
            box-shadow: none;
        }
        .tinkerdown-lvt-demo .tinkerdown-interactive-block,
        .tinkerdown-lvt-demo .tinkerdown-embed-lvt {
            margin: 0;
            border-radius: 0;
            border: 0;
            box-shadow: none;
            background: transparent;
        }
        .tinkerdown-lvt-demo .tinkerdown-interactive-block:hover,
        .tinkerdown-lvt-demo .tinkerdown-embed-lvt:hover {
            transform: none;
            box-shadow: none;
        }
        .tinkerdown-lvt-demo::before {
            content: "Source";
            display: block;
            padding: 0.5rem 1rem;
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: var(--muted-color, #888);
            background: rgba(127, 127, 127, 0.08);
            border-bottom: 1px solid var(--card-border);
        }
        .tinkerdown-lvt-demo .tinkerdown-interactive-block::before,
        .tinkerdown-lvt-demo .tinkerdown-embed-lvt::before {
            content: "Live preview";
            display: block;
            margin-bottom: 0.75rem;
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: var(--muted-color, #888);
            padding: 1rem 1rem 0;
        }
        @media (min-width: 900px) {
            .tinkerdown-lvt-demo {
                margin-left: calc((800px - 100%%) / 2 * -1);
                margin-right: calc((800px - 100%%) / 2 * -1);
                max-width: 1000px;
            }
        }

        /* Footer link below an include snippet — subtle, unobtrusive,
         * sits flush with the code block right edge. */
        .tinkerdown-include-source {
            margin: -0.5rem 0 1.5rem;
            text-align: right;
            font-size: 0.85em;
            color: var(--muted-color, #888);
        }
        .tinkerdown-include-source a {
            color: inherit;
            text-decoration: none;
            border-bottom: 1px dotted currentColor;
        }
        .tinkerdown-include-source a:hover {
            color: var(--primary, #06f);
            border-bottom-color: var(--primary, #06f);
        }

        /* Chart containers */
        .tinkerdown-chart {
            margin: 1.5rem 0;
            padding: 1rem;
            background: var(--card-bg);
            border-radius: 8px;
            border: 1px solid var(--card-border);
            box-shadow: 0 1px 3px var(--card-shadow);
            max-width: 100%%;
        }

        .tinkerdown-chart canvas {
            max-width: 100%%;
            height: auto !important;
        }

        /* Pre-rendered mermaid SVGs (G16) come back from Kroki at native
         * pixel dimensions (e.g. 1681×585 for sequence diagrams). Without
         * a responsive constraint they overflow mobile viewports and
         * iOS Safari auto-scales the WHOLE PAGE down to fit, making text
         * + the hamburger button appear squeezed. Force the SVG to scale
         * down to its container; container itself allows horizontal scroll
         * as a fallback for diagrams too detailed to readably scale. */
        .mermaid-prerendered {
            margin: 1.25rem 0;
            max-width: 100%%;
            overflow-x: auto;
        }
        .mermaid-prerendered svg {
            display: block;
            max-width: 100%%;
            height: auto;
        }
        /* Same defensive sizing for runtime-rendered .mermaid containers
         * (Mermaid 11+ honors useMaxWidth, but pin it explicitly so any
         * overflow path is covered). */
        .mermaid > svg {
            max-width: 100%%;
            height: auto;
        }

        .tinkerdown-chart-table {
            margin: 0.5rem 0 1.5rem;
            font-size: 0.875rem;
        }

        .tinkerdown-chart-table summary {
            cursor: pointer;
            color: var(--text-secondary);
            font-size: 0.8rem;
            padding: 0.25rem 0;
        }

        .tinkerdown-chart-table table {
            margin-top: 0.5rem;
        }

        /* Buttons - Let PicoCSS handle default styling */

        /* Counter display */
        .counter-display {
            font-size: 3rem;
            font-weight: 700;
            text-align: center;
            margin: 2rem 0;
            padding: 1.5rem;
            background: linear-gradient(135deg, #f5f7fa 0%%, #ffffff 100%%);
            border-radius: 16px;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            border: 2px solid #e1e4e8;
        }

        .counter-display.positive {
            color: #10b981;
            border-color: #10b981;
            box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.1);
        }

        .counter-display.negative {
            color: #ef4444;
            border-color: #ef4444;
            box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.1);
        }

        .counter-display.zero {
            color: #6b7280;
            border-color: #d1d5db;
        }

        /* Number transition animation */
        @keyframes numberPulse {
            0%%, 100%% { transform: scale(1); }
            50%% { transform: scale(1.1); }
        }

        .counter-display.changed {
            animation: numberPulse 0.3s ease;
        }

        /* Button groups */
        .button-group {
            display: flex;
            justify-content: center;
            flex-wrap: wrap;
            gap: 0.5rem;
            margin: 1rem 0;
        }

        /* Responsive design */
        @media (max-width: 768px) {
            body {
                padding: 1rem;
            }

            h1 {
                font-size: 1.875rem !important;
                font-weight: 900 !important;
            }

            h2 {
                font-size: 1.25rem !important;
                font-weight: 700 !important;
            }

            h3 {
                font-size: 1rem !important;
                font-weight: 700 !important;
            }

            .tinkerdown-wasm-block,
            .tinkerdown-interactive-block {
                padding: 1rem;
                border-radius: 10px;
            }

            .counter-display {
                font-size: 1.875rem;
                padding: 1rem;
            }
        }

        @media (max-width: 480px) {
            body {
                padding: 0.5rem;
            }

            h1 {
                font-size: 1.5rem !important;
                font-weight: 900 !important;
            }

            h2 {
                font-size: 1.125rem !important;
                font-weight: 700 !important;
            }

            h3 {
                font-size: 1rem !important;
                font-weight: 700 !important;
            }

            .tinkerdown-wasm-block,
            .tinkerdown-interactive-block {
                padding: 1rem;
                margin: 1rem 0;
            }

            .counter-display {
                font-size: 2rem;
                padding: 1rem;
            }

            .button-group {
                flex-direction: column;
            }
        }

        /* Unified Page Toolbar */
        .page-toolbar {
            position: fixed;
            top: 1rem;
            right: 1.5rem;
            z-index: 1000;
            display: flex;
            gap: 0.5rem;
            align-items: center;
            background: var(--card-bg);
            padding: 0.35rem;
            border-radius: 8px;
            box-shadow: 0 2px 8px var(--card-shadow);
            border: 1px solid var(--card-border);
            opacity: 0.6;
            transition: opacity 0.3s ease;
        }

        .page-toolbar:hover {
            opacity: 1;
            box-shadow: 0 4px 12px var(--card-shadow);
        }

        /* Presentation Mode Button */
        .presentation-btn {
            background: transparent;
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            padding: 0.5rem;
            margin: 0;
            border-radius: 6px;
            width: 2.25rem;
            height: 2.25rem;
            display: flex;
            align-items: center;
            justify-content: center;
            cursor: pointer;
            transition: all 0.2s ease;
            flex-shrink: 0;
        }

        .presentation-btn svg {
            width: 1.1rem;
            height: 1.1rem;
            display: block;
        }

        .presentation-btn:hover {
            background: var(--code-bg);
        }

        .presentation-btn.active {
            background: var(--accent);
            color: white;
            border-color: var(--accent);
        }

        .presentation-btn:active {
            transform: scale(0.95);
        }

        /* Theme Toggle */
        .theme-toggle {
            display: flex;
            gap: 0.35rem;
            align-items: center;
        }

        .theme-toggle button {
            background: transparent;
            border: 1px solid var(--border-color);
            color: var(--text-primary);
            padding: 0.5rem;
            margin: 0;
            border-radius: 6px;
            font-size: 1rem;
            width: 2.25rem;
            height: 2.25rem;
            display: flex;
            align-items: center;
            justify-content: center;
            box-shadow: none;
            transition: all 0.2s ease;
            flex-shrink: 0;
        }

        .theme-toggle button svg {
            width: 1.1rem;
            height: 1.1rem;
            display: block;
        }

        .theme-toggle button:hover {
            background: var(--code-bg);
            transform: none;
            box-shadow: none;
        }

        .theme-toggle button.active {
            background: var(--accent);
            color: white;
            border-color: var(--accent);
        }

        .theme-toggle button:active {
            transform: scale(0.95);
        }

        /* Presentation Mode Styles */
        body.presentation-mode {
            margin-left: 0 !important;
            max-width: 100%% !important;
            width: 100%% !important;
            padding: 0 !important;
        }

        body.presentation-mode .tinkerdown-nav-sidebar {
            display: none;
        }

        body.presentation-mode .tinkerdown-nav-bottom {
            left: 0;
            width: 100%%;
        }

        body.presentation-mode .theme-toggle,
        body.presentation-mode .presentation-btn {
            opacity: 0.3;
            transition: opacity 0.3s ease;
        }

        body.presentation-mode .theme-toggle:hover,
        body.presentation-mode .presentation-btn:hover {
            opacity: 1;
        }

        /* Hide all H2 sections except current in presentation mode */
        body.presentation-mode .content-wrapper > * {
            display: none;
        }

        body.presentation-mode .presentation-current-section {
            display: block !important;
        }

        body.presentation-mode .content-wrapper {
            max-width: 100%%;
            width: 100%%;
            padding: 2rem 4rem;
            margin: 0 auto;
        }

        body.presentation-mode h2 {
            font-size: 2.5rem;
            margin-bottom: 2rem;
        }

        body.presentation-mode p,
        body.presentation-mode li {
            font-size: 1.25rem;
            line-height: 1.8;
        }

        body.presentation-mode code {
            font-size: 1.1rem;
        }

        body.presentation-mode pre {
            font-size: 1rem;
        }

        /* Toolbar position in presentation mode - top-right corner */
        body.presentation-mode .page-toolbar {
            position: fixed !important;
            top: 1rem !important;
            right: 1rem !important;
            bottom: auto !important;
            left: auto !important;
            z-index: 1001;
        }

        /* Tutorial Navigation - Sidebar TOC */
        .tinkerdown-nav-sidebar {
            position: fixed;
            left: 0;
            top: 0;
            bottom: 0;
            width: 360px;
            background: var(--card-bg);
            border-right: 1px solid var(--card-border);
            box-shadow: 2px 0 8px var(--card-shadow);
            z-index: 900;
            display: flex;
            flex-direction: column;
            /* Scroll nav vertically when its content exceeds the viewport.
             * The previous setting hid the overflow, which clipped bottom
             * nav items unreachably on a 10+-section sidebar. The contain
             * value on overscroll-behavior keeps nav scroll from chaining
             * into the page when the sidebar reaches its end. */
            overflow-y: auto;
            /* Per CSS spec, setting overflow-y to anything but visible
             * makes the orthogonal axis behave as auto too. Without an
             * explicit overflow-x, content that bleeds slightly past
             * the sidebar's content box (e.g. PicoCSS's nav ul with
             * negative horizontal margins) produces a tiny horizontal
             * scrollbar — making the sidebar "float" horizontally
             * during vertical scroll on touch devices. Clip to the
             * sidebar's column. */
            overflow-x: hidden;
            overscroll-behavior: contain;
        }

        .nav-sidebar-header {
            padding: 1.5rem;
            border-bottom: 1px solid var(--border-color);
            background: var(--bg-secondary);
        }

        .nav-sidebar-header h3 {
            margin: 0;
            font-size: 1rem;
            font-weight: 600;
            color: var(--text-heading);
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        .nav-sidebar-steps {
            flex: 1;
            overflow-y: auto;
            padding: 0;
            margin: 0;
            list-style: none;
            /* Override PicoCSS nav > ol horizontal layout */
            display: flex;
            flex-direction: column;
            flex-wrap: nowrap;
        }

        .nav-step {
            border-bottom: 1px solid var(--border-color);
            /* Override PicoCSS nav > ol > li horizontal layout */
            display: block;
            width: 100%%;
        }

        .nav-step a {
            display: flex;
            align-items: center;
            justify-content: flex-start;
            gap: 1rem;
            padding: 1rem 1.5rem;
            text-decoration: none;
            color: var(--text-secondary);
            transition: all 0.2s ease;
            width: 100%%;
            text-align: left;
        }

        .nav-step:hover a {
            background: var(--code-bg);
            color: var(--text-primary);
        }

        .nav-step.active a {
            background: var(--accent);
            color: white;
            font-weight: 500;
        }

        .step-number {
            display: flex;
            align-items: center;
            justify-content: center;
            width: 28px;
            height: 28px;
            border-radius: 50%%;
            background: var(--code-bg);
            color: var(--text-primary);
            font-size: 0.875rem;
            font-weight: 600;
            flex-shrink: 0;
        }

        .nav-step.active .step-number {
            background: rgba(255, 255, 255, 0.2);
            color: white;
        }

        .step-title {
            flex: 1;
            font-size: 0.9rem;
            line-height: 1.4;
        }

        /* Site Navigation Styles */
        .nav-header {
            padding: 2rem 2.5rem;
            border-bottom: 1px solid var(--border-color);
        }

        .nav-header h2 {
            margin: 0;
            font-size: 1.2rem;
            font-weight: 600;
            color: var(--text-heading);
        }

        .nav-section {
            border-bottom: 1px solid var(--border-color);
        }

        .nav-section-title {
            padding: 1rem 2.5rem;
            font-size: 0.9rem;
            font-weight: 600;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            background: var(--code-bg);
        }

        .nav-pages {
            /* Override PicoCSS nav ul { display: flex } which would otherwise
             * lay out nav items horizontally and crush them in the sidebar. */
            display: block;
            list-style: none;
            margin: 0;
            padding: 0;
        }

        .nav-pages li {
            /* Override PicoCSS nav li { display: inline-block }. The previous
             * fix on .nav-pages only switched the UL to block; PicoCSS still
             * laid out the LIs as inline-block at ~135px each, so they
             * packed horizontally in the sidebar. Force list-item layout. */
            display: list-item;
            margin: 0;
            padding: 0;
            width: auto;
        }

        /* Mobile navigation toggle. Hidden on desktop; appears at <=768px
         * to expose the otherwise-translated-off-screen sidebar. */
        .tinkerdown-nav-toggle {
            display: none;
            position: fixed;
            top: 0.75rem;
            left: 0.75rem;
            z-index: 1100;
            /* Use the brand accent for the background so the hamburger
             * actually contrasts against a white page — the previous
             * white-on-white styling was discoverability-broken. */
            background: var(--accent, #5a67d8);
            color: #fff;
            border: 0;
            border-radius: 8px;
            width: 44px;
            height: 44px;
            padding: 0;
            cursor: pointer;
            box-shadow: 0 4px 10px rgba(0, 0, 0, 0.15);
            align-items: center;
            justify-content: center;
        }
        .tinkerdown-nav-toggle:hover,
        .tinkerdown-nav-toggle:focus-visible {
            filter: brightness(1.1);
            outline: 0;
        }
        .tinkerdown-nav-toggle svg {
            display: block;
            stroke: currentColor;
        }

        /* Backdrop shown behind the open sidebar on mobile so taps outside
         * the menu close it. Click handler attached in the toggle script. */
        .tinkerdown-nav-backdrop {
            display: none;
            position: fixed;
            inset: 0;
            background: rgba(0, 0, 0, 0.4);
            z-index: 850;
        }
        .tinkerdown-nav-backdrop.open {
            display: block;
        }

        .nav-pages li a {
            display: block;
            padding: 0.85rem 2.5rem 0.85rem 3rem;
            color: var(--text-secondary);
            text-decoration: none;
            font-size: 0.95rem;
            transition: all 0.2s ease;
            border-left: 4px solid transparent;
            margin: 2px 0;
        }

        .nav-pages li a:hover {
            background: var(--code-bg);
            color: var(--text-primary);
        }

        .nav-pages li a.active {
            background: rgba(0, 102, 204, 0.1);
            color: var(--accent);
            border-left-color: var(--accent);
            font-weight: 600;
        }

        [data-theme="dark"] .nav-pages li a.active {
            background: rgba(77, 166, 255, 0.15);
        }

        /* Sidebar Footer - for toolbar when inside sidebar */
        .nav-sidebar-footer {
            padding: 1rem;
            border-top: 1px solid var(--border-color);
            background: var(--bg-primary);
            margin-top: auto;
        }

        .nav-sidebar-footer .page-toolbar {
            position: static;
            width: 100%%;
            justify-content: center;
            opacity: 1;
            background: transparent;
            box-shadow: none;
            border: none;
            padding: 0;
        }

        /* Breadcrumbs */
        .breadcrumbs {
            padding: 1rem 0;
            margin-bottom: 1.5rem;
            border-bottom: 1px solid var(--border-color);
        }

        .breadcrumbs ol {
            list-style: none;
            margin: 0;
            padding: 0;
            display: flex;
            align-items: baseline;
            flex-wrap: wrap;
            gap: 0;
        }

        .breadcrumbs li {
            display: inline-flex;
            align-items: baseline;
        }

        .breadcrumbs a,
        .breadcrumbs .current,
        .breadcrumbs .separator {
            font-size: 0.9rem;
            line-height: 1.5;
            vertical-align: baseline;
        }

        .breadcrumbs a {
            color: var(--accent);
            text-decoration: none;
            cursor: pointer;
            transition: all 0.2s ease;
        }

        .breadcrumbs a:hover {
            text-decoration: underline;
        }

        .breadcrumbs .separator {
            color: var(--text-secondary);
            margin: 0 0.5rem;
        }

        .breadcrumbs .current {
            color: var(--text-primary);
            font-weight: 500;
        }

        /* Page Navigation (Prev/Next) */
        .page-nav {
            display: flex;
            justify-content: space-between;
            gap: 1rem;
            margin-top: 3rem;
            padding-top: 2rem;
            border-top: 1px solid var(--border-color);
        }

        .page-nav a {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            padding: 1rem;
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            text-decoration: none;
            color: var(--text-primary);
            transition: all 0.2s ease;
            flex: 1;
            max-width: 45%%;
        }

        .page-nav a:hover {
            background: var(--code-bg);
            border-color: var(--accent);
            transform: translateY(-2px);
            box-shadow: 0 4px 8px var(--card-shadow);
        }

        .page-nav-prev {
            justify-content: flex-start;
        }

        .page-nav-next {
            justify-content: flex-end;
            margin-left: auto;
        }

        .page-nav .arrow {
            font-size: 1.2rem;
            color: var(--accent);
        }

        .page-nav .label {
            font-size: 0.9rem;
            font-weight: 500;
        }

        .page-nav-spacer {
            flex: 1;
        }

        /* Tutorial Navigation - Bottom Bar */
        .tinkerdown-nav-bottom {
            position: fixed;
            bottom: 0;
            left: 360px;
            right: 0;
            height: 60px;
            background: var(--card-bg);
            border-top: 1px solid var(--card-border);
            box-shadow: 0 -2px 8px var(--card-shadow);
            z-index: 900;
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 0 2rem;
            gap: 1rem;
        }

        .nav-btn {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.75rem 1.5rem;
            background: var(--accent);
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 1rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
            box-shadow: 0 2px 8px rgba(0, 102, 204, 0.3);
        }

        .nav-btn:hover:not(:disabled) {
            background: #0052a3;
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(0, 102, 204, 0.4);
        }

        .nav-btn:active:not(:disabled) {
            transform: translateY(0);
        }

        .nav-btn:disabled {
            background: var(--code-bg);
            color: var(--text-secondary);
            cursor: not-allowed;
            box-shadow: none;
        }

        .nav-arrow {
            font-size: 1.2rem;
            line-height: 1;
        }

        .nav-progress {
            font-size: 0.9rem;
            color: var(--text-secondary);
            font-weight: 500;
        }

        .current-step,
        .total-steps {
            color: var(--accent);
            font-weight: 600;
        }

        /* Adjust main content to make room for navigation.
         * width must subtract the sidebar width: body's natural width
         * is 100vw, and margin-left does NOT shrink it (margins live
         * outside the box even with box-sizing: border-box). Without
         * the explicit width, body is 100vw wide AND shifted right by
         * the sidebar width, creating ~360px of horizontal page
         * overflow on desktop and a wide visible gap between the
         * sidebar's right edge and the centered content-wrapper. */
        body:has(.tinkerdown-nav-sidebar) {
            margin-left: 360px;
            margin-right: 0;
            margin-bottom: 60px;
            max-width: none;
            width: calc(100%% - 360px);
        }

        /* Responsive Navigation */
        @media (max-width: 1024px) {
            .tinkerdown-nav-sidebar {
                width: 320px;
            }

            .tinkerdown-nav-bottom {
                left: 320px;
            }

            body:has(.tinkerdown-nav-sidebar) {
                margin-left: 320px;
                width: calc(100%% - 320px);
            }
        }

        @media (max-width: 768px) {
            /* Hide sidebar on mobile, show hamburger menu */
            .tinkerdown-nav-sidebar {
                transform: translateX(-100%%);
                transition: transform 0.3s ease;
            }

            .tinkerdown-nav-sidebar.open {
                transform: translateX(0);
            }

            /* Reveal the toggle button at mobile widths. */
            .tinkerdown-nav-toggle {
                display: flex;
            }

            /* Make room for the fixed-position toggle so it doesn't overlap
             * the page H1. Only applied when a sidebar exists.
             * width: 100%% explicitly resets the desktop calc(100%% - 360px)
             * — without this override, body collapses to 33px on a 393px
             * mobile viewport and content stacks one character per line. */
            body:has(.tinkerdown-nav-sidebar) {
                margin-left: 0;
                padding-top: 3.5rem;
                width: 100%%;
            }

            .tinkerdown-nav-bottom {
                left: 0;
                padding: 0 1rem;
            }

            .nav-btn {
                padding: 0.5rem 1rem;
                font-size: 0.9rem;
            }

            .nav-label {
                display: none;
            }

            .nav-arrow {
                font-size: 1.5rem;
            }

            .nav-progress {
                font-size: 0.85rem;
            }
        }

        @media (max-width: 480px) {
            .tinkerdown-nav-bottom {
                height: 60px;
                padding: 0 0.75rem;
            }

            body:has(.tinkerdown-nav-sidebar) {
                margin-bottom: 60px;
            }

            .nav-btn {
                padding: 0.5rem 0.75rem;
                min-width: 40px;
            }

            .nav-progress {
                font-size: 0.75rem;
            }
        }

        /* Counter Variations Styling */
        .counter-container {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 12px;
            padding: 1.5rem;
            margin: 1rem 0;
        }

        .counter-header {
            display: flex;
            justify-content: space-between;
            margin-bottom: 1rem;
        }

        .bounds-label {
            font-size: 0.875rem;
            color: var(--text-secondary);
            font-weight: 500;
        }

        .counter-display.at-max {
            background: #fef3c7;
            border-color: #f59e0b;
            color: #78350f;
        }

        .counter-display.at-min {
            background: #fee2e2;
            border-color: #ef4444;
            color: #7f1d1d;
        }

        .counter-display.in-range {
            background: var(--accent);
            color: white;
        }

        .bounds-bar {
            width: 100%%;
            height: 6px;
            background: var(--code-bg);
            border-radius: 3px;
            margin-top: 1rem;
            overflow: hidden;
        }

        .bounds-progress {
            height: 100%%;
            background: var(--accent);
            transition: width 0.3s ease;
        }

        /* Step Counter */
        .step-buttons {
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
        }

        .button-row {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .row-label {
            font-weight: 500;
            min-width: 80px;
            color: var(--text-secondary);
        }

        .step-btn {
            flex: 1;
            padding: 0.5rem 1rem;
            font-size: 0.9rem;
        }

        .reset-btn {
            width: 100%%;
            margin-top: 0.5rem;
        }

        /* Dual Counter */
        .dual-counter-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1.5rem;
            margin: 1.5rem 0;
        }

        .dual-counter-item {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 12px;
            padding: 1.5rem;
        }

        .counter-label {
            font-size: 0.875rem;
            font-weight: 600;
            color: var(--text-secondary);
            margin-bottom: 1rem;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        @media (max-width: 768px) {
            .dual-counter-container {
                grid-template-columns: 1fr;
            }
        }

        /* Shopping Cart Product */
        .product-card {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 12px;
            padding: 1.5rem;
            margin: 1.5rem 0;
            box-shadow: 0 2px 8px var(--card-shadow);
        }

        .product-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1rem;
            padding-bottom: 1rem;
            border-bottom: 1px solid var(--border-color);
        }

        .product-header h4 {
            margin: 0;
            color: var(--text-heading);
        }

        .product-price {
            font-size: 1.25rem;
            font-weight: 600;
            color: var(--accent);
        }

        .quantity-selector {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin: 1rem 0;
            padding: 0.75rem;
            background: var(--code-bg);
            border-radius: 8px;
        }

        .quantity-label {
            font-weight: 500;
            color: var(--text-secondary);
        }

        .quantity-controls {
            display: flex;
            align-items: center;
            gap: 1rem;
        }

        .qty-btn {
            width: 36px;
            height: 36px;
            padding: 0;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.25rem;
            border-radius: 50%%;
        }

        .quantity-display {
            min-width: 40px;
            text-align: center;
            font-weight: 600;
            font-size: 1.125rem;
        }

        .product-total {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 1rem;
            margin: 1rem 0;
            background: var(--bg-secondary);
            border-radius: 8px;
        }

        .total-label {
            font-weight: 500;
            color: var(--text-secondary);
        }

        .total-amount {
            font-size: 1.5rem;
            font-weight: 700;
            color: var(--accent);
        }

        .remove-btn {
            background: transparent;
            border: 1px solid #ef4444;
            color: #ef4444;
            box-shadow: none;
        }

        .remove-btn:hover {
            background: #ef4444;
            color: white;
        }

        .removed-message {
            text-align: center;
            padding: 2rem;
            color: var(--text-secondary);
            font-style: italic;
        }

        /* Prism.js syntax highlighting overrides */
        pre[class*="language-"] {
            margin: 1.5rem 0;
            padding: 1rem;
            border-radius: 8px;
            overflow-x: auto;
        }

        code[class*="language-"],
        pre[class*="language-"] {
            font-family: 'Consolas', 'Monaco', 'Andale Mono', 'Ubuntu Mono', monospace;
            font-size: 0.9rem;
            line-height: 1.5;
        }

        /* Ensure code blocks are styled properly */
        :not(pre) > code[class*="language-"] {
            padding: 0.1em 0.3em;
            border-radius: 0.3em;
        }
    </style>
    %s

    <!-- Prism.js for syntax highlighting (embedded) -->
    <link href="/assets/prism.css" rel="stylesheet" />
    %s
</head>
<body>
    <!-- Unified Toolbar -->
    <div class="page-toolbar">
        <!-- Presentation Mode Toggle -->
        <button id="presentation-toggle" class="presentation-btn" title="Toggle presentation mode (F key)" aria-label="Toggle presentation mode">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="2" y="7" width="20" height="15" rx="2" ry="2"/>
                <polyline points="17 2 12 7 7 2"/>
            </svg>
        </button>

        <!-- Theme Toggle -->
        <div class="theme-toggle">
            <button id="theme-light" title="Light theme" aria-label="Light theme">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="12" cy="12" r="5"/>
                    <line x1="12" y1="1" x2="12" y2="3"/>
                    <line x1="12" y1="21" x2="12" y2="23"/>
                    <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/>
                    <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
                    <line x1="1" y1="12" x2="3" y2="12"/>
                    <line x1="21" y1="12" x2="23" y2="12"/>
                    <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
                    <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
                </svg>
            </button>
            <button id="theme-dark" title="Dark theme" aria-label="Dark theme">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
                </svg>
            </button>
            <button id="theme-auto" title="Auto theme (system preference)" aria-label="Auto theme">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
                    <line x1="8" y1="21" x2="16" y2="21"/>
                    <line x1="12" y1="17" x2="12" y2="21"/>
                </svg>
            </button>
        </div>
    </div>

    %s
    %s

    <script>
        // Theme management
        (function() {
            const STORAGE_KEY = 'tinkerdown-theme';
            const html = document.documentElement;

            // Get current theme from localStorage or default from server config.
            function getStoredTheme() {
                return localStorage.getItem(STORAGE_KEY) || %q;
            }

            // Get system preference
            function getSystemTheme() {
                return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
            }

            // Apply theme to HTML element
            function applyTheme(theme) {
                const effectiveTheme = theme === 'auto' ? getSystemTheme() : theme;
                html.setAttribute('data-theme', effectiveTheme);

                // Update button states
                document.querySelectorAll('.theme-toggle button').forEach(btn => {
                    btn.classList.remove('active');
                });
                const activeBtn = document.getElementById('theme-' + theme);
                if (activeBtn) {
                    activeBtn.classList.add('active');
                }
            }

            // Set and save theme
            function setTheme(theme) {
                localStorage.setItem(STORAGE_KEY, theme);
                applyTheme(theme);
            }

            // Initialize theme on page load (before paint to avoid flash)
            const storedTheme = getStoredTheme();
            applyTheme(storedTheme);

            // Listen for system theme changes when in auto mode
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
                if (getStoredTheme() === 'auto') {
                    applyTheme('auto');
                }
            });

            // Add click handlers after DOM is ready
            window.addEventListener('DOMContentLoaded', () => {
                document.getElementById('theme-light').addEventListener('click', () => setTheme('light'));
                document.getElementById('theme-dark').addEventListener('click', () => setTheme('dark'));
                document.getElementById('theme-auto').addEventListener('click', () => setTheme('auto'));

                // Keyboard shortcut: Ctrl+Shift+D
                document.addEventListener('keydown', (e) => {
                    if (e.ctrlKey && e.shiftKey && e.key === 'D') {
                        e.preventDefault();
                        const current = getStoredTheme();
                        const next = current === 'light' ? 'dark' : current === 'dark' ? 'auto' : 'light';
                        setTheme(next);
                    }
                });
            });
        })();

        // Move toolbar into sidebar footer when sidebar exists
        (function() {
            function moveToolbarToSidebar() {
                const sidebar = document.querySelector('.tinkerdown-nav-sidebar');
                const toolbar = document.querySelector('.page-toolbar');

                if (sidebar && toolbar) {
                    // Check if footer already exists
                    let footer = sidebar.querySelector('.nav-sidebar-footer');
                    if (!footer) {
                        footer = document.createElement('div');
                        footer.className = 'nav-sidebar-footer';
                        sidebar.appendChild(footer);
                    }

                    // Move toolbar into footer
                    footer.appendChild(toolbar);
                }
            }

            // Run on DOMContentLoaded
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', moveToolbarToSidebar);
            } else {
                moveToolbarToSidebar();
            }

            // Also watch for dynamically created sidebars (tutorial mode)
            const observer = new MutationObserver((mutations) => {
                for (const mutation of mutations) {
                    for (const node of mutation.addedNodes) {
                        if (node.nodeType === 1 && node.classList?.contains('tinkerdown-nav-sidebar')) {
                            moveToolbarToSidebar();
                            return;
                        }
                    }
                }
            });
            observer.observe(document.body, { childList: true, subtree: true });
        })();

        // Presentation Mode
        (function() {
            let presentationMode = false;
            let currentSectionIndex = 0;
            let sections = [];

            // Expose presentation mode state globally so other scripts can check it
            window.tinkerdownPresentationMode = {
                isActive: () => presentationMode
            };

            function getSections() {
                // Get all H2 elements (tutorial sections)
                const h2Elements = document.querySelectorAll('.content-wrapper h2');
                sections = [];

                h2Elements.forEach((h2, index) => {
                    const section = {
                        heading: h2,
                        elements: [h2],
                        index: index
                    };

                    // Collect all elements until the next H2
                    let nextElement = h2.nextElementSibling;
                    while (nextElement && nextElement.tagName !== 'H2') {
                        section.elements.push(nextElement);
                        nextElement = nextElement.nextElementSibling;
                    }

                    sections.push(section);
                });

                return sections;
            }

            function showSection(index) {
                if (sections.length === 0) return;

                currentSectionIndex = Math.max(0, Math.min(index, sections.length - 1));

                // Remove current section class from all
                document.querySelectorAll('.presentation-current-section').forEach(el => {
                    el.classList.remove('presentation-current-section');
                });

                // Add class to current section elements
                const section = sections[currentSectionIndex];
                section.elements.forEach(el => {
                    el.classList.add('presentation-current-section');
                });

                // Update step counter in bottom navigation
                const currentStepEl = document.querySelector('.current-step');
                if (currentStepEl) {
                    currentStepEl.textContent = currentSectionIndex + 1;
                }

                // Update button disabled states
                const prevBtn = document.querySelector('.nav-prev');
                const nextBtn = document.querySelector('.nav-next');
                if (prevBtn) {
                    prevBtn.disabled = currentSectionIndex === 0;
                }
                if (nextBtn) {
                    nextBtn.disabled = currentSectionIndex === sections.length - 1;
                }

                // Scroll to section
                section.heading.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }

            function togglePresentationMode() {
                presentationMode = !presentationMode;
                const btn = document.getElementById('presentation-toggle');
                const toolbar = document.querySelector('.page-toolbar');
                const sidebarFooter = document.querySelector('.nav-sidebar-footer');

                if (presentationMode) {
                    // Enter presentation mode
                    document.body.classList.add('presentation-mode');
                    btn.classList.add('active');

                    // Move toolbar to body (sidebar will be hidden)
                    if (toolbar && sidebarFooter) {
                        document.body.appendChild(toolbar);
                    }

                    // Get sections and show first one
                    getSections();
                    showSection(0);
                } else {
                    // Exit presentation mode
                    document.body.classList.remove('presentation-mode');
                    btn.classList.remove('active');

                    // Move toolbar back to sidebar footer
                    if (toolbar && sidebarFooter) {
                        sidebarFooter.appendChild(toolbar);
                    }

                    // Remove all presentation classes
                    document.querySelectorAll('.presentation-current-section').forEach(el => {
                        el.classList.remove('presentation-current-section');
                    });
                }
            }

            function nextSection() {
                if (presentationMode && currentSectionIndex < sections.length - 1) {
                    showSection(currentSectionIndex + 1);
                }
            }

            function previousSection() {
                if (presentationMode && currentSectionIndex > 0) {
                    showSection(currentSectionIndex - 1);
                }
            }

            // Initialize after DOM is ready
            window.addEventListener('DOMContentLoaded', () => {
                const btn = document.getElementById('presentation-toggle');
                if (btn) {
                    btn.addEventListener('click', togglePresentationMode);
                }

                // Keyboard shortcuts - use capture phase to intercept before TutorialNavigation
                document.addEventListener('keydown', (e) => {
                    // 'f' key to toggle presentation mode
                    if (e.key === 'f' && !e.ctrlKey && !e.metaKey && !e.altKey) {
                        // Only if not typing in an input
                        if (document.activeElement.tagName !== 'INPUT' &&
                            document.activeElement.tagName !== 'TEXTAREA') {
                            e.preventDefault();
                            e.stopImmediatePropagation(); // Prevent other handlers
                            togglePresentationMode();
                        }
                    }

                    // Arrow keys for navigation in presentation mode
                    if (presentationMode) {
                        if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
                            e.preventDefault();
                            e.stopImmediatePropagation(); // Prevent TutorialNavigation
                            nextSection();
                        } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
                            e.preventDefault();
                            e.stopImmediatePropagation(); // Prevent TutorialNavigation
                            previousSection();
                        } else if (e.key === 'Escape') {
                            e.preventDefault();
                            e.stopImmediatePropagation(); // Prevent other handlers
                            togglePresentationMode();
                        }
                    }
                }, true); // Use capture phase

                // Hook into navigation buttons (use class selectors, not IDs)
                // Use capture phase to intercept before TutorialNavigation handles it
                document.addEventListener('click', (e) => {
                    if (!presentationMode) return;

                    const target = e.target;
                    // Check if clicked element or its parent is a nav button
                    const navBtn = target.closest('.nav-next, .nav-prev');
                    if (!navBtn) return;

                    // Prevent the default navigation
                    e.preventDefault();
                    e.stopPropagation();

                    // Handle presentation mode navigation
                    if (navBtn.classList.contains('nav-next')) {
                        nextSection();
                    } else if (navBtn.classList.contains('nav-prev')) {
                        previousSection();
                    }
                }, true); // Use capture phase
            });
        })();
    </script>

    <script defer src="/assets/tinkerdown-client.js"></script>

    <!-- Prism.js for syntax highlighting (embedded). defer eliminates render-block
         and preserves source order so language plugins load after prism core. -->
    <script defer src="/assets/prism.js"></script>
    %s
    <script defer src="/assets/prism-go.js"></script>
    <script defer src="/assets/prism-javascript.js"></script>
    <script defer src="/assets/prism-jsx.js"></script>
    <script defer src="/assets/prism-markup.js"></script>
    <script defer src="/assets/prism-css.js"></script>
    <script defer src="/assets/prism-yaml.js"></script>
    <script defer src="/assets/prism-json.js"></script>
    <script defer src="/assets/prism-bash.js"></script>
    <script defer src="/assets/prism-markdown.js"></script>
    <script>
        // Highlight all code blocks on page load
        document.addEventListener('DOMContentLoaded', function() {
            Prism.highlightAll();
        });
    </script>

    %s
%s
</body>
</html>`, wsURL, showSidebar, page.Title, seoTags, stylingOverrideCSS, prismIncludeCSS, sidebar, contentWithNav, defaultTheme, prismIncludeJS, mermaidScript, chartScript)

	return html
}

// renderContent renders the page content with code blocks
func (s *Server) renderContent(page *tinkerdown.Page) string {
	content := page.StaticHTML

	// TODO: Enhance markdown parser to add data attributes to code blocks
	// For now, the client will need to discover blocks by parsing the HTML
	// In Phase 4.5, we'll improve this to inject proper data attributes during parsing

	// Return content directly - the .content-wrapper will be added by the page template
	// This avoids double-nesting of .content-wrapper divs
	return content
}

// mdToPattern converts a markdown file path to a URL pattern.
// Examples:
//   - "index.md" → "/"
//   - "counter.md" → "/counter"
//   - "tutorials/intro.md" → "/tutorials/intro"
//   - "tutorials/index.md" → "/tutorials/"
func mdToPattern(relPath string) string {
	// Remove .md extension
	path := strings.TrimSuffix(relPath, ".md")

	// Convert to URL path
	path = filepath.ToSlash(path)

	// Handle index files
	if path == "index" {
		return "/"
	}
	if strings.HasSuffix(path, "/index") {
		return "/" + strings.TrimSuffix(path, "index")
	}

	return "/" + path
}

// sortRoutes sorts routes with index routes first.
func sortRoutes(routes []*Route) {
	// Simple sort: / first, then /foo/, then /foo
	// This is a basic implementation; could be more sophisticated
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			if shouldSwap(routes[i], routes[j]) {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}
}

func shouldSwap(a, b *Route) bool {
	// Root path comes first
	if a.Pattern == "/" {
		return false
	}
	if b.Pattern == "/" {
		return true
	}

	// Directory index paths come before other paths
	aIsIndex := strings.HasSuffix(a.Pattern, "/")
	bIsIndex := strings.HasSuffix(b.Pattern, "/")

	if aIsIndex && !bIsIndex {
		return false
	}
	if !aIsIndex && bIsIndex {
		return true
	}

	// Alphabetical otherwise
	return a.Pattern > b.Pattern
}

// RegisterConnection adds a WebSocket connection to the tracked connections.
func (s *Server) RegisterConnection(conn *websocket.Conn, handler *WebSocketHandler) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.connections[conn] = handler
	log.Printf("[Server] WebSocket connection registered: %d active connections", len(s.connections))
}

// UnregisterConnection removes a WebSocket connection from tracked connections.
func (s *Server) UnregisterConnection(conn *websocket.Conn) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	delete(s.connections, conn)
	log.Printf("[Server] WebSocket connection unregistered: %d active connections", len(s.connections))
}

// BroadcastReload sends a reload message to all connected WebSocket clients.
func (s *Server) BroadcastReload(filePath string) {
	s.connMu.RLock()
	defer s.connMu.RUnlock()

	if len(s.connections) == 0 {
		return
	}

	msg := map[string]interface{}{
		"action":   "reload",
		"filePath": filePath,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[Server] Failed to marshal reload message: %v", err)
		return
	}

	log.Printf("[Server] Broadcasting reload for %s to %d connections", filePath, len(s.connections))

	for conn, handler := range s.connections {
		if handler != nil {
			// Use handler's writeMu to serialize with other WebSocket writes
			handler.writeMu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, data)
			handler.writeMu.Unlock()
			if err != nil {
				log.Printf("[Server] Failed to send reload to connection: %v", err)
			}
		} else {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("[Server] Failed to send reload to connection: %v", err)
			}
		}
	}
}

// EnableWatch enables file watching for live reload.
func (s *Server) EnableWatch(debug bool) error {
	watcher, err := NewWatcher(s.rootDir, func(filePath string) error {
		log.Printf("[Watch] File changed: %s", filePath)

		// Check if this is a page file or a source file
		isPageFile := s.isPageFile(filePath)
		isSourceFile := s.isTrackedSourceFile(filePath)
		isIncludedFile := s.isIncludedFile(filePath)

		if isPageFile && isSourceFile && s.isRecentSourceWrite(filePath) {
			// File was modified by a source action (e.g., checkbox toggle).
			// Only refresh sources — no full page reload needed.
			s.RefreshSourcesForFile(filePath)
		} else if isPageFile {
			// External edit to page file (user or other tool).
			// Refresh any tracked sources, then do full Discover + reload.
			if isSourceFile {
				s.RefreshSourcesForFile(filePath)
			}
			// Re-discover pages for page file changes
			if err := s.Discover(); err != nil {
				return fmt.Errorf("failed to re-discover pages: %w", err)
			}

			// Broadcast reload to all connected clients
			s.BroadcastReload(filePath)
		} else if isIncludedFile {
			// File is referenced by `LANG include="..."` on at least one
			// page — re-discover so the included content is re-read,
			// then broadcast reload.
			if err := s.Discover(); err != nil {
				return fmt.Errorf("failed to re-discover pages: %w", err)
			}
			s.BroadcastReload(filePath)
		} else {
			// For non-page files (external source files), just refresh affected sources
			s.RefreshSourcesForFile(filePath)
		}

		return nil
	}, debug)

	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	s.watcher = watcher
	s.watcher.Start()

	log.Printf("[Watch] File watcher started for %s", s.rootDir)
	return nil
}

// isPageFile checks if a file path corresponds to a page (vs an external data source file)
func (s *Server) isPageFile(filePath string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, route := range s.routes {
		if route.FilePath == filePath {
			return true
		}
	}
	return false
}

// isIncludedFile reports whether filePath is referenced by any page's
// `LANG include="..."` fences. Watcher events arrive with the path
// relative to s.rootDir; pages store IncludedFiles as absolutes — so
// we compare on absolute paths.
//
// Reads s.routes / s.siteManager via s.mu to avoid racing Discover().
// Skips the .md fast path: if the watcher fired for a .md file the
// existing isPageFile branch handles it; we'd never need to also
// declare it an included-file candidate.
func (s *Server) isIncludedFile(filePath string) bool {
	if filepath.Ext(filePath) == ".md" {
		return false
	}
	abs := filePath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.rootDir, filePath)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	pages := s.collectPagesForAutoRoutes()
	for _, page := range pages {
		for _, f := range page.IncludedFiles {
			if f == abs {
				return true
			}
		}
	}
	return false
}

// isTrackedSourceFile checks if any active WebSocket connection tracks this file as a source.
// Used to detect same-file sources (e.g., auto-tasks where the page file is also the data source).
func (s *Server) isTrackedSourceFile(filePath string) bool {
	s.connMu.RLock()
	defer s.connMu.RUnlock()

	for _, handler := range s.connections {
		if handler != nil && handler.TracksSourceFile(filePath) {
			return true
		}
	}
	return false
}

// sourceWriteDebounceWindow is how long after a source action write we consider
// a file change to be "from the source" rather than an external edit.
const sourceWriteDebounceWindow = 2 * time.Second

// MarkSourceWrite records that a source action just wrote to this file.
// Used by the watcher to distinguish source writes (e.g., checkbox toggle)
// from external edits that require a full page reload.
func (s *Server) MarkSourceWrite(filePath string) {
	s.sourceWriteMu.Lock()
	s.recentSourceWrites[filePath] = time.Now()
	s.sourceWriteMu.Unlock()
}

// isRecentSourceWrite returns true if the file was written by a source action
// within sourceWriteDebounceWindow. Does not delete the entry so that multiple
// watcher events for the same file change are all handled correctly.
func (s *Server) isRecentSourceWrite(filePath string) bool {
	s.sourceWriteMu.Lock()
	t, ok := s.recentSourceWrites[filePath]
	if ok && time.Since(t) >= sourceWriteDebounceWindow {
		// Expired — clean up
		delete(s.recentSourceWrites, filePath)
		ok = false
	}
	s.sourceWriteMu.Unlock()
	return ok
}

// RefreshSourcesForFile triggers a refresh on all sources that use the given file.
func (s *Server) RefreshSourcesForFile(filePath string) {
	s.connMu.RLock()
	defer s.connMu.RUnlock()

	if len(s.connections) == 0 {
		return
	}

	log.Printf("[Server] Refreshing sources for file: %s (%d connections)", filePath, len(s.connections))

	for _, handler := range s.connections {
		if handler != nil {
			handler.RefreshSourcesForFile(filePath)
		}
	}
}

// StopWatch stops the file watcher if it's running.
func (s *Server) StopWatch() error {
	if s.watcher != nil {
		return s.watcher.Stop()
	}
	return nil
}

// StartSchedules starts the schedule runner for timed job execution.
// This should be called after Discover() to ensure all page schedules are registered.
func (s *Server) StartSchedules(ctx context.Context) error {
	if s.scheduleRunner != nil {
		return s.scheduleRunner.Start(ctx)
	}
	return nil
}

// StopSchedules stops the schedule runner.
func (s *Server) StopSchedules() error {
	if s.scheduleRunner != nil {
		return s.scheduleRunner.Stop()
	}
	return nil
}

// StopRateLimiter cancels the rate limiter's background cleanup goroutine and
// blocks until it exits. If a cleanup sweep is in progress, this may block
// briefly until the sweep finishes and the goroutine observes the cancellation.
func (s *Server) StopRateLimiter() {
	if s.rateLimitCancel == nil || s.rateLimitDone == nil {
		return
	}
	s.rateLimitCancel()
	<-s.rateLimitDone
}

// GetScheduledJobCount returns the number of scheduled jobs.
func (s *Server) GetScheduledJobCount() int {
	if s.scheduleRunner == nil {
		return 0
	}
	return len(s.scheduleRunner.GetAllJobs())
}

// renderSidebar renders the navigation sidebar for site mode
func (s *Server) renderSidebar(currentPath string) string {
	if s.siteManager == nil {
		return ""
	}

	nav := s.siteManager.GetNavigation()
	if len(nav) == 0 {
		return ""
	}

	var html strings.Builder

	// Mobile hamburger toggle + backdrop. Hidden on desktop via CSS; on
	// mobile the hamburger is fixed top-left and the backdrop covers the
	// page when the sidebar is open. Inline SVG keeps the bundle CSP-clean
	// (no font-icon library, no external image fetch).
	html.WriteString(`<button type="button" class="tinkerdown-nav-toggle" aria-label="Toggle navigation" aria-expanded="false" aria-controls="tinkerdown-sidebar"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg></button>`)
	html.WriteString(`<div class="tinkerdown-nav-backdrop" aria-hidden="true"></div>`)

	html.WriteString(`<nav id="tinkerdown-sidebar" class="tinkerdown-nav-sidebar">`)

	// Site title/logo
	if s.config.Site != nil && s.config.Title != "" {
		html.WriteString(fmt.Sprintf(`<div class="nav-header"><h2>%s</h2></div>`, s.config.Title))
	}

	// Navigation sections
	for _, section := range nav {
		html.WriteString(`<div class="nav-section">`)
		html.WriteString(fmt.Sprintf(`<div class="nav-section-title">%s</div>`, section.Title))

		if len(section.Children) > 0 {
			html.WriteString(`<ul class="nav-pages">`)
			for _, page := range section.Children {
				activeClass := ""
				if page.Path == currentPath {
					activeClass = " active"
				}
				html.WriteString(fmt.Sprintf(`<li><a href="%s" class="nav-page-link%s">%s</a></li>`, page.Path, activeClass, page.Title))
			}
			html.WriteString(`</ul>`)
		}

		html.WriteString(`</div>`)
	}

	html.WriteString(`</nav>`)

	// Toggle wiring. Idempotent re-init guard so hot-reload / re-mounted
	// scripts don't double-bind handlers. CSP-friendly (no inline event
	// attributes — only event listeners on a single inline script).
	html.WriteString(`<script>(function(){
		var toggle = document.querySelector('.tinkerdown-nav-toggle');
		var sidebar = document.querySelector('.tinkerdown-nav-sidebar');
		var backdrop = document.querySelector('.tinkerdown-nav-backdrop');
		if (!toggle || !sidebar || toggle.dataset.bound === '1') return;
		toggle.dataset.bound = '1';
		function setOpen(open) {
			sidebar.classList.toggle('open', open);
			if (backdrop) backdrop.classList.toggle('open', open);
			toggle.setAttribute('aria-expanded', String(open));
		}
		toggle.addEventListener('click', function() {
			setOpen(!sidebar.classList.contains('open'));
		});
		if (backdrop) backdrop.addEventListener('click', function() { setOpen(false); });
		// Close when a nav link is tapped — mobile UX expectation.
		sidebar.addEventListener('click', function(e) {
			if (e.target && e.target.closest && e.target.closest('a')) setOpen(false);
		});
		// Close on Escape key.
		document.addEventListener('keydown', function(e) {
			if (e.key === 'Escape' && sidebar.classList.contains('open')) setOpen(false);
		});
	})();</script>`)

	return html.String()
}

// renderBreadcrumbs renders breadcrumb navigation
func (s *Server) renderBreadcrumbs(currentPath string) string {
	if s.siteManager == nil {
		return ""
	}

	breadcrumbs := s.siteManager.GetBreadcrumbs(currentPath)
	if len(breadcrumbs) <= 1 {
		return ""
	}

	var html strings.Builder
	html.WriteString(`<nav class="breadcrumbs" aria-label="Breadcrumb"><ol>`)

	for i, crumb := range breadcrumbs {
		if i < len(breadcrumbs)-1 {
			html.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`, crumb.Path, crumb.Title))
			html.WriteString(`<li class="separator">›</li>`)
		} else {
			html.WriteString(fmt.Sprintf(`<li class="current">%s</li>`, crumb.Title))
		}
	}

	html.WriteString(`</ol></nav>`)
	return html.String()
}

// renderPrevNext renders previous/next page navigation
func (s *Server) renderPrevNext(currentPath string) string {
	if s.siteManager == nil {
		return ""
	}

	prev, next := s.siteManager.GetPrevNext(currentPath)

	if prev == nil && next == nil {
		return ""
	}

	var html strings.Builder
	html.WriteString(`<nav class="page-nav">`)

	if prev != nil {
		html.WriteString(fmt.Sprintf(`<a href="%s" class="page-nav-prev"><span class="arrow">←</span><span class="label">%s</span></a>`, prev.Path, prev.Title))
	} else {
		html.WriteString(`<span class="page-nav-spacer"></span>`)
	}

	if next != nil {
		html.WriteString(fmt.Sprintf(`<a href="%s" class="page-nav-next"><span class="label">%s</span><span class="arrow">→</span></a>`, next.Path, next.Title))
	}

	html.WriteString(`</nav>`)
	return html.String()
}

// executeWebhookAction executes an action triggered by a webhook.
// This creates a minimal runtime context to execute SQL, HTTP, or exec actions.
func (s *Server) executeWebhookAction(actionName string, params map[string]interface{}) error {
	if s.config == nil || s.config.Actions == nil {
		return fmt.Errorf("no actions configured")
	}

	action, exists := s.config.Actions[actionName]
	if !exists || action == nil {
		return fmt.Errorf("action %q not found", actionName)
	}

	// Create a webhook action executor
	executor := newWebhookActionExecutor(s.config, s.rootDir)
	return executor.execute(action, params)
}
