package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// nset returns the set of the numbers given, by their text.
func nset(texts ...string) tenon.Value {
	ms := make([]tenon.Value, len(texts))
	for i, t := range texts {
		ms[i] = num(t)
	}
	return tenon.Set(tenon.NumberType(), ms...)
}

// unknownNumber is a number not known yet.
var unknownNumber = tenon.Unknown(tenon.NumberType())

// isBool checks that got is the known Bool want.
func isBool(t *testing.T, what string, got tenon.Value, want bool) {
	t.Helper()
	if !got.Equal(tenon.Bool(want)) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// isOpenBool checks that got is a Bool not known yet, and not null.
func isOpenBool(t *testing.T, what string, got tenon.Value) {
	t.Helper()
	if got.IsKnown() || got.IsError() || !got.Type().Equal(tenon.BoolType()) || !notNull(got) {
		t.Errorf("%s = %v, want a Bool not known yet, not null", what, got)
	}
}

func TestConformance_LC040_Contains(t *testing.T) {
	conformance.Covers(t, "LC-040")
	nums := tenon.List(tenon.NumberType(), num("1"), num("2"))
	isBool(t, "Contains([1, 2], 2)", call(stdlib.ContainsFunc, nums, num("2")), true)
	isBool(t, "Contains([1, 2], 3)", call(stdlib.ContainsFunc, nums, num("3")), false)
	isBool(t, "Contains(tuple(a, 1), 1)", call(stdlib.ContainsFunc, tenon.Tuple(a, num("1")), num("1")), true)
	isBool(t, "Contains({1, 2}, 1)", call(stdlib.ContainsFunc, nset("1", "2"), num("1")), true)
	// A member not known yet leaves a miss open, and not a hit.
	partly := tenon.List(tenon.NumberType(), num("1"), unknownNumber)
	isBool(t, "Contains([1, unknown], 1)", call(stdlib.ContainsFunc, partly, num("1")), true)
	isOpenBool(t, "Contains([1, unknown], 2)", call(stdlib.ContainsFunc, partly, num("2")))
	// An empty collection holds nothing, whatever the value.
	for _, v := range []tenon.Value{num("1"), unknownNumber, tenon.Pending(tenon.Any())} {
		isBool(t, "Contains([], "+v.String()+")", call(stdlib.ContainsFunc, tenon.Tuple(), v), false)
		isBool(t, "Contains(empty list, "+v.String()+")", call(stdlib.ContainsFunc, tenon.List(str), v), false)
		isBool(t, "Contains(empty set, "+v.String()+")", call(stdlib.ContainsFunc, tenon.Set(str), v), false)
	}
	failsWith(t, "Contains of a map", call(stdlib.ContainsFunc, numbers(nil), num("1")), tenon.CodeOperationWrongType, at(0))
	if !stdlib.ContainsFunc.NotNull() {
		t.Error("Contains does not declare its result never null")
	}
	// The boundary carries the marks of what is read: the collection, and
	// every member.
	marked := tenon.List(str, a, tenon.WithMarks(b, bare("member")))
	if got := call(stdlib.ContainsFunc, marked, a); !tenon.HasMark(got, bare("member")) {
		t.Errorf("Contains of a list with a marked member = %v, want its mark", got)
	}
}

func TestConformance_LC041_Needle(t *testing.T) {
	conformance.Covers(t, "LC-041")
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	nested := tenon.List(tenon.ListType(tenon.NumberType()), tenon.List(tenon.NumberType(), num("1")))
	nulls := tenon.List(str, a, tenon.Null(str))
	for _, tt := range []struct {
		what    string
		list, v tenon.Value
		want    bool
	}{
		{what: "a tuple among lists", list: nested, v: tenon.Tuple(num("1")), want: true},
		{what: "an untyped null among nulls", list: nulls, v: untyped, want: true},
		{what: "an untyped null among strings", list: list(a), v: untyped, want: false},
		{what: "a string among numbers", list: tenon.List(tenon.NumberType(), num("1")), v: tenon.String("1"), want: false},
		{what: "a number among strings", list: list(tenon.String("1")), v: num("1"), want: false},
	} {
		isBool(t, "Contains: "+tt.what, call(stdlib.ContainsFunc, tt.list, tt.v), tt.want)
		// Under Unsafe too: the value is converted under Safe whatever the
		// call's policy.
		isBool(t, "Contains under Unsafe: "+tt.what, tenon.Call(stdlib.ContainsFunc, []tenon.Value{tt.list, tt.v}, tenon.Unsafe), tt.want)
	}
	nullSet := tenon.Set(str, a, tenon.Null(str))
	isBool(t, "SetHasElement of an untyped null among nulls", call(stdlib.SetHasElementFunc, nullSet, untyped), true)
	setOfLists := tenon.Set(tenon.ListType(tenon.NumberType()), tenon.List(tenon.NumberType(), num("1")))
	isBool(t, "SetHasElement of a tuple among lists", call(stdlib.SetHasElementFunc, setOfLists, tenon.Tuple(num("1"))), true)
}

func TestConformance_LC042_ContainsUnknown(t *testing.T) {
	conformance.Covers(t, "LC-042")
	numberList := tenon.ListType(tenon.NumberType())
	isOpenBool(t, "Contains(unknown list, 1)", call(stdlib.ContainsFunc, tenon.Unknown(numberList), num("1")))
	isBool(t, "Contains(unknown empty list, 1)", call(stdlib.ContainsFunc, tenon.Narrow(tenon.Unknown(numberList), tenon.LengthMax(0)), num("1")), false)
	// A value that cannot be of the members' type is not among them.
	isBool(t, "Contains(unknown list of numbers, \"x\")", call(stdlib.ContainsFunc, tenon.Unknown(numberList), tenon.String("x")), false)
	isBool(t, "Contains(unknown tuple(number), \"x\")", call(stdlib.ContainsFunc, tenon.Unknown(tenon.TupleType(tenon.NumberType())), tenon.String("x")), false)
	// What the members' ranges rule out.
	big := tenon.Narrow(unknownNumber, tenon.NumberMin(num("5"), false))
	isBool(t, "Contains([1], unknown > 5)", call(stdlib.ContainsFunc, tenon.List(tenon.NumberType(), num("1")), big), false)
	isOpenBool(t, "Contains(pending, 1)", call(stdlib.ContainsFunc, tenon.Pending(tenon.ListOf(tenon.Any())), num("1")))
	// A set answers from its range.
	listedSet := tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.NumberType())), tenon.Members(num("1")))
	isBool(t, "Contains(unknown set listing 1, 1)", call(stdlib.ContainsFunc, listedSet, num("1")), true)
}

