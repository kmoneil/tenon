package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// lengthBetween reports whether v is an unknown list, not null, of a length
// from lo to hi.
func lengthBetween(v tenon.Value, lo, hi int64) bool {
	if v.IsKnown() || v.IsError() || v.IsPending() || !notNull(v) {
		return false
	}
	r := v.Range()
	max, ok := r.LengthMax()
	return r.LengthMin() == lo && ok && max == hi
}

func TestConformance_LC010_Slice(t *testing.T) {
	conformance.Covers(t, "LC-010")
	abc := list(a, b, c)
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	if got := call(stdlib.SliceFunc, abc, n(1), n(2)); !got.Equal(list(b)) {
		t.Errorf("Slice(abc, 1, 2) = %v, want [b]", got)
	}
	if got := call(stdlib.SliceFunc, abc, n(3), n(3)); !got.Equal(tenon.List(str)) {
		t.Errorf("Slice(abc, 3, 3) = %v, want []", got)
	}
	tuple := tenon.Tuple(a, num("1"), tenon.Bool(true))
	if got := call(stdlib.SliceFunc, tuple, n(1), n(3)); !got.Equal(tenon.Tuple(num("1"), tenon.Bool(true))) {
		t.Errorf("Slice(%v, 1, 3) = %v", tuple, got)
	}
	for _, tt := range []struct {
		start, end tenon.Value
		at         int
	}{
		{n(2), n(1), 1}, {n(-1), n(1), 1}, {n(0), n(4), 2}, {num("0.5"), n(1), 1}, {n(0), num("1e30"), 2},
	} {
		failsWith(t, "Slice(abc, "+tt.start.String()+", "+tt.end.String()+")", call(stdlib.SliceFunc, abc, tt.start, tt.end), tenon.CodeFunctionInvalidArgument, at(tt.at))
	}
	failsWith(t, "Slice of a set", call(stdlib.SliceFunc, tenon.Set(str, a), n(0), n(1)), tenon.CodeOperationWrongType, at(0))
	// A list not known yet: the length the indexes give, or a failure now.
	unknown := tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(3))
	if got := call(stdlib.SliceFunc, unknown, n(0), n(2)); !lengthBetween(got, 2, 2) {
		t.Errorf("Slice(unknown, 0, 2) = %v, want an unknown list of 2", got)
	}
	failsWith(t, "Slice(a list of at most 3, 0, 5)", call(stdlib.SliceFunc, unknown, n(0), n(5)), tenon.CodeFunctionInvalidArgument, at(2))
}

func TestConformance_LC011_ReverseList(t *testing.T) {
	conformance.Covers(t, "LC-011")
	if got := call(stdlib.ReverseListFunc, list(a, b, c)); !got.Equal(list(c, b, a)) {
		t.Errorf("ReverseList([a, b, c]) = %v", got)
	}
	if got := call(stdlib.ReverseListFunc, tenon.Tuple(num("1"), a)); !got.Equal(tenon.Tuple(a, num("1"))) {
		t.Errorf("ReverseList(tuple(1, a)) = %v", got)
	}
	set := tenon.Set(tenon.NumberType(), num("3"), num("1"), num("2"))
	if got := call(stdlib.ReverseListFunc, set); !got.Equal(tenon.List(tenon.NumberType(), num("3"), num("2"), num("1"))) {
		t.Errorf("ReverseList(%v) = %v, want [3, 2, 1]", set, got)
	}
	// A set whose length is a range answers the unknown list of it.
	partly := tenon.Set(tenon.NumberType(), num("1"), tenon.Unknown(tenon.NumberType()))
	if got := call(stdlib.ReverseListFunc, partly); !lengthBetween(got, 1, 2) {
		t.Errorf("ReverseList(%v) = %v, want an unknown list of 1 to 2", partly, got)
	}
	failsWith(t, "ReverseList(\"abc\")", call(stdlib.ReverseListFunc, tenon.String("abc")), tenon.CodeOperationWrongType, at(0))
}

