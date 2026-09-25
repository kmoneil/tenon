package tenon_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// point is a capsule payload for the equality tests.
type point struct{ x, y int }

func TestConformance_EQ001_EqualsIsAValueOperation(t *testing.T) {
	conformance.Covers(t, "EQ-001")
	str := tenon.StringType()
	one := tenon.NumberFromInt(1)
	// The answer is a Bool value, known or not, and never anything else.
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"two known values", tenon.Equals(one, one)},
		{"a value and an unknown", tenon.Equals(one, tenon.Unknown(tenon.NumberType()))},
		{"a value and a pending", tenon.Equals(one, tenon.Pending(tenon.Any()))},
		{"two nulls", tenon.Equals(tenon.NullVal(str), tenon.NullVal(str))},
		{"values of different types", tenon.Equals(one, tenon.String("1"))},
	} {
		if !tt.got.IsResolved() || tt.got.Type() != tenon.BoolType() {
			t.Errorf("%s: Equals gave %v, want a Bool value", tt.name, tt.got)
		}
	}
	// An error operand gives an error value, as it does through any operation.
	bad := tenon.Equals(tenon.String("\xff"), one)
	if !bad.IsError() || bad.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("Equals with an error operand gave %v, want its diagnostics", bad)
	}
}

func TestConformance_EQ002_EqualsComparesValuesNotRepresentations(t *testing.T) {
	conformance.Covers(t, "EQ-002")
	str, num := tenon.StringType(), tenon.NumberType()
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"a number written two ways", tenon.NumberFromInt(1), tenon.NumberFromText("1.00"), "true"},
		{"a number in scientific notation", tenon.NumberFromText("1e2"), tenon.NumberFromInt(100), "true"},
		{"different numbers", tenon.NumberFromInt(1), tenon.NumberFromInt(2), "false"},
		{"a string in two normal forms", tenon.String("e\U00000301"), tenon.String("\U000000e9"), "true"},
		{"different strings", tenon.String("a"), tenon.String("b"), "false"},
		{"booleans", tenon.Bool(true), tenon.Bool(true), "true"},
		{
			"lists, member by member",
			tenon.ListVal(num, tenon.NumberFromInt(1), tenon.NumberFromText("2.0")),
			tenon.ListVal(num, tenon.NumberFromText("1.000"), tenon.NumberFromInt(2)),
			"true",
		},
		{
			"lists of different lengths",
			tenon.ListVal(num, tenon.NumberFromInt(1)),
			tenon.ListVal(num, tenon.NumberFromInt(1), tenon.NumberFromInt(1)),
			"false",
		},
		{
			"objects, attribute by attribute",
			tenon.ObjectVal(map[string]tenon.Value{"a": tenon.NumberFromInt(1), "b": tenon.String("x")}),
			tenon.ObjectVal(map[string]tenon.Value{"b": tenon.String("x"), "a": tenon.NumberFromText("1.0")}),
			"true",
		},
		{
			"maps, entry by entry",
			tenon.MapVal(num, map[string]tenon.Value{"k": tenon.NumberFromInt(1)}),
			tenon.MapVal(num, map[string]tenon.Value{"k": tenon.NumberFromText("1.0")}),
			"true",
		},
		{
			"maps with different keys",
			tenon.MapVal(num, map[string]tenon.Value{"k": tenon.NumberFromInt(1)}),
			tenon.MapVal(num, map[string]tenon.Value{"j": tenon.NumberFromInt(1)}),
			"false",
		},
		{
			"tuples",
			tenon.TupleVal(tenon.NumberFromInt(1), tenon.String("x")),
			tenon.TupleVal(tenon.NumberFromText("1.0"), tenon.String("x")),
			"true",
		},
		// A set is its members: the order they were given in is not part of
		// the value, and neither is a member given twice.
		{
			"sets in different orders",
			tenon.SetVal(str, tenon.String("a"), tenon.String("b")),
			tenon.SetVal(str, tenon.String("b"), tenon.String("a")),
			"true",
		},
		{
			"a set with a member given twice",
			tenon.SetVal(str, tenon.String("a"), tenon.String("a")),
			tenon.SetVal(str, tenon.String("a")),
			"true",
		},
		{
			"sets with different members",
			tenon.SetVal(str, tenon.String("a")),
			tenon.SetVal(str, tenon.String("b")),
			"false",
		},
	} {
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: %v equals %v is %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
		// Equality does not depend on which operand comes first.
		if got := tenon.Equals(tt.b, tt.a).String(); got != tt.want {
			t.Errorf("%s: the other way about is %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestConformance_TY042_CapsuleEquality(t *testing.T) {
	conformance.Covers(t, "TY-042")
	// Without a declared equality, two capsule values are equal when they
	// encapsulate the same pointer and not when they merely look alike.
	opaque := tenon.Capsule("opaque", tenon.CapsuleOps[point]{})
	p := &point{1, 2}
	if got := tenon.Equals(tenon.CapsuleVal(opaque, p), tenon.CapsuleVal(opaque, p)).String(); got != "true" {
		t.Errorf("one pointer is not equal to itself: %s", got)
	}
	if got := tenon.Equals(tenon.CapsuleVal(opaque, p), tenon.CapsuleVal(opaque, &point{1, 2})).String(); got != "false" {
		t.Errorf("two pointers that look alike are equal: %s", got)
	}
	// With one, it decides.
	compared := tenon.Capsule("compared", tenon.CapsuleOps[point]{
		Equals: func(a, b *point) bool { return a.x == b.x && a.y == b.y },
		Hash:   func(v *point) uint64 { return uint64(v.x)<<32 | uint64(v.y) },
	})
	if got := tenon.Equals(tenon.CapsuleVal(compared, p), tenon.CapsuleVal(compared, &point{1, 2})).String(); got != "true" {
		t.Errorf("a declared equality was not used: %s", got)
	}
	if got := tenon.Equals(tenon.CapsuleVal(compared, p), tenon.CapsuleVal(compared, &point{3, 4})).String(); got != "false" {
		t.Errorf("a declared equality found two different points equal: %s", got)
	}
	// Every capsule type is its own type, so values of two of them are never
	// equal whatever they encapsulate.
	if got := tenon.Equals(tenon.CapsuleVal(opaque, p), tenon.CapsuleVal(compared, p)).String(); got != "false" {
		t.Errorf("values of two capsule types are equal: %s", got)
	}
}

func TestConformance_EQ003_EqualsWithAnOperandThatIsNotKnown(t *testing.T) {
	conformance.Covers(t, "EQ-003")
	num, str := tenon.NumberType(), tenon.StringType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	between := func(lo, hi int64) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(lo), true), tenon.NumberMax(n(hi), true))
	}
	notNull := func(v tenon.Value) tenon.Value { return tenon.Narrow(v, tenon.NotNull()) }
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		// Ranges that cannot meet settle the answer, once neither of them can
		// be null. Two that could each still be null could each turn out to
		// be null, which is one value, whatever their numbers cannot do.
		{"bounds that do not overlap", notNull(between(1, 2)), notNull(between(3, 4)), "false"},
		{"bounds that do not overlap, but both could be null", between(1, 2), between(3, 4), "unknown(bool, not null)"},
		{"bounds that do not overlap, and one cannot be null", notNull(between(1, 2)), between(3, 4), "false"},
		{"bounds that do overlap", between(1, 3), between(2, 4), "unknown(bool, not null)"},
		{"a value below the bounds", between(3, 4), n(1), "false"},
		{"a value inside the bounds", between(1, 4), n(2), "unknown(bool, not null)"},
		{
			"prefixes that diverge",
			tenon.Narrow(tenon.Unknown(str), tenon.NotNull(), tenon.StringPrefix("ab-")),
			tenon.Narrow(tenon.Unknown(str), tenon.NotNull(), tenon.StringPrefix("ax-")),
			"false",
		},
		{
			"prefixes that diverge, but both could be null",
			tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab-")),
			tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ax-")),
			"unknown(bool, not null)",
		},
		{
			"a prefix against a value that does not begin with it",
			tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab-")),
			tenon.String("xy"),
			"false",
		},
		{
			"a prefix against a value that does",
			tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab-")),
			tenon.String("ab-cd"),
			"unknown(bool, not null)",
		},
		{
			"lengths that do not overlap",
			tenon.Narrow(tenon.Unknown(str), tenon.NotNull(), tenon.LengthMax(2)),
			tenon.Narrow(tenon.Unknown(str), tenon.NotNull(), tenon.LengthMin(5)),
			"false",
		},
		{
			"lengths that do not overlap, but both could be null",
			tenon.Narrow(tenon.Unknown(str), tenon.LengthMax(2)),
			tenon.Narrow(tenon.Unknown(str), tenon.LengthMin(5)),
			"unknown(bool, not null)",
		},
		{
			"a length against a value that is too long",
			tenon.Narrow(tenon.Unknown(str), tenon.LengthMax(2)),
			tenon.String("abc"),
			"false",
		},
		// Two unknowns of a type with one value are the same value, since
		// there is nothing else for either of them to be.
		{
			"two unknowns that cannot differ",
			tenon.Narrow(tenon.Unknown(tenon.Tuple()), tenon.NotNull()),
			tenon.Narrow(tenon.Unknown(tenon.Tuple()), tenon.NotNull()),
			"true",
		},
		{"two fresh unknowns", tenon.Unknown(num), tenon.Unknown(num), "unknown(bool, not null)"},
		// Containers say what their members say.
		{
			"lists of different lengths",
			tenon.ListVal(num, tenon.Unknown(num)),
			tenon.ListVal(num, n(1), n(2)),
			"false",
		},
		{
			"lists with a member that cannot match",
			tenon.ListVal(num, n(1), tenon.Unknown(num)),
			tenon.ListVal(num, n(2), tenon.Unknown(num)),
			"false",
		},
		{
			"lists that could still match",
			tenon.ListVal(num, n(1), tenon.Unknown(num)),
			tenon.ListVal(num, n(1), tenon.Unknown(num)),
			"unknown(bool, not null)",
		},
		// A set holding a member that is not known has a length between the
		// members provably distinct and all of them, and two sets are equal
		// only where those lengths meet and every member of each could be a
		// member of the other.
		{"a set of one and an unknown, and the empty set", tenon.SetVal(num, n(1), tenon.Unknown(num)), tenon.SetVal(num), "false"},
		{"a set of one unknown, and a set of two", tenon.SetVal(num, tenon.Unknown(num)), tenon.SetVal(num, n(1), n(2)), "false"},
		{
			"sets whose lengths cannot meet",
			tenon.SetVal(num, n(1), tenon.Unknown(num)),
			tenon.SetVal(num, n(2), n(3), n(4)),
			"false",
		},
		{
			"a member provably absent from the other set",
			tenon.SetVal(num, n(1), between(5, 9)),
			tenon.SetVal(num, n(1), n(2)),
			"false",
		},
		{
			"sets that could still match",
			tenon.SetVal(num, n(1), tenon.Unknown(num)),
			tenon.SetVal(num, n(1), n(2)),
			"unknown(bool, not null)",
		},
		{
			"two unknown members that could be one",
			tenon.SetVal(num, tenon.Unknown(num), between(0, 9)),
			tenon.SetVal(num, n(1)),
			"unknown(bool, not null)",
		},
		// An unknown collection says as much against a container holding
		// unknowns as against a known one.
		{
			"an unknown set too long for a set holding an unknown",
			tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.LengthMin(3)),
			tenon.SetVal(num, n(1), tenon.Unknown(num)),
			"false",
		},
		{
			"an unknown list too short for a list holding unknowns",
			tenon.Narrow(tenon.Unknown(tenon.List(num)), tenon.LengthMax(1)),
			tenon.ListVal(num, tenon.Unknown(num), tenon.Unknown(num)),
			"false",
		},
		{
			"an unknown set requiring a member a set holding an unknown lacks",
			tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.Members(n(3))),
			tenon.SetVal(num, n(1), between(5, 9)),
			"false",
		},
		{
			"an unknown set a set holding an unknown could be",
			tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.LengthMin(1)),
			tenon.SetVal(num, n(1), tenon.Unknown(num)),
			"unknown(bool, not null)",
		},
		// A set holding unknowns is a known set only if its members can be
		// that set's members, each its own: the three members between 1 and 2
		// leave the one between 3 and 4 to be both 3 and 4.
		{
			"a known set no member could be twice over",
			tenon.SetVal(num, n(1), n(2), n(3), n(4)),
			tenon.SetVal(num, notNull(between(1, 2)), notNull(between(1, 2)), notNull(between(1, 2)), notNull(between(3, 4))),
			"false",
		},
		{
			"a known set the members can cover between them",
			tenon.SetVal(num, n(1), n(2)),
			tenon.SetVal(num, notNull(between(1, 2)), notNull(between(1, 2))),
			"unknown(bool, not null)",
		},
		{
			"a known set with a member that no member of the other could be",
			tenon.SetVal(num, n(1), n(2)),
			tenon.SetVal(num, notNull(between(1, 2)), notNull(between(5, 9))),
			"false",
		},
	} {
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: %v equals %v is %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
		if got := tenon.Equals(tt.b, tt.a).String(); got != tt.want {
			t.Errorf("%s: the other way about is %s, want %s", tt.name, got, tt.want)
		}
	}

	// A member that provably differs settles the answer however the members
	// are walked, even beside a member that is not known. Objects and maps
	// built from Go maps are walked in no fixed order, so each pair is built
	// afresh many times, and the answer is false every time.
	for i := range conformance.Iterations(t, 200) {
		for _, pair := range [][2]tenon.Value{
			{
				tenon.ObjectVal(map[string]tenon.Value{"a": tenon.Unknown(str), "b": tenon.String("z")}),
				tenon.ObjectVal(map[string]tenon.Value{"a": tenon.String("x"), "b": tenon.String("y")}),
			},
			{
				tenon.MapVal(str, map[string]tenon.Value{"a": tenon.Unknown(str), "b": tenon.String("z")}),
				tenon.MapVal(str, map[string]tenon.Value{"a": tenon.String("x"), "b": tenon.String("y")}),
			},
		} {
			if got := tenon.Equals(pair[0], pair[1]).String(); got != "false" {
				t.Fatalf("pass %d: %v equals %v is %s, want false", i, pair[0], pair[1], got)
			}
		}
	}
}

