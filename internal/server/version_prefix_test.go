package server

import "testing"

func TestStripVersionPrefix(t *testing.T) {
	cases := []struct {
		name, path, prefix, want string
	}{
		{"empty prefix is no-op", "/foo/bar", "", "/foo/bar"},
		{"empty prefix and root", "/", "", "/"},
		{"exact prefix match becomes root", "/v1", "v1", "/"},
		{"prefix with leading slash works", "/v1", "/v1", "/"},
		{"strip prefix from sub-path", "/v1/foo/bar", "v1", "/foo/bar"},
		{"do not strip if not present", "/foo/bar", "v1", "/foo/bar"},
		{"do not strip when prefix is a substring of segment", "/v1foo/bar", "v1", "/v1foo/bar"},
		{"do not strip when prefix is part of deeper segment", "/foo/v1/bar", "v1", "/foo/v1/bar"},
		{"trailing slash on prefix is normalized", "/v1/x", "v1/", "/x"},
		{"latest as prefix", "/latest/getting-started/install", "latest", "/getting-started/install"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stripVersionPrefix(c.path, c.prefix)
			if got != c.want {
				t.Errorf("stripVersionPrefix(%q, %q) = %q, want %q", c.path, c.prefix, got, c.want)
			}
		})
	}
}