func TestConformance_LC012_Concat(t *testing.T) {
	conformance.Covers(t, "LC-012")
	one, two := num("1"), num("2")
	nums := func(vs ...tenon.Value) tenon.Value { return tenon.List(tenon.NumberType(), vs...) }
	if got := call(stdlib.ConcatFunc, nums(one), nums(two)); !got.Equal(nums(one, two)) {
		t.Errorf("Concat([1], [2]) = %v", got)
	}
	// Element types unify under the call's policy, or the answer is a tuple.
	if got := tenon.Call(stdlib.ConcatFunc, []tenon.Value{nums(one), list(a)}, tenon.Unsafe); !got.Equal(list(tenon.String("1"), a)) {
		t.Errorf("Concat([1], [a]) under Unsafe = %v, want [\"1\", \"a\"]", got)
	}
	if got := call(stdlib.ConcatFunc, nums(one), list(a)); !got.Equal(tenon.Tuple(one, a)) {
		t.Errorf("Concat([1], [a]) under Safe = %v, want the tuple (1, \"a\")", got)
	}
	if got := call(stdlib.ConcatFunc, nums(one), tenon.Tuple(a)); !got.Equal(tenon.Tuple(one, a)) {
		t.Errorf("Concat([1], tuple(a)) = %v", got)
	}
	// Lists not known yet sum their lengths.
	unknown := tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.NumberType())), tenon.LengthMin(2), tenon.LengthMax(4))
	if got := call(stdlib.ConcatFunc, unknown, nums(one)); !lengthBetween(got, 3, 5) {
		t.Errorf("Concat(unknown 2..4, [1]) = %v, want an unknown list of 3 to 5", got)
	}
	failsWith(t, "Concat([1], a set)", call(stdlib.ConcatFunc, nums(one), tenon.Set(tenon.NumberType(), two)), tenon.CodeOperationWrongType, at(1))
	// Each argument's own marks; the elements keep theirs.
	if got := call(stdlib.ConcatFunc, tenon.WithMarks(nums(one), bare("own")), nums(tenon.WithMarks(two, bare("held")))); !got.Equal(tenon.WithMarks(nums(one, tenon.WithMarks(two, bare("held"))), bare("own"))) {
		t.Errorf("Concat of marked lists = %v", got)
	}
}

func TestConformance_LC013_Chunklist(t *testing.T) {
	conformance.Covers(t, "LC-013")
	nums := func(ns ...string) tenon.Value {
		var vs []tenon.Value
		for _, s := range ns {
			vs = append(vs, num(s))
		}
		return tenon.List(tenon.NumberType(), vs...)
	}
	lists := func(vs ...tenon.Value) tenon.Value { return tenon.List(tenon.ListType(tenon.NumberType()), vs...) }
	five := nums("1", "2", "3", "4", "5")
	for _, tt := range []struct {
		size string
		want tenon.Value
	}{
		{"2", lists(nums("1", "2"), nums("3", "4"), nums("5"))},
		{"0", lists(five)},
		{"10", lists(five)},
		{"1e30", lists(five)},
	} {
		if got := call(stdlib.ChunklistFunc, five, num(tt.size)); !got.Equal(tt.want) {
			t.Errorf("Chunklist(%v, %s) = %v, want %v", five, tt.size, got, tt.want)
		}
	}
	if got := call(stdlib.ChunklistFunc, tenon.Tuple(), num("2")); !got.Equal(tenon.Tuple()) {
		t.Errorf("Chunklist([], 2) = %v, want []", got)
	}
	if got := call(stdlib.ChunklistFunc, tenon.Tuple(num("1"), num("2")), num("1")); !got.Equal(lists(nums("1"), nums("2"))) {
		t.Errorf("Chunklist(tuple(1, 2), 1) = %v", got)
	}
	for _, size := range []string{"-1", "1.5"} {
		failsWith(t, "Chunklist(five, "+size+")", call(stdlib.ChunklistFunc, five, num(size)), tenon.CodeFunctionInvalidArgument, at(1))
	}
	unknown := tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.NumberType())), tenon.LengthMin(3), tenon.LengthMax(5))
	if got := call(stdlib.ChunklistFunc, unknown, num("2")); !lengthBetween(got, 2, 3) {
		t.Errorf("Chunklist(unknown 3..5, 2) = %v, want an unknown list of 2 to 3", got)
	}
	if got := call(stdlib.ChunklistFunc, five, tenon.Unknown(tenon.NumberType())); !lengthBetween(got, 1, 5) {
		t.Errorf("Chunklist(five, unknown) = %v, want an unknown list of 1 to 5", got)
	}
}