// Every answer that rests on two ranges being disjoint rests on both of them
// having ruled out null first. A range that has not is a range that still
// holds one value in common with every other range of its type.
func TestConformance_EQ003_NullIsDecidedBeforeRangesAreCompared(t *testing.T) {
	conformance.Covers(t, "EQ-003", "EQ-004", "EQ-042", "EQ-043", "UN-002", "UN-007")
	num, set, lst := tenon.NumberType(), tenon.Set(tenon.NumberType()), tenon.List(tenon.NumberType())
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	// a and b hold no number in common, and each still holds null.
	a := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true))
	b := tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(n(3), true))
	if got, want := tenon.Equals(a, b).String(), "unknown(bool, not null)"; got != want {
		t.Errorf("two ranges that could both be null are %s equal, want %s", got, want)
	}
	if got, want := tenon.Equals(b, a).String(), "unknown(bool, not null)"; got != want {
		t.Errorf("the other way about is %s, want %s", got, want)
	}
	// A set of the two holds one member if both are null, and two otherwise.
	if got, want := tenon.Length(tenon.SetVal(num, a, b)).String(), "unknown(number, not null, >= 1, <= 2)"; got != want {
		t.Errorf("the length of a set of the two is %s, want %s", got, want)
	}
	if got := tenon.Contains(tenon.SetVal(num, a), b); got.IsKnown() {
		t.Errorf("membership of one in a set of the other is %v, want an unknown Bool", got)
	}
	// The set {null} satisfies a listing of the two at a length of one, so the
	// listing is no contradiction.
	if got := tenon.Narrow(tenon.Unknown(set), tenon.Members(a, b), tenon.LengthMax(1)); got.IsError() {
		t.Errorf("listing two members that could be one member is %v, want a range", got)
	}
	// A set that could hold one member converts to a tuple of one.
	if got := tenon.Convert(tenon.SetVal(num, a, b), tenon.TupleOf(tenon.Any()), tenon.Unsafe); got.IsError() {
		t.Errorf("converting a set of the two to a tuple of one is %v, want a value", got)
	}
	// Lengths that cannot meet say no more than bounds that cannot, since the
	// null list has no length to disagree about.
	tooLong := tenon.Narrow(tenon.Unknown(lst), tenon.LengthMin(3))
	tooShort := tenon.Narrow(tenon.Unknown(lst), tenon.LengthMax(1))
	if got, want := tenon.Equals(tooLong, tooShort).String(), "unknown(bool, not null)"; got != want {
		t.Errorf("two lists whose lengths cannot meet are %s equal, want %s", got, want)
	}

	// Null is the whole of what left those answers open: ruling it out on
	// either side settles them again.
	an, bn := tenon.Narrow(a, tenon.NotNull()), tenon.Narrow(b, tenon.NotNull())
	if got, want := tenon.Equals(an, bn).String(), "false"; got != want {
		t.Errorf("two disjoint ranges that cannot be null are %s equal, want %s", got, want)
	}
	if got, want := tenon.Equals(an, b).String(), "false"; got != want {
		t.Errorf("a disjoint range against one that could be null is %s equal, want %s", got, want)
	}
	if got, want := tenon.Length(tenon.SetVal(num, an, bn)).String(), "2"; got != want {
		t.Errorf("the length of a set of two that cannot be null is %s, want %s", got, want)
	}
	// Two listed values that cannot be one need two members, which a set
	// holding one has not got; the same two while each could still be null
	// could both be null, which is one member.
	one := tenon.SetVal(num, tenon.Unknown(num))
	if got := tenon.Narrow(one, tenon.Members(an, bn)); !got.IsError() {
		t.Errorf("a set of one narrowed by two members that cannot be one is %v, want a contradiction", got)
	}
	if got := tenon.Narrow(one, tenon.Members(a, b)); got.IsError() {
		t.Errorf("a set of one narrowed by two members that could both be null is %v, want the set", got)
	}
}

