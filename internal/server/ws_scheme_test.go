package server

import (
	"crypto/tls"
	"net/http"
	"testing"
)

// TestDetectWSScheme covers the full matrix of how a tinkerdown server
// might learn that the public-facing scheme is HTTPS:
//   - X-Forwarded-Proto from an edge / reverse proxy (fly, cloudflare,
//     nginx, traefik). Most production deployments hit this path.
//   - r.TLS != nil for a server terminating TLS itself.
//   - Neither set: fall back to plain ws.
//
// A regression here means every interactive block on the page silently
// fails under HTTPS — chrome rejects mixed-content WS upgrades without
// any user-visible error. The browser's DevTools console is the only
// signal, which most operators won't see.
func TestDetectWSScheme(t *testing.T) {
	cases := []struct {
		name string
		req  *http.Request
		want string
	}{
		{
			name: "x-forwarded-proto https (fly/cloudflare/etc)",
			req: &http.Request{
				Header: http.Header{"X-Forwarded-Proto": []string{"https"}},
			},
			want: "wss",
		},
		{
			name: "x-forwarded-proto http",
			req: &http.Request{
				Header: http.Header{"X-Forwarded-Proto": []string{"http"}},
			},
			want: "ws",
		},
		{
			name: "direct https (r.TLS set)",
			req: &http.Request{
				Header: http.Header{},
				TLS:    &tls.ConnectionState{},
			},
			want: "wss",
		},
		{
			name: "plain http, no proxy",
			req: &http.Request{
				Header: http.Header{},
			},
			want: "ws",
		},
		{
			// The case where the priority order between the two checks
			// actually matters: the proxy advertises plain http but our
			// listening socket happens to be TLS. The proxy's signal is
			// authoritative — it knows what the client actually spoke.
			name: "x-forwarded-proto http overrides r.TLS (proxy is authoritative)",
			req: &http.Request{
				Header: http.Header{"X-Forwarded-Proto": []string{"http"}},
				TLS:    &tls.ConnectionState{},
			},
			want: "ws",
		},
		{
			// Defensive: net/http canonicalises header names but not
			// values. Most proxies send lowercase, but EqualFold guards
			// against the rare ones that don't.
			name: "x-forwarded-proto HTTPS (uppercase) still detected as wss",
			req: &http.Request{
				Header: http.Header{"X-Forwarded-Proto": []string{"HTTPS"}},
			},
			want: "wss",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectWSScheme(c.req); got != c.want {
				t.Errorf("detectWSScheme = %q, want %q", got, c.want)
			}
		})
	}
}
