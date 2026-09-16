package tenon_test

import (
	"testing"

	"tenon"
	"tenon/conformance"
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
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		// Ranges that cannot meet settle the answer.
		{"bounds that do not overlap", between(1, 2), between(3, 4), "false"},
		{"bounds that do overlap", between(1, 3), between(2, 4), "unknown(bool, not null)"},
		{"a value below the bounds", between(3, 4), n(1), "false"},
		{"a value inside the bounds", between(1, 4), n(2), "unknown(bool, not null)"},
		{
			"prefixes that diverge",
			tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab-")),
			tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ax-")),
			"false",
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
			tenon.Narrow(tenon.Unknown(str), tenon.LengthMax(2)),
			tenon.Narrow(tenon.Unknown(str), tenon.LengthMin(5)),
			"false",
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
	} {
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: %v equals %v is %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
		if got := tenon.Equals(tt.b, tt.a).String(); got != tt.want {
			t.Errorf("%s: the other way about is %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestConformance_EQ004_EqualsAndNull(t *testing.T) {
	conformance.Covers(t, "EQ-004")
	str, num := tenon.StringType(), tenon.NumberType()
	null := tenon.NullVal(str)
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
	conformance.Covers(t, "UN-023")
	num, str := tenon.NumberType(), tenon.StringType()
	pending := tenon.Pending(tenon.Any())
	// The answer is an unknown Bool and never another pending value, however
	// little is known about the operand.
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"a value and a pending", tenon.Equals(tenon.NumberFromInt(1), pending)},
		{"two pendings", tenon.Equals(pending, pending)},
		{"an unknown and a pending", tenon.Equals(tenon.Unknown(num), pending)},
	} {
		if tt.got.IsPending() {
			t.Errorf("%s: Equals gave a pending value", tt.name)
			continue
		}
		if got, want := tt.got.String(), "unknown(bool, not null)"; got != want {
			t.Errorf("%s: Equals gave %s, want %s", tt.name, got, want)
		}
	}
	// A pending operand whose constraint names its type says that much: a
	// value of another type can never equal it. The rule asks for a resolved
	// Bool rather than a pending value, and a narrower answer is still one.
	if got := tenon.Equals(tenon.Pending(tenon.Exactly(str)), tenon.NumberFromInt(1)).String(); got != "false" {
		t.Errorf("a pending string equals a number: %s, want false", got)
	}
}
