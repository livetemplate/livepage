package tinkerdown_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestDocumentedAttributesExist guards the invariant that every lvt-* attribute the
// docs teach is one that something actually implements — either the client bundle we
// ship or Tinkerdown's own Go code.
//
// This exists because the reference rotted badly enough to matter: an audit found
// attributes documented under names the client had migrated away from
// (lvt-scroll -> lvt-fx:scroll), attributes absent from the client entirely
// (lvt-modal-open), and one — lvt-filter — that was never implemented anywhere yet was
// taught in the skill reference an LLM reads as generation context.
//
// Nothing downstream catches this: `tinkerdown validate` checks that a document parses,
// not that its attributes exist, so unknown lvt-* attributes pass through as inert HTML.
// A doc-level check is therefore the only thing standing between a stale reference and
// a generated app that silently does nothing.
//
// Checked against the *vendored* bundle rather than a sibling client checkout, so this
// runs anywhere the repo does (CI included) and tests the client that actually ships.
func TestDocumentedAttributesExist(t *testing.T) {
	const bundlePath = "internal/assets/client/tinkerdown-client.browser.js"

	// The surfaces that teach the vocabulary. A human reads the first; an LLM is given
	// the rest as context, which is why their accuracy is load-bearing.
	docSurfaces := []string{
		"docs/reference/lvt-attributes.md",
		"docs/llms.txt",
		"skills/tinkerdown/reference.md",
		"docs/llm-system-prompt.md",
	}

	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read client bundle: %v", err)
	}
	goSources := readGoSources(t)

	// (?:^|[^-\w]) keeps the match from starting inside a longer name — without it,
	// "data-lvt-force-update" yields a phantom "lvt-force-update".
	attrPattern := regexp.MustCompile(`(?:^|[^-\w])(lvt-[a-zA-Z0-9:_-]+)`)

	found := map[string][]string{} // attribute -> surfaces naming it
	for _, surface := range docSurfaces {
		content, err := os.ReadFile(surface)
		if err != nil {
			t.Fatalf("read %s: %v", surface, err)
		}
		for _, m := range attrPattern.FindAllStringSubmatch(string(content), -1) {
			attr := normalizeAttribute(strings.TrimRight(m[1], "-:"))
			if attr == "" {
				continue
			}
			if !contains(found[attr], surface) {
				found[attr] = append(found[attr], surface)
			}
		}
	}

	if len(found) == 0 {
		t.Fatal("extracted no attributes from the doc surfaces — the regex or the paths are wrong")
	}

	var missing []string
	for attr := range found {
		if !implemented(attr, bundle) && !implemented(attr, goSources) {
			missing = append(missing, attr)
		}
	}
	sort.Strings(missing)

	for _, attr := range missing {
		t.Errorf("attribute %q is documented in %s but implemented in neither %s nor any .go file",
			attr, strings.Join(found[attr], ", "), bundlePath)
	}
}

// implemented reports whether attr appears in src as a whole attribute name rather than
// as a prefix of a longer one. Substring matching is not good enough: "lvt-scroll" is a
// substring of "data-lvt-scroll-sticky", so a naive check reports the long-dead
// lvt-scroll as live.
func implemented(attr string, src []byte) bool {
	idx := 0
	for {
		rel := strings.Index(string(src[idx:]), attr)
		if rel < 0 {
			return false
		}
		start := idx + rel
		end := start + len(attr)

		beforeOK := start == 0 || !isAttrChar(rune(src[start-1]))
		// A namespace/family prefix ("lvt-form:", "lvt-value-") is expected to be
		// followed by its member, so only the leading boundary is meaningful.
		afterOK := strings.HasSuffix(attr, ":") || strings.HasSuffix(attr, "-") ||
			end == len(src) || !isAttrChar(rune(src[end]))
		if beforeOK && afterOK {
			return true
		}
		idx = start + 1
	}
}

