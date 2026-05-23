package include

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLineRange(t *testing.T) {
	cases := []struct {
		in           string
		wantStart    int
		wantEnd      int
		wantErrOnArg string // substring expected in err.Error(); "" = expect no error
	}{
		{"20-32", 20, 32, ""},
		{"1-1", 1, 1, ""},
		{"5", 5, 5, ""}, // single-line shorthand
		{" 7 - 10 ", 7, 10, ""},
		{"32-20", 0, 0, "before start"},
		{"abc", 0, 0, "invalid syntax"},
		{"0-5", 0, 0, "start must be >= 1"},
		// "-3" splits to ("", "3"); the empty start fails Atoi before we
		// reach the >=1 check. Either error message is fine — the input
		// is just wrong.
		{"-3", 0, 0, "lines="},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			start, end, err := parseLineRange(c.in)
			if c.wantErrOnArg == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if start != c.wantStart || end != c.wantEnd {
					t.Errorf("got (%d,%d), want (%d,%d)", start, end, c.wantStart, c.wantEnd)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErrOnArg) {
				t.Errorf("got err %v, want substring %q", err, c.wantErrOnArg)
			}
		})
	}
}

func TestResolve_PathConfinement(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "docs")
	if err := os.MkdirAll(page, 0o755); err != nil {
		t.Fatal(err)
	}

	// In-tree relative path: ok.
	target := filepath.Join(root, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(page, root, "../src/main.go")
	if err != nil {
		t.Fatalf("Resolve in-tree: %v", err)
	}
	wantResolved, _ := filepath.EvalSymlinks(target)
	if got != wantResolved {
		t.Errorf("got %q, want %q", got, wantResolved)
	}

	// Escape attempt: rejected.
	if _, err := Resolve(page, root, "../../etc/passwd"); err == nil {
		t.Errorf("expected escape to be rejected")
	}

	// Empty include: rejected.
	if _, err := Resolve(page, root, ""); err == nil {
		t.Errorf("expected empty path to be rejected")
	}

	// Nonexistent file: rejected (we EvalSymlinks the candidate).
	if _, err := Resolve(page, root, "./nope.go"); err == nil {
		t.Errorf("expected missing file to be rejected")
	}
}

