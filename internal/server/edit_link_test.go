package server

import "testing"

func TestBuildEditURL_FrontmatterWins(t *testing.T) {
	// When source_repo + source_path are both set, they take precedence.
	got := buildEditURL(
		"https://github.com/livetemplate/docs",
		"https://github.com/livetemplate/livetemplate",
		"docs/guides/x.md",
		"reference/some-page.md",
	)
	want := "https://github.com/livetemplate/livetemplate/edit/main/docs/guides/x.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildEditURL_FallsBackToSiteRepo(t *testing.T) {
	// No frontmatter source → use site repo + page's own relative path.
	got := buildEditURL(
		"https://github.com/livetemplate/docs",
		"",
		"",
		"recipes/foo.md",
	)
	want := "https://github.com/livetemplate/docs/edit/main/recipes/foo.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildEditURL_PartialFrontmatterFallsBack(t *testing.T) {
	// source_repo set but source_path empty → fall back fully.
	got := buildEditURL(
		"https://github.com/livetemplate/docs",
		"https://github.com/livetemplate/livetemplate",
		"",
		"reference/some-page.md",
	)
	want := "https://github.com/livetemplate/docs/edit/main/reference/some-page.md"
	if got != want {
		t.Errorf("partial frontmatter should fall back: got %q, want %q", got, want)
	}
}

func TestBuildEditURL_EmptyWhenNothingSet(t *testing.T) {
	if got := buildEditURL("", "", "", ""); got != "" {
		t.Errorf("expected empty URL, got %q", got)
	}
	if got := buildEditURL("", "", "", "page.md"); got != "" {
		t.Errorf("no repo at all → empty, got %q", got)
	}
}

func TestBuildEditURL_NormalizesSlashes(t *testing.T) {
	got := buildEditURL(
		"https://github.com/livetemplate/docs/", // trailing /
		"",
		"",
		"/reference/foo.md", // leading /
	)
	want := "https://github.com/livetemplate/docs/edit/main/reference/foo.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildEditURL_NonGitHubReturnsEmpty(t *testing.T) {
	// Phase 1 only supports GitHub. GitLab/Bitbucket would need per-host
	// templates; returning "" here avoids rendering a broken link.
	got := buildEditURL(
		"https://gitlab.com/example/docs",
		"",
		"",
		"page.md",
	)
	if got != "" {
		t.Errorf("non-GitHub repo should return empty, got %q", got)
	}
}
