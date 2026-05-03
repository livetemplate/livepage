package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/livetemplate/tinkerdown/internal/config"
)

// proxyRoute is the runtime representation of a config.RouteEntry with
// type=proxy. It pairs a path matcher with a configured reverse-proxy
// handler. WebSocket upgrades are forwarded automatically by the stdlib
// reverse proxy (Go 1.12+).
type proxyRoute struct {
	pattern  string
	isPrefix bool
	upstream string
	handler  http.Handler
}

// newProxyRoute constructs a proxyRoute from a config entry. Returns an
// error if the entry's type is unsupported or upstream URL is unparseable.
func newProxyRoute(re config.RouteEntry) (*proxyRoute, error) {
	if re.Type != "proxy" {
		return nil, fmt.Errorf("unsupported route type %q (only %q supported)", re.Type, "proxy")
	}
	if re.Pattern == "" || !strings.HasPrefix(re.Pattern, "/") {
		return nil, fmt.Errorf("pattern %q must start with /", re.Pattern)
	}
	u, err := url.Parse(re.Upstream)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream %q: %w", re.Upstream, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("upstream %q must be http or https", re.Upstream)
	}

	rp := httputil.NewSingleHostReverseProxy(u)
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		// Restore the original request path. NewSingleHostReverseProxy's
		// default Director joins upstream.Path with req.URL.Path, which
		// is wrong for our passthrough model where upstream owns the
		// full URL space.
		req.URL.Path = req.URL.RawPath
		if req.URL.Path == "" {
			req.URL.Path = req.RequestURI
			if i := strings.IndexByte(req.URL.Path, '?'); i >= 0 {
				req.URL.Path = req.URL.Path[:i]
			}
		}
		req.Host = u.Host
	}

	return &proxyRoute{
		pattern:  re.Pattern,
		isPrefix: strings.HasSuffix(re.Pattern, "/"),
		upstream: re.Upstream,
		handler:  rp,
	}, nil
}

// matches reports whether the given request path falls under this route.
// Prefix routes (pattern ending in "/") match the path equal to
// pattern-without-trailing-slash, and any path starting with pattern.
// Exact routes match only on identical path.
func (p *proxyRoute) matches(path string) bool {
	if p.isPrefix {
		bare := strings.TrimSuffix(p.pattern, "/")
		return path == bare || strings.HasPrefix(path, p.pattern)
	}
	return path == p.pattern
}

// buildProxyRoutes converts config entries into runtime routes, skipping
// (and logging) any entries with errors so a single bad route does not
// abort startup. Returns the constructed slice and the count of skipped
// entries for the caller to log.
func buildProxyRoutes(entries []config.RouteEntry) ([]*proxyRoute, []error) {
	var routes []*proxyRoute
	var errs []error
	for i, re := range entries {
		pr, err := newProxyRoute(re)
		if err != nil {
			errs = append(errs, fmt.Errorf("route %d (%q): %w", i, re.Pattern, err))
			continue
		}
		routes = append(routes, pr)
	}
	return routes, errs
}
