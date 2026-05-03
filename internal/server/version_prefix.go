package server

import "strings"

// stripVersionPrefix returns the request path with the configured version
// prefix removed. Empty prefix is a no-op. The prefix is matched as a path
// segment, not a substring: prefix "v1" strips "/v1" and "/v1/x" but not
// "/v1foo". An exact match of "/" + prefix is normalized to "/".
//
// Caller must apply the result to BOTH r.URL.Path AND r.URL.RawPath when
// the latter is set; otherwise downstream handlers reading RawPath (e.g.
// reverse-proxy directors) see an out-of-sync original-prefixed path.
func stripVersionPrefix(path, prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return path
	}
	full := "/" + prefix
	switch {
	case path == full:
		return "/"
	case strings.HasPrefix(path, full+"/"):
		return path[len(full):]
	default:
		return path
	}
}
