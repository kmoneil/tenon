package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// nlist returns the list of the numbers given, by their text.
func nlist(texts ...string) tenon.Value {
	ms := make([]tenon.Value, len(texts))
	for i, t := range texts {
		ms[i] = num(t)
	}
	return tenon.List(tenon.NumberType(), ms...)
}

// nums returns the numbers given, by their text, as arguments.
func nums(texts ...string) []tenon.Value {
	out := make([]tenon.Value, len(texts))
	for i, t := range texts {
		out[i] = num(t)
	}
	return out
}

func TestConformance_LC050_RangeForms(t *testing.T) {
	conformance.Covers(t, "LC-050")
	for _, tt := range []struct {
		args []string
		want tenon.Value
	}{
		{[]string{"3"}, nlist("0", "1", "2")},
		{[]string{"-3"}, nlist("0", "-1", "-2")},
		{[]string{"1.5"}, nlist("0", "1")},
		{[]string{"0"}, nlist()},
		{[]string{"4", "1"}, nlist("4", "3", "2")},
		{[]string{"0.5", "-1"}, nlist("0.5", "-0.5")},
		{[]string{"10", "1", "-3"}, nlist("10", "7", "4")},
		{[]string{"3", "3", "-1"}, nlist()},
	} {
		if got := call(stdlib.RangeFunc, nums(tt.args...)...); !got.Equal(tt.want) {
			t.Errorf("Range(%s) = %v, want %v", strings.Join(tt.args, ", "), got, tt.want)
		}
	}
	if !stdlib.RangeFunc.NotNull() {
		t.Error("Range does not declare its result never null")
	}
	// The boundary carries an argument's marks.
	if got := call(stdlib.RangeFunc, tenon.WithMarks(num("2"), bare("limit"))); !tenon.HasMark(got, bare("limit")) {
		t.Errorf("Range of a marked limit = %v, want its mark", got)
	}
}

func TestConformance_LC051_RangeFailures(t *testing.T) {
	conformance.Covers(t, "LC-051")
	// A step of zero, however it is written, at the step.
	for _, args := range [][]string{{"0", "5", "0"}, {"0", "5", "0.0"}, {"5", "5", "0"}, {"5", "0", "-0"}} {
		failsWith(t, "Range("+strings.Join(args, ", ")+")", call(stdlib.RangeFunc, nums(args...)...), tenon.CodeFunctionInvalidArgument, at(2))
	}
	// A step leading away from the limit, at the limit.
	failsWith(t, "Range(0, 3, -1)", call(stdlib.RangeFunc, nums("0", "3", "-1")...), tenon.CodeFunctionInvalidArgument, at(1))
	failsWith(t, "Range(5, 0, 1)", call(stdlib.RangeFunc, nums("5", "0", "1")...), tenon.CodeFunctionInvalidArgument, at(1))
	for _, args := range [][]tenon.Value{nil, nums("1", "2", "3", "4")} {
		if got := call(stdlib.RangeFunc, args...); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionArity {
			t.Errorf("Range of %d arguments = %v, want %s", len(args), got, tenon.CodeFunctionArity)
		}
	}
}

func TestConformance_LC052_RangeCount(t *testing.T) {
	conformance.Covers(t, "LC-052")
	// Exact elements and counts: no step drifts.
	tenths := call(stdlib.RangeFunc, nums("0", "1", "0.1")...)
	if tenths.Len() != 10 || !tenths.Index(7).Equal(num("0.7")) {
		t.Errorf("Range(0, 1, 0.1) = %v, want ten elements, the eighth 0.7", tenths)
	}
	for _, tt := range []struct {
		args []string
		n    int
	}{
		{[]string{"0", "0.05", "0.01"}, 5},
		{[]string{"0.3", "0", "-0.1"}, 3},
		{[]string{"0", "1024"}, 1024},
		{[]string{"-1024"}, 1024},
	} {
		if got := call(stdlib.RangeFunc, nums(tt.args...)...); got.IsError() || got.Len() != tt.n {
			t.Errorf("Range(%s) = %v, want %d elements", strings.Join(tt.args, ", "), got, tt.n)
		}
	}
	// Large numbers are exact too.
	huge := num("1e300")
	if got := call(stdlib.RangeFunc, huge, tenon.Add(huge, num("3"))); got.IsError() || got.Len() != 3 {
		t.Errorf("Range(1e300, 1e300 + 3) = %v, want 3 elements", got)
	}
	failsWith(t, "Range(0, 1025)", call(stdlib.RangeFunc, nums("0", "1025")...), tenon.CodeFunctionTooLarge, at(1))
	failsWith(t, "Range(1025)", call(stdlib.RangeFunc, num("1025")), tenon.CodeFunctionTooLarge, at(0))
	failsWith(t, "Range(0, 1, 1e-300)", call(stdlib.RangeFunc, nums("0", "1", "1e-300")...), tenon.CodeFunctionTooLarge, at(1))
}

