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

	names := make([]string, 0, len(knownAttributes))
	for name := range knownAttributes {
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
		{name: "data-lvt-* passes", markup: `<input data-lvt-force-update>`},
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
			case strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go"):
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
