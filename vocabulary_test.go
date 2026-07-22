package tinkerdown

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// TestKnownAttributesAreReal keeps the allowlist honest.
//
// An allowlist is only as good as its correspondence to reality, and this one is
// hand-maintained so validate can stay fast. Without this check it is an unfalsifiable
// claim — exactly the failure mode the attribute reference itself fell into, where
// 8 of 11 documented attributes had stopped being real without anything noticing.
//
// Checked against the vendored bundle rather than a sibling client checkout, so it runs
// wherever the repo does.
func TestKnownAttributesAreReal(t *testing.T) {
	bundle, err := os.ReadFile("internal/assets/client/tinkerdown-client.browser.js")
	if err != nil {
		t.Fatalf("read client bundle: %v", err)
	}
	goSrc := readProductionGo(t)

	names := make([]string, 0, len(knownAttributes)+len(knownDataAttributes))
	for name := range knownAttributes {
		names = append(names, name)
	}
	for name := range knownDataAttributes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !appearsAsWholeName(name, bundle) && !appearsAsWholeName(name, goSrc) {
			t.Errorf("allowlist entry %q is implemented by neither the client bundle nor Tinkerdown — "+
				"validate would accept an attribute that does nothing", name)
		}
	}
}

func TestUnknownAttributes(t *testing.T) {
	tests := []struct {
		name     string
		markup   string
		wantName string
		wantHint string
	}{
		{
			// The case this exists for: a plausible invention that validate would
			// otherwise pass clean, leaving a generated page silently inert.
			name:     "an invented attribute is reported",
			markup:   `<table lvt-source="x" lvt-sortable></table>`,
			wantName: "lvt-sortable",
		},
		{
			name:     "a superseded name is reported with its replacement",
			markup:   `<div lvt-scroll="bottom"></div>`,
			wantName: "lvt-scroll",
			wantHint: "use lvt-fx:scroll",
		},
		{
			name:     "the fabricated lvt-value-* family is explained",
			markup:   `<button lvt-value-name="#x"></button>`,
			wantName: "lvt-value-name",
			wantHint: "not implemented",
		},
		{name: "a real attribute passes", markup: `<table lvt-source="x" lvt-columns="a"></table>`},
		{name: "a namespaced member passes", markup: `<form lvt-el:reset:on:success></form>`},
		{name: "an arbitrary lvt-on event passes", markup: `<div lvt-on:mouseenter="Hover"></div>`},
		{name: "a real data-lvt-* passes", markup: `<input data-lvt-force-update>`},
		{
			// data-lvt-* is a closed set, so an invented one must be caught like any
			// other. Allowing the bare prefix would have let this through — the same
			// hole this file exists to close, one namespace over.
			name:     "an invented data-lvt-* is reported",
			markup:   `<table data-lvt-sortable></table>`,
			wantName: "data-lvt-sortable",
		},
		{name: "plain HTML is untouched", markup: `<div class="x" data-id="1"><input name="q"></div>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page := &Page{ServerBlocks: map[string]*ServerBlock{"b": {Content: tc.markup}}}
			got := page.UnknownAttributes()

			if tc.wantName == "" {
				if len(got) != 0 {
					t.Fatalf("expected no unknown attributes, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Name != tc.wantName {
				t.Fatalf("expected %q reported, got %+v", tc.wantName, got)
			}
			if tc.wantHint != "" && !strings.Contains(got[0].Hint, tc.wantHint) {
				t.Errorf("hint %q should mention %q — a diagnostic without a usable hint cannot be self-corrected against",
					got[0].Hint, tc.wantHint)
			}
		})
	}
}

func readProductionGo(t *testing.T) []byte {
	t.Helper()
	var all []byte
	var walk func(string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			path := dir + "/" + name
			switch {
			case e.IsDir():
				if name == "node_modules" || name == ".git" || name == ".worktrees" {
					continue
				}
				walk(path)
			// vocabulary.go must be excluded, for the same reason collectGo excludes
			// it in attribute_docs_test.go: a substring scan cannot tell mentioning an
			// attribute from implementing one. Without this the guard is circular —
			// knownAttributes' own map keys live in this file, so every entry matches
			// itself and the test can never fail. Adding "lvt-totally-made-up" to the
			// allowlist passed cleanly until this line existed.
			case strings.HasSuffix(name, ".go") &&
				!strings.HasSuffix(name, "_test.go") &&
				name != "vocabulary.go":
				if b, err := os.ReadFile(path); err == nil {
					all = append(all, b...)
				}
			}
		}
	}
	walk(".")
	return all
}

// appearsAsWholeName matches an attribute name at token boundaries. Substring matching
// would report the dead lvt-scroll as live because data-lvt-scroll-sticky contains it.
func appearsAsWholeName(name string, src []byte) bool {
	s := string(src)
	idx := 0
	for {
		rel := strings.Index(s[idx:], name)
		if rel < 0 {
			return false
		}
		start := idx + rel
		end := start + len(name)
		beforeOK := start == 0 || !isNameChar(rune(s[start-1]))
		afterOK := end == len(s) || !isNameChar(rune(s[end]))
		if beforeOK && afterOK {
			return true
		}
		idx = start + 1
	}
}

func isNameChar(r rune) bool {
	return r == '-' || r == ':' || r == '_' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// TestInertAttributes covers the failure I produced by following this project's own
// skill instructions: a table in the markdown body rather than in an ```lvt fence.
//
// It validated clean, summarised as privileged, served without error, and rendered
// nothing — no source read, no console error, no server-log warning. Every available
// signal said success. That makes it the hardest of the three "clean but broken" cases
// to notice, and the easiest to write, since putting markup in the body is the natural
// thing to do.
func TestInertAttributes(t *testing.T) {
	tests := []struct {
		name   string
		static string
		want   []string
	}{
		{
			name:   "binding markup in the body is inert",
			static: `<table lvt-source="requests" lvt-columns="id"></table>`,
			want:   []string{"lvt-columns", "lvt-source"},
		},
		{
			name:   "an action binding in the body is inert",
			static: `<div lvt-on:click="Approve"></div>`,
			want:   []string{"lvt-on:click"},
		},
		{
			// data-lvt-* are client-side hints that mean something on ordinary
			// markup, so they are not inert out here.
			name:   "data-lvt-* is meaningful outside a block",
			static: `<input data-lvt-force-update>`,
		},
		{name: "plain HTML is untouched", static: `<div class="x"><input name="q"></div>`},
		{name: "empty static HTML", static: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := (&Page{StaticHTML: tc.static}).InertAttributes()
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got %v, want %v", got, tc.want)
					break
				}
			}
		})
	}

	t.Run("markup inside a block is not inert", func(t *testing.T) {
		page := &Page{ServerBlocks: map[string]*ServerBlock{
			"b": {Content: `<table lvt-source="requests"></table>`},
		}}
		if got := page.InertAttributes(); len(got) != 0 {
			t.Errorf("attributes inside an lvt block bind correctly; got %v", got)
		}
	})
}
