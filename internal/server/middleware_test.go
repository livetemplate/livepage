package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livetemplate/tinkerdown/internal/config"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func reqFromIP(ip string) *http.Request {
	r := httptest.NewRequest("GET", "/api/sources/test", nil)
	r.RemoteAddr = ip + ":12345"
	return r
}

// rateLimitWrap creates a rate-limited handler with a context that is
// cancelled when the test finishes, preventing goroutine leaks.
func rateLimitWrap(t *testing.T, rps float64, burst, maxIPs int, next http.Handler) http.Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mw, done := RateLimitMiddleware(ctx, rps, burst, maxIPs)
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return mw(next)
}

// TestRateLimitLRUEviction verifies that when the IP map is full, a new IP
// evicts the least-recently-used entry instead of returning 503.
func TestRateLimitLRUEviction(t *testing.T) {
	wrapped := rateLimitWrap(t, 100, 100, 3, okHandler())

	// Fill to capacity with 3 IPs
	for _, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// 4th IP should succeed (LRU eviction), not 503
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("4.4.4.4"))
	if w.Code != http.StatusOK {
		t.Errorf("4th IP at capacity: expected 200, got %d", w.Code)
	}
}

// TestRateLimitEvictedIPGetsFreshLimiter verifies that an evicted IP returning
// gets a fresh token bucket, not a stale one.
func TestRateLimitEvictedIPGetsFreshLimiter(t *testing.T) {
	// burst=1 so the first request consumes the token
	wrapped := rateLimitWrap(t, 100, 1, 2, okHandler())

	// IP "1.1.1.1" uses its burst token
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("1.1.1.1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Second request from same IP should be rate-limited (burst exhausted)
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("1.1.1.1"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	// Push 1.1.1.1 out by filling capacity with 2 other IPs
	for _, ip := range []string{"2.2.2.2", "3.3.3.3"} {
		w = httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// 1.1.1.1 returns — should get a fresh limiter with a full burst token
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("1.1.1.1"))
	if w.Code != http.StatusOK {
		t.Errorf("evicted IP returning: expected 200 (fresh limiter), got %d", w.Code)
	}
}

// TestRateLimitMRUNotEvicted verifies that accessing an IP moves it to the
// front of the LRU, protecting it from eviction.
func TestRateLimitMRUNotEvicted(t *testing.T) {
	wrapped := rateLimitWrap(t, 100, 100, 3, okHandler())

	// Fill: A, B, C (order: C=front, B, A=back)
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// Touch A → moves to front (order: A=front, C, B=back)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("10.0.0.1"))
	if w.Code != http.StatusOK {
		t.Fatalf("touch A: expected 200, got %d", w.Code)
	}

	// New IP D → evicts B (back), not A
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("10.0.0.4"))
	if w.Code != http.StatusOK {
		t.Fatalf("new IP D: expected 200, got %d", w.Code)
	}

	// A should still be present (not evicted)
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("10.0.0.1"))
	if w.Code != http.StatusOK {
		t.Errorf("A after eviction: expected 200 (still present), got %d", w.Code)
	}
}

// TestRateLimitNo503AtCapacity is a regression test ensuring that 503 is never
// returned when the rate limiter is at capacity.
func TestRateLimitNo503AtCapacity(t *testing.T) {
	wrapped := rateLimitWrap(t, 100, 100, 5, okHandler())

	// Send requests from 20 unique IPs — all should get 200, never 503
	for i := 0; i < 20; i++ {
		ip := fmt.Sprintf("192.168.%d.%d", i/256, i%256)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code == http.StatusServiceUnavailable {
			t.Fatalf("IP %s: got 503 (should never happen with LRU eviction)", ip)
		}
		if w.Code != http.StatusOK {
			t.Errorf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}
}

// TestRateLimitConcurrentAccess verifies no races or panics under concurrent load.
func TestRateLimitConcurrentAccess(t *testing.T) {
	wrapped := rateLimitWrap(t, 1000, 1000, 100, okHandler())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.0.%d.%d", id/256, id%256)
			for j := 0; j < 10; j++ {
				w := httptest.NewRecorder()
				wrapped.ServeHTTP(w, reqFromIP(ip))
				if w.Code == http.StatusServiceUnavailable {
					t.Errorf("IP %s: got 503 under concurrent load", ip)
				}
			}
		}(i)
	}
	wg.Wait()
}