func TestConformance_LC020_Flatten(t *testing.T) {
	conformance.Covers(t, "LC-020")
	nums := func(vs ...tenon.Value) tenon.Value { return tenon.List(tenon.NumberType(), vs...) }
	one, two, three := num("1"), num("2"), num("3")
	for _, tt := range []struct {
		v, want tenon.Value
	}{
		{tenon.List(tenon.ListType(tenon.NumberType()), nums(one, two), nums(three)), tenon.Tuple(one, two, three)},
		{tenon.Tuple(a, tenon.Tuple(b, list(c))), tenon.Tuple(a, b, c)},
		{tenon.List(tenon.ListType(tenon.NumberType()), tenon.Null(tenon.ListType(tenon.NumberType())), nums(one)), tenon.Tuple(tenon.Null(tenon.ListType(tenon.NumberType())), one)},
		{tenon.List(tenon.SetType(tenon.NumberType()), tenon.Set(tenon.NumberType(), two, one)), tenon.Tuple(one, two)},
		{tenon.List(tenon.ListType(tenon.NumberType()), nums(one), nums(tenon.Unknown(tenon.NumberType()))), tenon.Tuple(one, tenon.Unknown(tenon.NumberType()))},
		{tenon.Tuple(tenon.Map(tenon.NumberType(), map[string]tenon.Value{"k": one})), tenon.Tuple(tenon.Map(tenon.NumberType(), map[string]tenon.Value{"k": one}))},
	} {
		if got := call(stdlib.FlattenFunc, tt.v); !got.Equal(tt.want) {
			t.Errorf("Flatten(%v) = %v, want %v", tt.v, got, tt.want)
		}
	}
	// A nested list not known yet leaves the answer pending, not null.
	nested := tenon.List(tenon.ListType(tenon.NumberType()), nums(one), tenon.Unknown(tenon.ListType(tenon.NumberType())))
	if got := call(stdlib.FlattenFunc, nested); !got.IsPending() || !notNull(got) {
		t.Errorf("Flatten(%v) = %v, want a pending value, not null", nested, got)
	}
	failsWith(t, "Flatten of a map", call(stdlib.FlattenFunc, tenon.Map(tenon.NumberType(), map[string]tenon.Value{"a": one})), tenon.CodeOperationWrongType, at(0))
	// The containers' own marks, the leaves keeping theirs.
	v := tenon.Tuple(tenon.WithMarks(nums(one), bare("box")), tenon.WithMarks(two, bare("leaf")))
	if got := call(stdlib.FlattenFunc, v); !got.Equal(tenon.WithMarks(tenon.Tuple(one, tenon.WithMarks(two, bare("leaf"))), bare("box"))) {
		t.Errorf("Flatten(%v) = %v", v, got)
	}
}

func TestConformance_LC021_Compact(t *testing.T) {
	conformance.Covers(t, "LC-021")
	empty, null := tenon.String(""), tenon.Null(str)
	if got := call(stdlib.CompactFunc, list(a, empty, null, b)); !got.Equal(list(a, b)) {
		t.Errorf("Compact([a, \"\", null, b]) = %v, want [a, b]", got)
	}
	sure := tenon.Narrow(tenon.Unknown(str), tenon.NotNull(), tenon.StringPrefix("ab"))
	if got := call(stdlib.CompactFunc, list(a, sure)); !got.Equal(list(a, sure)) {
		t.Errorf("Compact([a, %v]) = %v, want it kept in place", sure, got)
	}
	if got := call(stdlib.CompactFunc, list(a, tenon.Unknown(str), empty)); !lengthBetween(got, 1, 2) {
		t.Errorf("Compact([a, unknown, \"\"]) = %v, want an unknown list of 1 to 2", got)
	}
	if got := call(stdlib.CompactFunc, tenon.Tuple()); !got.Equal(tenon.List(str)) {
		t.Errorf("Compact([]) = %v, want the empty list of strings", got)
	}
}