func TestConformance_LC043_SetHasElement(t *testing.T) {
	conformance.Covers(t, "LC-043")
	s := nset("1", "2")
	isBool(t, "SetHasElement({1, 2}, 1)", call(stdlib.SetHasElementFunc, s, num("1")), true)
	isBool(t, "SetHasElement({1, 2}, \"1\")", call(stdlib.SetHasElementFunc, s, tenon.String("1")), false)
	isOpenBool(t, "SetHasElement({1, unknown}, 5)", call(stdlib.SetHasElementFunc, tenon.Set(tenon.NumberType(), num("1"), unknownNumber), num("5")))
	// A list is read as a set under Unsafe, as a language converts it, and
	// refused under Safe.
	l := tenon.List(tenon.NumberType(), num("1"))
	isBool(t, "SetHasElement([1], 1) under Unsafe", tenon.Call(stdlib.SetHasElementFunc, []tenon.Value{l, num("1")}, tenon.Unsafe), true)
	failsWith(t, "SetHasElement([1], 1) under Safe", call(stdlib.SetHasElementFunc, l, num("1")), tenon.CodeConvertUnsafe, at(0))
	// A null can be asked after.
	isBool(t, "SetHasElement({null}, null)", call(stdlib.SetHasElementFunc, tenon.Set(tenon.NumberType(), tenon.Null(tenon.NumberType())), tenon.Null(tenon.NumberType())), true)
}