func isAttrChar(r rune) bool {
	return r == '-' || r == ':' || r == '_' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// documentedAsRemoved are names the docs mention only in order to say they are gone —
// the § Namespace migration table and the "Removed." callouts. They must NOT resolve
// against the client; that is the point of naming them. Deleting a row from that table
// without deleting it here simply stops checking something, so keep the two in sync.
var documentedAsRemoved = map[string]bool{
	"lvt-scroll":       true, // -> lvt-fx:scroll
	"lvt-highlight":    true, // -> lvt-fx:highlight
	"lvt-animate":      true, // -> lvt-fx:animate
	"lvt-throttle":     true, // -> lvt-mod:throttle
	"lvt-debounce":     true, // -> lvt-mod:debounce
	"lvt-disable-with": true, // -> lvt-form:disable-with
	"lvt-preserve":     true, // -> lvt-ignore or lvt-form:preserve
	"lvt-click-away":   true, // removed; survives only as an lvt-el: lifecycle state
	"lvt-window":       true, // lvt-window-{event}, removed
	"lvt-focus-trap":   true, // removed; use native <dialog>
	"lvt-modal-open":   true, // removed; use native <dialog>
	"lvt-modal-close":  true, // removed; use native <dialog>
	"lvt-value-name":   true, // instance of the fabricated lvt-value-* family, below
}

// neverImplemented are attributes the docs once taught that no implementation ever had.
// They are named now only to say so. Distinct from documentedAsRemoved, which migrated
// to a new name — these had no destination, which is why they were hard to spot: there
// was no rename to notice, just an attribute that quietly did nothing.
var neverImplemented = map[string]bool{
	"lvt-filter": true, // "filtering" on datatables — filter at the source instead
	"lvt-value-": true, // "extract values from elements" — use data-* or a form
}

// normalizeAttribute reduces a documented token to the name worth looking up, or ""
// to skip it. Three kinds of token are not literal attribute names:
//
//   - placeholders — lvt-{action}-on:{event}
//   - namespaced *instances* — lvt-el:addClass:on:error is one use of the
//     lvt-el:{method}:on:{state} form; the client implements the namespace and
//     dispatches members at runtime, so only the namespace is checkable
//   - wildcard families — lvt-value-name is an instance of lvt-value-*
func normalizeAttribute(attr string) string {
	if strings.ContainsAny(attr, "{}") || documentedAsRemoved[attr] || neverImplemented[attr] {
		return ""
	}
	if ns, _, ok := strings.Cut(attr, ":"); ok {
		switch ns {
		case "lvt-el", "lvt-fx", "lvt-mod", "lvt-form", "lvt-nav", "lvt-on":
			// Check the namespace is real; members are dispatched at runtime.
			return ns + ":"
		}
	}
	if strings.HasPrefix(attr, "lvt-value-") {
		return "lvt-value-" // the lvt-value-* family
	}
	switch attr {
	case "lvt-fx", "lvt-el", "lvt-mod", "lvt-form", "lvt-nav", "lvt-on":
		return "" // bare prefix, no member
	}
	return attr
}

func readGoSources(t *testing.T) []byte {
	t.Helper()
	var all []byte
	for _, dir := range []string{".", "internal", "cmd", "pkg"} {
		collectGo(t, dir, &all)
	}
	return all
}

func collectGo(t *testing.T, dir string, out *[]byte) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := dir + "/" + e.Name()
		switch {
		case e.IsDir():
			if e.Name() == "node_modules" || e.Name() == ".git" || e.Name() == ".worktrees" {
				continue
			}
			collectGo(t, path, out)
		// Production code only. Test files must not count as evidence that an
		// attribute exists: a fixture using a made-up attribute would vouch for the
		// docs that invented it — and this very file names lvt-filter in its comments,
		// which silently defeated the check until _test.go was excluded.
		case strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go"):
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			*out = append(*out, b...)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
