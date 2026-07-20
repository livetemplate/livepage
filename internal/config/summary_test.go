package config

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestSummarize(t *testing.T) {
	ro := &Config{
		Sources: map[string]SourceConfig{
			"queue": {Type: "sqlite", Describes: "Pending requests", Readonly: boolPtr(true)},
		},
		Generation: &GenerationConfig{Sources: []string{"queue"}},
	}

	t.Run("a read-only console is not privileged", func(t *testing.T) {
		s := ro.Summarize(DocumentRefs{Sources: []string{"queue"}})
		if s == nil {
			t.Fatal("expected a summary")
		}
		if s.Privileged {
			t.Error("a console that only reads should not demand operator review")
		}
		if len(s.Operations) != 1 || s.Operations[0].Describes != "Pending requests" {
			t.Errorf("describes metadata should reach the summary, got %+v", s.Operations)
		}
	})

	t.Run("a writable source makes it privileged", func(t *testing.T) {
		c := &Config{
			Sources:    map[string]SourceConfig{"queue": {Type: "sqlite", Readonly: boolPtr(false)}},
			Generation: &GenerationConfig{Sources: []string{"queue"}},
		}
		s := c.Summarize(DocumentRefs{Sources: []string{"queue"}})
		if !s.Privileged || !s.Operations[0].Writes {
			t.Errorf("a writable source should be privileged, got %+v", s)
		}
	})

	t.Run("an exec source is flagged as executing", func(t *testing.T) {
		c := &Config{
			Sources:    map[string]SourceConfig{"pods": {Type: "exec"}},
			Generation: &GenerationConfig{Sources: []string{"pods"}},
		}
		s := c.Summarize(DocumentRefs{Sources: []string{"pods"}})
		if !s.Operations[0].Execs || !s.Privileged {
			t.Errorf("exec must be surfaced, got %+v", s)
		}
	})

	t.Run("a rest source is flagged as reaching the network", func(t *testing.T) {
		c := &Config{
			Sources:    map[string]SourceConfig{"api": {Type: "rest"}},
			Generation: &GenerationConfig{Sources: []string{"api"}},
		}
		if s := c.Summarize(DocumentRefs{Sources: []string{"api"}}); !s.Operations[0].Network {
			t.Error("rest reaches off-host and must say so")
		}
	})

	t.Run("actions are writes and carry their describes", func(t *testing.T) {
		c := &Config{
			Actions:    map[string]*Action{"approve": {Kind: "sql", Describes: "Grants scoped access"}},
			Generation: &GenerationConfig{Actions: []string{"approve"}},
		}
		s := c.Summarize(DocumentRefs{Actions: []string{"approve"}})
		if !s.Privileged || !s.Operations[0].Writes {
			t.Errorf("an action changes something and is privileged, got %+v", s)
		}
		if s.Operations[0].Describes != "Grants scoped access" {
			t.Error("action describes should reach the summary")
		}
	})

	// An unapproved name is a policy violation. Describing it here would present
	// something the document may not do as though it were part of the plan.
	t.Run("unapproved names are not summarized", func(t *testing.T) {
		s := ro.Summarize(DocumentRefs{Sources: []string{"queue", "secrets"}})
		if len(s.Operations) != 1 || s.Operations[0].Name != "queue" {
			t.Errorf("only approved names belong in the summary, got %+v", s.Operations)
		}
	})

	t.Run("no manifest means no summary", func(t *testing.T) {
		c := &Config{Sources: map[string]SourceConfig{"queue": {Type: "sqlite"}}}
		if s := c.Summarize(DocumentRefs{Sources: []string{"queue"}}); s != nil {
			t.Errorf("without an approved surface there is no review step to feed, got %+v", s)
		}
	})
}
