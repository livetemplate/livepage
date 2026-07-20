package config

import (
	"fmt"
	"sort"
	"strings"
)

// PolicyViolation is one way a document steps outside the approved surface.
type PolicyViolation struct {
	Kind    string // "source" or "action"
	Name    string // the offending name
	Reason  string // what is wrong
	Hint    string // what to do instead
	Shadows bool   // the name is approved, and the document tried to redefine it
}

func (v PolicyViolation) Error() string {
	if v.Hint == "" {
		return fmt.Sprintf("%s %q: %s", v.Kind, v.Name, v.Reason)
	}
	return fmt.Sprintf("%s %q: %s (%s)", v.Kind, v.Name, v.Reason, v.Hint)
}

// DocumentRefs is what a policy check needs to know about a document: the names it
// reaches for, and the names it brought itself.
//
// This mirrors tinkerdown.DocRefs. It is redeclared here so the config package — which
// owns the approved set — does not import the root package that produces the refs.
type DocumentRefs struct {
	Sources         []string
	Actions         []string
	DeclaredSources []string
	DeclaredActions []string
}

// CheckPolicy reports every way a document steps outside this project's approved
// surface. It returns nothing when the project declares no generation block: approval
// is opt-in, and a project without a manifest has no approved set to violate.
//
// Two distinct checks, because two distinct things can be wrong:
//
//   - A *reference* to an unapproved name reaches outside the approved surface.
//   - A *declaration* of an unapproved name brings its own. Checking only references
//     would miss this entirely — a document that declares `evil` and then references
//     `evil` references only names it defined, so every reference resolves and the
//     document looks clean.
//
// Shadowing — declaring a name that *is* approved — is reported but is not a security
// boundary here. Precedence already pins approved names at runtime, so the declaration
// is inert. The diagnostic exists so a generating agent learns why its definition had
// no effect, rather than silently producing a document whose frontmatter is ignored.
func (c *Config) CheckPolicy(refs DocumentRefs) []PolicyViolation {
	if !c.IsManifest() {
		return nil
	}

	var out []PolicyViolation

	for _, name := range refs.Sources {
		if c.ApprovedSource(name) {
			continue
		}
		// A source the document declared itself is reported as a declaration
		// violation below; reporting it twice would be noise.
		if contains(refs.DeclaredSources, name) {
			continue
		}
		out = append(out, PolicyViolation{
			Kind:   "source",
			Name:   name,
			Reason: "not in the approved set",
			Hint:   approvedHint("sources", c.Generation.Sources),
		})
	}

	for _, name := range refs.Actions {
		if c.ApprovedAction(name) || contains(refs.DeclaredActions, name) {
			continue
		}
		out = append(out, PolicyViolation{
			Kind:   "action",
			Name:   name,
			Reason: "not in the approved set",
			Hint:   approvedHint("actions", c.Generation.Actions),
		})
	}

	for _, name := range refs.DeclaredSources {
		if c.ApprovedSource(name) {
			out = append(out, PolicyViolation{
				Kind:    "source",
				Name:    name,
				Reason:  "is approved and cannot be redefined; this definition is ignored",
				Hint:    "remove it from frontmatter and use the approved source",
				Shadows: true,
			})
			continue
		}
		out = append(out, PolicyViolation{
			Kind:   "source",
			Name:   name,
			Reason: "is declared in this document but not approved",
			Hint:   approvedHint("sources", c.Generation.Sources),
		})
	}

	for _, name := range refs.DeclaredActions {
		if c.ApprovedAction(name) {
			out = append(out, PolicyViolation{
				Kind:    "action",
				Name:    name,
				Reason:  "is approved and cannot be redefined; this definition is ignored",
				Hint:    "remove it from frontmatter and use the approved action",
				Shadows: true,
			})
			continue
		}
		out = append(out, PolicyViolation{
			Kind:   "action",
			Name:   name,
			Reason: "is declared in this document but not approved",
			Hint:   approvedHint("actions", c.Generation.Actions),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func approvedHint(field string, approved []string) string {
	if len(approved) == 0 {
		return fmt.Sprintf("generation.%s is empty; nothing is approved", field)
	}
	return fmt.Sprintf("approved %s: %s", field, strings.Join(approved, ", "))
}

func contains(list []string, name string) bool {
	for _, s := range list {
		if s == name {
			return true
		}
	}
	return false
}
