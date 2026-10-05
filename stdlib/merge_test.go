package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// object returns the object of the attributes given.
func object(attrs map[string]tenon.Value) tenon.Value { return tenon.Object(attrs) }

// objectType returns the object type of the attribute types given.
func objectType(attrs map[string]tenon.Type) tenon.Type { return tenon.ObjectType(attrs) }

// mergeResult returns the constraint a call of Merge promises.
func mergeResult(t *testing.T, args ...tenon.Value) tenon.Constraint {
	t.Helper()
	c, err := tenon.ResultConstraint(stdlib.MergeFunc, args, tenon.Safe)
	if err != nil {
		t.Fatalf("ResultConstraint(Merge, %v) failed: %v", args, err)
	}
	return c
}

// admits checks which of the types the constraint c admits.
func admits(t *testing.T, what string, c tenon.Constraint, yes []tenon.Type, no []tenon.Type) {
	t.Helper()
	for _, ty := range yes {
		if !tenon.Satisfies(c, ty) {
			t.Errorf("%s: %v does not admit %v", what, c, ty)
		}
	}
	for _, ty := range no {
		if tenon.Satisfies(c, ty) {
			t.Errorf("%s: %v admits %v", what, c, ty)
		}
	}
}

// hasMemberMark reports whether the map m's member at key carries the mark
// "member".
func hasMemberMark(m tenon.Value, key string) bool {
	v, ok := m.LookupMapElement(key)
	return ok && tenon.HasMark(v, bare("member"))
}

func TestConformance_LC034_MergeType(t *testing.T) {
	conformance.Covers(t, "LC-034")
	one, two, x := num("1"), num("2"), tenon.String("x")
	for _, tt := range []struct {
		what string
		args []tenon.Value
		want tenon.Value
	}{
		{"no arguments", nil, object(nil)},
		{"maps of one type", []tenon.Value{numbers(map[string]tenon.Value{"a": one}), numbers(map[string]tenon.Value{"b": two})},
			numbers(map[string]tenon.Value{"a": one, "b": two})},
		{"maps of two types", []tenon.Value{numbers(map[string]tenon.Value{"a": one}), tenon.Map(str, map[string]tenon.Value{"a": x})},
			object(map[string]tenon.Value{"a": x})},
		{"objects", []tenon.Value{object(map[string]tenon.Value{"a": one}), object(map[string]tenon.Value{"a": x, "b": two})},
			object(map[string]tenon.Value{"a": x, "b": two})},
		{"a map and an object", []tenon.Value{numbers(map[string]tenon.Value{"a": one, "b": one}), object(map[string]tenon.Value{"b": x})},
			object(map[string]tenon.Value{"a": one, "b": x})},
	} {
		if got := call(stdlib.MergeFunc, tt.args...); !got.Equal(tt.want) {
			t.Errorf("Merge of %s = %v, want %v", tt.what, got, tt.want)
		}
	}
	failsWith(t, "Merge of a list", call(stdlib.MergeFunc, list(a)), tenon.CodeOperationWrongType, at(0))
	failsWith(t, "Merge of a null string", call(stdlib.MergeFunc, object(nil), tenon.Null(str)), tenon.CodeOperationWrongType, at(1))
	// The type is the maps' whatever their keys, known or not.
	if got := mergeResult(t, tenon.Unknown(tenon.MapType(tenon.NumberType())), numbers(nil)); !got.Equal(tenon.Exactly(tenon.MapType(tenon.NumberType()))) {
		t.Errorf("Merge of an unknown map and a map promises %v, want the map type", got)
	}
	if !stdlib.MergeFunc.NotNull() {
		t.Error("Merge does not declare its result never null")
	}
}

func TestConformance_LC035_MergeValue(t *testing.T) {
	conformance.Covers(t, "LC-035")
	one, two := num("1"), num("2")
	// A later argument's key takes the place of an earlier one's, the same
	// key once normalized.
	nfc, nfd := "\U000000e9", "e\U00000301"
	got := call(stdlib.MergeFunc, numbers(map[string]tenon.Value{nfc: one}), numbers(map[string]tenon.Value{nfd: two}))
	if want := numbers(map[string]tenon.Value{nfc: two}); !got.Equal(want) {
		t.Errorf("Merge of one key in two forms = %v, want %v", got, want)
	}
	// Every argument's own marks reach the answer; a member keeps its own,
	// and one taken away is not read.
	m := tenon.WithMarks(numbers(map[string]tenon.Value{"a": tenon.WithMarks(one, bare("member"))}), bare("arg"))
	got = call(stdlib.MergeFunc, m, numbers(map[string]tenon.Value{"b": two}))
	if !tenon.HasMark(got, bare("arg")) || tenon.HasMark(got, bare("member")) {
		t.Errorf("Merge of a marked map = %v, want its mark on the answer alone", got)
	}
	if v, _ := tenon.Unmark(got); !hasMemberMark(v, "a") {
		t.Errorf("Merge of a marked member = %v, want the member to keep its mark", got)
	}
	got = call(stdlib.MergeFunc, m, numbers(map[string]tenon.Value{"a": two}))
	if _, marks := tenon.UnmarkDeep(got); len(marks) != 1 {
		t.Errorf("Merge taking a marked member's place = %v, want the argument's mark alone", got)
	}
}