func TestConformance_LC044_SetArguments(t *testing.T) {
	conformance.Covers(t, "LC-044")
	strs := tenon.Set(str, a)
	// Element types unify under the call's policy.
	if got := tenon.Call(stdlib.SetUnionFunc, []tenon.Value{nset("1"), strs}, tenon.Unsafe); !got.Equal(tenon.Set(str, tenon.String("1"), a)) {
		t.Errorf("SetUnion({1}, {a}) under Unsafe = %v, want {\"1\", a}", got)
	}
	failsWith(t, "SetUnion({1}, {a}) under Safe", call(stdlib.SetUnionFunc, nset("1"), strs), tenon.CodeUnifyNoCommonConstraint, tenon.Path{})
	// The empty tuple is the empty set, with no element type of its own.
	if got := call(stdlib.SetUnionFunc, tenon.Tuple(), nset("1")); !got.Equal(nset("1")) {
		t.Errorf("SetUnion([], {1}) = %v, want {1}", got)
	}
	failsWith(t, "SetUnion([], [])", call(stdlib.SetUnionFunc, tenon.Tuple(), tenon.Tuple()), tenon.CodeConvertNoCommonType, at(0))
	failsWith(t, "SetUnion of a number", call(stdlib.SetUnionFunc, nset("1"), num("1")), tenon.CodeOperationWrongType, at(1))
	failsWith(t, "SetUnion of a list under Safe", call(stdlib.SetUnionFunc, nset("1"), tenon.List(tenon.NumberType(), num("2"))), tenon.CodeConvertUnsafe, at(1))
	if got := tenon.Call(stdlib.SetUnionFunc, []tenon.Value{nset("1"), tenon.List(tenon.NumberType(), num("2"))}, tenon.Unsafe); !got.Equal(nset("1", "2")) {
		t.Errorf("SetUnion({1}, [2]) under Unsafe = %v, want {1, 2}", got)
	}
	for name, f := range map[string]tenon.Function{
		"SetUnion": stdlib.SetUnionFunc, "SetIntersection": stdlib.SetIntersectionFunc,
		"SetSubtract": stdlib.SetSubtractFunc, "SetSymmetricDifference": stdlib.SetSymmetricDifferenceFunc,
	} {
		if !f.NotNull() {
			t.Errorf("%s does not declare its result never null", name)
		}
	}
	// A pending argument takes the element type the others give (CV-040),
	// and one of no type yet leaves the answer pending, a set.
	got := call(stdlib.SetUnionFunc, tenon.Pending(tenon.Any()), nset("1"))
	if got.IsPending() || !got.Type().Equal(tenon.SetType(tenon.NumberType())) || !tenon.Contains(got, num("1")).Equal(tenon.Bool(true)) {
		t.Errorf("SetUnion(pending, {1}) = %v, want a set of numbers holding 1", got)
	}
	got = call(stdlib.SetUnionFunc, tenon.Pending(tenon.Any()), tenon.Tuple())
	if !got.IsPending() || got.Constraint().Kind() != tenon.ConstraintSetOf {
		t.Errorf("SetUnion(pending, []) = %v, want a pending set", got)
	}
}

func TestConformance_LC045_SetUnion(t *testing.T) {
	conformance.Covers(t, "LC-045")
	if got := call(stdlib.SetUnionFunc, nset("1", "2"), nset("2", "3")); !got.Equal(nset("1", "2", "3")) {
		t.Errorf("SetUnion({1, 2}, {2, 3}) = %v", got)
	}
	// A member not known yet is kept.
	want := tenon.Set(tenon.NumberType(), num("1"), num("2"), unknownNumber)
	if got := call(stdlib.SetUnionFunc, tenon.Set(tenon.NumberType(), num("1"), unknownNumber), nset("2")); !got.Equal(want) {
		t.Errorf("SetUnion({1, unknown}, {2}) = %v, want %v", got, want)
	}
	// A set not known yet: the others' members listed, its lengths added.
	u := tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.NumberType())), tenon.NotNull(), tenon.LengthMin(3), tenon.LengthMax(4))
	got := call(stdlib.SetUnionFunc, u, nset("1"))
	if got.IsKnown() || got.Range().LengthMin() != 3 || !tenon.Contains(got, num("1")).Equal(tenon.Bool(true)) {
		t.Errorf("SetUnion(unknown set of 3 to 4, {1}) = %v, want 1 listed and at least 3", got)
	}
	if hi, ok := got.Range().LengthMax(); !ok || hi != 5 {
		t.Errorf("SetUnion(unknown set of 3 to 4, {1}) = %v, want at most 5", got)
	}
}

func TestConformance_LC046_SetIntersection(t *testing.T) {
	conformance.Covers(t, "LC-046")
	if got := call(stdlib.SetIntersectionFunc, nset("1", "2"), nset("2", "3")); !got.Equal(nset("2")) {
		t.Errorf("SetIntersection({1, 2}, {2, 3}) = %v", got)
	}
	if got := call(stdlib.SetIntersectionFunc, nset("1")); !got.Equal(nset("1")) {
		t.Errorf("SetIntersection({1}) = %v", got)
	}
	// The empty set settles it, whatever the other holds.
	if got := call(stdlib.SetIntersectionFunc, nset(), tenon.Set(tenon.NumberType(), unknownNumber)); !got.Equal(nset()) {
		t.Errorf("SetIntersection({}, {unknown}) = %v, want {}", got)
	}
	// Partly known: 1 is in, 2 undecided.
	got := call(stdlib.SetIntersectionFunc, nset("1", "2"), tenon.Set(tenon.NumberType(), num("1"), unknownNumber))
	if got.IsKnown() || !tenon.Contains(got, num("1")).Equal(tenon.Bool(true)) || got.Range().LengthMin() != 1 {
		t.Errorf("SetIntersection({1, 2}, {1, unknown}) = %v, want 1 listed", got)
	}
	if hi, ok := got.Range().LengthMax(); !ok || hi != 2 {
		t.Errorf("SetIntersection({1, 2}, {1, unknown}) = %v, want at most 2", got)
	}
	// No set holding its members: no longer than the shortest.
	short := tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.NumberType())), tenon.LengthMax(2))
	got = call(stdlib.SetIntersectionFunc, short, tenon.Unknown(tenon.SetType(tenon.NumberType())))
	if hi, ok := got.Range().LengthMax(); !ok || hi != 2 {
		t.Errorf("SetIntersection(unknown of at most 2, unknown) = %v, want at most 2", got)
	}
}