func TestConformance_LC053_RangeUnknown(t *testing.T) {
	conformance.Covers(t, "LC-053")
	got := call(stdlib.RangeFunc, unknownNumber)
	if got.IsKnown() || !lengthBetween(got, 0, 1024) || !notNull(got) {
		t.Errorf("Range(unknown) = %v, want an unknown list of at most 1024", got)
	}
	// What the known arguments settle fails now.
	failsWith(t, "Range(unknown, 3, 0)", call(stdlib.RangeFunc, unknownNumber, num("3"), num("0")), tenon.CodeFunctionInvalidArgument, at(2))
	above5 := tenon.Narrow(unknownNumber, tenon.NumberMin(num("5"), false))
	failsWith(t, "Range(unknown > 5, 0, 1)", call(stdlib.RangeFunc, above5, num("0"), num("1")), tenon.CodeFunctionInvalidArgument, at(1))
	big := tenon.Narrow(unknownNumber, tenon.NumberMin(num("5000"), true))
	failsWith(t, "Range(0, unknown >= 5000)", call(stdlib.RangeFunc, num("0"), big), tenon.CodeFunctionTooLarge, at(1))
	failsWith(t, "Range(unknown >= 5000)", call(stdlib.RangeFunc, big), tenon.CodeFunctionTooLarge, at(0))
}

func TestConformance_LB031_Bound(t *testing.T) {
	conformance.Covers(t, "LB-031")
	// The failure names the bound, at the argument that drives the size.
	got := call(stdlib.RangeFunc, num("2000"))
	failsWith(t, "Range(2000)", got, tenon.CodeFunctionTooLarge, at(0))
	if !strings.Contains(got.Diagnostics()[0].Message, "1024") {
		t.Errorf("Range(2000) fails with %q, not naming the bound", got.Diagnostics()[0].Message)
	}
	// Decided before the work: 64 lists of two would be 2^64 tuples.
	two := tenon.List(str, a, b)
	args := make([]tenon.Value, 64)
	for i := range args {
		args[i] = two
	}
	got = call(stdlib.SetProductFunc, args...)
	failsWith(t, "SetProduct of 64 lists of two", got, tenon.CodeFunctionTooLarge, at(20))
	if !strings.Contains(got.Diagnostics()[0].Message, "1048576") {
		t.Errorf("SetProduct of 64 lists fails with %q, not naming the bound", got.Diagnostics()[0].Message)
	}
}

func TestConformance_LC054_SetProductTypes(t *testing.T) {
	conformance.Covers(t, "LC-054")
	ab, ones := list(a, b), nlist("1", "2")
	tuples := tenon.TupleType(str, tenon.NumberType())
	got := call(stdlib.SetProductFunc, ab, ones)
	if !got.Type().Equal(tenon.ListType(tuples)) || got.Len() != 4 {
		t.Errorf("SetProduct([a, b], [1, 2]) = %v, want a list of four tuples", got)
	}
	got = call(stdlib.SetProductFunc, tenon.Set(str, a, b), ones)
	if !got.Type().Equal(tenon.SetType(tuples)) || got.Len() != 4 {
		t.Errorf("SetProduct({a, b}, [1, 2]) = %v, want a set of four tuples", got)
	}
	// A tuple is read as a list, its element types unified under the
	// call's policy.
	mixed := tenon.Tuple(num("1"), a)
	got = tenon.Call(stdlib.SetProductFunc, []tenon.Value{mixed, tenon.List(tenon.BoolType(), tenon.Bool(true))}, tenon.Unsafe)
	want := tenon.List(tenon.TupleType(str, tenon.BoolType()), tenon.Tuple(tenon.String("1"), tenon.Bool(true)), tenon.Tuple(a, tenon.Bool(true)))
	if !got.Equal(want) {
		t.Errorf("SetProduct(tuple(1, a), [true]) under Unsafe = %v, want %v", got, want)
	}
	failsWith(t, "SetProduct(tuple(1, a), [true]) under Safe", call(stdlib.SetProductFunc, mixed, tenon.List(tenon.BoolType(), tenon.Bool(true))), tenon.CodeOperationWrongType, at(0))
	// The empty tuple makes the answer the empty tuple.
	if got := call(stdlib.SetProductFunc, tenon.Tuple(), ones); !got.Equal(tenon.Tuple()) {
		t.Errorf("SetProduct([], [1, 2]) = %v, want the empty tuple", got)
	}
	if got := call(stdlib.SetProductFunc, tenon.List(str), ones); !got.Equal(tenon.List(tuples)) {
		t.Errorf("SetProduct(empty list, [1, 2]) = %v, want the empty list of tuples", got)
	}
	failsWith(t, "SetProduct of a map", call(stdlib.SetProductFunc, ab, numbers(nil)), tenon.CodeOperationWrongType, at(1))
	if got := call(stdlib.SetProductFunc, ab); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionArity {
		t.Errorf("SetProduct of one argument = %v, want %s", got, tenon.CodeFunctionArity)
	}
	if !stdlib.SetProductFunc.NotNull() {
		t.Error("SetProduct does not declare its result never null")
	}
}