func TestConformance_LC036_MergeNulls(t *testing.T) {
	conformance.Covers(t, "LC-036")
	one, x := num("1"), tenon.String("x")
	nullObject := tenon.Null(objectType(map[string]tenon.Type{"a": tenon.NumberType()}))
	nullNumbers := tenon.Null(tenon.MapType(tenon.NumberType()))
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	ones := numbers(map[string]tenon.Value{"a": one})
	for _, tt := range []struct {
		what string
		args []tenon.Value
		want tenon.Value
	}{
		{"a null object", []tenon.Value{nullObject}, object(nil)},
		{"a null map", []tenon.Value{nullNumbers}, numbers(map[string]tenon.Value{})},
		{"a null map and a map of its type", []tenon.Value{nullNumbers, ones}, ones},
		{"a null map and a map of another", []tenon.Value{tenon.Null(tenon.MapType(str)), ones}, object(map[string]tenon.Value{"a": one})},
		{"a null object and a map", []tenon.Value{ones, nullObject}, object(map[string]tenon.Value{"a": one})},
		{"an untyped null and a map", []tenon.Value{untyped, ones, untyped}, ones},
		{"untyped nulls", []tenon.Value{untyped, untyped}, object(nil)},
		{"an untyped null and an object", []tenon.Value{object(map[string]tenon.Value{"a": x}), untyped}, object(map[string]tenon.Value{"a": x})},
	} {
		if got := call(stdlib.MergeFunc, tt.args...); !got.Equal(tt.want) {
			t.Errorf("Merge of %s = %v, want %v", tt.what, got, tt.want)
		}
	}
	// A null argument's marks reach the answer: whether it was null decided
	// what the answer is.
	for _, n := range []tenon.Value{nullObject, nullNumbers, untyped} {
		if got := call(stdlib.MergeFunc, tenon.WithMarks(n, bare("null")), ones); !tenon.HasMark(got, bare("null")) {
			t.Errorf("Merge of a marked %v = %v, want its mark", n, got)
		}
	}
}