func TestConformance_EQ004_EqualsAndNull(t *testing.T) {
	conformance.Covers(t, "EQ-004")
	str, num := tenon.StringType(), tenon.NumberType()
	null := tenon.NullVal(str)
	untypedNull := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"two nulls of one type", null, tenon.NullVal(str), "true"},
		{"null and a value of its type", null, tenon.String("x"), "false"},
		{"null and a value of another type", null, tenon.NumberFromInt(1), "false"},
		{"null and a list, which is not null", tenon.NullVal(tenon.List(str)), tenon.ListVal(str), "false"},
		// An unknown that has given up null cannot be it; one that has not
		// might still turn out to be.
		{"null and an unknown that cannot be null", null, tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), "false"},
		{"null and an unknown that may be null", null, tenon.Unknown(str), "unknown(bool, not null)"},
		{"null and a pending known not to be null", tenon.NullVal(num), tenon.Narrow(tenon.Pending(tenon.Exactly(num)), tenon.NotNull()), "false"},
		// A pending value known to be null is null, whatever its type turns
		// out to be: it never equals a value that cannot be null, and it
		// equals a null of the type its constraint names.
		{"a value and a pending known to be null", tenon.NumberFromInt(1), untypedNull, "false"},
		{"a list and a pending known to be null", tenon.ListVal(str), untypedNull, "false"},
		{"an unknown that cannot be null and a pending known to be null", tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), untypedNull, "false"},
		{"null and a pending of any type known not to be null", null, tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull()), "false"},
		{"null and a pending known to be null of its type", null, tenon.Narrow(tenon.Pending(tenon.Exactly(str)), tenon.Null()), "true"},
		{"null and a pending known to be null of another type", null, tenon.Narrow(tenon.Pending(tenon.Exactly(num)), tenon.Null()), "false"},
		{"null and a pending known to be null of any type", null, untypedNull, "unknown(bool, not null)"},
		{
			"two pendings known to be null of one named type",
			tenon.Narrow(tenon.Pending(tenon.Exactly(num)), tenon.Null()),
			tenon.Narrow(tenon.Pending(tenon.Exactly(num)), tenon.Null()),
			"true",
		},
		{"two pendings known to be null of any type", untypedNull, untypedNull, "unknown(bool, not null)"},
	} {
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: %v equals %v is %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
		if got := tenon.Equals(tt.b, tt.a).String(); got != tt.want {
			t.Errorf("%s: the other way about is %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestConformance_EQ005_ValuesOfDifferentTypes(t *testing.T) {
	conformance.Covers(t, "EQ-005")
	str, num := tenon.StringType(), tenon.NumberType()
	for _, tt := range []struct {
		name string
		a, b tenon.Value
	}{
		{"a number and a string", tenon.NumberFromInt(1), tenon.String("1")},
		{"a bool and a number", tenon.Bool(true), tenon.NumberFromInt(1)},
		{"a list and a set of one element type", tenon.ListVal(str), tenon.SetVal(str)},
		{"lists of different element types", tenon.ListVal(str), tenon.ListVal(num)},
		{"a tuple and a list that look alike", tenon.TupleVal(tenon.String("a")), tenon.ListVal(str, tenon.String("a"))},
		{"nulls of different types", tenon.NullVal(str), tenon.NullVal(num)},
		{"an unknown and a value of another type", tenon.Unknown(str), tenon.NumberFromInt(1)},
		// A pending value whose constraint no type of the other operand's
		// satisfies will have another type, whatever it turns out to be.
		{
			"a pending of one of two other types and a number",
			tenon.Pending(tenon.OneOf(tenon.Exactly(str), tenon.Exactly(tenon.BoolType()))),
			tenon.NumberFromInt(1),
		},
		{"a pending list and a string", tenon.Pending(tenon.ListOf(tenon.Any())), tenon.String("x")},
		{"a pending list and an unknown string", tenon.Pending(tenon.ListOf(tenon.Any())), tenon.Unknown(str)},
	} {
		got := tenon.Equals(tt.a, tt.b)
		if got.IsError() {
			t.Errorf("%s: comparing them is an error: %v", tt.name, got)
			continue
		}
		if got.String() != "false" {
			t.Errorf("%s: %v equals %v is %s, want false", tt.name, tt.a, tt.b, got)
		}
	}
}

func TestConformance_UN023_EqualsWithAPendingOperand(t *testing.T) {
	conformance.Covers(t, "UN-023", "EQ-005")
	num, str := tenon.NumberType(), tenon.StringType()
	pending := tenon.Pending(tenon.Any())
	anyList := tenon.Pending(tenon.ListOf(tenon.Any()))
	// The answer is an unknown Bool and never another pending value, however
	// little is known about the operand.
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"a value and a pending", tenon.Equals(tenon.NumberFromInt(1), pending)},
		{"two pendings", tenon.Equals(pending, pending)},
		{"an unknown and a pending", tenon.Equals(tenon.Unknown(num), pending)},
		{"a null and a pending that may be null", tenon.Equals(tenon.NullVal(num), pending)},
		{"two pendings that could share a type", tenon.Equals(anyList, tenon.Pending(tenon.ListOf(tenon.Exactly(num))))},
		{"a pending list and a pending that could be one", tenon.Equals(anyList, tenon.Pending(tenon.OneOf(tenon.Exactly(num), tenon.ListOf(tenon.Any()))))},
	} {
		if tt.got.IsPending() {
			t.Errorf("%s: Equals gave a pending value", tt.name)
			continue
		}
		if got, want := tt.got.String(), "unknown(bool, not null)"; got != want {
			t.Errorf("%s: Equals gave %s, want %s", tt.name, got, want)
		}
	}
	// What a pending operand already says settles the answer where it can: a
	// constraint that rules out the other operand's type, and a nullness fact
	// that rules out equality with a value that cannot be null. Two pending
	// operands whose constraints share no type can never have one type, and
	// so are never equal, however their constraints are written.
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"a pending string and a number", tenon.Equals(tenon.Pending(tenon.Exactly(str)), tenon.NumberFromInt(1))},
		{"a pending list and a number", tenon.Equals(anyList, tenon.NumberFromInt(1))},
		{"a pending known to be null and a number", tenon.Equals(tenon.Narrow(pending, tenon.Null()), tenon.NumberFromInt(1))},
		{"a pending list and a pending set", tenon.Equals(anyList, tenon.Pending(tenon.SetOf(tenon.Any())))},
		{
			"two pendings of one of several types each, none of them shared",
			tenon.Equals(
				tenon.Pending(tenon.OneOf(tenon.Exactly(num), tenon.ListOf(tenon.Any()))),
				tenon.Pending(tenon.OneOf(tenon.Exactly(str), tenon.SetOf(tenon.Any()))),
			),
		},
		{"a pending tuple of one and a pending empty tuple", tenon.Equals(tenon.Pending(tenon.TupleOf(tenon.Any())), tenon.Pending(tenon.TupleOf()))},
	} {
		if got := tt.got.String(); got != "false" {
			t.Errorf("%s: Equals gave %s, want false", tt.name, got)
		}
	}
	// A pending value known to be null equals the null of the one type its
	// constraint admits, however the constraint names that type.
	empty := tenon.Tuple()
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"exactly the empty tuple", tenon.Equals(tenon.Narrow(tenon.Pending(tenon.Exactly(empty)), tenon.Null()), tenon.NullVal(empty))},
		{"a tuple of nothing", tenon.Equals(tenon.Narrow(tenon.Pending(tenon.TupleOf()), tenon.Null()), tenon.NullVal(empty))},
		{
			"two pendings, each written its own way",
			tenon.Equals(tenon.Narrow(tenon.Pending(tenon.TupleOf()), tenon.Null()), tenon.Narrow(tenon.Pending(tenon.OneOf(tenon.Exactly(empty))), tenon.Null())),
		},
	} {
		if got := tt.got.String(); got != "true" {
			t.Errorf("a pending null of %s and the null of the empty tuple: Equals gave %s, want true", tt.name, got)
		}
	}
}

