package tenon

import "testing"

// TestObjectOfIsObjectVal holds objectOf, which takes the type its caller
// already has, to ObjectVal, which takes a map and works the type out: the
// two give the same value, of the same type, holding the same, over
// attributes in every state a decoded one can be in.
func TestObjectOfIsObjectVal(t *testing.T) {
	num, str := Type{numberType}, Type{stringType}
	list := List(num)
	// One value of each state an attribute can have, and containers that hold
	// one, since what a container holds decides the flags an object keeps.
	states := []struct {
		name  string
		typ   Type
		value Value
	}{
		{"known", num, NumberFromInt(1)},
		{"null", str, NullVal(str)},
		{"unknown", num, Unknown(num)},
		{"narrowed", num, Narrow(Unknown(num), NotNull())},
		{"marked", str, WithMarks(String("x"), probe{id: "m"})},
		{"holds an unknown", list, ListVal(num, Unknown(num))},
		{"holds a marked value", list, ListVal(num, WithMarks(NumberFromInt(2), probe{id: "m"}))},
		{"\U000000e9", num, NumberFromInt(3)}, // a name of more than one byte
	}
	for take := 1; take <= len(states); take++ {
		for start := range states {
			attrs := map[string]Type{}
			vals := map[string]Value{}
			for i := range take {
				s := states[(start+i)%len(states)]
				attrs[s.name] = s.typ
				vals[s.name] = s.value
			}
			typ := Object(attrs)
			// The decoder holds the attributes in the type's order, which is
			// the order objectOf takes them in.
			ordered := make([]Value, len(typ.t.attrs))
			for i, a := range typ.t.attrs {
				ordered[i] = vals[a.name]
			}
			got, want := objectOf(typ, ordered), ObjectVal(vals)
			switch {
			case !Identical(got, want):
				t.Fatalf("%d attributes from %d: objectOf gave %v, ObjectVal %v", take, start, got, want)
			case got.n.typ != want.n.typ:
				t.Fatalf("%d attributes from %d: objectOf gave type %v, ObjectVal %v", take, start, got.n.typ, want.n.typ)
			case got.n.partial != want.n.partial:
				t.Fatalf("%d attributes from %d: objectOf said partial %v, ObjectVal %v", take, start, got.n.partial, want.n.partial)
			case got.n.markedWithin != want.n.markedWithin:
				t.Fatalf("%d attributes from %d: objectOf said markedWithin %v, ObjectVal %v", take, start, got.n.markedWithin, want.n.markedWithin)
			}
		}
	}
}
