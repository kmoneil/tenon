package tenon

import (
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// TestEqualsReadsNothingWithinWhatItShares holds Equals to settling a known
// node compared with itself without reading what it holds. A comparison that
// walked the node and asked its members nothing, as one comparing each member
// with itself would, costs the whole value all the same, and no count of what
// the members are asked can see it. The node here is a list whose content is
// not a list, which no constructor makes and which reading panics on, so a
// comparison that reads it fails whatever it would have answered: one that
// walks through it, one that stops only at what it holds, and one that stops
// at its members but still goes through them.
func TestEqualsReadsNothingWithinWhatItShares(t *testing.T) {
	conformance.Covers(t, "EQ-002")
	numbers := List(NumberType())
	unreadable := Value{&node{state: stateKnown, typ: numbers, data: "not a list"}}
	part := ListVal(numbers, unreadable)
	prior := ObjectVal(map[string]Value{"part": part, "version": NumberFromInt(1)})
	planned := ObjectVal(map[string]Value{"part": part, "version": NumberFromText("1.0")})
	for _, tt := range []struct {
		name string
		a, b Value
	}{
		{"the node compared with itself", unreadable, unreadable},
		{"a value holding it compared with itself", part, part},
		{"two values sharing it", prior, planned},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: Equals read what the two share: %v", tt.name, r)
				}
			}()
			// The operands are not printed: printing reads them.
			if got := Equals(tt.a, tt.b).String(); got != "true" {
				t.Errorf("%s: Equals gave %s, want true", tt.name, got)
			}
		}()
	}
}