// TestGetMaxTrackedIPs tests the config accessor with defaults and explicit values.
func TestGetMaxTrackedIPs(t *testing.T) {
	// nil APIConfig → default
	var nilCfg *config.APIConfig
	if got := nilCfg.GetMaxTrackedIPs(); got != 10000 {
		t.Errorf("nil APIConfig: expected 10000, got %d", got)
	}

	// nil RateLimit → default
	cfg := &config.APIConfig{}
	if got := cfg.GetMaxTrackedIPs(); got != 10000 {
		t.Errorf("nil RateLimit: expected 10000, got %d", got)
	}

	// Zero value → default
	cfg = &config.APIConfig{RateLimit: &config.RateLimitConfig{MaxTrackedIPs: 0}}
	if got := cfg.GetMaxTrackedIPs(); got != 10000 {
		t.Errorf("zero MaxTrackedIPs: expected 10000, got %d", got)
	}

	// Explicit value
	cfg = &config.APIConfig{RateLimit: &config.RateLimitConfig{MaxTrackedIPs: 500}}
	if got := cfg.GetMaxTrackedIPs(); got != 500 {
		t.Errorf("explicit MaxTrackedIPs: expected 500, got %d", got)
	}
}

// TestRateLimitCleanupStopsOnCancel verifies that cancelling the context
// causes the cleanup goroutine to exit, confirmed via the done channel.
func TestRateLimitCleanupStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, done := RateLimitMiddleware(ctx, 100, 100, 100)

	cancel()

	select {
	case <-done:
		// Goroutine exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup goroutine did not exit within 2s")
	}
}

// rateLimitWrapInternal creates a rate-limited handler using the internal
// constructor with configurable durations, for testing cleanup and logging.
func rateLimitWrapInternal(t *testing.T, rps float64, burst, maxIPs int,
	sweepInterval, staleThreshold, evictLogInterval time.Duration,
	next http.Handler,
) http.Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mw, done := rateLimitMiddlewareInternal(ctx, rps, burst, maxIPs,
		sweepInterval, staleThreshold, evictLogInterval)
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return mw(next)
}

// countLogLines returns the number of lines in output that contain substr.
func countLogLines(output, substr string) int {
	n := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}

// TestRateLimitCleanupRemovesStaleEntries verifies that the cleanup goroutine
// removes IP entries that haven't been seen for longer than the stale threshold.
func TestRateLimitCleanupRemovesStaleEntries(t *testing.T) {
	// rps=0.001 ensures the token bucket won't refill naturally (1 token/1000s),
	// so a 200 after sleeping proves the cleanup created a fresh limiter.
	wrapped := rateLimitWrapInternal(t,
		0.001, 1, 100,
		50*time.Millisecond,  // sweepInterval
		100*time.Millisecond, // staleThreshold
		time.Hour,            // evictLogInterval (irrelevant here)
		okHandler(),
	)

	// First request uses the burst token → 200
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("5.5.5.5"))
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}

	// Second request from same IP → 429 (burst exhausted, rps too low to refill)
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("5.5.5.5"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", w.Code)
	}

	// Wait for cleanup to fire. The stale clock starts from the second request
	// (which returned 429) because lastSeen is refreshed on every access.
	// Using 400ms for CI resilience (staleThreshold=100ms + sweepInterval=50ms + margin).
	time.Sleep(400 * time.Millisecond)

	// Entry should be gone — new request gets a fresh limiter with burst token → 200
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("5.5.5.5"))
	if w.Code != http.StatusOK {
		t.Errorf("after cleanup: expected 200 (fresh limiter), got %d", w.Code)
	}
}

// TestRateLimitCleanupKeepsActiveEntries verifies that entries accessed
// recently (within the stale threshold) survive the cleanup goroutine.
func TestRateLimitCleanupKeepsActiveEntries(t *testing.T) {
	// burst=1, rps=0.001 — once burst is used, only a fresh limiter gives 200
	wrapped := rateLimitWrapInternal(t,
		0.001, 1, 100,
		50*time.Millisecond,  // sweepInterval
		300*time.Millisecond, // staleThreshold (wide margin for CI)
		time.Hour,            // evictLogInterval (irrelevant here)
		okHandler(),
	)

	// Use burst token → 200
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("6.6.6.6"))
	if w.Code != http.StatusOK {
		t.Fatalf("initial request: expected 200, got %d", w.Code)
	}

	// Keep entry alive by sending requests every 100ms (< 300ms staleThreshold)
	// across multiple sweep cycles (each 50ms)
	for i := 0; i < 4; i++ {
		time.Sleep(100 * time.Millisecond)
		w = httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP("6.6.6.6"))
		// Should stay 429 — same exhausted limiter, not a fresh one
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("tick %d: expected 429 (entry kept alive), got %d", i, w.Code)
		}
	}
}