func TestConformance_LC055_SetProductOrder(t *testing.T) {
	conformance.Covers(t, "LC-055")
	got := call(stdlib.SetProductFunc, list(a, b), nlist("1", "2"))
	want := tenon.List(tenon.TupleType(str, tenon.NumberType()),
		tenon.Tuple(a, num("1")), tenon.Tuple(a, num("2")), tenon.Tuple(b, num("1")), tenon.Tuple(b, num("2")))
	if !got.Equal(want) {
		t.Errorf("SetProduct([a, b], [1, 2]) = %v, want %v", got, want)
	}
	// A list keeps its repeated members, and so its repeated tuples.
	if got := call(stdlib.SetProductFunc, nlist("1", "1"), list(a)); got.Len() != 2 {
		t.Errorf("SetProduct([1, 1], [a]) = %v, want two tuples", got)
	}
}

func TestConformance_LC056_SetProductBound(t *testing.T) {
	conformance.Covers(t, "LC-056")
	thousand := make([]tenon.Value, 1024)
	for i := range thousand {
		thousand[i] = tenon.NumberFromInt(int64(i))
	}
	long := tenon.List(tenon.NumberType(), thousand...)
	atLeast := func(n int64) tenon.Value {
		return tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.NotNull(), tenon.LengthMin(n))
	}
	// 1024 by at least 1024 is the bound itself, and is not past it.
	got := call(stdlib.SetProductFunc, atLeast(1024), long)
	if got.IsError() || got.Range().LengthMin() != 1<<20 {
		t.Errorf("SetProduct(unknown of at least 1024, 1024 numbers) = %v, want an unknown list of 1048576", got)
	}
	failsWith(t, "SetProduct(unknown of at least 1025, 1024 numbers)", call(stdlib.SetProductFunc, atLeast(1025), long), tenon.CodeFunctionTooLarge, at(1))
	failsWith(t, "SetProduct(1024 numbers, 1024 numbers, [a, b])", call(stdlib.SetProductFunc, long, long, list(a, b)), tenon.CodeFunctionTooLarge, at(2))
}

func TestConformance_LC057_SetProductUnknown(t *testing.T) {
	conformance.Covers(t, "LC-057")
	ones := nlist("1", "2")
	upTo3 := tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.NotNull(), tenon.LengthMax(3))
	got := call(stdlib.SetProductFunc, upTo3, ones)
	if got.IsKnown() || !lengthBetween(got, 0, 6) || !notNull(got) {
		t.Errorf("SetProduct(unknown list of at most 3, [1, 2]) = %v, want an unknown list of 0 to 6", got)
	}
	// An argument that can hold nothing makes the product empty.
	none := tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(0))
	if got := call(stdlib.SetProductFunc, none, tenon.Unknown(tenon.ListType(str))); !got.Equal(tenon.List(tenon.TupleType(str, str))) {
		t.Errorf("SetProduct(unknown empty list, unknown list) = %v, want the empty list", got)
	}
	// A set holding a member not known yet: one or two tuples.
	got = call(stdlib.SetProductFunc, tenon.Set(tenon.NumberType(), num("1"), unknownNumber), list(a))
	if n := tenon.Length(got); n.IsKnown() || got.Len() != 2 {
		t.Errorf("SetProduct({1, unknown}, [a]) = %v, want a set of one or two tuples", got)
	}
	if got := call(stdlib.SetProductFunc, tenon.Pending(tenon.Any()), ones); !got.IsPending() || !notNull(got) {
		t.Errorf("SetProduct(pending, [1, 2]) = %v, want pending and not null", got)
	}
	// Every argument's own marks reach the answer; a list's members keep
	// theirs within the tuples, and a set of tuples carries them itself.
	marked := tenon.List(str, tenon.WithMarks(a, bare("member")))
	got = call(stdlib.SetProductFunc, tenon.WithMarks(marked, bare("arg")), ones)
	if !tenon.HasMark(got, bare("arg")) || tenon.HasMark(got, bare("member")) {
		t.Errorf("SetProduct of a marked list = %v, want its own mark alone on the answer", got)
	}
	got = call(stdlib.SetProductFunc, marked, tenon.Set(tenon.NumberType(), num("1")))
	if !tenon.HasMark(got, bare("member")) {
		t.Errorf("SetProduct of a marked member into a set = %v, want the member's mark on the set", got)
	}
}
