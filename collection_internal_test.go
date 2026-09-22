package tenon

import (
	"math/rand"
	"slices"
	"testing"
)

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

// TestWithNothingLeftToBeIsTheScan holds the members a set over an element
// type holding few values keeps to what comparing every one of those values
// with every member gave, which is what working out once what the set does
// not hold stands in for.
func TestWithNothingLeftToBeIsTheScan(t *testing.T) {
	// scan is what withNothingLeftToBe did before: for each member that is
	// not known, every value of the element type, compared with the member
	// and then looked for among the members kept.
	scan := func(typ Type, members []Value) []Value {
		c := setCeiling(typ)
		if !c.set || c.n > maxDomainSet {
			return members
		}
		known := knownMembers(members)
		if known == len(members) {
			return members
		}
		values := memberValues(typ.t.elem)
		kept, dropped := members[:known:known], false
		for _, m := range members[known:] {
			only := true
			for _, v := range values {
				if eq, settled := equality(v.n, m.n); (!settled || eq) && !sameAsSome(members[:known], v) {
					only = false
					break
				}
			}
			if only {
				dropped = true
				continue
			}
			kept = append(kept, m)
		}
		if !dropped {
			return members
		}
		return kept
	}
	boo := Type{boolType}
	pair := Tuple(boo, boo)
	object := Object(map[string]Type{"a": boo, "b": boo})
	r := rand.New(rand.NewSource(20260922))
	cases, dropped := 0, 0
	for _, elem := range []Type{boo, pair, object, Set(boo)} {
		values := memberValues(elem)
		// Values that are not known, of the element type: an unknown, one
		// that cannot be null, and for a tuple one that is known in part.
		open := []Value{Unknown(elem), Narrow(Unknown(elem), NotNull())}
		if elem == pair {
			open = append(open, TupleVal(Bool(true), Unknown(boo)), TupleVal(Unknown(boo), NullVal(boo)))
		}
		for range 200 {
			var raw []Value
			for _, v := range values {
				if r.Intn(3) > 0 {
					raw = append(raw, v)
				}
			}
			for range r.Intn(3) + 1 {
				raw = append(raw, open[r.Intn(len(open))])
			}
			r.Shuffle(len(raw), func(i, j int) { raw[i], raw[j] = raw[j], raw[i] })
			prepared := orderMembers(distinctMembers(slices.Clone(raw)))
			want := scan(Set(elem), slices.Clone(prepared))
			got := withNothingLeftToBe(Set(elem), slices.Clone(prepared))
			if len(got) != len(want) {
				t.Fatalf("%v over %d members kept %d, the scan kept %d", elem, len(prepared), len(got), len(want))
			}
			for i := range got {
				if !Identical(got[i], want[i]) {
					t.Fatalf("%v: member %d is %v, the scan kept %v", elem, i, got[i], want[i])
				}
			}
			cases++
			if len(got) < len(prepared) {
				dropped++
			}
		}
	}
	// The comparison is worth making only where some of these sets leave a
	// member nothing to be, which is what both ways of deciding it do.
	if cases < 500 || dropped < 50 {
		t.Errorf("%d sets compared, %d of which dropped a member", cases, dropped)
	}
}