// TestRateLimitEvictionLogThrottling verifies that eviction log messages
// are throttled: at most one message per evictLogInterval window.
// NOTE: log.SetOutput mutates global state; this test cannot use t.Parallel().
func TestRateLimitEvictionLogThrottling(t *testing.T) {
	var buf bytes.Buffer
	origWriter := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(origWriter) })

	wrapped := rateLimitWrapInternal(t,
		100, 100, 1,
		time.Hour,             // sweepInterval (irrelevant here)
		time.Hour,             // staleThreshold (irrelevant here)
		100*time.Millisecond,  // evictLogInterval
		okHandler(),
	)

	// 6 requests: 1st populates the slot, next 5 each evict the previous IP
	for i := 0; i < 6; i++ {
		ip := fmt.Sprintf("20.0.0.%d", i)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// First batch: only 1 log line (first eviction triggers it, rest are throttled)
	lines := countLogLines(buf.String(), "[RateLimit] Evicted")
	if lines != 1 {
		t.Errorf("first batch: expected 1 log line, got %d\nlog output:\n%s", lines, buf.String())
	}

	// Wait for throttle window to expire
	time.Sleep(150 * time.Millisecond)

	// Trigger more evictions. These in-memory operations complete well within
	// the 100ms evictLogInterval, so exactly one additional log line is expected.
	for i := 10; i < 16; i++ {
		ip := fmt.Sprintf("20.0.0.%d", i)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// Cumulative total should now be 2 log lines
	lines = countLogLines(buf.String(), "[RateLimit] Evicted")
	if lines != 2 {
		t.Errorf("second batch: expected 2 total log lines, got %d\nlog output:\n%s", lines, buf.String())
	}
}

// ---------------------------------------------------------------------------
// Benchmark helpers
// ---------------------------------------------------------------------------

// benchSink prevents dead-code elimination in benchmarks.
// Uses atomic to avoid data races in parallel benchmarks.
var benchSink atomic.Int64

// benchRateLimitHandler creates a single-mutex rate-limited handler for benchmarks.
// Uses time.Hour durations to disable cleanup/logging during benchmark runs.
func benchRateLimitHandler(b *testing.B, rps float64, burst, maxIPs int) (http.Handler, func()) {
	b.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mw, done := rateLimitMiddlewareInternal(ctx, rps, burst, maxIPs,
		time.Hour, time.Hour, time.Hour)
	handler := mw(okHandler())
	cleanup := func() {
		cancel()
		<-done
	}
	return handler, cleanup
}

// benchShardedRateLimitHandler creates a sharded rate-limited handler for benchmarks.
func benchShardedRateLimitHandler(b *testing.B, rps float64, burst, maxIPs, numShards int) (http.Handler, func()) {
	b.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mw, done := shardedRateLimitMiddlewareInternal(ctx, rps, burst, maxIPs,
		time.Hour, time.Hour, time.Hour, numShards)
	handler := mw(okHandler())
	cleanup := func() {
		cancel()
		<-done
	}
	return handler, cleanup
}

// ---------------------------------------------------------------------------
// Single-mutex baseline benchmarks (#154)
// ---------------------------------------------------------------------------

// BenchmarkRateLimit_SingleIP measures hot-path cost: one goroutine, same IP.
func BenchmarkRateLimit_SingleIP(b *testing.B) {
	handler, cleanup := benchRateLimitHandler(b, 1e9, 1<<30, 10000)
	defer cleanup()

	req := reqFromIP("1.2.3.4")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Body.Reset()
		handler.ServeHTTP(w, req)
		benchSink.Add(int64(w.Code))
	}
}

