package server

import (
	"container/list"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/livetemplate/tinkerdown/internal/config"

	"golang.org/x/time/rate"
)

// CORSMiddleware adds CORS headers to responses.
// If origins is empty or nil, CORS headers are not added.
// authHeaderName is included in Access-Control-Allow-Headers when non-empty.
func CORSMiddleware(origins []string, authHeaderName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(origins) == 0 {
			return next
		}

		// Build allowed headers list, including the configured auth header
		allowHeaders := "Content-Type, Authorization, X-API-Key"
		if authHeaderName != "" && authHeaderName != "Authorization" && authHeaderName != "X-API-Key" {
			allowHeaders += ", " + authHeaderName
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			allowAll := false
			for _, o := range origins {
				if o == "*" {
					allowed = true
					allowAll = true
					break
				}
				if o == origin {
					allowed = true
					break
				}
			}

			if allowed && origin != "" {
				// When wildcard is configured, use "*" header; otherwise echo the specific origin
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
			}

			// Handle preflight request
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware adds security headers to all responses.
// frameSrc is appended to the CSP frame-src directive (in addition to
// 'self') and is typically sourced from config.SecurityConfig.FrameSrc.
// Pass nil when no cross-origin iframes are needed.
func SecurityHeadersMiddleware(frameSrc []string) func(http.Handler) http.Handler {
	csp := buildCSP(frameSrc)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", csp)

			next.ServeHTTP(w, r)
		})
	}
}

// buildCSP composes the Content-Security-Policy header value. The base
// policy mirrors what tinkerdown has emitted since v0.1.x: self-only
// scripts/styles plus inline (PicoCSS, LVT renders inline), unsafe-eval
// (Mermaid runtime), data: URIs (fonts/images), self-only connect-src
// (covers same-origin ws/wss). When frameSrc is non-empty a frame-src
// directive is emitted with 'self' plus each provided origin verbatim;
// otherwise frame-src is omitted and inherits default-src 'self', which
// blocks cross-origin iframes.
func buildCSP(frameSrc []string) string {
	csp := "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: https:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'"
	if len(frameSrc) > 0 {
		csp += "; frame-src 'self' " + strings.Join(frameSrc, " ")
	}
	return csp
}

// evictionLogInterval is the minimum time between eviction log messages.
const evictionLogInterval = 30 * time.Second

// defaultNumShards is the number of shards used by the sharded rate limiter.
// Sharding reduces lock contention under parallel workloads while keeping
// memory overhead minimal (16 mutexes + 16 small maps).
const defaultNumShards = 16