// TestConformance_EQ003_EqualsDecidesOnlyWhatCannotChange holds Equals to its promise: where it
// answers true or false although an operand is not known, every value that
// operand could turn out to be gives the same answer. It checks pending
// operands against the values they can resolve to, and sets holding bounded
// unknowns, and unknown sets, against every set they could turn out to be.
func TestConformance_EQ003_EqualsDecidesOnlyWhatCannotChange(t *testing.T) {
	conformance.Covers(t, "EQ-003", "EQ-042", "EQ-043", "UN-007")
	bl, num, str := tenon.BoolType(), tenon.NumberType(), tenon.StringType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	settledAnswer := func(v tenon.Value) (string, bool) {
		if v.IsError() || !v.IsKnown() {
			return "", false
		}
		u, _ := tenon.Unmark(v)
		return u.String(), true
	}

	// Pending operands, against every resolved value of the generator: a
	// settled answer must hold for every type the pending value can take.
	types := []tenon.Type{bl, num, str, tenon.List(str), tenon.Set(str), tenon.Map(num), tenon.Tuple(), tenon.Object(nil)}
	type pending struct {
		p       tenon.Value
		c       tenon.Constraint
		notNull bool
	}
	var pendings []pending
	for _, c := range []tenon.Constraint{
		tenon.Any(), tenon.Exactly(num), tenon.Exactly(str),
		tenon.OneOf(tenon.Exactly(str), tenon.Exactly(bl)), tenon.ListOf(tenon.Any()),
		tenon.SetOf(tenon.Any()), tenon.TupleOf(), tenon.OneOf(tenon.Exactly(tenon.Tuple())),
		tenon.OneOf(tenon.Exactly(num), tenon.SetOf(tenon.Any())),
	} {
		p := tenon.Pending(c)
		pendings = append(pendings, pending{p, c, false}, pending{tenon.Narrow(p, tenon.Null()), c, false}, pending{tenon.Narrow(p, tenon.NotNull()), c, true})
	}
	checked := 0
	for _, pc := range pendings {
		p := pc.p
		for _, v := range values.All() {
			if v.IsError() || v.IsPending() {
				continue
			}
			want, ok := settledAnswer(tenon.Equals(p, v))
			if !ok {
				continue
			}
			checked++
			for _, typ := range types {
				if !tenon.Satisfies(pc.c, typ) {
					continue
				}
				if got, ok := settledAnswer(tenon.Equals(tenon.Resolve(p, typ), v)); ok && got != want {
					t.Errorf("%v equals %v is %s, but resolved to %v it is %s", p, v, want, typ, got)
				}
			}
		}
	}
	// Pending operands against each other: a settled answer must hold for every
	// pair of types the two can take. Two that could both be null are settled
	// unequal only by their types, and some such pair must name no type in
	// either constraint, or this says nothing about constraints that name a
	// kind; some pair must be settled equal, too.
	byTypes, equal := 0, 0
	for _, a := range pendings {
		for _, b := range pendings {
			want, ok := settledAnswer(tenon.Equals(a.p, b.p))
			if !ok {
				continue
			}
			checked++
			switch {
			case want == "true":
				equal++
			case !a.notNull && !b.notNull && a.c.Kind() != tenon.ConstraintExactly && b.c.Kind() != tenon.ConstraintExactly:
				byTypes++
			}
			for _, ta := range types {
				for _, tb := range types {
					if !tenon.Satisfies(a.c, ta) || !tenon.Satisfies(b.c, tb) {
						continue
					}
					if got, ok := settledAnswer(tenon.Equals(tenon.Resolve(a.p, ta), tenon.Resolve(b.p, tb))); ok && got != want {
						t.Errorf("%v equals %v is %s, but resolved to %v and %v it is %s", a.p, b.p, want, ta, tb, got)
					}
				}
			}
		}
	}
	if byTypes == 0 || equal == 0 {
		t.Errorf("pending operands naming no type were settled unequal by their constraints %d times, and pending operands settled equal %d times; want some of each", byTypes, equal)
	}

	// Sets of small integers and of unknowns bounded within them, against
	// each other: a settled answer must hold for every way of choosing the
	// unknowns. Integers suffice: two sets equal for some choice of decimals
	// are equal for a choice of integers too.
	// A member is a known number where lo == hi, and otherwise an unknown
	// bounded within [lo, hi]. One that has not ruled null out can turn out to
	// be null as readily as it can turn out to be a number, so null is one of
	// the values it is checked against.
	type member struct {
		lo, hi int64
		null   bool
	}
	kinds := []member{
		{lo: 0, hi: 0}, {lo: 1, hi: 1}, {lo: 2, hi: 2},
		{lo: 0, hi: 1}, {lo: 1, hi: 2}, {lo: 0, hi: 2},
		{lo: 0, hi: 1, null: true}, {lo: 2, hi: 3, null: true},
	}
	var specs [][]member
	specs = append(specs, nil)
	for i, a := range kinds {
		specs = append(specs, []member{a})
		for _, b := range kinds[i:] {
			specs = append(specs, []member{a, b})
		}
	}
	build := func(spec []member) tenon.Value {
		elems := make([]tenon.Value, len(spec))
		for i, m := range spec {
			switch {
			case m.lo == m.hi:
				elems[i] = n(m.lo)
			case m.null:
				elems[i] = tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(m.lo), true), tenon.NumberMax(n(m.hi), true))
			default:
				elems[i] = tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(m.lo), true), tenon.NumberMax(n(m.hi), true))
			}
		}
		return tenon.SetVal(num, elems...)
	}
	// choices returns every set spec can turn out to be, each marked with
	// whether some member of it was chosen null.
	type choice struct {
		v    tenon.Value
		null bool
	}
	choices := func(spec []member) []choice {
		var out []choice
		var walk func(i int, picked []tenon.Value, sawNull bool)
		walk = func(i int, picked []tenon.Value, sawNull bool) {
			if i == len(spec) {
				out = append(out, choice{tenon.SetVal(num, picked...), sawNull})
				return
			}
			if spec[i].null {
				walk(i+1, append(slices.Clone(picked), tenon.NullVal(num)), true)
			}
			for v := spec[i].lo; v <= spec[i].hi; v++ {
				walk(i+1, append(slices.Clone(picked), n(v)), sawNull)
			}
		}
		walk(0, nil, false)
		return out
	}
	settledSets, nullMembers := 0, 0
	for _, a := range specs {
		for _, b := range specs {
			want, ok := settledAnswer(tenon.Equals(build(a), build(b)))
			if !ok {
				continue
			}
			settledSets++
			for _, ca := range choices(a) {
				for _, cb := range choices(b) {
					if ca.null || cb.null {
						nullMembers++
					}
					if got := tenon.Equals(ca.v, cb.v).String(); got != want {
						t.Errorf("%v equals %v is %s, but %v equals %v is %s", build(a), build(b), want, ca.v, cb.v, got)
					}
				}
			}
		}
	}

	// Unknown sets narrowed by lengths and listed members, against those sets:
	// where Equals settles false, no set the other could be satisfies the
	// narrowings. Every listing is generated both with and without NotNull,
	// since a range that has not ruled null out still holds the null set, and
	// a narrowing other than Null and NotNull says nothing about null.
	var ranges [][]tenon.Narrowing
	for lo := int64(0); lo <= 3; lo++ {
		for hi := int64(-1); hi <= 3; hi++ {
			for _, listed := range [][]tenon.Value{nil, {n(0)}, {n(1)}, {n(0), n(1)}} {
				for _, nullable := range []bool{false, true} {
					var ns []tenon.Narrowing
					if !nullable {
						ns = append(ns, tenon.NotNull())
					}
					ns = append(ns, tenon.LengthMin(lo))
					if hi >= 0 {
						ns = append(ns, tenon.LengthMax(hi))
					}
					if listed != nil {
						ns = append(ns, tenon.Members(listed...))
					}
					ranges = append(ranges, ns)
				}
			}
		}
	}
	nullSet := tenon.NullVal(tenon.Set(num))
	settledRanges, nullRanges := 0, 0
	for _, ns := range ranges {
		r := tenon.Narrow(tenon.Unknown(tenon.Set(num)), ns...)
		if r.IsError() || r.IsKnown() {
			continue
		}
		for _, spec := range specs {
			want, ok := settledAnswer(tenon.Equals(r, build(spec)))
			if !ok || want != "false" {
				continue
			}
			settledRanges++
			for _, c := range choices(spec) {
				if narrowed := tenon.Narrow(c.v, ns...); !narrowed.IsError() {
					t.Errorf("%v equals %v is false, but %v satisfies its narrowings", r, build(spec), c.v)
				}
			}
		}
		// The null set is one of the sets the range could be, and it satisfies
		// every listing that has not ruled null out.
		nullRanges++
		if want, ok := settledAnswer(tenon.Equals(r, nullSet)); ok && want == "false" {
			if narrowed := tenon.Narrow(nullSet, ns...); !narrowed.IsError() {
				t.Errorf("%v equals the null set is false, but the null set satisfies its narrowings", r)
			}
		}
	}
	// A property checked over nothing proves nothing, and one that never chose
	// null would have passed while Equals settled two nullable ranges unequal.
	if checked < 50 || settledSets < 50 || settledRanges < 50 {
		t.Errorf("too few settled answers to say much: %d pending, %d sets, %d ranges",
			checked, settledSets, settledRanges)
	}
	if nullMembers < 50 || nullRanges < 50 {
		t.Errorf("too few null choices to say much: %d members chosen null, %d ranges against the null set",
			nullMembers, nullRanges)
	}
}

