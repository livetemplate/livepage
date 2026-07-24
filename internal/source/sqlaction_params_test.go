package source

import (
	"reflect"
	"testing"
)

// TestReferencedParams guards the shared :name scanner that both SubstituteParams
// (runtime) and `tinkerdown validate` (action-param completeness) rely on — so
// the two agree on which names a statement requires.
func TestReferencedParams(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"basic", []string{"INSERT INTO t (a,b) VALUES (:aaa, :bbb)"}, []string{"aaa", "bbb"}},
		{"postgres cast not a param", []string{"SELECT :x::text"}, []string{"x"}},
		{"time literal not a param", []string{"WHERE created = '12:30:00'"}, nil},
		{"trailing colon", []string{"SELECT 1 :"}, nil},
		{"union distinct across statements", []string{"a = :x", "b = :y", "c = :x"}, []string{"x", "y"}},
		{"underscores and digits", []string{"VALUES (:row_cap, :ttl2)"}, []string{"row_cap", "ttl2"}},
		{"none", []string{"SELECT 1"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ReferencedParams(c.in...)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ReferencedParams(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
