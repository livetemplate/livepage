package tinkerdown

import (
	"reflect"
	"testing"
)

// TestPageRefs covers extraction of what a document reaches for.
//
// This machinery exists because nothing else records it: an action name reaches the
// server from the client when a control is used, so the parse pipeline never
// enumerates a document's action references. Policy checks need them before anything
// is served, which is too early for the runtime to have seen a click.
func TestPageRefs(t *testing.T) {
	tests := []struct {
		name string
		page *Page
		want DocRefs
	}{
		{
			name: "source bindings come from block metadata",
			page: &Page{ServerBlocks: map[string]*ServerBlock{
				"b1": {Metadata: map[string]string{"lvt-source": "requests"}},
			}},
			want: DocRefs{Sources: []string{"requests"}},
		},
		{
			name: "action invocations are read from button and form names",
			page: &Page{ServerBlocks: map[string]*ServerBlock{
				"b1": {Content: `<button name="approve">OK</button><form name="deny"></form>`},
			}},
			want: DocRefs{Actions: []string{"approve", "deny"}},
		},
		{
			// The reason this is an HTML parse rather than a regex. `name` is a
			// legitimate attribute on form fields, where it means something entirely
			// different; matching it as an action would report every input as one.
			name: "name on inputs and selects is a form field, not an action",
			page: &Page{ServerBlocks: map[string]*ServerBlock{
				"b1": {Content: `<form name="create"><input name="title"><select name="status"></select></form>`},
			}},
			want: DocRefs{Actions: []string{"create"}},
		},
		{
			name: "lvt-source in markup is picked up alongside block metadata",
			page: &Page{ServerBlocks: map[string]*ServerBlock{
				"b1": {
					Metadata: map[string]string{"lvt-source": "requests"},
					Content:  `<table lvt-source="audit"></table>`,
				},
			}},
			want: DocRefs{Sources: []string{"audit", "requests"}},
		},
		{
			name: "declarations are tracked separately from references",
			page: &Page{
				Config: PageConfig{
					Sources: map[string]SourceConfig{"mine": {Type: "json"}},
					Actions: map[string]Action{"my-action": {Kind: "sql"}},
				},
			},
			want: DocRefs{
				DeclaredSources: []string{"mine"},
				DeclaredActions: []string{"my-action"},
			},
		},
		{
			name: "interactive blocks and static HTML are scanned too",
			page: &Page{
				InteractiveBlocks: map[string]*InteractiveBlock{
					"i1": {Content: `<button name="from-interactive"></button>`},
				},
				StaticHTML: `<button name="from-static"></button>`,
			},
			want: DocRefs{Actions: []string{"from-interactive", "from-static"}},
		},
		{
			name: "duplicates across blocks collapse",
			page: &Page{ServerBlocks: map[string]*ServerBlock{
				"b1": {Content: `<button name="approve"></button>`},
				"b2": {Content: `<button name="approve"></button>`},
			}},
			want: DocRefs{Actions: []string{"approve"}},
		},
		{
			name: "a nil page yields nothing rather than panicking",
			page: nil,
			want: DocRefs{},
		},
		{
			// Broken markup must not error: html.Parse is lenient, and a document
			// this broken has already failed the parse gate.
			name: "malformed markup is survivable",
			page: &Page{ServerBlocks: map[string]*ServerBlock{
				"b1": {Content: `<button name="approve"><div><span>`},
			}},
			want: DocRefs{Actions: []string{"approve"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.page.Refs()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Refs() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
