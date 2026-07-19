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
			// Pass the raw token: normalizeAttribute trims internally, *after* its
			// allowlist lookups. Trimming here first would strip the trailing dash
			// from "lvt-value-" before the neverImplemented check could match it.
			attr := normalizeAttribute(m[1])
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
func normalizeAttribute(raw string) string {
	if strings.ContainsAny(raw, "{}") {
		return ""
	}
	// Look up the raw token *before* trimming, so family entries that end in a dash
	// ("lvt-value-") can match. Trimming first silently defeated this: "lvt-value-"
	// became "lvt-value", which resolves against the real, unrelated select-binding
	// attribute of that name — so the fabricated family passed by coincidence.
	if documentedAsRemoved[raw] || neverImplemented[raw] {
		return ""
	}
	attr := strings.TrimRight(raw, "-:")
	if documentedAsRemoved[attr] || neverImplemented[attr] {
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

// readGoSources concatenates every production .go file in the repo. Walking "." is
// enough — collectGo recurses, so naming internal/cmd/pkg as well would just scan them
// twice.
func readGoSources(t *testing.T) []byte {
	t.Helper()
	var all []byte
	collectGo(t, ".", &all)
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

// TestNormalizeAttribute pins the classification rules directly, rather than trusting
// the doc surfaces to keep exercising them by accident. That indirection already hid
// one bug: the "lvt-value-" family entry was dead because the caller trimmed the
// trailing dash before the lookup, and the check passed only because the unrelated
// real attribute "lvt-value" exists. Nothing failed, so nothing surfaced it.
func TestNormalizeAttribute(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"concrete attribute passes through", "lvt-source", "lvt-source"},
		{"placeholder skipped", "lvt-{action}-on:{event}", ""},
		{"namespaced instance reduces to its namespace", "lvt-el:addClass:on:error", "lvt-el:"},
		{"effect instance reduces to its namespace", "lvt-fx:highlight:on:success", "lvt-fx:"},
		{"bare namespace skipped", "lvt-el", ""},
		{"superseded name skipped", "lvt-scroll", ""},
		{"removed name skipped", "lvt-modal-open", ""},
		{"fabricated attribute skipped", "lvt-filter", ""},

		// The regression the bot caught. "lvt-value-" must be skipped as a fabricated
		// family and must NOT collapse onto the real "lvt-value", which is a distinct,
		// implemented select-binding attribute.
		{"fabricated family skipped even with trailing dash", "lvt-value-", ""},
		{"instance of the fabricated family skipped", "lvt-value-name", ""},
		{"the real lvt-value is unaffected", "lvt-value", "lvt-value"},

		// An unknown family must still be checked, not waved through. It normalizes to
		// its trimmed name, which resolves only if something implements it — so a
		// newly-invented family still fails the guard.
		{"unknown family is checked, not skipped", "lvt-widget-", "lvt-widget"},
		// An un-enumerated instance of the fabricated family folds onto the family
		// name, so listing every instance in the map is not required.
		{"un-enumerated instance folds onto its family", "lvt-value-foo", "lvt-value-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeAttribute(tc.in); got != tc.want {
				t.Errorf("normalizeAttribute(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestImplementedMatchesWholeNames pins the boundary rule. Substring matching would
// report the long-dead lvt-scroll as live, because "data-lvt-scroll-sticky" contains it.
func TestImplementedMatchesWholeNames(t *testing.T) {
	src := []byte(`"data-lvt-scroll-sticky","lvt-fx:scroll","lvt-ignore"`)
	cases := []struct {
		attr string
		want bool
	}{
		{"lvt-ignore", true},
		{"lvt-fx:scroll", true},
		{"lvt-scroll", false}, // only present inside data-lvt-scroll-sticky
		{"lvt-ign", false},    // prefix of a real name is not a match
		{"lvt-fx:", true},     // namespace prefix: only the leading boundary applies
		{"lvt-missing", false},
	}
	for _, tc := range cases {
		if got := implemented(tc.attr, src); got != tc.want {
			t.Errorf("implemented(%q) = %v, want %v", tc.attr, got, tc.want)
		}
	}
}