func TestConformance_LC047_SetSubtract(t *testing.T) {
	conformance.Covers(t, "LC-047")
	if got := call(stdlib.SetSubtractFunc, nset("1", "2"), nset("2", "3")); !got.Equal(nset("1")) {
		t.Errorf("SetSubtract({1, 2}, {2, 3}) = %v", got)
	}
	// Taking nothing away leaves the set as it is, members not known yet
	// and all.
	partly := tenon.Set(tenon.NumberType(), num("1"), unknownNumber)
	if got := call(stdlib.SetSubtractFunc, partly, nset()); !got.Equal(partly) {
		t.Errorf("SetSubtract({1, unknown}, {}) = %v, want the first", got)
	}
	if got := call(stdlib.SetSubtractFunc, nset(), tenon.Unknown(tenon.SetType(tenon.NumberType()))); !got.Equal(nset()) {
		t.Errorf("SetSubtract({}, unknown) = %v, want {}", got)
	}
	got := call(stdlib.SetSubtractFunc, nset("1", "2"), tenon.Set(tenon.NumberType(), unknownNumber))
	if got.IsKnown() || got.Range().LengthMin() != 1 {
		t.Errorf("SetSubtract({1, 2}, {unknown}) = %v, want at least 1", got)
	}
	// a not known yet: as long as a, less b.
	a := tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.NumberType())), tenon.NotNull(), tenon.LengthMin(5), tenon.LengthMax(7))
	got = call(stdlib.SetSubtractFunc, a, nset("1", "2"))
	if got.Range().LengthMin() != 3 {
		t.Errorf("SetSubtract(unknown of 5 to 7, {1, 2}) = %v, want at least 3", got)
	}
	if hi, ok := got.Range().LengthMax(); !ok || hi != 7 {
		t.Errorf("SetSubtract(unknown of 5 to 7, {1, 2}) = %v, want at most 7", got)
	}
}

func TestConformance_LC048_SetSymmetricDifference(t *testing.T) {
	conformance.Covers(t, "LC-048")
	for _, tt := range []struct {
		sets []tenon.Value
		want tenon.Value
	}{
		{[]tenon.Value{nset("1", "2"), nset("2", "3")}, nset("1", "3")},
		{[]tenon.Value{nset("1", "2"), nset("2", "3"), nset("3", "4")}, nset("1", "4")},
		{[]tenon.Value{nset("1", "2"), nset("1", "2"), nset("1", "2")}, nset("1", "2")},
	} {
		if got := call(stdlib.SetSymmetricDifferenceFunc, tt.sets...); !got.Equal(tt.want) {
			t.Errorf("SetSymmetricDifference(%v) = %v, want %v", tt.sets, got, tt.want)
		}
	}
	// 1 is in whatever the member not known yet is; it and 2 are open.
	got := call(stdlib.SetSymmetricDifferenceFunc, tenon.Set(tenon.NumberType(), num("1"), unknownNumber), nset("2"))
	if got.IsKnown() || got.Range().LengthMin() != 1 || !tenon.Contains(got, num("1")).Equal(tenon.Bool(true)) {
		t.Errorf("SetSymmetricDifference({1, unknown}, {2}) = %v, want an unknown set holding 1", got)
	}
	if hi, ok := got.Range().LengthMax(); !ok || hi != 3 {
		t.Errorf("SetSymmetricDifference({1, unknown}, {2}) = %v, want at most 3", got)
	}
}

func TestConformance_LC049_SetsNotKnown(t *testing.T) {
	conformance.Covers(t, "LC-049")
	// Each operation's answer not known yet is a set, not null, listing
	// the members known to be in it.
	u := tenon.Unknown(tenon.SetType(tenon.NumberType()))
	for name, f := range map[string]tenon.Function{
		"SetUnion": stdlib.SetUnionFunc, "SetIntersection": stdlib.SetIntersectionFunc,
		"SetSubtract": stdlib.SetSubtractFunc, "SetSymmetricDifference": stdlib.SetSymmetricDifferenceFunc,
	} {
		got := call(f, u, nset("1"))
		if got.IsKnown() || !got.Type().Equal(tenon.SetType(tenon.NumberType())) || !notNull(got) {
			t.Errorf("%s(unknown, {1}) = %v, want an unknown set not null", name, got)
		}
	}
	listed := tenon.Narrow(u, tenon.Members(num("1"), num("2")))
	if got := call(stdlib.SetSubtractFunc, listed, nset("2")); !tenon.Contains(got, num("1")).Equal(tenon.Bool(true)) {
		t.Errorf("SetSubtract(unknown listing 1 and 2, {2}) = %v, want 1 listed", got)
	}
}