func TestResolve_SiteRootedInclude(t *testing.T) {
	// Project layout:
	//   project/
	//     content/      ← discovery root passed to Resolve
	//       recipes/
	//         counter/
	//           index.md  (the page)
	//     examples/
	//       counter/
	//         counter.go  ← cited via /examples/counter/counter.go
	project := t.TempDir()
	contentRoot := filepath.Join(project, "content")
	pageDir := filepath.Join(contentRoot, "recipes", "counter")
	exampleFile := filepath.Join(project, "examples", "counter", "counter.go")
	for _, d := range []string{pageDir, filepath.Dir(exampleFile)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exampleFile, []byte("package counter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Leading-slash path resolves project-relative — reaches the sibling
	// examples/ folder that a page-relative include would be forbidden
	// from touching.
	got, err := Resolve(pageDir, contentRoot, "/examples/counter/counter.go")
	if err != nil {
		t.Fatalf("site-rooted include: %v", err)
	}
	wantResolved, _ := filepath.EvalSymlinks(exampleFile)
	if got != wantResolved {
		t.Errorf("got %q, want %q", got, wantResolved)
	}

	// Verify a page-relative escape attempt to the same file is still
	// rejected — the new branch only triggers on the leading slash.
	if _, err := Resolve(pageDir, contentRoot, "../../../examples/counter/counter.go"); err == nil {
		t.Errorf("page-relative escape should still be rejected")
	}
}

func TestResolve_SiteRootedRejectsProjectEscape(t *testing.T) {
	// Even with the broader project-root confinement, attempts to
	// escape beyond the project root must fail. Authors who genuinely
	// want to include arbitrary filesystem paths shouldn't be able to.
	project := t.TempDir()
	contentRoot := filepath.Join(project, "content")
	pageDir := filepath.Join(contentRoot, "page")
	outsideFile := filepath.Join(filepath.Dir(project), "outside.go")
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outsideFile)

	// The leading slash makes this project-rooted, but "/../outside.go"
	// resolves above the project root → still rejected.
	if _, err := Resolve(pageDir, contentRoot, "/../outside.go"); err == nil {
		t.Errorf("expected site-rooted escape above project to be rejected")
	}
}

func TestPreprocess_GitHubFooter_SiteRooted(t *testing.T) {
	// A site-rooted include can reach into a sibling folder that
	// page-relative semantics would forbid. The footer must use the
	// include attribute verbatim as the repo-relative path — NOT
	// PagePathInRepo (which describes the page's location, not the
	// included file's).
	project := t.TempDir()
	contentRoot := filepath.Join(project, "content")
	pageDir := filepath.Join(contentRoot, "recipes", "counter")
	src := filepath.Join(project, "examples", "counter", "counter.go")
	for _, d := range []string{pageDir, filepath.Dir(src)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(src, []byte("L1\nL2\nL3\nL4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	md := "```go include=\"/examples/counter/counter.go\" lines=\"2-3\"\n```\n"
	out, _, warnings := PreprocessWithLinks([]byte(md), pageDir, contentRoot, LinkOptions{
		RepoURL:        "https://github.com/foo/bar",
		Branch:         "main",
		PagePathInRepo: "content/recipes/counter",
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	got := string(out)
	// PagePathInRepo MUST be ignored for site-rooted paths — the URL
	// should reflect the actual examples/counter location.
	wantHref := `href="https://github.com/foo/bar/blob/main/examples/counter/counter.go#L2-L3"`
	if !strings.Contains(got, wantHref) {
		t.Errorf("missing expected site-rooted footer link; got:\n%s", got)
	}
	if strings.Contains(got, "content/recipes/counter/examples") {
		t.Errorf("PagePathInRepo should not be prefixed to site-rooted paths; got:\n%s", got)
	}
}

func TestSlice_FullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	content := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Slice(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestSlice_LineRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	content := "L1\nL2\nL3\nL4\nL5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		lines string
		want  string
	}{
		{"middle range", "2-4", "L2\nL3\nL4"},
		{"single line via shorthand", "3", "L3"},
		{"single line via N-N", "3-3", "L3"},
		{"first line only", "1-1", "L1"},
		{"last line only", "5-5", "L5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Slice(path, c.lines)
			if err != nil {
				t.Fatalf("Slice: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSlice_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("L1\nL2\nL3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// End past EOF: clamped, returns what's available.
	got, err := Slice(path, "2-9999")
	if err != nil {
		t.Fatalf("expected end-clamp to succeed, got %v", err)
	}
	if got != "L2\nL3" {
		t.Errorf("got %q, want clamped %q", got, "L2\nL3")
	}

	// Start past EOF: hard error (genuinely wrong).
	if _, err := Slice(path, "20-30"); err == nil {
		t.Errorf("expected start-out-of-range to error")
	}
}

func TestSlice_FileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("L1\nL2\nL3"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Slice(path, "1-3")
	if err != nil {
		t.Fatal(err)
	}
	if got != "L1\nL2\nL3" {
		t.Errorf("got %q, want %q", got, "L1\nL2\nL3")
	}
}

func TestDedent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "uniform 4-space indent",
			in:   "    line one\n    line two\n    line three",
			want: "line one\nline two\nline three",
		},
		{
			name: "preserves relative indent",
			in:   "    if x {\n        y()\n    }",
			want: "if x {\n    y()\n}",
		},
		{
			name: "blank lines ignored when computing prefix",
			in:   "    a\n\n    b",
			want: "a\n\nb",
		},
		{
			name: "no common prefix returns input unchanged",
			in:   "no indent\n    indented",
			want: "no indent\n    indented",
		},
		{
			name: "tabs and spaces don't merge",
			// tab vs four-spaces are different bytes; common prefix is empty.
			in:   "\ta\n    b",
			want: "\ta\n    b",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Dedent(c.in)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSlice_MissingFile(t *testing.T) {
	if _, err := Slice("/nonexistent/path/that/should/not/exist", ""); err == nil {
		t.Errorf("expected missing file to error")
	}
}

func TestPreprocess_SubstitutesFenceBody(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "# Page\n\n" +
		"```go include=\"./src.go\" lines=\"2-4\"\n" +
		"```\n" +
		"\nfollow-up paragraph\n"

	out, included, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(included) != 1 {
		t.Errorf("expected 1 included file, got %d", len(included))
	}
	got := string(out)
	if !strings.Contains(got, `<pre class="language-go line-numbers"`) {
		t.Errorf("expected raw <pre> with language and line-numbers classes; got:\n%s", got)
	}
	if !strings.Contains(got, `data-start="2"`) {
		t.Errorf("expected data-start matching first range; got:\n%s", got)
	}
	if !strings.Contains(got, "L2\nL3\nL4") {
		t.Errorf("expected sliced lines in body; got:\n%s", got)
	}
	if strings.Contains(got, "L1") || strings.Contains(got, "L5") {
		t.Errorf("lines outside range should NOT appear: %s", got)
	}
	if !strings.Contains(got, "follow-up paragraph") {
		t.Errorf("subsequent prose should pass through:\n%s", got)
	}
}

func TestPreprocess_WholeFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\"\n" +
		"```\n"
	out, included, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(included) != 1 {
		t.Errorf("expected 1 included file, got %d", len(included))
	}
	got := string(out)
	if !strings.Contains(got, `class="language-go line-numbers"`) {
		t.Errorf("expected line-numbers class; got:\n%s", got)
	}
	// Whole-file include implies no data-start (defaults to 1).
	if strings.Contains(got, "data-start=") {
		t.Errorf("whole-file include should not set data-start: %s", got)
	}
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Errorf("expected file content in body; got:\n%s", got)
	}
}

func TestPreprocess_MissingFileWarns(t *testing.T) {
	root := t.TempDir()
	md := "```go include=\"./nope.go\"\n```\n"
	out, included, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", warnings)
	}
	if len(included) != 0 {
		t.Errorf("expected no included files on failure, got %v", included)
	}
	// Original block should pass through unchanged.
	if !strings.Contains(string(out), "include=") {
		t.Errorf("expected original fence preserved: %s", out)
	}
}

func TestPreprocess_RegularBlocksUnchanged(t *testing.T) {
	root := t.TempDir()
	md := "# Title\n\n" +
		"```go\n" +
		"// regular block, untouched\n" +
		"x := 1\n" +
		"```\n" +
		"\nprose\n"
	out, included, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(included) != 0 {
		t.Errorf("expected no includes, got %v", included)
	}
	if string(out) != md {
		t.Errorf("regular markdown should round-trip:\n--- got ---\n%s\n--- want ---\n%s", out, md)
	}
}

func TestPreprocess_DropsIncludeAttrFromOutput(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	// All four include-specific attrs MUST disappear from the
	// rendered output — they're authoring metadata, not content.
	// Other fence attrs are also dropped because the include block
	// emits raw HTML rather than a markdown fence; documented as a
	// v1 trade-off.
	md := "```go include=\"./src.go\" lines=\"1\" highlight=\"1\"\n```\n"
	out, _, _ := Preprocess([]byte(md), root, root)
	got := string(out)
	for _, attr := range []string{`include=`, `lines="`, `region="`, `highlight=`} {
		if strings.Contains(got, attr) {
			t.Errorf("%q should be stripped from output: %s", attr, got)
		}
	}
}

func TestPreprocess_GitHubFooter(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\" lines=\"2-4\"\n```\n"

	out, _, warnings := PreprocessWithLinks([]byte(md), root, root, LinkOptions{
		RepoURL:        "https://github.com/foo/bar",
		Branch:         "main",
		PagePathInRepo: "examples/demo",
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	got := string(out)
	wantHref := `href="https://github.com/foo/bar/blob/main/examples/demo/src.go#L2-L4"`
	if !strings.Contains(got, wantHref) {
		t.Errorf("missing expected footer link; got:\n%s", got)
	}
	// Label should include the file basename plus the line range.
	if !strings.Contains(got, ">src.go:2-4</a>") {
		t.Errorf("missing footer label src.go:2-4: %s", got)
	}
}

func TestPreprocess_GitHubFooter_DefaultBranch(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\"\n```\n"
	out, _, _ := PreprocessWithLinks([]byte(md), root, root, LinkOptions{
		RepoURL: "https://github.com/foo/bar",
	})
	// No PagePathInRepo, no Branch → branch defaults to main, path is
	// just the include's relative path.
	if !strings.Contains(string(out), "/blob/main/src.go") {
		t.Errorf("expected default branch=main and root-relative path, got:\n%s", out)
	}
	// No line range means no fragment.
	if strings.Contains(string(out), "#L") {
		t.Errorf("whole-file include should not have a line fragment: %s", out)
	}
}

func TestPreprocess_NoFooterWhenRepoUnset(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\"\n```\n"
	out, _, _ := PreprocessWithLinks([]byte(md), root, root, LinkOptions{})
	if strings.Contains(string(out), "tinkerdown-include-source") {
		t.Errorf("footer should be omitted when RepoURL empty; got:\n%s", out)
	}
}

func TestPreprocess_RejectsNonHTTPSchemes(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\"\n```\n"
	for _, evil := range []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"vbscript:msgbox",
		"ftp://example.com/repo",
	} {
		out, _, _ := PreprocessWithLinks([]byte(md), root, root, LinkOptions{RepoURL: evil})
		if strings.Contains(string(out), "tinkerdown-include-source") {
			t.Errorf("footer should be omitted for non-http(s) RepoURL %q; got:\n%s", evil, out)
		}
	}
}

func TestPreprocess_NoFooterWhenIncludeOutsidePageDir(t *testing.T) {
	root := t.TempDir()
	pageDir := filepath.Join(root, "guides")
	otherDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(otherDir, "src.go")
	if err := os.WriteFile(src, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Include sits in a sibling directory of the page; filepath.Rel
	// from pageDir to src returns "../lib/src.go", which the footer
	// logic detects via the "..-prefix" check and skips.
	md := "```go include=\"../lib/src.go\"\n```\n"
	out, _, _ := PreprocessWithLinks([]byte(md), pageDir, root, LinkOptions{
		RepoURL: "https://github.com/foo/bar",
	})
	if strings.Contains(string(out), "tinkerdown-include-source") {
		t.Errorf("footer should be omitted when include is outside pageDir; got:\n%s", out)
	}
	// The snippet itself still renders.
	if !strings.Contains(string(out), "body") {
		t.Errorf("expected snippet body to render despite missing footer; got:\n%s", out)
	}
}

func TestSliceRanges_RejectsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	// Write maxIncludeBytes+1 bytes — just past the cap.
	big := make([]byte, maxIncludeBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(path, big, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Slice(path, ""); err == nil {
		t.Fatal("expected size-cap error, got nil")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' in error, got: %v", err)
	}
}

func TestSliceRanges_MultiRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("L1\nL2\nL3\nL4\nL5\nL6\nL7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := SliceRanges(path, "1-2,5-6", "// ...")
	if err != nil {
		t.Fatal(err)
	}
	want := "L1\nL2\n\n// ...\n\nL5\nL6"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSliceRanges_NoSeparator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Empty separator → ranges joined by blank line only.
	got, err := SliceRanges(path, "1-1,3-3,5-5", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "L1\n\nL3\n\nL5"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLanguageEllipsis(t *testing.T) {
	cases := map[string]string{
		"go":     "// ...",
		"GO":     "// ...",
		"python": "# ...",
		"yaml":   "# ...",
		"html":   "<!-- ... -->",
		"sql":    "-- ...",
		"":       "// ...",
	}
	for lang, want := range cases {
		if got := LanguageEllipsis(lang); got != want {
			t.Errorf("LanguageEllipsis(%q) = %q, want %q", lang, got, want)
		}
	}
}

func TestPreprocess_MultiRange(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	body := "package main\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\nfunc D() {}\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\" lines=\"3-3,5-5\"\n```\n"
	out, _, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if !strings.Contains(string(out), "func A()") || !strings.Contains(string(out), "func C()") {
		t.Errorf("expected funcs A and C in output:\n%s", out)
	}
	if !strings.Contains(string(out), "// ...") {
		t.Errorf("expected ellipsis between ranges:\n%s", out)
	}
	if strings.Contains(string(out), "func B()") || strings.Contains(string(out), "func D()") {
		t.Errorf("non-cited functions should not appear:\n%s", out)
	}
}

func TestFindRegion(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.go")
	body := "package main\n" +
		"\n" +
		"// >>> region:state\n" +
		"type Counter struct {\n" +
		"    Count int\n" +
		"}\n" +
		"// <<< region:state\n" +
		"\n" +
		"// >>> region:handler\n" +
		"func Increment() {}\n" +
		"// <<< region:handler\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	start, end, err := FindRegion(src, "state")
	if err != nil {
		t.Fatalf("FindRegion(state): %v", err)
	}
	// Markers at 1-idx lines 3 and 7 → content is lines 4-6.
	if start != 4 || end != 6 {
		t.Errorf("state region: got [%d,%d], want [4,6]", start, end)
	}

	start2, end2, err := FindRegion(src, "handler")
	if err != nil {
		t.Fatalf("FindRegion(handler): %v", err)
	}
	if start2 != 10 || end2 != 10 {
		t.Errorf("handler region: got [%d,%d], want [10,10]", start2, end2)
	}
}

func TestFindRegion_Missing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.go")
	if err := os.WriteFile(src, []byte("nothing here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := FindRegion(src, "ghost"); err == nil {
		t.Errorf("expected error for missing region")
	}
}

func TestFindRegion_OtherCommentStyles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.py")
	body := "# >>> region:py\nprint('hi')\n# <<< region:py\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	start, end, err := FindRegion(src, "py")
	if err != nil || start != 2 || end != 2 {
		t.Errorf("got [%d,%d] err=%v, want [2,2]", start, end, err)
	}

	src2 := filepath.Join(dir, "src.html")
	body2 := "<!-- >>> region:html -->\n<p>hi</p>\n<!-- <<< region:html -->\n"
	if err := os.WriteFile(src2, []byte(body2), 0o644); err != nil {
		t.Fatal(err)
	}
	start2, end2, err := FindRegion(src2, "html")
	if err != nil || start2 != 2 || end2 != 2 {
		t.Errorf("got [%d,%d] err=%v, want [2,2]", start2, end2, err)
	}
}

func TestPreprocess_RegionAttribute(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	body := "package main\n" +
		"// >>> region:state\n" +
		"type Counter struct{ Count int }\n" +
		"// <<< region:state\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\" region=\"state\"\n```\n"
	out, included, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(included) != 1 {
		t.Errorf("expected 1 included file, got %d", len(included))
	}
	if !strings.Contains(string(out), "type Counter struct{ Count int }") {
		t.Errorf("expected region content; got:\n%s", out)
	}
	if strings.Contains(string(out), ">>> region:") || strings.Contains(string(out), "<<< region:") {
		t.Errorf("markers should be excluded from output:\n%s", out)
	}
	if strings.Contains(string(out), "region=") {
		t.Errorf("region= attribute should be stripped from rendered fence:\n%s", out)
	}
}

func TestPreprocess_HighlightAttribute(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.go")
	if err := os.WriteFile(src, []byte("L1\nL2\nL3\nL4\nL5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"./src.go\" highlight=\"2-3\"\n```\n"
	out, _, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	got := string(out)
	if !strings.Contains(got, `data-line="2-3"`) {
		t.Errorf("expected data-line attribute on <pre>; got:\n%s", got)
	}
	if !strings.Contains(got, `<pre class="language-go line-numbers"`) {
		t.Errorf("expected raw <pre> with language + line-numbers class; got:\n%s", got)
	}
	if !strings.Contains(got, "<code class=\"language-go\">L1\nL2\nL3\nL4\nL5") {
		t.Errorf("expected escaped content inside <code>; got:\n%s", got)
	}
	if strings.Contains(got, "highlight=") {
		t.Errorf("highlight= should be stripped from rendered fence: %s", got)
	}
}

func TestPreprocess_NestedIncludeIgnored(t *testing.T) {
	// A page documenting the `include=` syntax — the nested 3-backtick
	// example MUST pass through verbatim, not get processed even if
	// the cited file exists.
	root := t.TempDir()
	src := filepath.Join(root, "real.go")
	if err := os.WriteFile(src, []byte("REAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "````markdown\n" +
		"```go include=\"./real.go\"\n" +
		"```\n" +
		"````\n"
	out, included, warnings := Preprocess([]byte(md), root, root)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if len(included) != 0 {
		t.Errorf("nested include must not register file: %v", included)
	}
	if !strings.Contains(string(out), "include=") {
		t.Errorf("nested include= text should pass through verbatim:\n%s", out)
	}
	if strings.Contains(string(out), "REAL") {
		t.Errorf("nested include should NOT have its body substituted:\n%s", out)
	}
}

func TestPreprocess_PathConfinement(t *testing.T) {
	// Page lives inside root; an include path that escapes the root
	// must produce a warning and pass the original fence through.
	root := t.TempDir()
	page := filepath.Join(root, "docs")
	if err := os.MkdirAll(page, 0o755); err != nil {
		t.Fatal(err)
	}
	// Real file that exists outside the root.
	outside := filepath.Join(t.TempDir(), "secret.go")
	if err := os.WriteFile(outside, []byte("password"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "```go include=\"" + outside + "\"\n```\n"
	_, included, warnings := Preprocess([]byte(md), page, root)
	if len(warnings) == 0 {
		t.Errorf("expected escape warning, got none")
	}
	if len(included) != 0 {
		t.Errorf("escape should not register the file: %v", included)
	}
}
