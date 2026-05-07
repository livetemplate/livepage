package server

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	tinkerdown "github.com/livetemplate/tinkerdown"
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
		// Save Path AND RawPath before the default Director runs.
		// NewSingleHostReverseProxy's default Director joins
		// upstream.Path with req.URL.Path, which is wrong for our
		// passthrough model where the upstream owns the full URL
		// space — restore both fields after to forward the request
		// path verbatim, including any percent-encoding.
		savedPath, savedRaw := req.URL.Path, req.URL.RawPath
		origDirector(req)
		req.URL.Path, req.URL.RawPath = savedPath, savedRaw
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

// newAutoProxyRoute builds a runtime proxy route from a (path, upstream)
// pair declared by an embed-lvt block. Same shape as a config entry —
// just routed through the same constructor so behavior matches exactly.
func newAutoProxyRoute(pattern, upstream string) (*proxyRoute, error) {
	return newProxyRoute(config.RouteEntry{
		Pattern:  pattern,
		Type:     "proxy",
		Upstream: upstream,
	})
}

// registerAutoEmbedRoutes walks every discovered page, collects unique
// (path, upstream) pairs declared by embed-lvt blocks, and appends them
// to the server's proxy route table. Conflicts (same path mapping to
// different upstreams) and overlaps with config-declared routes are
// logged as warnings — first registration wins.
//
// Caller must hold s.mu. Safe to call multiple times across re-discovery
// because we de-duplicate by pattern against the existing route slice.
func (s *Server) registerAutoEmbedRoutes() {
	if s.siteManager == nil && len(s.routes) == 0 {
		return
	}

	existing := make(map[string]string, len(s.proxyRoutes))
	for _, pr := range s.proxyRoutes {
		existing[pr.pattern] = pr.upstream
	}

	pages := s.collectPagesForAutoRoutes()
	added := 0
	for _, page := range pages {
		if page == nil {
			continue
		}
		for _, er := range page.EmbedRoutes {
			pattern := er.Path
			upstream := er.Upstream
			if pattern == "" || upstream == "" {
				continue
			}
			if existingUpstream, ok := existing[pattern]; ok {
				if existingUpstream != upstream {
					log.Printf("[Routes] embed-lvt route %s declares upstream %s but route already maps to %s — keeping existing",
						pattern, upstream, existingUpstream)
				}
				continue
			}
			pr, err := newAutoProxyRoute(pattern, upstream)
			if err != nil {
				log.Printf("[Routes] skipping invalid embed-lvt route %s -> %s: %v", pattern, upstream, err)
				continue
			}
			s.proxyRoutes = append(s.proxyRoutes, pr)
			existing[pattern] = upstream
			added++
		}
	}
	if added > 0 {
		log.Printf("[Routes] auto-registered %d embed-lvt proxy route(s)", added)
	}
}

// collectPagesForAutoRoutes returns every page in the server's known
// route table — covers both the legacy tutorial path (s.routes) and the
// site-mode path (s.siteManager).
//
// Lock contract: this function is itself lock-free, so callers must
// hold s.mu (read or write) for the duration of any iteration that
// reads from the returned slice's pages while Discover may run
// concurrently. Both existing callers (refreshWatcherIncludes under
// the write lock, isIncludedFile under the read lock) honor this.
func (s *Server) collectPagesForAutoRoutes() []*tinkerdown.Page {
	if s.siteManager != nil {
		nodes := s.siteManager.AllPages()
		pages := make([]*tinkerdown.Page, 0, len(nodes))
		for _, n := range nodes {
			if n != nil && n.Page != nil {
				pages = append(pages, n.Page)
			}
		}
		return pages
	}
	pages := make([]*tinkerdown.Page, 0, len(s.routes))
	for _, r := range s.routes {
		if r != nil && r.Page != nil {
			pages = append(pages, r.Page)
		}
	}
	return pages
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
