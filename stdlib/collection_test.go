package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

var (
	str = tenon.StringType()
	a   = tenon.String("a")
	b   = tenon.String("b")
	c   = tenon.String("c")
)

// list returns the list of strings given.
func list(vs ...tenon.Value) tenon.Value { return tenon.List(str, vs...) }

// failsWith holds got to failing with code at path.
func failsWith(t *testing.T, what string, got tenon.Value, code tenon.Code, path tenon.Path) {
	t.Helper()
	if !got.IsError() || got.Diagnostics()[0].Code != code || !got.Diagnostics()[0].Path.Equal(path) {
		t.Errorf("%s = %v, want %s at %s", what, got, code, path)
	}
}

func TestConformance_LC001_Length(t *testing.T) {
	conformance.Covers(t, "LC-001")
	unknownList := tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMin(2), tenon.LengthMax(5))
	for _, tt := range []struct {
		v, want tenon.Value
	}{
		{list(a, b, c), num("3")},
		{tenon.Tuple(a, num("1")), num("2")},
		{tenon.Object(map[string]tenon.Value{"x": a, "y": b}), num("2")},
		{tenon.String("e\U00000301\U0001F1FA\U0001F1F8"), num("2")},
		{tenon.Map(str, map[string]tenon.Value{"k": a}), num("1")},
		{tenon.Unknown(tenon.TupleType(str, str)), num("2")},
		{unknownList, tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(num("2"), true), tenon.NumberMax(num("5"), true))},
	} {
		if got := call(stdlib.LengthFunc, tt.v); !got.Equal(tt.want) {
			t.Errorf("Length(%v) = %v, want %v", tt.v, got, tt.want)
		}
	}
	// A set holding a member not known yet may hold fewer than it lists.
	set := tenon.Set(tenon.NumberType(), num("1"), tenon.Unknown(tenon.NumberType()))
	if got := call(stdlib.LengthFunc, set); got.IsKnown() {
		t.Errorf("Length(%v) = %v, want a length between 1 and 2", set, got)
	}
	// The value's own marks reach the answer, its members' do not.
	if got := call(stdlib.LengthFunc, tenon.WithMarks(list(tenon.WithMarks(a, bare("held"))), bare("own"))); !got.Equal(tenon.WithMarks(num("1"), bare("own"))) {
		t.Errorf("Length of a marked list = %v, want 1 carrying its own mark alone", got)
	}
	failsWith(t, "Length(1)", call(stdlib.LengthFunc, num("1")), tenon.CodeOperationWrongType, at(0))
}

func TestConformance_LC002_HasIndex(t *testing.T) {
	conformance.Covers(t, "LC-002")
	abc := list(a, b, c)
	for _, tt := range []struct {
		coll, key tenon.Value
		want      tenon.Value
	}{
		{abc, num("2"), tenon.Bool(true)}, {abc, num("3"), tenon.Bool(false)}, {abc, num("-1"), tenon.Bool(false)},
		{abc, num("1.5"), tenon.Bool(false)}, {abc, tenon.String("1"), tenon.Bool(false)}, {abc, num("1e30"), tenon.Bool(false)},
		{tenon.Map(str, map[string]tenon.Value{"a": a}), tenon.String("a"), tenon.Bool(true)},
		{tenon.Object(map[string]tenon.Value{"a": a}), tenon.String("a"), tenon.Bool(true)},
		{tenon.Object(map[string]tenon.Value{"a": a}), tenon.String("b"), tenon.Bool(false)},
		{tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"a": str})), tenon.String("a"), tenon.Bool(true)},
		{tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMin(1)), num("0"), tenon.Bool(true)},
		{tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(2)), num("2"), tenon.Bool(false)},
		{tenon.Tuple(a, num("1")), tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NumberMax(num("0"), false)), tenon.Bool(false)},
	} {
		if got := call(stdlib.HasIndexFunc, tt.coll, tt.key); !got.Equal(tt.want) {
			t.Errorf("HasIndex(%v, %v) = %v, want %v", tt.coll, tt.key, got, tt.want)
		}
	}
	if got := call(stdlib.HasIndexFunc, tenon.Unknown(tenon.ListType(str)), num("0")); got.IsKnown() || !notNull(got) {
		t.Errorf("HasIndex(unknown list, 0) = %v, want an unknown Bool, not null", got)
	}
	// It reads the length, not the members: a member's mark stays off.
	if got := call(stdlib.HasIndexFunc, list(tenon.WithMarks(a, secret{})), num("0")); !got.Equal(tenon.Bool(true)) {
		t.Errorf("HasIndex([redacted], 0) = %v, want true, unmarked", got)
	}
	failsWith(t, "HasIndex of a set", call(stdlib.HasIndexFunc, tenon.Set(str, a), num("0")), tenon.CodeOperationWrongType, at(0))
}