// TestConformance_EQ002_AKnownValueEqualsItselfAtOnce holds Equals to settling
// a known value compared with itself without comparing anything it holds, and
// a part that two values share the same way: a plan engine compares a prior
// tree with a planned one sharing every part that did not change. The members
// are values of a capsule type that counts how often its equality is asked,
// which a walk would ask once for each of them.
func TestConformance_EQ002_AKnownValueEqualsItselfAtOnce(t *testing.T) {
	conformance.Covers(t, "EQ-002", "TY-041")
	asked := 0
	counted := tenon.Capsule("counted", tenon.CapsuleOps[int]{
		Equals: func(a, b *int) bool { asked++; return *a == *b },
		Hash:   func(v *int) uint64 { return uint64(*v) },
	})
	const size = 1000
	built := func() []tenon.Value {
		members := make([]tenon.Value, size)
		for i := range members {
			v := i
			members[i] = tenon.CapsuleVal(counted, &v)
		}
		return members
	}
	members := built()
	entries := make(map[string]tenon.Value, size)
	for i, m := range members {
		entries[fmt.Sprintf("a%04d", i)] = m
	}
	parts := map[string]tenon.Value{
		"list":   tenon.ListVal(counted, members...),
		"set":    tenon.SetVal(counted, members...),
		"map":    tenon.MapVal(counted, entries),
		"tuple":  tenon.TupleVal(members...),
		"object": tenon.ObjectVal(entries),
	}
	whole := tenon.ObjectVal(parts)
	// A prior value and two planned ones sharing its parts. Attributes are
	// held in name order, so the parts are compared before the version.
	prior := tenon.ObjectVal(map[string]tenon.Value{"parts": whole, "version": tenon.NumberFromInt(1)})
	rewritten := tenon.ObjectVal(map[string]tenon.Value{"parts": whole, "version": tenon.NumberFromText("1.0")})
	moved := tenon.ObjectVal(map[string]tenon.Value{"parts": whole, "version": tenon.NumberFromInt(2)})
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"a capsule value compared with itself", members[0], members[0], "true"},
		{"a list compared with itself", parts["list"], parts["list"], "true"},
		{"a set compared with itself", parts["set"], parts["set"], "true"},
		{"a map compared with itself", parts["map"], parts["map"], "true"},
		{"a tuple compared with itself", parts["tuple"], parts["tuple"], "true"},
		{"an object compared with itself", parts["object"], parts["object"], "true"},
		{"a value holding all of them compared with itself", whole, whole, "true"},
		{"two values sharing their parts", prior, rewritten, "true"},
		{"two values sharing their parts and differing after them", prior, moved, "false"},
	} {
		asked = 0
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: Equals gave %s, want %s", tt.name, got, tt.want)
		}
		if asked != 0 {
			t.Errorf("%s: the capsule type was asked %d times, want none: what the two share was compared", tt.name, asked)
		}
	}
	// The count is real: the same members built apart are other values, and
	// are compared one by one.
	asked = 0
	if got := tenon.Equals(parts["list"], tenon.ListVal(counted, built()...)).String(); got != "true" || asked < size {
		t.Errorf("a list and its members built apart: Equals gave %s after asking %d times, want true after %d", got, asked, size)
	}
}

