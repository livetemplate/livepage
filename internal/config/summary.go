package config

import "sort"

// Operation is one privileged thing a document does: a source it reads or an action it
// runs, described in terms an operator can judge.
type Operation struct {
	Kind      string `json:"kind"`                // "source" or "action"
	Name      string `json:"name"`                //
	Type      string `json:"type"`                // source type, or action kind
	Describes string `json:"describes,omitempty"` // the manifest's human-readable note
	Execs     bool   `json:"execs,omitempty"`     // runs a shell command
	Writes    bool   `json:"writes,omitempty"`    // mutates stored data
	Network   bool   `json:"network,omitempty"`   // talks to something off-host
}

// OperationSummary is what a document does with its approved surface, in a form an
// operator can review before it is served.
//
// Privileged carries the proportionality rule. A console that only reads is not worth
// interrupting anyone over; one that executes shell commands, writes data, or reaches
// the network is. The distinction exists so review attention is spent where it matters
// — a prompt shown for every generated page is a prompt nobody reads.
type OperationSummary struct {
	Privileged bool        `json:"privileged"`
	Operations []Operation `json:"operations"`
}

// Summarize describes what a document does with the approved surface it uses.
//
// Only approved names are described. An unapproved name is a policy violation, and the
// lint reports it as such — summarizing it here would present something the document
// may not do as though it were part of the plan.
//
// Returns nil when the project declares no generation block: without an approved
// surface there is no operator review step for a summary to feed.
func (c *Config) Summarize(refs DocumentRefs) *OperationSummary {
	if !c.IsManifest() {
		return nil
	}

	summary := &OperationSummary{}

	for _, name := range refs.Sources {
		if !c.ApprovedSource(name) {
			continue
		}
		src, ok := c.Sources[name]
		if !ok {
			continue
		}
		op := Operation{
			Kind:      "source",
			Name:      name,
			Type:      src.Type,
			Describes: src.Describes,
			Execs:     src.Type == "exec",
			Network:   sourceReachesNetwork(src.Type),
			// A source that is not read-only can be written through the table and
			// form affordances Tinkerdown generates from it. IsReadonly owns the
			// "nil means read-only" default; re-deriving it here would drift if that
			// default ever changed.
			Writes: !src.IsReadonly(),
		}
		summary.Operations = append(summary.Operations, op)
	}

	for _, name := range refs.Actions {
		if !c.ApprovedAction(name) {
			continue
		}
		action, ok := c.Actions[name]
		if !ok || action == nil {
			continue
		}
		summary.Operations = append(summary.Operations, Operation{
			Kind:      "action",
			Name:      name,
			Type:      action.Kind,
			Describes: action.Describes,
			Execs:     action.Kind == "exec",
			// http reaches the network directly; a sql action inherits the reach of
			// the source it runs against, so one pointed at pg/rest/graphql is
			// talking off-host even though the action itself is "just SQL".
			Network: action.Kind == "http" || c.actionReachesNetwork(action),
			// Every action exists to change something; that is what distinguishes
			// one from a source. Treating them all as writes avoids guessing at SQL.
			Writes: true,
		})
	}

	sort.SliceStable(summary.Operations, func(i, j int) bool {
		if summary.Operations[i].Kind != summary.Operations[j].Kind {
			return summary.Operations[i].Kind < summary.Operations[j].Kind
		}
		return summary.Operations[i].Name < summary.Operations[j].Name
	})

	for _, op := range summary.Operations {
		if op.Execs || op.Writes || op.Network {
			summary.Privileged = true
			break
		}
	}
	return summary
}

// actionReachesNetwork reports whether an action's backing source leaves the host.
func (c *Config) actionReachesNetwork(action *Action) bool {
	if action.Source == "" {
		return false
	}
	src, ok := c.Sources[action.Source]
	return ok && sourceReachesNetwork(src.Type)
}

func sourceReachesNetwork(sourceType string) bool {
	switch sourceType {
	case "rest", "graphql", "pg":
		return true
	}
	return false
}
