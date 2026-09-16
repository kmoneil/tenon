package tenon_test

import (
	"slices"
	"testing"

	"tenon"
	"tenon/conformance"
	"tenon/conformance/values"
)

func TestConformance_EQ045_CanonicalOrder(t *testing.T) {
	conformance.Covers(t, "EQ-045")
	str, num := tenon.StringType(), tenon.NumberType()
	s := func(text string) tenon.Value { return tenon.String(text) }
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	// Null first, then the kinds in the order the rule gives, whatever each
	// value holds.
	ordered := []tenon.Value{
		tenon.NullVal(tenon.BoolType()),
		tenon.Bool(false),
		tenon.Bool(true),
		n(-1),
		n(1),
		s(""),
		s("a"),
		tenon.ListVal(str, s("a")),
		tenon.SetVal(str, s("a")),
		tenon.MapVal(str, map[string]tenon.Value{"k": s("a")}),
		tenon.TupleVal(s("a")),
		tenon.ObjectVal(map[string]tenon.Value{"a": s("a")}),
		tenon.CapsuleVal(tenon.Capsule("held", tenon.CapsuleOps[point]{}), &point{1, 2}),
	}
	for i := range ordered[:len(ordered)-1] {
		if got := tenon.CanonicalCompare(ordered[i], ordered[i+1]); got >= 0 {
			t.Errorf("%v does not sort before %v: %d", ordered[i], ordered[i+1], got)
		}
	}
	// Within a kind, the rule for that kind decides.
	for _, tt := range []struct {
		name string
		a, b tenon.Value
	}{
		{"false before true", tenon.Bool(false), tenon.Bool(true)},
		{"numbers numerically", n(2), n(10)},
		{"numbers across zero", n(-10), n(2)},
		{"strings by scalar value", s("Z"), s("a")},
		{"a string before one it starts", s("ab"), s("abc")},
		{"the normalized form is what sorts", s("f"), s("e\U00000301")},
		{"lists by element", tenon.ListVal(num, n(1)), tenon.ListVal(num, n(2))},
		{"a list before one it starts", tenon.ListVal(num, n(1)), tenon.ListVal(num, n(1), n(0))},
		{
			"sets by their members in order",
			tenon.SetVal(num, n(2), n(1)),
			tenon.SetVal(num, n(3), n(1)),
		},
		{
			"maps by name before value",
			tenon.MapVal(num, map[string]tenon.Value{"a": n(9)}),
			tenon.MapVal(num, map[string]tenon.Value{"b": n(1)}),
		},
		{
			"objects by name before value",
			tenon.ObjectVal(map[string]tenon.Value{"a": n(9)}),
			tenon.ObjectVal(map[string]tenon.Value{"b": n(1)}),
		},
		{"tuples by element", tenon.TupleVal(n(1)), tenon.TupleVal(n(2))},
		// Two values that the rules for their kind leave together sort by
		// type, which is the only thing left that differs.
		{"empty lists of different element types", tenon.ListVal(num), tenon.ListVal(str)},
		{"empty maps of different element types", tenon.MapVal(num, nil), tenon.MapVal(str, nil)},
		{"nulls of different types", tenon.NullVal(num), tenon.NullVal(str)},
		{"objects whose attributes differ in type", tenon.ObjectVal(nil), tenon.ObjectVal(map[string]tenon.Value{"a": n(1)})},
	} {
		if got := tenon.CanonicalCompare(tt.a, tt.b); got >= 0 {
			t.Errorf("%s: %v does not sort before %v: %d", tt.name, tt.a, tt.b, got)
		}
		if got := tenon.CanonicalCompare(tt.b, tt.a); got <= 0 {
			t.Errorf("%s: the other way about is %d", tt.name, got)
		}
	}
	// A set is its members, so one built two ways sorts together with itself.
	if got := tenon.CanonicalCompare(
		tenon.SetVal(num, n(1), n(2)),
		tenon.SetVal(num, n(2), n(1), n(1)),
	); got != 0 {
		t.Errorf("one set built two ways does not sort together with itself: %d", got)
	}
	mustPanicUsage(t, "which is not a known value", func() {
		tenon.CanonicalCompare(tenon.Unknown(num), n(1))
	})
}

func TestConformance_EQ046_TheOrderIsTheHostsAndNotTheLanguages(t *testing.T) {
	conformance.Covers(t, "EQ-046")
	num := tenon.NumberType()
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	// The host's order answers with a number, about every known value, and
	// never fails.
	if tenon.CanonicalCompare(one, two) >= 0 {
		t.Error("the host's order does not order two numbers")
	}
	if tenon.CanonicalCompare(tenon.NullVal(num), one) >= 0 {
		t.Error("the host's order does not put null first")
	}
	if tenon.CanonicalCompare(tenon.ListVal(num), tenon.Bool(true)) <= 0 {
		t.Error("the host's order does not order across kinds")
	}
	// The language's ordering is a different relation: it answers with a
	// value, it is defined for two types only, and it has nothing to say
	// about null or about a list.
	if got := tenon.LessThan(one, two).String(); got != "true" {
		t.Errorf("the language's ordering gave %s", got)
	}
	if got := tenon.LessThan(tenon.NullVal(num), one); !got.IsError() {
		t.Errorf("the language's ordering took null: %v", got)
	}
	mustPanicUsage(t, "does not satisfy one_of", func() {
		tenon.LessThan(tenon.ListVal(num), tenon.ListVal(num))
	})
}

func TestCanonicalOrderIsATotalOrder(t *testing.T) {
	all := values.Known()
	if len(all) < 20 {
		t.Fatalf("only %d known values, which is too few to say much", len(all))
	}
	together := 0
	for i, a := range all {
		if got := tenon.CanonicalCompare(a, a); got != 0 {
			t.Errorf("%v does not sort together with itself: %d", a, got)
		}
		for j, b := range all {
			ab, ba := tenon.CanonicalCompare(a, b), tenon.CanonicalCompare(b, a)
			// Antisymmetry, and the order's own equality is Identical.
			if (ab < 0) != (ba > 0) || (ab == 0) != (ba == 0) {
				t.Errorf("%v and %v sort %d one way about and %d the other", a, b, ab, ba)
			}
			if (ab == 0) != tenon.Identical(a, b) {
				t.Errorf("%v and %v sort %d but identical is %t", a, b, ab, tenon.Identical(a, b))
			}
			if ab == 0 && i != j {
				together++
			}
			if ab >= 0 {
				continue
			}
			for _, c := range all {
				if tenon.CanonicalCompare(b, c) < 0 && tenon.CanonicalCompare(a, c) >= 0 {
					t.Errorf("%v before %v before %v, but not %v before %v", a, b, c, a, c)
				}
			}
		}
	}
	// Values that sort together without being the same node are what the
	// agreement with Identical is about, so there had better be some.
	if together < 3 {
		t.Errorf("only %d pairs of distinct values sorted together", together)
	}
	// Sorting is repeatable: the same values, shuffled, come back in one order.
	first := slices.Clone(all)
	slices.SortFunc(first, tenon.CanonicalCompare)
	for range 20 {
		again := slices.Clone(all)
		slices.Reverse(again)
		slices.SortFunc(again, tenon.CanonicalCompare)
		for i := range first {
			if !tenon.Identical(first[i], again[i]) {
				t.Fatalf("sorting the same values twice put %v where %v had been", again[i], first[i])
			}
		}
	}
}