func TestConformance_LC037_MergeUnknown(t *testing.T) {
	conformance.Covers(t, "LC-037")
	number := tenon.NumberType()
	one := num("1")
	ab := objectType(map[string]tenon.Type{"a": number, "b": number})
	onlyA := objectType(map[string]tenon.Type{"a": number})
	onlyB := objectType(map[string]tenon.Type{"b": number})
	known := object(map[string]tenon.Value{"a": one})

	// An object not known yet, known not to be null, adds its attributes
	// not known yet beside the others.
	got := call(stdlib.MergeFunc, known, tenon.Narrow(tenon.Unknown(onlyB), tenon.NotNull()))
	if want := object(map[string]tenon.Value{"a": one, "b": tenon.Unknown(number)}); !got.Equal(want) {
		t.Errorf("Merge of an object and one not null = %v, want %v", got, want)
	}

	// One that may be null adds them or none: one of the two shapes.
	maybe := tenon.Unknown(onlyB)
	got = call(stdlib.MergeFunc, known, maybe)
	if !got.IsPending() || !notNull(got) {
		t.Errorf("Merge of an object and one that may be null = %v, want pending and not null", got)
	}
	admits(t, "an object and one that may be null", mergeResult(t, known, maybe), []tenon.Type{onlyA, ab}, []tenon.Type{onlyB})
	// Where both shapes are one type, the answer is that object, the
	// attribute it may take not known yet.
	got = call(stdlib.MergeFunc, tenon.WithMarks(known, bare("known")), tenon.Unknown(onlyA))
	v, _ := tenon.Unmark(got)
	if !v.HasMembers() || !v.Type().Equal(onlyA) {
		t.Fatalf("Merge of two objects of one type, one may be null = %v, want that object", got)
	}
	if a, _ := v.LookupAttribute("a"); a.IsKnown() {
		t.Errorf("Merge of two objects of one type, one may be null = %v, want a not known yet", got)
	}
	// Past four such arguments, the attributes they add are optional.
	var many []tenon.Value
	var yes []tenon.Type
	for _, name := range []string{"p", "q", "r", "s", "t"} {
		ty := objectType(map[string]tenon.Type{name: number})
		many = append(many, tenon.Unknown(ty))
		yes = append(yes, ty)
	}
	c := mergeResult(t, many...)
	if c.Kind() != tenon.ConstraintObjectWith || !c.Closed() {
		t.Errorf("Merge of five objects that may be null promises %v, want a closed ObjectWith", c)
	}
	admits(t, "five objects that may be null", c, append(yes, objectType(nil)), []tenon.Type{onlyA})
	if c := mergeResult(t, many[:4]...); c.Kind() != tenon.ConstraintOneOf || len(c.Members()) != 16 {
		t.Errorf("Merge of four objects that may be null promises %v, want sixteen shapes", c)
	}

	// A map not known yet adds keys not known yet: a map's answer is the
	// unknown map of the lengths the arguments allow.
	ones := numbers(map[string]tenon.Value{"a": one, "b": one})
	someMap := tenon.Narrow(tenon.Unknown(tenon.MapType(number)), tenon.NotNull(), tenon.LengthMin(3), tenon.LengthMax(4))
	if got := call(stdlib.MergeFunc, someMap, ones); got.IsKnown() || !got.Type().Equal(tenon.MapType(number)) || got.Range().LengthMin() != 3 {
		t.Errorf("Merge of a map not known yet and two keys = %v, want an unknown map of 3 to 6", got)
	} else if hi, ok := got.Range().LengthMax(); !ok || hi != 6 {
		t.Errorf("Merge of a map not known yet and two keys = %v, want at most 6", got)
	}
	if got := call(stdlib.MergeFunc, tenon.Unknown(tenon.MapType(number)), ones); got.Range().LengthMin() != 2 {
		t.Errorf("Merge of a map that may be null and two keys = %v, want at least 2", got)
	}
	// An object's answer is open, each key it knows of the types that may
	// reach it.
	strA := object(map[string]tenon.Value{"a": tenon.String("x")})
	c = mergeResult(t, strA, tenon.Unknown(tenon.MapType(number)))
	admits(t, "an object and a map not known yet", c,
		[]tenon.Type{objectType(map[string]tenon.Type{"a": str}), onlyA, ab},
		[]tenon.Type{onlyB, tenon.MapType(number)})
	c = mergeResult(t, tenon.Unknown(tenon.MapType(number)), strA)
	admits(t, "a map not known yet and an object", c,
		[]tenon.Type{objectType(map[string]tenon.Type{"a": str, "b": number})},
		[]tenon.Type{onlyA})

	// A map not known yet may add a key a later object that may be null
	// adds or not.
	c = mergeResult(t, tenon.Unknown(tenon.MapType(number)), tenon.Unknown(objectType(map[string]tenon.Type{"a": str})))
	admits(t, "a map not known yet and an object that may be null", c,
		[]tenon.Type{onlyA, objectType(map[string]tenon.Type{"a": str}), objectType(nil)},
		[]tenon.Type{objectType(map[string]tenon.Type{"a": tenon.BoolType()})})

	// A pending argument may settle to a map or an object.
	c = mergeResult(t, tenon.Pending(tenon.Any()), ones)
	admits(t, "a pending value and a map", c,
		[]tenon.Type{tenon.MapType(number), ab, objectType(map[string]tenon.Type{"a": number, "b": number, "c": str})},
		[]tenon.Type{tenon.MapType(str), onlyA})
	c = mergeResult(t, tenon.Pending(tenon.ObjectWith(nil, false)), ones)
	admits(t, "a pending object and a map", c, []tenon.Type{ab}, []tenon.Type{tenon.MapType(number)})

	// A pending object holding its members gives one, known arguments
	// giving one whose members are given.
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	held := object(map[string]tenon.Value{"b": untyped})
	got = call(stdlib.MergeFunc, known, held)
	if !got.IsPending() || !got.HasMembers() || got.Len() != 2 {
		t.Errorf("Merge of an object and a pending one = %v, want a pending object of two", got)
	}

	// An argument's marks reach the answer whether or not it is known.
	if got := call(stdlib.MergeFunc, tenon.WithMarks(tenon.Unknown(tenon.MapType(number)), bare("unknown")), ones); !tenon.HasMark(got, bare("unknown")) {
		t.Errorf("Merge of a marked map not known yet = %v, want its mark", got)
	}
	// An attribute a maybe-null object may take keeps the marks of the
	// member it may leave.
	marked := object(map[string]tenon.Value{"a": tenon.WithMarks(one, bare("member"))})
	got, _ = tenon.Unmark(call(stdlib.MergeFunc, marked, tenon.Unknown(onlyA)))
	if a, _ := got.LookupAttribute("a"); !tenon.HasMark(a, bare("member")) {
		t.Errorf("Merge leaving a marked member perhaps = %v, want the member's mark", got)
	}
}
