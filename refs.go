package tinkerdown

import (
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// DocRefs is what a document reaches for: the sources it binds to and the actions it
// invokes, plus what it declared for itself in frontmatter.
//
// Referenced and declared are tracked separately because policy cares about the
// difference. A document that *references* an unapproved name is reaching outside its
// approved surface; one that *declares* an unapproved name is trying to bring its own,
// which is how an approved-set check that looks only at references gets bypassed —
// declare `evil`, reference `evil`, and every name in the document is one it defined.
type DocRefs struct {
	Sources         []string // source names the document binds to
	Actions         []string // action names the document invokes
	DeclaredSources []string // sources the document declares in its own frontmatter
	DeclaredActions []string // actions the document declares in its own frontmatter
}

// Refs extracts the sources and actions a parsed page reaches for.
//
// Source bindings are read from block metadata, which the parser already records.
// Action invocations are not recorded anywhere: an action name reaches the server from
// the *client* when a control is used (GenericState.HandleAction), so nothing in the
// parse pipeline ever enumerates them. They are recovered here by parsing block markup
// for the `name` attribute on <button> and <form>, which is how an action is bound.
//
// Parsing rather than pattern-matching matters: `name` is a legitimate attribute on
// <input> and <select> (where it is a form field, not an action), so a regex over the
// markup would report every form field as an action reference.
func (p *Page) Refs() DocRefs {
	var refs DocRefs
	if p == nil {
		return refs
	}

	sources := map[string]bool{}
	actions := map[string]bool{}

	for _, block := range p.ServerBlocks {
		if block == nil {
			continue
		}
		if name := block.Metadata["lvt-source"]; name != "" {
			sources[name] = true
		}
		collectRefs(block.Content, sources, actions)
	}
	for _, block := range p.InteractiveBlocks {
		if block == nil {
			continue
		}
		collectRefs(block.Content, sources, actions)
	}
	collectRefs(p.StaticHTML, sources, actions)

	refs.Sources = sortedKeys(sources)
	refs.Actions = sortedKeys(actions)
	for name := range p.Config.Sources {
		refs.DeclaredSources = append(refs.DeclaredSources, name)
	}
	for name := range p.Config.Actions {
		refs.DeclaredActions = append(refs.DeclaredActions, name)
	}
	sort.Strings(refs.DeclaredSources)
	sort.Strings(refs.DeclaredActions)
	return refs
}

// collectRefs walks markup and records source bindings and action invocations.
//
// Malformed markup is not an error here: html.Parse is lenient by design, and a
// document broken enough to defeat it will have failed the parse gate long before
// anything asks what it references.
func collectRefs(markup string, sources, actions map[string]bool) {
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
				switch attr.Key {
				case "lvt-source":
					if attr.Val != "" {
						sources[attr.Val] = true
					}
				case "name":
					// Only on the elements that bind an action. On <input> and
					// <select>, `name` is a form field.
					if n.Data == "button" || n.Data == "form" {
						if attr.Val != "" {
							actions[attr.Val] = true
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
