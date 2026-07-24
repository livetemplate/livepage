package server

import "testing"

// TestBlockHelperFuncsReachParseAndTreeGen guards that Tinkerdown's base block
// helpers (split) reach BOTH funcMaps a block needs at serve time:
//   - the component set's Funcs, which reach the block's PARSE (New / Validate);
//   - getComponentFuncs, applied via tmpl.Funcs after New for TREE GENERATION.
//
// A helper present in only one would let the block parse and then fail to
// render, or vice versa — the exact split-brain that made split's absence
// invisible until serve.
func TestBlockHelperFuncsReachParseAndTreeGen(t *testing.T) {
	sets := ComponentTemplates()
	if len(sets) == 0 || sets[0].Funcs["split"] == nil {
		t.Fatal("split missing from ComponentTemplates()[0].Funcs — a block using it would fail to PARSE")
	}
	if getComponentFuncs()["split"] == nil {
		t.Fatal("split missing from getComponentFuncs() — a block using it would fail TREE GENERATION")
	}
}
