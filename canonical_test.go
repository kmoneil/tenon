package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
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
	// The order is not defined over marked values either, on either side.
	m := stamp{id: "m"}
	mustPanicUsage(t, "that carries marks", func() {
		tenon.CanonicalCompare(tenon.WithMarks(n(1), m), n(1))
	})
	mustPanicUsage(t, "that holds a marked value at .[0]", func() {
		tenon.CanonicalCompare(n(1), tenon.ListVal(num, tenon.WithMarks(n(1), m)))
	})
}

// A capsule type may declare equality and a hash that collides, and no order.
// The fallback then numbers what is left, and the numbers must belong to the
// type's equality classes: two values it reports equal are one value, so a
// numbering per pointer lets a third value sort between them, and a value that
// is one value iterates two ways.
func TestConformance_EQ045_CapsuleFallbackOrdersByEqualityClass(t *testing.T) {
	conformance.Covers(t, "EQ-045", "EQ-044", "DI-031", "SE-001")
	cv := func(x, y int) tenon.Value { return tenon.CapsuleVal(colliding, &point{x, y}) }
	// p and q are one value; r is another. They are numbered in the order they
	// are first compared, which is p, r, q.
	p, q, r := cv(1, 1), cv(1, 1), cv(2, 2)
	if got := tenon.CanonicalCompare(p, r); got == 0 {
		t.Fatal("two values the type reports unequal sort together")
	}
	_ = tenon.CanonicalCompare(r, q)
	if got := tenon.CanonicalCompare(p, q); got != 0 {
		t.Errorf("two values the type reports equal sort %d apart", got)
	}
	// A preorder: over every triple, ties are exactly the type's equality and
	// the order is transitive.
	all := []tenon.Value{p, q, r}
	for _, a := range all {
		for _, b := range all {
			eq := tenon.Equals(a, b).String() == "true"
			if tie := tenon.CanonicalCompare(a, b) == 0; tie != eq {
				t.Errorf("%v and %v: sort together is %v, Equals is %v", a, b, tie, eq)
			}
			if got, back := tenon.CanonicalCompare(a, b), tenon.CanonicalCompare(b, a); got != -back {
				t.Errorf("%v against %v is %d, and the other way about %d", a, b, got, back)
			}
			for _, c := range all {
				ab, bc, ac := tenon.CanonicalCompare(a, b), tenon.CanonicalCompare(b, c), tenon.CanonicalCompare(a, c)
				if ab <= 0 && bc <= 0 && ac > 0 {
					t.Errorf("%v <= %v <= %v, yet %v sorts after %v", a, b, c, a, c)
				}
			}
		}
	}
	// A set of tuples built from them iterates one way, whatever order it was
	// built in, and two such sets are identical with an empty diff.
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	A, B, C := tenon.TupleVal(p, n(2)), tenon.TupleVal(r, n(0)), tenon.TupleVal(q, n(1))
	order := func(s tenon.Value) []string {
		var out []string
		for _, e := range s.Elements() {
			out = append(out, e.Index(1).String())
		}
		return out
	}
	first := tenon.SetVal(A.Type(), A, B, C)
	second := tenon.SetVal(A.Type(), C, B, A)
	if got, want := order(second), order(first); !slices.Equal(got, want) {
		t.Errorf("one set built two ways iterates %v and %v", want, got)
	}
	if !tenon.Identical(first, second) {
		t.Fatalf("%v and %v are one set built two ways, but are not identical", first, second)
	}
	if got := tenon.CanonicalCompare(first, second); got != 0 {
		t.Errorf("two identical sets sort %d apart", got)
	}
	if got := tenon.Diff(first, second); len(got) != 0 {
		t.Errorf("two identical sets differ: %s", got)
	}
}

// colliding declares equality and a hash that is the same for every value,
// which a hash is allowed to be, and no order. It is declared here as well as
// in conformance/values because that package's point type is unexported, so
// values of values.Colliding cannot be built from outside it.
var colliding = tenon.Capsule("colliding_in_canonical_test", tenon.CapsuleOps[point]{
	Equals: func(a, b *point) bool { return *a == *b },
	Hash:   func(*point) uint64 { return 7 },
})

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
	all := values.Orderable()
	if len(all) < 20 {
		t.Fatalf("only %d orderable values, which is too few to say much", len(all))
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