// TestConformance_EQ003_AValueComparedWithItself holds Equals to answering for
// a value compared with itself what it answers for the value and a twin built
// apart, marks and all. A known value is equal to itself. One that is not
// known is not known to be: each operand could still turn out to be any value
// its range holds, so the two could turn out different, and the answer is the
// one the ranges give. An error value is an error value whatever it is
// compared with.
func TestConformance_EQ003_AValueComparedWithItself(t *testing.T) {
	conformance.Covers(t, "EQ-002", "EQ-003", "EQ-004", "MK-003")
	num := tenon.NumberType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	unknown := tenon.Unknown(num)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"a known value", tenon.ListVal(num, n(1), n(2)), "true"},
		{"null", tenon.NullVal(num), "true"},
		{"an unknown", unknown, "unknown(bool, not null)"},
		{"an unknown that cannot be null", tenon.Narrow(unknown, tenon.NotNull()), "unknown(bool, not null)"},
		{"a partly known list", tenon.ListVal(num, n(1), unknown), "unknown(bool, not null)"},
		{"a partly known set", tenon.SetVal(num, n(1), unknown), "unknown(bool, not null)"},
		{"a partly known object", tenon.ObjectVal(map[string]tenon.Value{"a": n(1), "b": unknown}), "unknown(bool, not null)"},
		{
			"a list holding a partly known list",
			tenon.ListVal(tenon.List(num), tenon.ListVal(num, n(1)), tenon.ListVal(num, unknown)),
			"unknown(bool, not null)",
		},
		{"a pending value", tenon.Pending(tenon.Any()), "unknown(bool, not null)"},
	} {
		if got := tenon.Equals(tt.v, tt.v).String(); got != tt.want {
			t.Errorf("%s: %v equals itself is %s, want %s", tt.name, tt.v, got, tt.want)
		}
	}

	// An error operand gives an error value, compared with itself as with
	// anything else, and its diagnostic appears once.
	bad := tenon.String("\xff")
	if got := tenon.Equals(bad, bad); !got.IsError() || len(got.Diagnostics()) != 1 || got.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("an error value compared with itself gave %v, want its diagnostic once", got)
	}

	// The result carries the Propagate marks of the value and of what it
	// holds, which equality reads, and not its Isolate mark.
	own, held, iso := stamp{id: "own"}, stamp{id: "held"}, stamp{id: "iso", policy: tenon.Isolate}
	marked := tenon.WithMarks(tenon.ListVal(num, n(1), tenon.WithMarks(n(2), held)), own, iso)
	if got, marks := tenon.Unmark(tenon.Equals(marked, marked)); got.String() != "true" || !slices.Equal(marks, []tenon.Mark{held, own}) {
		t.Errorf("a marked value compared with itself is %v carrying %v, want true carrying held and own", got, marks)
	}

	// Every shape the generator holds: the answer for a value and itself is,
	// marks and all, the answer for the value and a twin built apart. A capsule
	// value around a pointer of its own has no twin, and is left out.
	twins, compared := values.All(), 0
	for i, v := range values.All() {
		w := twins[i]
		if !tenon.Identical(v, w) {
			continue
		}
		compared++
		if self, apart := tenon.Equals(v, v), tenon.Equals(v, w); !tenon.Identical(self, apart) {
			t.Errorf("%v compared with itself is %v, and with its twin %v", v, self, apart)
		}
	}
	if skipped := len(twins) - compared; skipped > 1 {
		t.Errorf("%d values have no twin, where only the capsule value around a pointer of its own should lack one", skipped)
	}
}