// ipLimiter tracks a per-IP token bucket and its position in the LRU list.
type ipLimiter struct {
	ip       string
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimitMiddleware limits requests using a token bucket algorithm with per-IP tracking.
// rps is the rate limit in requests per second, burst is the maximum burst size,
// and maxIPs is the maximum number of unique IPs to track (LRU eviction when full).
// maxIPs <= 0 defaults to 10000.
//
// For maxIPs >= defaultNumShards (or maxIPs == 0, which defaults to 10000),
// a sharded implementation is used to reduce lock contention under parallel
// load. For 0 < maxIPs < defaultNumShards, a single-mutex implementation
// is used to avoid zero-capacity shards.
//
// The cleanup goroutine starts immediately when this function is called.
// Callers must cancel ctx and then receive from the returned channel
// to guarantee the goroutine has exited.
func RateLimitMiddleware(ctx context.Context, rps float64, burst int, maxIPs int) (func(http.Handler) http.Handler, <-chan struct{}) {
	if maxIPs > 0 && maxIPs < defaultNumShards {
		return rateLimitMiddlewareInternal(ctx, rps, burst, maxIPs,
			5*time.Minute, 10*time.Minute, evictionLogInterval)
	}
	return shardedRateLimitMiddlewareInternal(ctx, rps, burst, maxIPs,
		5*time.Minute, 10*time.Minute, evictionLogInterval, defaultNumShards)
}

// rateLimitMiddlewareInternal accepts configurable durations for testing.
func rateLimitMiddlewareInternal(
	ctx context.Context, rps float64, burst int, maxIPs int,
	sweepInterval, staleThreshold, evictLogInterval time.Duration,
) (func(http.Handler) http.Handler, <-chan struct{}) {
	if maxIPs <= 0 {
		maxIPs = 10000
	}

	var (
		items = make(map[string]*list.Element)
		order = list.New() // front = most recent, back = oldest
		mu    sync.Mutex

		// Eviction logging state (always accessed under mu).
		// lastEvictLog zero-value ensures the first eviction always logs.
		lastEvictLog time.Time
		evictCount   int
	)

	// Start cleanup goroutine — exits when ctx is cancelled.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				now := time.Now()
				// Iterate all entries and remove stale ones.
				// Cannot break early: LRU order tracks access recency,
				// not lastSeen time, so stale entries may appear anywhere.
				for e := order.Back(); e != nil; {
					lim := e.Value.(*ipLimiter)
					prev := e.Prev()
					if now.Sub(lim.lastSeen) > staleThreshold {
						order.Remove(e)
						delete(items, lim.ip)
					}
					e = prev
				}
				mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)

			mu.Lock()
			elem, exists := items[ip]
			if exists {
				// Move to front (most recently used) and update lastSeen
				order.MoveToFront(elem)
				elem.Value.(*ipLimiter).lastSeen = time.Now()
			} else {
				// Evict least recently used if at capacity
				if order.Len() >= maxIPs {
					back := order.Back()
					if back != nil {
						evicted := back.Value.(*ipLimiter)
						order.Remove(back)
						delete(items, evicted.ip)
						evictCount++
						if time.Since(lastEvictLog) >= evictLogInterval {
							log.Printf("[RateLimit] Evicted %d least-recent IP(s) (at capacity: %d IPs)", evictCount, maxIPs)
							lastEvictLog = time.Now()
							evictCount = 0
						}
					}
				}
				lim := &ipLimiter{
					ip:       ip,
					limiter:  rate.NewLimiter(rate.Limit(rps), burst),
					lastSeen: time.Now(),
				}
				elem = order.PushFront(lim)
				items[ip] = elem
			}
			lim := elem.Value.(*ipLimiter).limiter
			mu.Unlock()

			if !lim.Allow() {
				w.Header().Set("Retry-After", "1")
				writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	return middleware, done
}

// shard holds a subset of IP limiters behind its own mutex,
// reducing contention compared to a single global lock.
// The padding field ensures each shard occupies its own CPU cache line,
// preventing false sharing when adjacent shards are accessed by different cores.
type shard struct {
	mu           sync.Mutex
	items        map[string]*list.Element
	order        *list.List
	maxIPs       int
	lastEvictLog time.Time
	evictCount   int
	_            [64]byte // pad to 128 bytes (2 cache lines) to reduce false sharing
}

// Compile-time assertion: shard struct must be exactly 128 bytes.
// If a field is added/changed, adjust the padding to maintain cache-line alignment.
const _ = uint(128 - unsafe.Sizeof(shard{}))

// shardedRateLimiter distributes IPs across multiple shards to reduce
// mutex contention under parallel load.
type shardedRateLimiter struct {
	shards           []shard
	numShards        uint32
	rps              float64
	burst            int
	totalMaxIPs      int
	evictLogInterval time.Duration
}

// shardFor returns the shard responsible for the given IP using inline FNV-1a.
func (sl *shardedRateLimiter) shardFor(ip string) *shard {
	h := uint32(2166136261)
	for i := 0; i < len(ip); i++ {
		h ^= uint32(ip[i])
		h *= 16777619
	}
	return &sl.shards[h%sl.numShards]
}

// shardedRateLimitMiddlewareInternal creates a sharded rate limiter.
// Precondition: numShards must not exceed maxIPs (after defaulting),
// otherwise some shards will have zero capacity. The public API enforces
// this by falling back to single-mutex for maxIPs < defaultNumShards.
func shardedRateLimitMiddlewareInternal(
	ctx context.Context, rps float64, burst int, maxIPs int,
	sweepInterval, staleThreshold, evictLogInterval time.Duration,
	numShards int,
) (func(http.Handler) http.Handler, <-chan struct{}) {
	if numShards <= 0 {
		panic("shardedRateLimitMiddlewareInternal: numShards must be > 0")
	}
	if maxIPs <= 0 {
		maxIPs = 10000
	}

	sl := &shardedRateLimiter{
		shards:           make([]shard, numShards),
		numShards:        uint32(numShards),
		rps:              rps,
		burst:            burst,
		totalMaxIPs:      maxIPs,
		evictLogInterval: evictLogInterval,
	}

	// Distribute maxIPs across shards; remainder goes to early shards.
	base := maxIPs / numShards
	remainder := maxIPs % numShards
	for i := range sl.shards {
		shardCap := base
		if i < remainder {
			shardCap++
		}
		sl.shards[i] = shard{
			items:  make(map[string]*list.Element),
			order:  list.New(),
			maxIPs: shardCap,
		}
	}

	// Single cleanup goroutine iterates all shards, locking each briefly.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				for i := range sl.shards {
					s := &sl.shards[i]
					s.mu.Lock()
					for e := s.order.Back(); e != nil; {
						lim := e.Value.(*ipLimiter)
						prev := e.Prev()
						if now.Sub(lim.lastSeen) > staleThreshold {
							s.order.Remove(e)
							delete(s.items, lim.ip)
						}
						e = prev
					}
					s.mu.Unlock()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Hot path mirrors rateLimitMiddlewareInternal; keep in sync.
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			s := sl.shardFor(ip)

			s.mu.Lock()
			elem, exists := s.items[ip]
			if exists {
				s.order.MoveToFront(elem)
				elem.Value.(*ipLimiter).lastSeen = time.Now()
			} else {
				if s.order.Len() >= s.maxIPs {
					back := s.order.Back()
					if back != nil {
						evicted := back.Value.(*ipLimiter)
						s.order.Remove(back)
						delete(s.items, evicted.ip)
						s.evictCount++
						if time.Since(s.lastEvictLog) >= sl.evictLogInterval {
							log.Printf("[RateLimit] Evicted %d least-recent IP(s) (shard capacity: %d, total: %d IPs)",
								s.evictCount, s.maxIPs, sl.totalMaxIPs)
							s.lastEvictLog = time.Now()
							s.evictCount = 0
						}
					}
				}
				lim := &ipLimiter{
					ip:       ip,
					limiter:  rate.NewLimiter(rate.Limit(sl.rps), sl.burst),
					lastSeen: time.Now(),
				}
				elem = s.order.PushFront(lim)
				s.items[ip] = elem
			}
			lim := elem.Value.(*ipLimiter).limiter
			s.mu.Unlock()

			if !lim.Allow() {
				w.Header().Set("Retry-After", "1")
				writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	return middleware, done
}

// getClientIP extracts the client IP from the request.
// It only trusts X-Forwarded-For / X-Real-IP when the immediate peer is a
// loopback or private address (i.e., behind a reverse proxy).
func getClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	peerIP := net.ParseIP(host)
	trustedProxy := peerIP != nil && (peerIP.IsLoopback() || peerIP.IsPrivate())

	if trustedProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if parts := strings.SplitN(xff, ",", 2); len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	if peerIP != nil {
		return peerIP.String()
	}
	return host
}

// authContextKey is the type for auth-related context keys.
type authContextKey string

const (
	ctxKeyName        authContextKey = "auth_key_name"
	ctxKeyPermissions authContextKey = "auth_permissions"
)

// expandedKey pairs a pre-expanded key value with its config.
type expandedKey struct {
	value  string
	config *config.APIKeyConfig
}

// AuthMiddleware validates API key authentication with support for multiple keys.
// If no keys are configured, authentication is disabled and all requests pass through.
// Environment variables in key values are expanded once at construction time.
func AuthMiddleware(authCfg *config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if authCfg == nil {
			return next
		}

		apiKeys := authCfg.GetAPIKeys()
		if len(apiKeys) == 0 {
			return next
		}

		headerName := authCfg.GetHeaderName()

		// Expand env vars once at construction time rather than per-request,
		// eliminating per-request os.ExpandEnv overhead and a minor timing
		// variation between literal keys and env-var-referenced keys.
		keys := make([]expandedKey, len(apiKeys))
		for i := range apiKeys {
			expanded := os.ExpandEnv(apiKeys[i].Key)
			if expanded == "" {
				log.Printf("[Auth] Warning: key %q expanded to empty string (check env var config)", apiKeys[i].Name)
			}
			keys[i] = expandedKey{
				value:  expanded,
				config: &apiKeys[i],
			}
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get(headerName)
			if token == "" {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			// Handle "Authorization: Bearer <token>" format
			if headerName == "Authorization" {
				const bearerPrefix = "Bearer "
				if len(token) > len(bearerPrefix) && token[:len(bearerPrefix)] == bearerPrefix {
					token = token[len(bearerPrefix):]
				} else {
					writeJSONError(w, http.StatusUnauthorized, "invalid authorization format, expected Bearer token")
					return
				}
			}

			// Find matching key — iterate all keys to avoid leaking
			// which position matched via response time differences.
			var matched *config.APIKeyConfig
			for i := range keys {
				if secureCompare(token, keys[i].value) && matched == nil {
					matched = keys[i].config
				}
			}

			if matched == nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			// Set auth context
			ctx := context.WithValue(r.Context(), ctxKeyName, matched.Name)
			ctx = context.WithValue(ctx, ctxKeyPermissions, matched.Permissions)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MethodPermissionMiddleware checks that the authenticated key has permission
// for the requested HTTP method. Must be used after AuthMiddleware.
func MethodPermissionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// OPTIONS bypass (CORS preflight)
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			var required config.Permission
			switch r.Method {
			case http.MethodGet, http.MethodHead:
				required = config.PermRead
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				required = config.PermWrite
			case http.MethodDelete:
				required = config.PermDelete
			default:
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}

			if !HasPermission(r, required) {
				writeJSONError(w, http.StatusForbidden,
					fmt.Sprintf("insufficient permissions: %s required for %s", required, r.Method))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// HasPermission checks if the request has a specific permission from auth context.
func HasPermission(r *http.Request, perm config.Permission) bool {
	perms, ok := r.Context().Value(ctxKeyPermissions).([]config.Permission)
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// secureCompare performs a constant-time string comparison to prevent timing attacks.
// Inputs are hashed to fixed-length SHA-256 digests before comparison so the
// timing does not reveal whether the inputs differ in length.
func secureCompare(a, b string) bool {
	aHash := sha256.Sum256([]byte(a))
	bHash := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(aHash[:], bHash[:]) == 1
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