// BenchmarkRateLimit_UniqueIPs measures insert + eviction cost: new IP each iteration.
func BenchmarkRateLimit_UniqueIPs(b *testing.B) {
	handler, cleanup := benchRateLimitHandler(b, 1e9, 1<<30, 1000)
	defer cleanup()

	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := fmt.Sprintf("10.%d.%d.%d", (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
		handler.ServeHTTP(w, reqFromIP(ip))
		benchSink.Add(int64(w.Code))
	}
}

// BenchmarkRateLimit_Parallel_SingleMutex measures single-mutex contention:
// multiple goroutines, 100 IPs each. Uses the single-mutex path directly
// (not the public API) to establish a baseline for comparison with sharded.
func BenchmarkRateLimit_Parallel_SingleMutex(b *testing.B) {
	for _, maxIPs := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("MaxIPs_%d", maxIPs), func(b *testing.B) {
			handler, cleanup := benchRateLimitHandler(b, 1e9, 1<<30, maxIPs)
			defer cleanup()

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				w := httptest.NewRecorder()
				i := 0
				for pb.Next() {
					w.Body.Reset()
					ip := fmt.Sprintf("10.0.%d.%d", (i/256)%256, i%256)
					handler.ServeHTTP(w, reqFromIP(ip))
					benchSink.Add(int64(w.Code))
					i++
					if i >= 100 {
						i = 0
					}
				}
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Sharded benchmarks — comparison and varying shard counts
// ---------------------------------------------------------------------------

// BenchmarkRateLimit_Comparison runs single-mutex vs sharded-16 side by side.
func BenchmarkRateLimit_Comparison(b *testing.B) {
	const maxIPs = 10000

	b.Run("SingleMutex", func(b *testing.B) {
		handler, cleanup := benchRateLimitHandler(b, 1e9, 1<<30, maxIPs)
		defer cleanup()

		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			w := httptest.NewRecorder()
			i := 0
			for pb.Next() {
				w.Body.Reset()
				ip := fmt.Sprintf("10.0.%d.%d", (i/256)%256, i%256)
				handler.ServeHTTP(w, reqFromIP(ip))
				benchSink.Add(int64(w.Code))
				i++
				if i >= 100 {
					i = 0
				}
			}
		})
	})

	b.Run("Sharded16", func(b *testing.B) {
		handler, cleanup := benchShardedRateLimitHandler(b, 1e9, 1<<30, maxIPs, 16)
		defer cleanup()

		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			w := httptest.NewRecorder()
			i := 0
			for pb.Next() {
				w.Body.Reset()
				ip := fmt.Sprintf("10.0.%d.%d", (i/256)%256, i%256)
				handler.ServeHTTP(w, reqFromIP(ip))
				benchSink.Add(int64(w.Code))
				i++
				if i >= 100 {
					i = 0
				}
			}
		})
	})
}

