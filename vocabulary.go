package tinkerdown

import (
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// knownAttributes is the set of exact-match lvt-* attribute names something implements
// — either Tinkerdown itself or the client bundle it ships.
//
// This exists because `tinkerdown validate` checks that a document *parses*, not that
// its attributes are real: an unknown lvt-* attribute is emitted as inert HTML and does
// nothing, silently. That is tolerable for a hand-written page whose author notices it
// did not work. It is not tolerable for a generated one, where the loop is "self-correct
// until validate is clean" — clean would otherwise mean nothing about whether the page
// functions, and a hallucinated attribute would survive every iteration.
//
// The list is deliberately hand-maintained rather than derived at runtime, so validate
// stays fast and dependency-free. TestKnownAttributesAreReal keeps it honest by checking
// every entry against the vendored client bundle and Tinkerdown's own source — the same
// two-directional guard the attribute reference uses. An entry that stops being real
// fails the build.
var knownAttributes = map[string]bool{
	// Tinkerdown-owned: data binding and auto-rendering.
	"lvt-source":    true,
	"lvt-columns":   true,
	"lvt-field":     true,
	"lvt-value":     true,
	"lvt-label":     true,
	"lvt-empty":     true,
	"lvt-actions":   true,
	"lvt-datatable": true,
	"lvt-persist":   true,

	// Client-owned: events, effects, forms, lifecycle, DOM guards.
	"lvt-key":          true,
	"lvt-autofocus":    true,
	"lvt-focus-trap":   true,
	"lvt-ignore":       true,
	"lvt-ignore-attrs": true,
	"lvt-debounce":     true,
	"lvt-upload":       true,
	"lvt-spy":          true,
	"lvt-redact":       true,
	"lvt-scroll-away":  true,
}

// knownPrefixes are namespaces whose members are dispatched at runtime rather than
// enumerated. `lvt-on:` takes arbitrary DOM event names; `lvt-el:` takes a method and a
// lifecycle state; `lvt-fx:`, `lvt-mod:`, `lvt-form:` and `lvt-nav:` take a behavior.
//
// Validating only the namespace is a deliberate limit, not an oversight: there is no
// enumerable member set for lvt-on:, and hard-coding one for the others would create a
// second list to rot. A typo in a *member* (lvt-el:bogus:on:success) still passes. A
// typo in the namespace, or an invented attribute entirely, does not — and that is the
// larger share of what a generating model gets wrong.
var knownPrefixes = []string{
	"lvt-on:",
	"lvt-el:",
	"lvt-fx:",
	"lvt-mod:",
	"lvt-form:",
	"lvt-nav:",
	"data-lvt-", // data-lvt-force-update, data-lvt-target, and friends
}

// UnknownAttribute is an lvt-* attribute a document uses that nothing implements.
type UnknownAttribute struct {
	Name string
	Hint string
}

// UnknownAttributes reports lvt-* attributes the page uses that nothing implements.
//
// Returned in a stable order so a generating agent sees the same diagnostics on every
// run and can converge, rather than chasing a different subset each iteration.
func (p *Page) UnknownAttributes() []UnknownAttribute {
	if p == nil {
		return nil
	}

	found := map[string]bool{}
	scan := func(markup string) {
		if strings.TrimSpace(markup) == "" {
			return
		}
		doc, err := html.Parse(strings.NewReader(markup))
		if err != nil {
			return
		}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				for _, attr := range n.Attr {
					if !strings.HasPrefix(attr.Key, "lvt-") && !strings.HasPrefix(attr.Key, "data-lvt-") {
						continue
					}
					if knownAttributes[attr.Key] || hasKnownPrefix(attr.Key) {
						continue
					}
					found[attr.Key] = true
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	for _, block := range p.ServerBlocks {
		if block != nil {
			scan(block.Content)
		}
	}
	for _, block := range p.InteractiveBlocks {
		if block != nil {
			scan(block.Content)
		}
	}
	scan(p.StaticHTML)

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]UnknownAttribute, 0, len(names))
	for _, name := range names {
		out = append(out, UnknownAttribute{Name: name, Hint: suggestAttribute(name)})
	}
	return out
}

func hasKnownPrefix(name string) bool {
	for _, prefix := range knownPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// suggestAttribute points at the right name when the wrong one is a known migration.
// A bare guess would be worse than none — a wrong suggestion sends a self-correcting
// agent somewhere specific and wrong, which is harder to recover from than "unknown".
func suggestAttribute(name string) string {
	migrations := map[string]string{
		"lvt-scroll":       "lvt-fx:scroll",
		"lvt-highlight":    "lvt-fx:highlight",
		"lvt-animate":      "lvt-fx:animate",
		"lvt-throttle":     "lvt-mod:throttle",
		"lvt-disable-with": "lvt-form:disable-with",
		"lvt-preserve":     "lvt-ignore (or lvt-form:preserve for form values)",
		"lvt-click-away":   "an lvt-el: lifecycle state, e.g. lvt-el:removeClass:on:click-away",
		"lvt-modal-open":   "a native <dialog> element",
		"lvt-modal-close":  "a native <dialog> element",
		"lvt-filter":       "filter at the source instead (a SQL query, or a computed source)",
	}
	if to, ok := migrations[name]; ok {
		return "use " + to
	}
	if strings.HasPrefix(name, "lvt-value-") {
		return "lvt-value-* is not implemented; pass extra values as data-* attributes or a form field"
	}
	return ""
}