// BenchmarkEqualsSharedParts measures Equals where what it compares is shared,
// at a size and four times it: a configuration compared with itself, and with
// a planned one sharing its resources. Neither reads what is shared, so the
// time does not grow with it, and the growth from one size to the other is
// the reading, not the wall clock.
func BenchmarkEqualsSharedParts(b *testing.B) {
	str := tenon.StringType()
	for _, size := range []int{1000, 4000} {
		resources := make([]tenon.Value, size)
		for i := range resources {
			resources[i] = tenon.ObjectVal(map[string]tenon.Value{
				"name":  tenon.String(fmt.Sprintf("r%05d", i)),
				"count": tenon.NumberFromInt(int64(i)),
				"tags":  tenon.ListVal(str, tenon.String("a"), tenon.String("b")),
			})
		}
		shared := tenon.ListVal(resources[0].Type(), resources...)
		prior := tenon.ObjectVal(map[string]tenon.Value{"resources": shared, "version": tenon.NumberFromInt(1)})
		planned := tenon.ObjectVal(map[string]tenon.Value{"resources": shared, "version": tenon.NumberFromText("1.0")})
		for _, shape := range []struct {
			name string
			a, b tenon.Value
		}{
			{"itself", prior, prior},
			{"shared", prior, planned},
		} {
			if got := tenon.Equals(shape.a, shape.b).String(); got != "true" {
				b.Fatalf("%s at %d: Equals gave %s, want true", shape.name, size, got)
			}
			b.Run(fmt.Sprintf("%s/%d", shape.name, size), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					tenon.Equals(shape.a, shape.b)
				}
			})
		}
	}
}