func TestConformance_LC003_Index(t *testing.T) {
	conformance.Covers(t, "LC-003")
	abc := list(a, b, tenon.WithMarks(c, bare("held")))
	if got := call(stdlib.IndexFunc, abc, num("1")); !got.Equal(b) {
		t.Errorf("Index(%v, 1) = %v, want b, unmarked", abc, got)
	}
	if got := call(stdlib.IndexFunc, abc, num("2")); !got.Equal(tenon.WithMarks(c, bare("held"))) {
		t.Errorf("Index(%v, 2) = %v, want c with its own mark", abc, got)
	}
	tuple := tenon.Tuple(a, num("1"))
	if got := call(stdlib.IndexFunc, tuple, num("1")); !got.Equal(num("1")) {
		t.Errorf("Index(%v, 1) = %v, want 1", tuple, got)
	}
	obj := tenon.Object(map[string]tenon.Value{"x": a})
	if got := call(stdlib.IndexFunc, obj, tenon.String("x")); !got.Equal(a) {
		t.Errorf("Index(%v, \"x\") = %v, want a", obj, got)
	}
	for _, key := range []string{"3", "-1", "1.5", "1e30"} {
		failsWith(t, "Index(abc, "+key+")", call(stdlib.IndexFunc, abc, num(key)), tenon.CodeFunctionInvalidArgument, at(1))
	}
	failsWith(t, "Index(abc, \"0\")", call(stdlib.IndexFunc, abc, tenon.String("0")), tenon.CodeOperationWrongType, at(1))
	failsWith(t, "Index(obj, \"y\")", call(stdlib.IndexFunc, obj, tenon.String("y")), tenon.CodeFunctionInvalidArgument, at(1))
	short := tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(2))
	failsWith(t, "Index(a list of at most 2, 2)", call(stdlib.IndexFunc, short, num("2")), tenon.CodeFunctionInvalidArgument, at(1))
	// An unknown key into a tuple gives one of its members' types.
	rc, err := tenon.ResultConstraint(stdlib.IndexFunc, []tenon.Value{tuple, tenon.Unknown(tenon.NumberType())}, tenon.Safe)
	if want := tenon.OneOf(tenon.Exactly(str), tenon.Exactly(tenon.NumberType())); err != nil || !rc.Equal(want) {
		t.Errorf("the result of Index(%v, unknown) is %v, %v; want %v", tuple, rc, err, want)
	}
	if got := call(stdlib.IndexFunc, tenon.Unknown(tenon.ListType(str)), num("0")); got.IsKnown() || got.IsError() {
		t.Errorf("Index(unknown list, 0) = %v, want the unknown String", got)
	}
}

func TestConformance_LC004_Element(t *testing.T) {
	conformance.Covers(t, "LC-004")
	abc := list(a, b, c)
	for _, tt := range [][2]string{{"0", "a"}, {"4", "b"}, {"-1", "c"}, {"-4", "c"}, {"1e30", "b"}} {
		if got := call(stdlib.ElementFunc, abc, num(tt[0])); !got.Equal(tenon.String(tt[1])) {
			t.Errorf("Element(abc, %s) = %v, want %s", tt[0], got, tt[1])
		}
	}
	if got := call(stdlib.ElementFunc, tenon.Tuple(a, num("1")), num("-1")); !got.Equal(num("1")) {
		t.Errorf("Element(tuple(a, 1), -1) = %v, want 1", got)
	}
	failsWith(t, "Element(abc, 1.5)", call(stdlib.ElementFunc, abc, num("1.5")), tenon.CodeFunctionInvalidArgument, at(1))
	failsWith(t, "Element([], 0)", call(stdlib.ElementFunc, tenon.List(str), num("0")), tenon.CodeFunctionInvalidArgument, at(0))
	failsWith(t, "Element(a list of none, 0)", call(stdlib.ElementFunc, tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(0)), num("0")), tenon.CodeFunctionInvalidArgument, at(0))
	failsWith(t, "Element of a set", call(stdlib.ElementFunc, tenon.Set(str, a), num("0")), tenon.CodeOperationWrongType, at(0))
	// The list's own marks reach the answer, carried on an unknown one too.
	marked := tenon.WithMarks(abc, secret{})
	if got := call(stdlib.ElementFunc, marked, tenon.Unknown(tenon.NumberType())); !tenon.HasMark(got, secret{}) || got.IsKnown() {
		t.Errorf("Element(a redacted list, unknown) = %v, want an unknown carrying the mark", got)
	}
}

func TestConformance_LB012_PartlyKnownCollections(t *testing.T) {
	conformance.Covers(t, "LB-012")
	// A list holding a member not known yet is read for the members it
	// has: its length is known, and a known member is the answer.
	partly := list(a, tenon.Unknown(str))
	if got := call(stdlib.LengthFunc, partly); !got.Equal(num("2")) {
		t.Errorf("Length(%v) = %v, want 2", partly, got)
	}
	if got := call(stdlib.ElementFunc, partly, num("0")); !got.Equal(a) {
		t.Errorf("Element(%v, 0) = %v, want a", partly, got)
	}
	if got := call(stdlib.HasIndexFunc, partly, num("1")); !got.Equal(tenon.Bool(true)) {
		t.Errorf("HasIndex(%v, 1) = %v, want true", partly, got)
	}
	if got := call(stdlib.IndexFunc, partly, num("1")); got.IsKnown() || got.IsError() {
		t.Errorf("Index(%v, 1) = %v, want the member not known yet", partly, got)
	}
}

func TestConformance_LB013_Sequences(t *testing.T) {
	conformance.Covers(t, "LB-013")
	// A tuple of any length is a sequence, the empty one among them, and
	// another kind is refused by the derivation, located at it.
	if got := call(stdlib.LengthFunc, tenon.Tuple()); !got.Equal(num("0")) {
		t.Errorf("Length([]) = %v, want 0", got)
	}
	failsWith(t, "Element(tuple(), 0)", call(stdlib.ElementFunc, tenon.Tuple(), num("0")), tenon.CodeFunctionInvalidArgument, at(0))
	_, err := tenon.ResultConstraint(stdlib.ElementFunc, []tenon.Value{num("1"), num("0")}, tenon.Safe)
	if err == nil || err.Diagnostics()[0].Code != tenon.CodeOperationWrongType || !err.Diagnostics()[0].Path.Equal(at(0)) {
		t.Errorf("the result of Element(1, 0) refused with %v, want %s at [0]", err, tenon.CodeOperationWrongType)
	}
}