// BenchmarkShardedRateLimit_VaryingShards measures throughput at different shard counts.
func BenchmarkShardedRateLimit_VaryingShards(b *testing.B) {
	for _, numShards := range []int{1, 4, 8, 16, 32} {
		b.Run(fmt.Sprintf("Shards_%d", numShards), func(b *testing.B) {
			handler, cleanup := benchShardedRateLimitHandler(b, 1e9, 1<<30, 10000, numShards)
			defer cleanup()

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				w := httptest.NewRecorder()
				i := 0
				for pb.Next() {
					w.Body.Reset()
					ip := fmt.Sprintf("10.0.%d.%d", (i/256)%256, i%256)
					handler.ServeHTTP(w, reqFromIP(ip))
					benchSink.Add(int64(w.Code))
					i++
					if i >= 100 {
						i = 0
					}
				}
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Functional tests for sharded variant
// ---------------------------------------------------------------------------

// fnv1aShard computes the shard index for an IP using the same FNV-1a hash
// as shardedRateLimiter.shardFor, for deterministic test setup.
func fnv1aShard(ip string, numShards uint32) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(ip); i++ {
		h ^= uint32(ip[i])
		h *= 16777619
	}
	return h % numShards
}

// shardedRateLimitWrapInternal creates a sharded rate-limited handler for testing.
func shardedRateLimitWrapInternal(t *testing.T, rps float64, burst, maxIPs int,
	sweepInterval, staleThreshold, evictLogInterval time.Duration,
	numShards int, next http.Handler,
) http.Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mw, done := shardedRateLimitMiddlewareInternal(ctx, rps, burst, maxIPs,
		sweepInterval, staleThreshold, evictLogInterval, numShards)
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return mw(next)
}

// TestShardedRateLimitTotalCapacity verifies that the total number of tracked IPs
// across all shards never exceeds maxIPs, and that evicted IPs get fresh limiters.
func TestShardedRateLimitTotalCapacity(t *testing.T) {
	const maxIPs = 16
	const numShards = 16

	// Part 1: Verify eviction works — 32 unique IPs through a capacity-16 limiter.
	ctx, cancel := context.WithCancel(context.Background())
	mw, done := shardedRateLimitMiddlewareInternal(ctx, 100, 100, maxIPs,
		time.Hour, time.Hour, time.Hour, numShards)
	t.Cleanup(func() {
		cancel()
		<-done
	})
	handler := mw(okHandler())

	for i := 0; i < 32; i++ {
		ip := fmt.Sprintf("10.0.%d.%d", i/256, i%256)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// Part 2: Verify evicted IP gets a fresh limiter (burst=1).
	// Deterministically find an IP that hashes to the same shard as "1.1.1.1"
	// so eviction is guaranteed with a single request.
	const targetIP = "1.1.1.1"
	targetShard := fnv1aShard(targetIP, numShards)
	var evictorIP string
	for i := 0; i < 10000; i++ {
		candidate := fmt.Sprintf("10.%d.%d.%d", (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
		if candidate != targetIP && fnv1aShard(candidate, numShards) == targetShard {
			evictorIP = candidate
			break
		}
	}
	if evictorIP == "" {
		t.Fatal("could not find an IP that hashes to the same shard as 1.1.1.1")
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	mw2, done2 := shardedRateLimitMiddlewareInternal(ctx2, 0.001, 1, maxIPs,
		time.Hour, time.Hour, time.Hour, numShards)
	t.Cleanup(func() {
		cancel2()
		<-done2
	})
	handler2 := mw2(okHandler())

	// Use burst token for target IP
	w := httptest.NewRecorder()
	handler2.ServeHTTP(w, reqFromIP(targetIP))
	if w.Code != http.StatusOK {
		t.Fatalf("burst token: expected 200, got %d", w.Code)
	}

	// Should be rate-limited now
	w = httptest.NewRecorder()
	handler2.ServeHTTP(w, reqFromIP(targetIP))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("burst exhausted: expected 429, got %d", w.Code)
	}

	// Evict target IP by sending a single IP to the same shard
	// (each shard has capacity 1 with maxIPs=16, numShards=16).
	w = httptest.NewRecorder()
	handler2.ServeHTTP(w, reqFromIP(evictorIP))
	if w.Code != http.StatusOK {
		t.Fatalf("evictor IP: expected 200, got %d", w.Code)
	}

	// Target IP should get a fresh limiter with a full burst token
	w = httptest.NewRecorder()
	handler2.ServeHTTP(w, reqFromIP(targetIP))
	if w.Code != http.StatusOK {
		t.Errorf("after eviction: expected 200 (fresh limiter), got %d", w.Code)
	}
}

// TestShardedRateLimitCleanup verifies the cleanup goroutine sweeps stale entries
// across all shards.
func TestShardedRateLimitCleanup(t *testing.T) {
	wrapped := shardedRateLimitWrapInternal(t,
		0.001, 1, 100,
		50*time.Millisecond,  // sweepInterval
		100*time.Millisecond, // staleThreshold
		time.Hour,            // evictLogInterval
		4,                    // numShards
		okHandler(),
	)

	// Use burst tokens from 4 IPs (likely hitting different shards)
	ips := []string{"5.5.5.5", "6.6.6.6", "7.7.7.7", "8.8.8.8"}
	for _, ip := range ips {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s first request: expected 200, got %d", ip, w.Code)
		}
	}

	// All should be rate-limited now (burst exhausted)
	for _, ip := range ips {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("IP %s second request: expected 429, got %d", ip, w.Code)
		}
	}

	// Wait for cleanup
	time.Sleep(400 * time.Millisecond)

	// All should get fresh limiters
	for _, ip := range ips {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Errorf("IP %s after cleanup: expected 200 (fresh limiter), got %d", ip, w.Code)
		}
	}
}

// TestShardedRateLimitBoundary verifies that maxIPs == defaultNumShards
// (exactly 1 IP per shard) works correctly through the public API.
func TestShardedRateLimitBoundary(t *testing.T) {
	// maxIPs == 16 routes to sharded with base capacity 1 per shard.
	wrapped := rateLimitWrap(t, 100, 100, defaultNumShards, okHandler())

	// All 16 unique IPs should succeed
	for i := 0; i < defaultNumShards; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Fatalf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}

	// Additional IPs should still succeed (eviction)
	for i := defaultNumShards; i < defaultNumShards+10; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Errorf("IP %s beyond capacity: expected 200, got %d", ip, w.Code)
		}
	}
}