func TestConformance_LC022_Distinct(t *testing.T) {
	conformance.Covers(t, "LC-022")
	nums := func(ns ...string) tenon.Value {
		var vs []tenon.Value
		for _, s := range ns {
			vs = append(vs, num(s))
		}
		return tenon.List(tenon.NumberType(), vs...)
	}
	if got := call(stdlib.DistinctFunc, nums("1", "2", "1", "3", "2")); !got.Equal(nums("1", "2", "3")) {
		t.Errorf("Distinct([1, 2, 1, 3, 2]) = %v", got)
	}
	if got := call(stdlib.DistinctFunc, nums("1", "1.0")); !got.Equal(nums("1")) {
		t.Errorf("Distinct([1, 1.0]) = %v", got)
	}
	nulls := tenon.List(tenon.NumberType(), tenon.Null(tenon.NumberType()), num("1"), tenon.Null(tenon.NumberType()))
	if got := call(stdlib.DistinctFunc, nulls); !got.Equal(tenon.List(tenon.NumberType(), tenon.Null(tenon.NumberType()), num("1"))) {
		t.Errorf("Distinct(%v) = %v, want [null, 1]", nulls, got)
	}
	if got := call(stdlib.DistinctFunc, tenon.Tuple()); !got.Equal(tenon.Tuple()) {
		t.Errorf("Distinct([]) = %v, want []", got)
	}
	partly := tenon.List(tenon.NumberType(), num("1"), tenon.Unknown(tenon.NumberType()))
	if got := call(stdlib.DistinctFunc, partly); !lengthBetween(got, 1, 2) {
		t.Errorf("Distinct(%v) = %v, want an unknown list of 1 to 2", partly, got)
	}
	apart := tenon.List(tenon.NumberType(), num("1"), tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NumberMin(num("5"), true)))
	if got := call(stdlib.DistinctFunc, apart); !got.Equal(apart) {
		t.Errorf("Distinct(%v) = %v, want it as it is", apart, got)
	}
	// Known members are told apart by hash: many of them cost little.
	var many []tenon.Value
	for i := range 20000 {
		many = append(many, tenon.NumberFromInt(int64(i%10000)))
	}
	if got := call(stdlib.DistinctFunc, tenon.List(tenon.NumberType(), many...)); got.Len() != 10000 {
		t.Errorf("Distinct of 20,000 numbers, 10,000 distinct, has %d", got.Len())
	}
}

func TestConformance_LC023_CoalesceList(t *testing.T) {
	conformance.Covers(t, "LC-023")
	nums := func(vs ...tenon.Value) tenon.Value { return tenon.List(tenon.NumberType(), vs...) }
	one := num("1")
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	for _, tt := range []struct {
		args []tenon.Value
		want tenon.Value
	}{
		{[]tenon.Value{nums(), nums(one)}, nums(one)},
		{[]tenon.Value{tenon.Null(tenon.ListType(tenon.NumberType())), nums(one)}, nums(one)},
		{[]tenon.Value{untyped, nums(one)}, nums(one)},
		{[]tenon.Value{nums(one), tenon.Unknown(tenon.ListType(tenon.NumberType()))}, nums(one)},
		{[]tenon.Value{nums(tenon.Unknown(tenon.NumberType())), nums(one)}, nums(tenon.Unknown(tenon.NumberType()))},
	} {
		if got := call(stdlib.CoalesceListFunc, tt.args...); !got.Equal(tt.want) {
			t.Errorf("CoalesceList(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
	if got := call(stdlib.CoalesceListFunc, tenon.Unknown(tenon.ListType(tenon.NumberType())), nums(one)); got.IsKnown() || got.IsError() {
		t.Errorf("CoalesceList(unknown, [1]) = %v, want the unknown list", got)
	}
	failsWith(t, "CoalesceList([], [])", call(stdlib.CoalesceListFunc, nums(), nums()), tenon.CodeFunctionInvalidArgument, tenon.Path{})
	failsWith(t, "CoalesceList([1], a set)", call(stdlib.CoalesceListFunc, nums(one), tenon.Set(tenon.NumberType(), one)), tenon.CodeOperationWrongType, at(1))
	// The marks of the arguments examined, none of one after the choice.
	got := call(stdlib.CoalesceListFunc, tenon.WithMarks(nums(), bare("passed")), nums(one), tenon.WithMarks(nums(one), bare("after")))
	if !got.Equal(tenon.WithMarks(nums(one), bare("passed"))) {
		t.Errorf("CoalesceList with marks = %v", got)
	}
}