// TestShardedRateLimitFallback verifies that maxIPs < defaultNumShards
// falls back to the single-mutex implementation (via RateLimitMiddleware).
func TestShardedRateLimitFallback(t *testing.T) {
	// maxIPs=5 < defaultNumShards(16), should use single-mutex path.
	// This test verifies basic functionality still works through the public API.
	wrapped := rateLimitWrap(t, 100, 1, 5, okHandler())

	// burst=1: first request succeeds
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("9.9.9.9"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Second request from same IP should be rate-limited
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, reqFromIP("9.9.9.9"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	// Fill remaining capacity + evict — should not panic
	for i := 0; i < 10; i++ {
		ip := fmt.Sprintf("30.0.0.%d", i)
		w = httptest.NewRecorder()
		wrapped.ServeHTTP(w, reqFromIP(ip))
		if w.Code != http.StatusOK {
			t.Errorf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}
}

// defaultCSP locks the byte-exact CSP value emitted when no security
// overrides are configured. Touching this string means the default CSP
// changed — review the change deliberately, then update both the test
// and any operator-facing docs that quote the policy.
const defaultCSP = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'"

func TestBuildCSP_ByteIdenticalDefault(t *testing.T) {
	if got := buildCSP(nil); got != defaultCSP {
		t.Errorf("default CSP drifted\n got: %s\nwant: %s", got, defaultCSP)
	}
	if got := buildCSP([]string{}); got != defaultCSP {
		t.Errorf("empty-slice CSP drifted\n got: %s\nwant: %s", got, defaultCSP)
	}
}

func TestBuildCSP(t *testing.T) {
	cases := []struct {
		name     string
		frameSrc []string
		want     []string // substrings that must appear
		notWant  []string // substrings that must NOT appear
	}{
		{
			name:     "no frame-src when nil",
			frameSrc: nil,
			want:     []string{"default-src 'self'", "frame-ancestors 'none'"},
			notWant:  []string{"frame-src"},
		},
		{
			name:     "no frame-src when empty slice",
			frameSrc: []string{},
			want:     []string{"default-src 'self'"},
			notWant:  []string{"frame-src"},
		},
		{
			name:     "single origin",
			frameSrc: []string{"https://lt-landing-demo.fly.dev"},
			want:     []string{"frame-src 'self' https://lt-landing-demo.fly.dev"},
		},
		{
			name:     "multiple origins separated by spaces",
			frameSrc: []string{"https://a.example.com", "https://b.example.com"},
			want:     []string{"frame-src 'self' https://a.example.com https://b.example.com"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildCSP(c.frameSrc)
			for _, sub := range c.want {
				if !strings.Contains(got, sub) {
					t.Errorf("CSP missing %q\nfull: %s", sub, got)
				}
			}
			for _, sub := range c.notWant {
				if strings.Contains(got, sub) {
					t.Errorf("CSP unexpectedly contains %q\nfull: %s", sub, got)
				}
			}
		})
	}
}

func TestSecurityHeadersMiddleware_FrameSrcEmittedFromConfig(t *testing.T) {
	mw := SecurityHeadersMiddleware([]string{"https://upstream.example.com"})
	wrapped := mw(okHandler())
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-src 'self' https://upstream.example.com") {
		t.Errorf("expected frame-src directive with upstream origin, got: %s", csp)
	}
	// Sanity-check the unrelated security headers still ship.
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}

func TestSecurityHeadersMiddleware_NoFrameSrcByDefault(t *testing.T) {
	mw := SecurityHeadersMiddleware(nil)
	wrapped := mw(okHandler())
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	csp := w.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "frame-src") {
		t.Errorf("frame-src should be omitted when not configured, got: %s", csp)
	}
}
