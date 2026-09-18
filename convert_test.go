package tenon_test

import (
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
)

// Shorthands for the conversion tests.
var (
	num  = tenon.NumberType()
	str  = tenon.StringType()
	boo  = tenon.BoolType()
	n    = tenon.NumberFromInt
	s    = tenon.String
	safe = tenon.Safe
	uns  = tenon.Unsafe
)

// is returns Exactly(t).
func is(t tenon.Type) tenon.Constraint { return tenon.Exactly(t) }

// oneFieldOfTwoTypes is an open object with one required field admitting
// either of two types. Converting to it is pending while the keys that would
// settle the field are not in hand, and the type a member would have with no
// keys at all need not convert to it, which is the pair that C3 turned on.
var oneFieldOfTwoTypes = tenon.ObjectWith(map[string]tenon.Field{
	"a": tenon.Required(tenon.OneOf(is(num), is(boo))),
}, false)

// obj returns an object value.
func obj(attrs map[string]tenon.Value) tenon.Value { return tenon.ObjectVal(attrs) }

// wantValue fails t unless got is identical to want.
func wantValue(t *testing.T, what string, got, want tenon.Value) {
	t.Helper()
	if !tenon.Identical(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// wantDiag is a diagnostic a test expects: its code, and its path rendered.
type wantDiag struct {
	code tenon.Code
	path string
}

// wantErrors fails t unless got is an error value with exactly these
// diagnostics, in order.
func wantErrors(t *testing.T, what string, got tenon.Value, want ...wantDiag) {
	t.Helper()
	if !got.IsError() {
		t.Errorf("%s = %v, want an error value", what, got)
		return
	}
	var have []wantDiag
	for _, d := range got.Diagnostics() {
		have = append(have, wantDiag{d.Code, d.Path.String()})
	}
	if !slices.Equal(have, want) {
		t.Errorf("%s = %v, want diagnostics %v", what, got, want)
	}
}

func TestConformance_CV001_ConversionUnderAPolicy(t *testing.T) {
	conformance.Covers(t, "CV-001")
	// The caller chooses the policy, and the answer follows it.
	wantValue(t, `Convert("5", number, unsafe)`, tenon.Convert(s("5"), is(num), uns), n(5))
	wantErrors(t, `Convert("5", number, safe)`, tenon.Convert(s("5"), is(num), safe), wantDiag{tenon.CodeConvertUnsafe, "."})

	// The result satisfies the constraint.
	for _, c := range []tenon.Constraint{is(str), tenon.Any(), tenon.OneOf(is(boo), is(str))} {
		if r := tenon.Convert(n(5), c, uns); !tenon.Satisfies(c, r.Type()) {
			t.Errorf("Convert(5, %v) = %v, whose type does not satisfy it", c, r)
		}
	}

	// Nothing converts unless asked: operations take their operands as they
	// are, so a string that reads as a number is not one.
	wantValue(t, `Equals("1", 1)`, tenon.Equals(s("1"), n(1)), tenon.Bool(false))
	mustPanicUsage(t, "does not satisfy exactly(number)", func() { tenon.Add(s("1"), n(1)) })

	// A policy is always chosen.
	mustPanicUsage(t, "neither Safe nor Unsafe", func() { tenon.Convert(n(1), is(str), 0) })
	mustPanicUsage(t, "use of the zero Constraint", func() { tenon.Convert(n(1), tenon.Constraint{}, safe) })
	if safe.String() != "safe" || uns.String() != "unsafe" {
		t.Errorf("the policies are named %q and %q", safe, uns)
	}
}

func TestConformance_CV002_ConversionIsAnOperation(t *testing.T) {
	conformance.Covers(t, "CV-002")
	carried := stamp{id: "carried"}
	isolated := stamp{id: "isolated", policy: tenon.Isolate}

	// An error value converts to an error value carrying its diagnostics.
	bad := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"})
	for _, p := range []tenon.Policy{safe, uns} {
		wantValue(t, "Convert(error)", tenon.Convert(bad, is(num), p), bad)
	}

	// The result carries the Propagate marks of the value converted.
	marked := tenon.WithMarks(n(5), carried, isolated)
	wantValue(t, "Convert(marked 5, string)", tenon.Convert(marked, is(str), uns), tenon.WithMarks(s("5"), carried))

	// A value whose type satisfies the constraint converts to itself under
	// either policy, carrying only its Propagate marks.
	list := tenon.ListVal(str, tenon.WithMarks(s("a"), isolated))
	for _, p := range []tenon.Policy{safe, uns} {
		for _, c := range []tenon.Constraint{is(tenon.List(str)), tenon.ListOf(tenon.Any()), tenon.Any()} {
			wantValue(t, "Convert(list, "+c.String()+")", tenon.Convert(tenon.WithMarks(list, carried, isolated), c, p),
				tenon.WithMarks(list, carried))
		}
	}
	// Even under the safe policy, a string stays a string.
	wantValue(t, `Convert("x", string, safe)`, tenon.Convert(s("x"), is(str), safe), s("x"))
}

func TestConformance_CV003_ResultTypeFollowsFromTypes(t *testing.T) {
	conformance.Covers(t, "CV-003")
	// Whatever a string holds, converting it to a number gives a number or an
	// error value, never a value of some other type.
	for _, text := range []string{"1", "1.5e3", "x", "", "true", "1e9999999"} {
		r := tenon.Convert(s(text), is(num), uns)
		if !r.IsError() && r.Type() != num {
			t.Errorf("Convert(%q, number) = %v", text, r)
		}
	}

	// An unknown collection converts to the unknown of the target's type, not
	// of its own element type (go-cty #216).
	listOfStrings := tenon.ListOf(is(str))
	for _, v := range []tenon.Value{
		tenon.Unknown(tenon.List(num)),
		tenon.Narrow(tenon.Unknown(tenon.List(num)), tenon.LengthMin(1)),
		tenon.ListVal(num, tenon.Unknown(num)),
		tenon.Unknown(tenon.Set(num)),
		tenon.Unknown(tenon.Tuple(num, boo)),
	} {
		r := tenon.Convert(v, listOfStrings, uns)
		if r.IsError() || r.IsPending() || r.Type() != tenon.List(str) {
			t.Errorf("Convert(%v, %v) = %v, want a value of list(string)", v, listOfStrings, r)
		}
	}

	// The keys of a map settle the type of the object it converts to.
	open := tenon.ObjectWith(nil, false)
	one := tenon.Convert(tenon.MapVal(num, map[string]tenon.Value{"a": n(1)}), open, uns)
	two := tenon.Convert(tenon.MapVal(num, map[string]tenon.Value{"a": n(1), "b": n(2)}), open, uns)
	if one.Type() != tenon.Object(map[string]tenon.Type{"a": num}) ||
		two.Type() != tenon.Object(map[string]tenon.Type{"a": num, "b": num}) {
		t.Errorf("maps converted to objects gave %v and %v", one, two)
	}
}

func TestConformance_CV010_ConversionTable(t *testing.T) {
	conformance.Covers(t, "CV-010")
	listNum := tenon.ListVal(num, n(2), n(1), n(2))
	setNum := tenon.SetVal(num, n(2), n(1))
	tupleNum := tenon.TupleVal(n(2), n(1))
	mapNum := tenon.MapVal(num, map[string]tenon.Value{"a": n(1), "b": n(2)})
	objNum := obj(map[string]tenon.Value{"a": n(1), "b": n(2)})
	for _, tt := range []struct {
		name   string
		v      tenon.Value
		to     tenon.Type
		want   tenon.Value
		unsafe bool
	}{
		{"number to string", n(-15), str, s("-15"), true},
		{"number to string, canonical", tenon.NumberFromText("1.50e30"), str, s("1.5e30"), true},
		{"string to number", s("-1.50"), num, tenon.Div(n(-15), n(10)), true},
		{"bool to string", tenon.Bool(false), str, s("false"), true},
		{"string to bool", s("true"), boo, tenon.Bool(true), true},
		{"list to list", tenon.ListVal(str, s("a")), tenon.List(str), tenon.ListVal(str, s("a")), false},
		{"list to list, unsafe members", listNum, tenon.List(str), tenon.ListVal(str, s("2"), s("1"), s("2")), true},
		{"set to set", setNum, tenon.Set(str), tenon.SetVal(str, s("1"), s("2")), true},
		{"map to map", mapNum, tenon.Map(str), tenon.MapVal(str, map[string]tenon.Value{"a": s("1"), "b": s("2")}), true},
		{"set to list", setNum, tenon.List(num), tenon.ListVal(num, n(1), n(2)), false},
		{"tuple to list", tupleNum, tenon.List(num), tenon.ListVal(num, n(2), n(1)), false},
		{"object to map", objNum, tenon.Map(num), mapNum, false},
		{"tuple to tuple", tupleNum, tenon.Tuple(str, num), tenon.TupleVal(s("2"), n(1)), true},
		{"object to object", objNum, tenon.Object(map[string]tenon.Type{"a": str, "b": num}), obj(map[string]tenon.Value{"a": s("1"), "b": n(2)}), true},
		{"list to set", listNum, tenon.Set(num), setNum, true},
		{"tuple to set", tupleNum, tenon.Set(num), setNum, true},
		{"list to tuple", tenon.ListVal(num, n(2), n(1)), tenon.Tuple(num, num), tupleNum, true},
		{"set to tuple", setNum, tenon.Tuple(num, str), tenon.TupleVal(n(1), s("2")), true},
		{"map to object", mapNum, tenon.Object(map[string]tenon.Type{"a": num, "b": num}), objNum, true},
	} {
		wantValue(t, tt.name+", unsafe", tenon.Convert(tt.v, is(tt.to), uns), tt.want)
		got := tenon.Convert(tt.v, is(tt.to), safe)
		if tt.unsafe {
			if !got.IsError() || !slices.ContainsFunc(got.Diagnostics(), func(d tenon.Diagnostic) bool {
				return d.Code == tenon.CodeConvertUnsafe
			}) {
				t.Errorf("%s, safe = %v, want convert.unsafe", tt.name, got)
			}
		} else {
			wantValue(t, tt.name+", safe", got, tt.want)
		}
	}

	// A pair the table does not list has no conversion under either policy.
	for _, tt := range []struct {
		v  tenon.Value
		to tenon.Type
	}{
		{tenon.Bool(true), num},
		{n(1), boo},
		{n(1), tenon.List(num)},
		{listNum, num},
		{listNum, tenon.Map(num)},
		{mapNum, tenon.List(num)},
		{objNum, tenon.List(num)},
		{tupleNum, tenon.Map(num)},
		{mapNum, tenon.Set(num)},
		{objNum, tenon.Tuple(num, num)},
		{tupleNum, tenon.Object(map[string]tenon.Type{"a": num})},
		{tupleNum, tenon.Tuple(num)},
	} {
		for _, p := range []tenon.Policy{safe, uns} {
			wantErrors(t, "Convert("+tt.v.String()+", "+tt.to.String()+")", tenon.Convert(tt.v, is(tt.to), p),
				wantDiag{tenon.CodeConvertNoConversion, "."})
		}
	}
}

// celsius is what a capsule type in the conversion tests encapsulates.
type celsius struct{ degrees int64 }

func TestConformance_CV011_CapsuleConversions(t *testing.T) {
	conformance.Covers(t, "CV-011")
	var temp tenon.Type
	temp = tenon.Capsule("celsius", tenon.CapsuleOps[celsius]{
		// A temperature converts safely to a number, and unsafely to a
		// string; nothing else is declared.
		ConvertTo: func(to tenon.Type) (func(*celsius) tenon.Value, bool) {
			switch to {
			case num:
				return func(v *celsius) tenon.Value { return n(v.degrees) }, true
			case str:
				return func(v *celsius) tenon.Value { return s(tenon.Convert(n(v.degrees), is(str), uns).AsString() + "C") }, false
			}
			return nil, false
		},
		// A number converts unsafely to a temperature, where it is whole.
		ConvertFrom: func(from tenon.Type) (func(tenon.Value) tenon.Value, bool) {
			if from != num {
				return nil, false
			}
			return func(v tenon.Value) tenon.Value {
				i, ok := v.AsInt64()
				if !ok {
					return tenon.ErrorVal(tenon.Diagnostic{Code: "app.fractional", Message: "not whole"})
				}
				return tenon.CapsuleVal(temp, &celsius{i})
			}, false
		},
	})
	warm := tenon.CapsuleVal(temp, &celsius{21})

	wantValue(t, "Convert(21C, number, safe)", tenon.Convert(warm, is(num), safe), n(21))
	wantValue(t, "Convert(21C, string, unsafe)", tenon.Convert(warm, is(str), uns), s("21C"))
	wantErrors(t, "Convert(21C, string, safe)", tenon.Convert(warm, is(str), safe), wantDiag{tenon.CodeConvertUnsafe, "."})
	wantErrors(t, "Convert(21C, bool)", tenon.Convert(warm, is(boo), uns), wantDiag{tenon.CodeConvertNoConversion, "."})

	back := tenon.Convert(n(7), is(temp), uns)
	if back.IsError() || back.Type() != temp || tenon.CapsuleValue[celsius](back).degrees != 7 {
		t.Errorf("Convert(7, celsius) = %v", back)
	}
	wantErrors(t, "Convert(7, celsius, safe)", tenon.Convert(n(7), is(temp), safe), wantDiag{tenon.CodeConvertUnsafe, "."})
	wantErrors(t, "Convert(7.5, celsius)", tenon.Convert(tenon.NumberFromText("7.5"), is(temp), uns), wantDiag{"app.fractional", "."})

	// Null and unknown values convert by what the type declares, without the
	// declared function, which has no value to take.
	wantValue(t, "Convert(null celsius, number)", tenon.Convert(tenon.NullVal(temp), is(num), safe), tenon.NullVal(num))
	wantValue(t, "Convert(unknown celsius, number)", tenon.Convert(tenon.Unknown(temp), is(num), safe), tenon.Unknown(num))

	// Between two capsule types, the source's conversion to the target comes
	// before the target's conversion from the source.
	var kelvin tenon.Type
	fromSource := &celsius{1}
	fromTarget := &celsius{2}
	kelvin = tenon.Capsule("kelvin", tenon.CapsuleOps[celsius]{
		ConvertFrom: func(from tenon.Type) (func(tenon.Value) tenon.Value, bool) {
			return func(tenon.Value) tenon.Value { return tenon.CapsuleVal(kelvin, fromTarget) }, true
		},
	})
	var other tenon.Type
	other = tenon.Capsule("other", tenon.CapsuleOps[celsius]{
		ConvertTo: func(to tenon.Type) (func(*celsius) tenon.Value, bool) {
			if to != kelvin {
				return nil, false
			}
			return func(*celsius) tenon.Value { return tenon.CapsuleVal(kelvin, fromSource) }, true
		},
	})
	if r := tenon.Convert(tenon.CapsuleVal(other, &celsius{0}), is(kelvin), safe); tenon.CapsuleValue[celsius](r) != fromSource {
		t.Errorf("the source's declared conversion was not the one applied: %v", r)
	}
	if r := tenon.Convert(warm, is(kelvin), safe); tenon.CapsuleValue[celsius](r) != fromTarget {
		t.Errorf("the target's declared conversion was not applied: %v", r)
	}

	// A declared conversion that returns something other than a value of the
	// type it declared is a defect in the capsule type.
	liar := tenon.Capsule("liar", tenon.CapsuleOps[celsius]{
		ConvertTo: func(tenon.Type) (func(*celsius) tenon.Value, bool) {
			return func(*celsius) tenon.Value { return tenon.Unknown(num) }, true
		},
	})
	mustPanicUsage(t, `capsule type "liar" declares from`, func() {
		tenon.Convert(tenon.CapsuleVal(liar, &celsius{}), is(num), safe)
	})
}

func TestConformance_CV012_ConversionsDoNotCompose(t *testing.T) {
	conformance.Covers(t, "CV-012")
	// A number converts to a string, and "1" converts to a bool no better than
	// a number does: there is no chain from number to bool through string.
	wantErrors(t, "Convert(1, bool)", tenon.Convert(n(1), is(boo), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	wantErrors(t, "Convert(true, number)", tenon.Convert(tenon.Bool(true), is(num), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	// A tuple becomes a list, and a list a set, but a map does not become a
	// list by way of an object.
	m := tenon.MapVal(num, map[string]tenon.Value{"a": n(1)})
	wantErrors(t, "Convert(map, list)", tenon.Convert(m, tenon.ListOf(tenon.Any()), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	// Within a container each member takes one conversion too.
	wantErrors(t, "Convert([1], list(bool))", tenon.Convert(tenon.ListVal(num, n(1)), is(tenon.List(boo)), uns),
		wantDiag{tenon.CodeConvertNoConversion, ".[0]"})
}

func TestConformance_CV020_ConvertingToExactly(t *testing.T) {
	conformance.Covers(t, "CV-020")
	wantValue(t, "Convert(true, exactly(string))", tenon.Convert(tenon.Bool(true), is(str), uns), s("true"))
	wantValue(t, "Convert(tuple, exactly(list))", tenon.Convert(tenon.TupleVal(s("a"), s("b")), is(tenon.List(str)), safe),
		tenon.ListVal(str, s("a"), s("b")))
	// The policy decides whether an unsafe conversion applies.
	wantErrors(t, "Convert(true, exactly(string), safe)", tenon.Convert(tenon.Bool(true), is(str), safe),
		wantDiag{tenon.CodeConvertUnsafe, "."})
	// The one conversion for the pair, and nothing else.
	wantErrors(t, "Convert([1, 2], exactly(tuple(number)))", tenon.Convert(tenon.ListVal(num, n(1), n(2)), is(tenon.Tuple(num)), uns),
		wantDiag{tenon.CodeConvertLengthMismatch, "."})
}

func TestConformance_CV021_ConvertingToCollections(t *testing.T) {
	conformance.Covers(t, "CV-021")
	// A list, a set or a tuple converts to ListOf, in order, or in iteration
	// order for a set.
	wantValue(t, "tuple to list_of", tenon.Convert(tenon.TupleVal(n(3), n(1)), tenon.ListOf(is(num)), safe), tenon.ListVal(num, n(3), n(1)))
	wantValue(t, "set to list_of", tenon.Convert(tenon.SetVal(num, n(3), n(1)), tenon.ListOf(tenon.Any()), safe), tenon.ListVal(num, n(1), n(3)))

	// The element type unifies what the members convert to.
	wantValue(t, "mixed tuple to list_of(any), unsafe", tenon.Convert(tenon.TupleVal(n(1), tenon.Bool(true)), tenon.ListOf(tenon.Any()), uns),
		tenon.ListVal(str, s("1"), s("true")))
	wantErrors(t, "mixed tuple to list_of(any), safe", tenon.Convert(tenon.TupleVal(n(1), tenon.Bool(true)), tenon.ListOf(tenon.Any()), safe),
		wantDiag{tenon.CodeConvertNoCommonType, "."})

	// An empty list keeps the type its element type converts to; an empty
	// tuple has none, unless the constraint names one.
	wantValue(t, "empty list to list_of(any)", tenon.Convert(tenon.ListVal(num), tenon.SetOf(tenon.Any()), uns), tenon.SetVal(num))
	wantErrors(t, "empty tuple to list_of(any)", tenon.Convert(tenon.TupleVal(), tenon.ListOf(tenon.Any()), safe),
		wantDiag{tenon.CodeConvertNoCommonType, "."})
	wantValue(t, "empty tuple to list_of(string)", tenon.Convert(tenon.TupleVal(), tenon.ListOf(is(str)), safe), tenon.ListVal(str))

	// Objects that differ in the optional attributes they have share one
	// element type, and a member that lacks one gains it, null.
	server := tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(is(str)),
		"port": tenon.Optional(is(num)),
	}, true)
	servers := tenon.TupleVal(
		obj(map[string]tenon.Value{"name": s("a")}),
		obj(map[string]tenon.Value{"name": s("b"), "port": n(80)}),
	)
	withPort := tenon.Object(map[string]tenon.Type{"name": str, "port": num})
	wantValue(t, "servers to list_of", tenon.Convert(servers, tenon.ListOf(server), safe), tenon.ListVal(withPort,
		obj(map[string]tenon.Value{"name": s("a"), "port": tenon.NullVal(num)}),
		obj(map[string]tenon.Value{"name": s("b"), "port": n(80)}),
	))
	// At any depth.
	nested := tenon.TupleVal(
		obj(map[string]tenon.Value{"inner": tenon.ListVal(tenon.Object(nil), obj(nil))}),
		obj(map[string]tenon.Value{"inner": tenon.ListVal(tenon.Object(map[string]tenon.Type{"x": num}), obj(map[string]tenon.Value{"x": n(1)}))}),
	)
	xType := tenon.Object(map[string]tenon.Type{"x": num})
	innerType := tenon.Object(map[string]tenon.Type{"inner": tenon.List(xType)})
	wantValue(t, "nested objects to list_of(any)", tenon.Convert(nested, tenon.ListOf(tenon.Any()), safe), tenon.ListVal(innerType,
		obj(map[string]tenon.Value{"inner": tenon.ListVal(xType, obj(map[string]tenon.Value{"x": tenon.NullVal(num)}))}),
		obj(map[string]tenon.Value{"inner": tenon.ListVal(xType, obj(map[string]tenon.Value{"x": n(1)}))}),
	))

	// An element type that unification finds but the constraint rejects is no
	// common type.
	choice := tenon.OneOf(tenon.ListOf(is(num)), tenon.TupleOf(is(str)))
	wantErrors(t, "no element type satisfying the constraint",
		tenon.Convert(tenon.TupleVal(tenon.TupleVal(n(1)), tenon.TupleVal(s("x"))), tenon.ListOf(choice), uns),
		wantDiag{tenon.CodeConvertNoCommonType, "."})

	// SetOf merges members by equality, and takes a list or a tuple only
	// unsafely.
	wantValue(t, "list to set_of", tenon.Convert(tenon.ListVal(num, n(2), n(1), n(2)), tenon.SetOf(is(str)), uns), tenon.SetVal(str, s("1"), s("2")))
	wantErrors(t, "list to set_of, safe", tenon.Convert(tenon.ListVal(num, n(2)), tenon.SetOf(is(num)), safe), wantDiag{tenon.CodeConvertUnsafe, "."})
	wantValue(t, "set to set_of, safe", tenon.Convert(tenon.SetVal(num, n(2)), tenon.SetOf(tenon.Any()), safe), tenon.SetVal(num, n(2)))

	// MapOf takes a map or an object.
	wantValue(t, "object to map_of", tenon.Convert(obj(map[string]tenon.Value{"a": n(1), "b": tenon.Bool(false)}), tenon.MapOf(tenon.Any()), uns),
		tenon.MapVal(str, map[string]tenon.Value{"a": s("1"), "b": s("false")}))
	wantErrors(t, "list to map_of", tenon.Convert(tenon.ListVal(num), tenon.MapOf(tenon.Any()), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
}

func TestConformance_CV022_ConvertingToTuples(t *testing.T) {
	conformance.Covers(t, "CV-022")
	pair := tenon.TupleOf(is(str), tenon.Any())
	wantValue(t, "tuple", tenon.Convert(tenon.TupleVal(n(1), n(2)), pair, uns), tenon.TupleVal(s("1"), n(2)))
	wantValue(t, "list", tenon.Convert(tenon.ListVal(num, n(1), n(2)), pair, uns), tenon.TupleVal(s("1"), n(2)))
	wantValue(t, "set", tenon.Convert(tenon.SetVal(num, n(2), n(1)), pair, uns), tenon.TupleVal(s("1"), n(2)))
	wantErrors(t, "list, safe", tenon.Convert(tenon.ListVal(str, s("1"), s("2")), pair, safe), wantDiag{tenon.CodeConvertUnsafe, "."})
	wantErrors(t, "short list", tenon.Convert(tenon.ListVal(num, n(1)), pair, uns), wantDiag{tenon.CodeConvertLengthMismatch, "."})
	wantErrors(t, "long set", tenon.Convert(tenon.SetVal(num, n(1), n(2), n(3)), pair, uns), wantDiag{tenon.CodeConvertLengthMismatch, "."})
	wantErrors(t, "short tuple", tenon.Convert(tenon.TupleVal(n(1)), pair, uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	wantErrors(t, "map", tenon.Convert(tenon.MapVal(num, nil), pair, uns), wantDiag{tenon.CodeConvertNoConversion, "."})
}

func TestConformance_CV023_ConvertingToObjects(t *testing.T) {
	conformance.Covers(t, "CV-023")
	fields := map[string]tenon.Field{"name": tenon.Required(is(str)), "port": tenon.Optional(is(num))}
	closed, open := tenon.ObjectWith(fields, true), tenon.ObjectWith(fields, false)

	// An optional attribute that is absent is added as null where its
	// constraint gives a type, and stays absent where it gives none.
	noPort := tenon.NullVal(num)
	wantValue(t, "name only", tenon.Convert(obj(map[string]tenon.Value{"name": n(1)}), closed, uns),
		obj(map[string]tenon.Value{"name": s("1"), "port": noPort}))
	loose := tenon.ObjectWith(map[string]tenon.Field{"name": tenon.Required(is(str)), "note": tenon.Optional(tenon.Any())}, true)
	wantValue(t, "optional any", tenon.Convert(obj(map[string]tenon.Value{"name": s("a")}), loose, safe), obj(map[string]tenon.Value{"name": s("a")}))
	wantValue(t, "name and port", tenon.Convert(obj(map[string]tenon.Value{"name": s("a"), "port": s("80")}), closed, uns),
		obj(map[string]tenon.Value{"name": s("a"), "port": n(80)}))

	// A required attribute that is absent fails, located at the object.
	wantErrors(t, "no name", tenon.Convert(obj(map[string]tenon.Value{"port": n(80)}), closed, uns), wantDiag{tenon.CodeConvertMissingAttribute, "."})

	// An attribute no field names is carried unchanged where the constraint is
	// open, marks and all, and fails where it is closed.
	isolated := stamp{id: "isolated", policy: tenon.Isolate}
	extra := tenon.WithMarks(tenon.Bool(true), isolated)
	wantValue(t, "open, extra", tenon.Convert(obj(map[string]tenon.Value{"name": s("a"), "debug": extra}), open, safe),
		obj(map[string]tenon.Value{"name": s("a"), "debug": extra, "port": noPort}))
	wantErrors(t, "closed, extra", tenon.Convert(obj(map[string]tenon.Value{"name": s("a"), "prot": n(80)}), closed, safe),
		wantDiag{tenon.CodeConvertUnexpectedAttribute, ".prot"})

	// A map converts only unsafely, its keys becoming the attributes.
	m := tenon.MapVal(str, map[string]tenon.Value{"name": s("a"), "port": s("80")})
	wantValue(t, "map", tenon.Convert(m, closed, uns), obj(map[string]tenon.Value{"name": s("a"), "port": n(80)}))
	wantErrors(t, "map, safe", tenon.Convert(tenon.MapVal(str, map[string]tenon.Value{"name": s("a")}), closed, safe),
		wantDiag{tenon.CodeConvertUnsafe, "."})
	// A member that fails is reported first, where it fails.
	wantErrors(t, "map with a member to convert, safe", tenon.Convert(m, closed, safe), wantDiag{tenon.CodeConvertUnsafe, `.["port"]`})
	wantErrors(t, "map, extra key", tenon.Convert(tenon.MapVal(str, map[string]tenon.Value{"name": s("a"), "x": s("1")}), closed, uns),
		wantDiag{tenon.CodeConvertUnexpectedAttribute, `.["x"]`})
	// No attribute can be named by the empty key, open or closed.
	for _, c := range []tenon.Constraint{closed, open} {
		wantErrors(t, "map, empty key", tenon.Convert(tenon.MapVal(str, map[string]tenon.Value{"name": s("a"), "": s("1")}), c, uns),
			wantDiag{tenon.CodeConvertUnexpectedAttribute, `.[""]`})
	}
	wantErrors(t, "number", tenon.Convert(n(1), open, uns), wantDiag{tenon.CodeConvertNoConversion, "."})
}

func TestConformance_CV024_ConvertingToOneOf(t *testing.T) {
	conformance.Covers(t, "CV-024")
	numOrList := tenon.OneOf(is(num), tenon.ListOf(is(str)))
	// A value whose type satisfies a member is itself.
	wantValue(t, "number", tenon.Convert(n(1), numOrList, safe), n(1))
	// Otherwise the first member a conversion exists to, in the order given.
	wantValue(t, "string, unsafe", tenon.Convert(s("2"), numOrList, uns), n(2))
	wantValue(t, "tuple, safe", tenon.Convert(tenon.TupleVal(s("a")), numOrList, safe), tenon.ListVal(str, s("a")))
	listOrNum := tenon.OneOf(tenon.ListOf(is(str)), is(num))
	wantValue(t, "string to list or number", tenon.Convert(s("2"), listOrNum, uns), n(2))
	// A failure for the value does not move on to a later member.
	strOrBool := tenon.OneOf(is(num), is(boo))
	wantErrors(t, "string failing as a number", tenon.Convert(s("true"), strOrBool, uns), wantDiag{tenon.CodeNumberInvalidSyntax, "."})
	// A member whose conversion does not exist for the type is passed over.
	missing := tenon.ObjectWith(map[string]tenon.Field{"b": tenon.Required(tenon.Any())}, true)
	wantValue(t, "object past a member it lacks attributes for", tenon.Convert(obj(map[string]tenon.Value{"a": n(1)}), tenon.OneOf(missing, tenon.MapOf(tenon.Any())), safe),
		tenon.MapVal(num, map[string]tenon.Value{"a": n(1)}))
	// Where no member has a conversion, the conversion does not exist.
	wantErrors(t, "string, safe", tenon.Convert(s("2"), numOrList, safe), wantDiag{tenon.CodeConvertUnsafe, "."})
	wantErrors(t, "bool", tenon.Convert(tenon.Bool(true), numOrList, uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	wantErrors(t, "none", tenon.Convert(n(1), tenon.OneOf(), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
}

func TestConformance_CV025_ConvertingToAny(t *testing.T) {
	conformance.Covers(t, "CV-025")
	for _, v := range []tenon.Value{n(1), tenon.NullVal(str), tenon.Unknown(tenon.List(num)), tenon.TupleVal(s("a"), n(1))} {
		for _, p := range []tenon.Policy{safe, uns} {
			wantValue(t, "Convert("+v.String()+", any)", tenon.Convert(v, tenon.Any(), p), v)
		}
	}
}

func TestConformance_CV026_OneTypeConstraintsAreExactly(t *testing.T) {
	conformance.Covers(t, "CV-026")
	// A capsule type converts to a constraint naming the one type it declares
	// a conversion to, however that constraint is written.
	var words tenon.Type
	words = tenon.Capsule("words", tenon.CapsuleOps[celsius]{
		ConvertTo: func(to tenon.Type) (func(*celsius) tenon.Value, bool) {
			if to != tenon.List(str) {
				return nil, false
			}
			return func(*celsius) tenon.Value { return tenon.ListVal(str, s("hello")) }, true
		},
	})
	hello := tenon.CapsuleVal(words, &celsius{})
	for _, c := range []tenon.Constraint{is(tenon.List(str)), tenon.ListOf(is(str)), tenon.OneOf(tenon.ListOf(is(str)))} {
		wantValue(t, "Convert(words, "+c.String()+")", tenon.Convert(hello, c, safe), tenon.ListVal(str, s("hello")))
	}

	// An unknown map converted to an object constraint that admits one type has
	// that type, where its keys would otherwise have settled it.
	single := tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(is(num))}, true)
	wantValue(t, "unknown map to one object type", tenon.Convert(tenon.Unknown(tenon.Map(str)), single, uns),
		tenon.Unknown(tenon.Object(map[string]tenon.Type{"a": num})))
	wantValue(t, "pending map to one object type", tenon.Convert(tenon.Pending(is(tenon.Map(str))), single, uns),
		tenon.Unknown(tenon.Object(map[string]tenon.Type{"a": num})))
}

func TestConformance_CV030_NullsConvertToNulls(t *testing.T) {
	conformance.Covers(t, "CV-030")
	wantValue(t, "null number to string", tenon.Convert(tenon.NullVal(num), is(str), uns), tenon.NullVal(str))
	wantValue(t, "null list to set_of", tenon.Convert(tenon.NullVal(tenon.List(num)), tenon.SetOf(is(str)), uns), tenon.NullVal(tenon.Set(str)))
	wantErrors(t, "null bool to number", tenon.Convert(tenon.NullVal(boo), is(num), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	wantErrors(t, "null number to string, safe", tenon.Convert(tenon.NullVal(num), is(str), safe), wantDiag{tenon.CodeConvertUnsafe, "."})
	// A null empty tuple has no element type to give a list either.
	wantErrors(t, "null tuple to list_of(any)", tenon.Convert(tenon.NullVal(tenon.Tuple()), tenon.ListOf(tenon.Any()), safe),
		wantDiag{tenon.CodeConvertNoCommonType, "."})

	// A null map has no keys, so it becomes the null of the object type that
	// holds the required attributes, at any depth.
	server := tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(is(str)),
		"port": tenon.Optional(is(num)),
	}, true)
	wantValue(t, "null map of maps to map_of(object_with)", tenon.Convert(tenon.NullVal(tenon.Map(tenon.Map(str))), tenon.MapOf(server), uns),
		tenon.NullVal(tenon.Map(tenon.Object(map[string]tenon.Type{"name": str, "port": num}))))
	nested := tenon.ObjectWith(map[string]tenon.Field{
		"tags": tenon.Required(tenon.ObjectWith(map[string]tenon.Field{"env": tenon.Required(is(str))}, false)),
		"note": tenon.Optional(tenon.Any()),
	}, true)
	wantValue(t, "null map of maps to nested object_with", tenon.Convert(tenon.NullVal(tenon.Map(tenon.Map(str))), nested, uns),
		tenon.NullVal(tenon.Object(map[string]tenon.Type{"tags": tenon.Object(map[string]tenon.Type{"env": str})})))
	wantValue(t, "null map of strings to object_with", tenon.Convert(tenon.NullVal(tenon.Map(str)), tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(is(str)), "port": tenon.Optional(is(num)),
	}, false), uns), tenon.NullVal(tenon.Object(map[string]tenon.Type{"name": str, "port": num})))
	wantErrors(t, "null map whose element cannot fill a required field", tenon.Convert(tenon.NullVal(tenon.Map(boo)),
		tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(is(num))}, false), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
}

// A member of a container can convert to a pending value: the keys that would
// settle its type are not in hand yet. Such a member contributes the type it
// would have with no keys at all, as the least it can have. Where even that
// does not convert, there is no type to contribute, and dropping the failure
// while keeping its zero Type was a nil dereference on data.
func TestConformance_CV031_PendingMembersWhoseLeastTypeDoesNotConvert(t *testing.T) {
	conformance.Covers(t, "CV-031", "ER-002")
	maps := tenon.Tuple(tenon.Map(num), tenon.Map(boo))
	nested := tenon.Tuple(maps, tenon.List(tenon.Map(num)))
	target := tenon.ListOf(tenon.ListOf(oneFieldOfTwoTypes))
	for _, tt := range []struct {
		name string
		v    tenon.Value
	}{
		{
			"a tuple holding an unknown tuple of maps",
			tenon.TupleVal(
				tenon.ListVal(tenon.Object(map[string]tenon.Type{"a": num}), obj(map[string]tenon.Value{"a": n(1)})),
				tenon.Unknown(maps)),
		},
		{"an unknown of the nesting", tenon.Unknown(nested)},
		{"a pending of the nesting", tenon.Pending(is(nested))},
	} {
		got := tenon.Convert(tt.v, target, uns)
		if got.IsError() {
			t.Errorf("%s: converting %v is %v, want a value", tt.name, tt.v, got)
			continue
		}
		if !got.IsPending() {
			t.Errorf("%s: converting %v is %v, want a pending value", tt.name, tt.v, got)
		}
	}
}

func TestConformance_CV031_UnknownsConvertToUnknowns(t *testing.T) {
	conformance.Covers(t, "CV-031")
	unknownNum := tenon.Unknown(num)
	// The nullness fact stays; narrowings the conversion cannot keep go.
	wantValue(t, "unknown number", tenon.Convert(unknownNum, is(str), uns), tenon.Unknown(str))
	wantValue(t, "not null", tenon.Convert(tenon.Narrow(unknownNum, tenon.NotNull()), is(str), uns), tenon.Narrow(tenon.Unknown(str), tenon.NotNull()))
	wantValue(t, "bounded", tenon.Convert(tenon.Narrow(unknownNum, tenon.NumberMin(n(1), true)), is(str), uns), tenon.Unknown(str))
	// Lengths that the conversion keeps are kept.
	bounded := tenon.Narrow(tenon.Unknown(tenon.List(num)), tenon.LengthMin(1), tenon.LengthMax(3))
	wantValue(t, "bounded list", tenon.Convert(bounded, tenon.ListOf(is(str)), uns),
		tenon.Narrow(tenon.Unknown(tenon.List(str)), tenon.LengthMin(1), tenon.LengthMax(3)))
	wantValue(t, "unknown tuple to list", tenon.Convert(tenon.Unknown(tenon.Tuple(num, boo)), tenon.ListOf(tenon.Any()), uns),
		tenon.Narrow(tenon.Unknown(tenon.List(str)), tenon.LengthMin(2), tenon.LengthMax(2)))
	wantErrors(t, "unknown bool to number", tenon.Convert(tenon.Unknown(boo), is(num), uns), wantDiag{tenon.CodeConvertNoConversion, "."})

	// A container converts member by member.
	wantValue(t, "list holding an unknown", tenon.Convert(tenon.ListVal(num, n(1), unknownNum), tenon.ListOf(is(str)), uns),
		tenon.ListVal(str, s("1"), tenon.Unknown(str)))
	// A set holding an unknown has no settled order or count.
	partial := tenon.SetVal(num, n(1), unknownNum)
	wantValue(t, "set holding an unknown to list_of", tenon.Convert(partial, tenon.ListOf(tenon.Any()), safe),
		tenon.Narrow(tenon.Unknown(tenon.List(num)), tenon.NotNull(), tenon.LengthMin(1), tenon.LengthMax(2)))
	wantValue(t, "set holding an unknown to tuple_of", tenon.Convert(partial, tenon.TupleOf(tenon.Any(), is(str)), uns),
		tenon.Narrow(tenon.Unknown(tenon.Tuple(num, str)), tenon.NotNull()))
	wantErrors(t, "set holding an unknown to a longer tuple", tenon.Convert(partial, tenon.TupleOf(tenon.Any(), tenon.Any(), tenon.Any()), uns),
		wantDiag{tenon.CodeConvertLengthMismatch, "."})

	// An unknown map converted to an object has no keys to settle its type.
	open := tenon.ObjectWith(nil, false)
	wantValue(t, "unknown map", tenon.Convert(tenon.Unknown(tenon.Map(num)), open, uns), tenon.Pending(open))
	wantValue(t, "unknown map, not null", tenon.Convert(tenon.Narrow(tenon.Unknown(tenon.Map(num)), tenon.NotNull()), open, uns),
		tenon.Narrow(tenon.Pending(open), tenon.NotNull()))
	listOfOpen := tenon.ListOf(open)
	wantValue(t, "unknown list of maps", tenon.Convert(tenon.Unknown(tenon.List(tenon.Map(num))), listOfOpen, uns), tenon.Pending(listOfOpen))
	// Nor can a container hold the pending value such a member becomes.
	wantValue(t, "list holding an unknown map", tenon.Convert(tenon.ListVal(tenon.Map(num), tenon.Unknown(tenon.Map(num))), listOfOpen, uns),
		tenon.Narrow(tenon.Pending(listOfOpen), tenon.NotNull()))
}

func TestConformance_CV032_PendingValuesConvert(t *testing.T) {
	conformance.Covers(t, "CV-032")
	pendingBool := tenon.Pending(is(boo))
	pendingNum := tenon.Pending(is(num))
	// No type the constraint admits converts.
	wantErrors(t, "pending bool to number", tenon.Convert(pendingBool, is(num), uns), wantDiag{tenon.CodeOperationWrongType, "."})
	wantErrors(t, "pending number to string, safe", tenon.Convert(pendingNum, is(str), safe), wantDiag{tenon.CodeOperationWrongType, "."})
	// Nor does any to a constraint that no type satisfies, whatever the pending
	// value's constraint admits: a pending value carrying it could never be
	// resolved.
	for _, c := range []tenon.Constraint{
		tenon.OneOf(), tenon.ListOf(tenon.OneOf()), tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.OneOf())}, false),
	} {
		for _, p := range []tenon.Value{tenon.Pending(tenon.Any()), tenon.Pending(tenon.ListOf(tenon.Any()))} {
			wantErrors(t, p.String()+" to "+c.String(), tenon.Convert(p, c, uns), wantDiag{tenon.CodeOperationWrongType, "."})
		}
	}
	// One result type: the unknown of it, carrying the nullness fact.
	wantValue(t, "pending number to string", tenon.Convert(pendingNum, is(str), uns), tenon.Unknown(str))
	wantValue(t, "pending null number", tenon.Convert(tenon.Narrow(pendingNum, tenon.Null()), is(str), uns), tenon.NullVal(str))
	anyNotNull := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull())
	wantValue(t, "pending any to string", tenon.Convert(anyNotNull, is(str), safe), tenon.Narrow(tenon.Unknown(str), tenon.NotNull()))
	wantValue(t, "null literal to string", tenon.Convert(tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null()), is(str), safe), tenon.NullVal(str))
	// Otherwise a pending value with the constraint converted to.
	listOf := tenon.ListOf(tenon.Any())
	wantValue(t, "pending any to list_of", tenon.Convert(tenon.Pending(tenon.Any()), listOf, safe), tenon.Pending(listOf))
	wantValue(t, "pending null to list_of", tenon.Convert(tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null()), listOf, safe),
		tenon.Narrow(tenon.Pending(listOf), tenon.Null()))
	wantValue(t, "pending map to object", tenon.Convert(tenon.Pending(is(tenon.Map(num))), tenon.ObjectWith(nil, false), uns),
		tenon.Pending(tenon.ObjectWith(nil, false)))
}

func TestConformance_CV033_MembersKeepTheirMarks(t *testing.T) {
	conformance.Covers(t, "CV-033")
	carried := stamp{id: "carried"}
	isolated := stamp{id: "isolated", policy: tenon.Isolate}

	// A member converted carries what its own conversion gives it.
	wantValue(t, "converted member", tenon.Convert(tenon.ListVal(num, tenon.WithMarks(n(1), carried, isolated)), tenon.ListOf(is(str)), uns),
		tenon.ListVal(str, tenon.WithMarks(s("1"), carried)))
	// A member carried across unchanged keeps every mark.
	extra := tenon.WithMarks(n(1), carried, isolated)
	wantValue(t, "carried member", tenon.Convert(obj(map[string]tenon.Value{"x": extra}), tenon.ObjectWith(nil, false), safe),
		obj(map[string]tenon.Value{"x": extra}))

	// A member placed into a set gives the set its marks at every depth,
	// Isolate ones included, since a set's members carry none.
	deep := tenon.ListVal(num, tenon.WithMarks(n(1), isolated))
	got := tenon.Convert(tenon.TupleVal(tenon.WithMarks(n(2), carried, isolated), n(3)), tenon.SetOf(is(num)), uns)
	wantValue(t, "members into a set", got, tenon.WithMarks(tenon.SetVal(num, n(2), n(3)), carried))
	got = tenon.Convert(tenon.TupleVal(deep), tenon.SetOf(tenon.Any()), uns)
	wantValue(t, "a member holding marks into a set", got, tenon.WithMarks(tenon.SetVal(tenon.List(num), tenon.ListVal(num, n(1))), isolated))

	// A failed member's error value carries its marks, and so does the error
	// value that holds its diagnostics.
	failed := tenon.Convert(tenon.ListVal(str, tenon.WithMarks(s("x"), carried)), tenon.ListOf(is(num)), uns)
	if !failed.IsError() || !tenon.HasMark(failed, carried) {
		t.Errorf("a failed marked member gave %v, want an error value carrying its mark", failed)
	}

	// What a redacting mark withholds stays withheld in a diagnostic, whether
	// the mark is on the member or on a value holding it.
	secret := stamp{id: "secret", redact: true}
	for _, v := range []tenon.Value{
		tenon.ListVal(str, tenon.WithMarks(s("hunter2"), secret)),
		tenon.WithMarks(tenon.ListVal(str, s("hunter2")), secret),
		tenon.ListVal(tenon.List(str), tenon.WithMarks(tenon.ListVal(str, s("hunter2")), secret)),
	} {
		c := tenon.ListOf(is(num))
		if inner, _ := tenon.UnmarkDeep(v); inner.Type() == tenon.List(tenon.List(str)) {
			c = tenon.ListOf(tenon.ListOf(is(num)))
		}
		r := tenon.Convert(v, c, uns)
		if !r.IsError() || strings.Contains(r.Diagnostics()[0].Message, "hunter2") || !strings.Contains(r.Diagnostics()[0].Message, `redacted("secret")`) {
			t.Errorf("Convert(%v) = %v, which does not withhold the redacted text", v, r)
		}
	}
	hidden := tenon.WithMarks(tenon.MapVal(str, map[string]tenon.Value{"hunter2": s("x")}), secret)
	r := tenon.Convert(hidden, tenon.ObjectWith(nil, true), uns)
	if !r.IsError() || strings.Contains(r.Diagnostics()[0].Message, "hunter2") {
		t.Errorf("Convert(redacted map) = %v, which shows a key", r)
	}
}

func TestConformance_CV044_TypesUnify(t *testing.T) {
	conformance.Covers(t, "CV-044")
	list := tenon.ListOf(tenon.Any())
	objA := obj(map[string]tenon.Value{"a": n(1)})
	for _, tt := range []struct {
		name  string
		elems []tenon.Value
		p     tenon.Policy
		want  tenon.Type
	}{
		{"one type", []tenon.Value{n(1), n(2)}, safe, num},
		{"primitives, unsafe", []tenon.Value{n(1), tenon.Bool(true)}, uns, str},
		{"string and number, unsafe", []tenon.Value{s("a"), n(1)}, uns, str},
		{"lists", []tenon.Value{tenon.ListVal(num, n(1)), tenon.ListVal(str)}, uns, tenon.List(str)},
		{"a set and a list", []tenon.Value{tenon.SetVal(num, n(1)), tenon.ListVal(num)}, safe, tenon.List(num)},
		{"tuples of one length", []tenon.Value{tenon.TupleVal(n(1), s("a")), tenon.TupleVal(n(2), s("b"))}, safe, tenon.Tuple(num, str)},
		{"tuples of two lengths", []tenon.Value{tenon.TupleVal(n(1)), tenon.TupleVal(n(2), n(3))}, safe, tenon.List(num)},
		{"a tuple and a list", []tenon.Value{tenon.TupleVal(n(1)), tenon.ListVal(num)}, safe, tenon.List(num)},
		{"an object and a map", []tenon.Value{objA, tenon.MapVal(num, nil)}, safe, tenon.Map(num)},
		{"objects of one shape", []tenon.Value{objA, obj(map[string]tenon.Value{"a": n(2)})}, safe, tenon.Object(map[string]tenon.Type{"a": num})},
		{"objects of two shapes", []tenon.Value{objA, obj(map[string]tenon.Value{"b": s("x")})}, safe, tenon.Object(map[string]tenon.Type{"a": num, "b": str})},
	} {
		r := tenon.Convert(tenon.TupleVal(tt.elems...), list, tt.p)
		if r.IsError() || r.Type() != tenon.List(tt.want) {
			t.Errorf("%s: %v, want a list of %v", tt.name, r, tt.want)
		}
	}
	// The objects of two shapes each gain the attribute they lack, null.
	wantValue(t, "objects of two shapes", tenon.Convert(tenon.TupleVal(objA, obj(map[string]tenon.Value{"b": s("x")})), list, safe),
		tenon.ListVal(tenon.Object(map[string]tenon.Type{"a": num, "b": str}),
			obj(map[string]tenon.Value{"a": n(1), "b": tenon.NullVal(str)}),
			obj(map[string]tenon.Value{"a": tenon.NullVal(num), "b": s("x")})))

	for _, elems := range [][]tenon.Value{
		{n(1), tenon.Bool(true)},
		{n(1), tenon.ListVal(num)},
		{tenon.ListVal(num), tenon.MapVal(num, nil)},
		{objA, obj(map[string]tenon.Value{"a": s("x")})},
		{tenon.CapsuleVal(tenon.Capsule("a", tenon.CapsuleOps[celsius]{}), &celsius{}), tenon.CapsuleVal(tenon.Capsule("a", tenon.CapsuleOps[celsius]{}), &celsius{})},
	} {
		wantErrors(t, "no common type", tenon.Convert(tenon.TupleVal(elems...), list, safe), wantDiag{tenon.CodeConvertNoCommonType, "."})
	}

	// The order of the members does not change the element type.
	pool := []tenon.Value{tenon.TupleVal(n(1)), tenon.TupleVal(n(2), s("x")), tenon.ListVal(str), tenon.SetVal(boo), tenon.TupleVal(s("y"), n(3))}
	var first tenon.Type
	for _, perm := range permutations(len(pool)) {
		elems := make([]tenon.Value, len(pool))
		for i, j := range perm {
			elems[i] = pool[j]
		}
		r := tenon.Convert(tenon.TupleVal(elems...), list, uns)
		switch {
		case r.IsError():
			t.Fatalf("members %v gave %v", elems, r)
		case first == (tenon.Type{}):
			first = r.Type()
		case r.Type() != first:
			t.Errorf("members %v gave element type %v, and another order gave %v", elems, r.Type(), first)
		}
	}
}

// permutations returns every ordering of 0 to n-1.
func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{nil}
	}
	var out [][]int
	for _, p := range permutations(n - 1) {
		for i := 0; i <= len(p); i++ {
			q := append(append(append([]int{}, p[:i]...), n-1), p[i:]...)
			out = append(out, q)
		}
	}
	return out
}

func TestConformance_CV050_DiagnosticsPerMember(t *testing.T) {
	conformance.Covers(t, "CV-050")
	// One diagnostic for each member that fails, in member order.
	wantErrors(t, "list", tenon.Convert(tenon.ListVal(str, s("x"), s("1"), s("y")), tenon.ListOf(is(num)), uns),
		wantDiag{tenon.CodeNumberInvalidSyntax, ".[0]"}, wantDiag{tenon.CodeNumberInvalidSyntax, ".[2]"})
	// In attribute name order for an object, where an absent attribute is
	// located at the object that lacks it.
	fields := tenon.ObjectWith(map[string]tenon.Field{
		"a": tenon.Required(is(num)), "c": tenon.Required(is(num)), "d": tenon.Required(tenon.Any()),
	}, true)
	wantErrors(t, "object", tenon.Convert(obj(map[string]tenon.Value{"c": s("x"), "b": n(1)}), fields, uns),
		wantDiag{tenon.CodeConvertMissingAttribute, "."},
		wantDiag{tenon.CodeConvertUnexpectedAttribute, ".b"},
		wantDiag{tenon.CodeNumberInvalidSyntax, ".c"},
		wantDiag{tenon.CodeConvertMissingAttribute, "."})
	// A conversion that fails as a whole has one diagnostic, at the empty path.
	wantErrors(t, "whole", tenon.Convert(n(1), tenon.ListOf(tenon.Any()), uns), wantDiag{tenon.CodeConvertNoConversion, "."})
	// Exact duplicates are removed.
	var twice tenon.Type
	twice = tenon.Capsule("twice", tenon.CapsuleOps[celsius]{
		ConvertTo: func(tenon.Type) (func(*celsius) tenon.Value, bool) {
			return func(*celsius) tenon.Value {
				d := tenon.Diagnostic{Code: "app.twice", Message: "said twice"}
				return tenon.ErrorVal(d, d)
			}, true
		},
	})
	wantErrors(t, "duplicates", tenon.Convert(tenon.ListVal(twice, tenon.CapsuleVal(twice, &celsius{})), tenon.ListOf(is(num)), safe),
		wantDiag{"app.twice", ".[0]"})
}

func TestConformance_CV051_InnermostFailure(t *testing.T) {
	conformance.Covers(t, "CV-051")
	// A failure deep within a structure is reported where it happens, with
	// its own code, not as a mismatch of the containers (go-cty #211).
	schema := tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(is(str)),
		"tags": tenon.Optional(tenon.ListOf(is(num))),
	}, true))
	input := tenon.TupleVal(
		obj(map[string]tenon.Value{"name": s("a"), "tags": tenon.TupleVal(n(1), s("two"))}),
		obj(map[string]tenon.Value{"name": s("b")}),
	)
	got := tenon.Convert(input, schema, uns)
	wantErrors(t, "nested", got, wantDiag{tenon.CodeNumberInvalidSyntax, ".[0].tags[1]"})
	if msg := got.Diagnostics()[0].Message; !strings.Contains(msg, `"two"`) {
		t.Errorf("the message %q does not say what failed", msg)
	}
}

// TestConformance_CV001_EveryResultSatisfiesItsTarget converts every value the
// generator holds, unknown collections and containers holding unknowns
// included, to constraints of every kind, and requires each result that is not
// an error to satisfy its target: a resolved result by its type, and a pending
// one by carrying the target as its constraint (go-cty #216). It requires the
// same answer twice, and a known or error result for a known value.
func TestConformance_CV001_EveryResultSatisfiesItsTarget(t *testing.T) {
	conformance.Covers(t, "CV-001", "CV-003")
	anyC := tenon.Any()
	targets := []tenon.Constraint{
		anyC, is(boo), is(num), is(str), is(tenon.List(str)), is(tenon.Set(num)), is(tenon.Map(num)),
		is(tenon.Tuple()), is(tenon.Tuple(str)), is(tenon.Object(nil)), is(tenon.Object(map[string]tenon.Type{"a": str})),
		is(values.Opaque),
		tenon.ListOf(anyC), tenon.ListOf(is(str)), tenon.SetOf(anyC), tenon.SetOf(is(num)), tenon.MapOf(anyC), tenon.MapOf(is(str)),
		tenon.TupleOf(), tenon.TupleOf(anyC), tenon.TupleOf(anyC, anyC),
		tenon.ObjectWith(nil, false), tenon.ObjectWith(nil, true),
		tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Optional(is(num))}, true),
		tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(anyC)}, false),
		tenon.ObjectWith(map[string]tenon.Field{"k": tenon.Required(is(str))}, true),
		tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{"k": tenon.Optional(anyC)}, false)),
		tenon.OneOf(is(num), tenon.ListOf(anyC)), tenon.OneOf(tenon.MapOf(is(boo)), tenon.ObjectWith(nil, false)), tenon.OneOf(),
		// Nested collections of an object whose one field admits either of two
		// types. A member converting to it is pending until the keys are in
		// hand, and the type it would have with none of them may not convert
		// at all, which is the shape that dropped a failure and unified a
		// zero Type.
		tenon.ListOf(tenon.ListOf(oneFieldOfTwoTypes)),
	}
	// The generator holds nothing nested deeply enough to reach a member that
	// converts to a pending value whose no-keys type does not convert, so the
	// shapes that dropped such a failure are swept here beside it.
	maps := tenon.Tuple(tenon.Map(num), tenon.Map(boo))
	nested := tenon.Tuple(maps, tenon.List(tenon.Map(num)))
	deep := []tenon.Value{
		tenon.TupleVal(
			tenon.ListVal(tenon.Object(map[string]tenon.Type{"a": num}), obj(map[string]tenon.Value{"a": n(1)})),
			tenon.Unknown(maps)),
		tenon.Unknown(nested),
		tenon.Pending(is(nested)),
		tenon.Unknown(maps),
	}
	checked := 0
	for _, v := range append(values.All(), deep...) {
		for _, c := range targets {
			for _, p := range []tenon.Policy{safe, uns} {
				r := tenon.Convert(v, c, p)
				checked++
				what := "Convert(" + v.String() + ", " + c.String() + ", " + p.String() + ")"
				if again := tenon.Convert(v, c, p); !tenon.Identical(again, r) {
					t.Errorf("%s gave %v and then %v", what, r, again)
				}
				switch {
				case r.IsError():
				case r.IsPending():
					if again := tenon.Convert(r, c, p); !tenon.Identical(again, r) {
						t.Errorf("%s = %v, which converts again to %v", what, r, again)
					}
					if got := r.Constraint(); !got.Equal(c) {
						t.Errorf("%s = %v, a pending value whose constraint is not the target", what, r)
					}
					if v.IsKnown() {
						t.Errorf("%s = %v: a known value converted to a pending one", what, r)
					}
				case !tenon.Satisfies(c, r.Type()):
					t.Errorf("%s = %v, whose type does not satisfy the target", what, r)
				case v.IsKnown() && !r.IsKnown():
					t.Errorf("%s = %v: a known value converted to one that is not known", what, r)
				default:
					// A result converts again to itself.
					if again := tenon.Convert(r, c, p); !tenon.Identical(again, r) {
						t.Errorf("%s = %v, which converts again to %v", what, r, again)
					}
				}
			}
		}
	}
	if checked < 5000 {
		t.Errorf("only %d conversions were checked", checked)
	}
}

// TestConformance_CV033_ResultsThatHoldNoMembersCarryTheirMarks checks the
// conversions whose result cannot hold the members they read: a pending value
// in place of a container, and a capsule value made from one. Each carries the
// Propagate marks of the members, and a member refitted to a shared element
// type is converted, so it keeps only its Propagate marks.
func TestConformance_CV033_ResultsThatHoldNoMembersCarryTheirMarks(t *testing.T) {
	conformance.Covers(t, "CV-033", "MK-003")
	prop := stamp{id: "prop"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	open := tenon.ObjectWith(nil, false)
	unknownMap := tenon.WithMarks(tenon.Unknown(tenon.Map(num)), prop)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		c    tenon.Constraint
	}{
		{"list", tenon.ListVal(tenon.Map(num), unknownMap), tenon.ListOf(open)},
		{"tuple", tenon.TupleVal(unknownMap), tenon.TupleOf(open)},
		{"object", obj(map[string]tenon.Value{"m": unknownMap}), tenon.ObjectWith(map[string]tenon.Field{"m": tenon.Required(open)}, true)},
	} {
		wantValue(t, tt.name, tenon.Convert(tt.v, tt.c, uns), tenon.WithMarks(tenon.Narrow(tenon.Pending(tt.c), tenon.NotNull()), prop))
	}

	var capT tenon.Type
	capT = tenon.Capsule("sum", tenon.CapsuleOps[celsius]{
		ConvertFrom: func(from tenon.Type) (func(tenon.Value) tenon.Value, bool) {
			if from != tenon.List(num) {
				return nil, false
			}
			return func(v tenon.Value) tenon.Value {
				total := int64(0)
				for _, e := range v.Elements() {
					i, _ := e.AsInt64()
					total += i
				}
				return tenon.CapsuleVal(capT, &celsius{total})
			}, true
		},
	})
	made := tenon.Convert(tenon.ListVal(num, tenon.WithMarks(n(1), prop), n(2)), is(capT), safe)
	if made.IsError() || tenon.CapsuleValue[celsius](made).degrees != 3 || !tenon.HasMark(made, prop) {
		t.Errorf("a capsule made from a list holding a marked member = %v, want 3 carrying the mark", made)
	}
	// A list holding an unknown is not given to the declared function.
	wantValue(t, "capsule from a list holding an unknown", tenon.Convert(tenon.ListVal(num, n(1), tenon.Unknown(num)), is(capT), safe),
		tenon.Narrow(tenon.Unknown(capT), tenon.NotNull()))

	got := tenon.Convert(tenon.TupleVal(tenon.TupleVal(tenon.WithMarks(n(1), iso, prop)), tenon.TupleVal(s("a"))), tenon.ListOf(tenon.Any()), uns)
	wantValue(t, "refitted member", got, tenon.ListVal(tenon.Tuple(str),
		tenon.TupleVal(tenon.WithMarks(s("1"), prop)), tenon.TupleVal(s("a"))))
}

// TestConformance_CV032_TypesHoldingMapsFailWhereEveryValueFails checks that a
// conversion whose result type keys would settle still fails where no keys
// could make it succeed, and that a OneOf target stays the pending value's
// constraint.
func TestConformance_CV032_TypesHoldingMapsFailWhereEveryValueFails(t *testing.T) {
	conformance.Covers(t, "CV-032", "CV-031", "CV-003")
	open := tenon.ObjectWith(nil, false)
	target := tenon.ListOf(tenon.OneOf(is(num), open))
	pair := tenon.Tuple(num, tenon.Map(num))
	wantErrors(t, "pending", tenon.Convert(tenon.Pending(is(pair)), target, uns), wantDiag{tenon.CodeOperationWrongType, "."})
	wantErrors(t, "unknown", tenon.Convert(tenon.Unknown(pair), target, uns), wantDiag{tenon.CodeConvertNoCommonType, "."})
	wantErrors(t, "tuple holding an unknown map", tenon.Convert(tenon.TupleVal(n(1), tenon.Unknown(tenon.Map(num))), target, uns),
		wantDiag{tenon.CodeConvertNoCommonType, "."})
	wantErrors(t, "null", tenon.Convert(tenon.NullVal(pair), target, uns), wantDiag{tenon.CodeConvertNoCommonType, "."})
	// Where some keys could succeed, the result is pending.
	maps := tenon.TupleVal(obj(map[string]tenon.Value{"a": n(1)}), tenon.Unknown(tenon.Map(num)))
	listOfOpen := tenon.ListOf(open)
	wantValue(t, "objects and an unknown map", tenon.Convert(maps, listOfOpen, uns), tenon.Narrow(tenon.Pending(listOfOpen), tenon.NotNull()))

	choice := tenon.OneOf(listOfOpen, is(num))
	wantValue(t, "one_of, list holding an unknown map", tenon.Convert(tenon.ListVal(tenon.Map(num), tenon.Unknown(tenon.Map(num))), choice, uns),
		tenon.Narrow(tenon.Pending(choice), tenon.NotNull()))
	wantValue(t, "one_of, unknown list of maps", tenon.Convert(tenon.Unknown(tenon.List(tenon.Map(num))), choice, uns), tenon.Pending(choice))
}

// TestConformance_CV026_OneTypeWrittenAnyWay checks that a constraint admitting
// one type converts as Exactly of it however it is written, diagnostics
// included.
func TestConformance_CV026_OneTypeWrittenAnyWay(t *testing.T) {
	conformance.Covers(t, "CV-026", "CV-051")
	strings2 := tenon.ListVal(str, s("1"), s("2"))
	for _, c := range []tenon.Constraint{is(tenon.List(num)), tenon.ListOf(is(num)), tenon.OneOf(tenon.ListOf(is(num))), tenon.OneOf(tenon.OneOf(), is(tenon.List(num)))} {
		wantErrors(t, "Convert(strings, "+c.String()+", safe)", tenon.Convert(strings2, c, safe),
			wantDiag{tenon.CodeConvertUnsafe, ".[0]"}, wantDiag{tenon.CodeConvertUnsafe, ".[1]"})
	}
	// A field that can never be present leaves one type.
	one := tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(is(num)), "b": tenon.Optional(tenon.OneOf())}, true)
	wantValue(t, "pending map", tenon.Convert(tenon.Pending(is(tenon.Map(num))), one, uns), tenon.Unknown(tenon.Object(map[string]tenon.Type{"a": num})))

	// An attribute of that field's name is then one the constraint does not
	// allow, as Exactly of the type says, rather than one that fails to
	// convert to the field's constraint: in an object or a map, known, null,
	// unknown or pending, and alone or in a list.
	onlyA := tenon.Object(map[string]tenon.Type{"a": num})
	az := tenon.Object(map[string]tenon.Type{"a": num, "z": num})
	bare := tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(is(num)), "z": tenon.Optional(tenon.OneOf())}, true)
	spellings := []tenon.Constraint{
		bare,
		tenon.OneOf(bare),
		tenon.OneOf(tenon.OneOf(), bare),
		tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.OneOf(is(num))), "z": tenon.Optional(tenon.ListOf(tenon.OneOf()))}, true),
	}
	objAZ := obj(map[string]tenon.Value{"a": n(1), "z": n(2)})
	mapAZ := tenon.MapVal(num, map[string]tenon.Value{"a": n(1), "z": n(2)})
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want wantDiag
	}{
		{"an object", objAZ, wantDiag{tenon.CodeConvertUnexpectedAttribute, ".z"}},
		{"a map", mapAZ, wantDiag{tenon.CodeConvertUnexpectedAttribute, `.["z"]`}},
		{"a null object", tenon.NullVal(az), wantDiag{tenon.CodeConvertUnexpectedAttribute, "."}},
		{"an unknown object", tenon.Unknown(az), wantDiag{tenon.CodeConvertUnexpectedAttribute, "."}},
		{"a pending object", tenon.Pending(is(az)), wantDiag{tenon.CodeOperationWrongType, "."}},
	} {
		want := tenon.Convert(tt.v, is(onlyA), uns)
		wantErrors(t, tt.name+" to "+is(onlyA).String(), want, tt.want)
		for _, c := range spellings {
			wantValue(t, tt.name+" to "+c.String(), tenon.Convert(tt.v, c, uns), want)
		}
	}
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want wantDiag
	}{
		{"a list of objects", tenon.ListVal(az, objAZ), wantDiag{tenon.CodeConvertUnexpectedAttribute, ".[0].z"}},
		{"a list of maps", tenon.ListVal(tenon.Map(num), mapAZ), wantDiag{tenon.CodeConvertUnexpectedAttribute, `.[0]["z"]`}},
		{"a null list", tenon.NullVal(tenon.List(az)), wantDiag{tenon.CodeConvertUnexpectedAttribute, "."}},
		{"an unknown list", tenon.Unknown(tenon.List(az)), wantDiag{tenon.CodeConvertUnexpectedAttribute, "."}},
		{"a list holding an unknown object", tenon.ListVal(az, tenon.Unknown(az)), wantDiag{tenon.CodeConvertUnexpectedAttribute, ".[0]"}},
	} {
		want := tenon.Convert(tt.v, is(tenon.List(onlyA)), uns)
		wantErrors(t, tt.name+" to "+is(tenon.List(onlyA)).String(), want, tt.want)
		for _, c := range spellings {
			wantValue(t, tt.name+" to list_of("+c.String()+")", tenon.Convert(tt.v, tenon.ListOf(c), uns), want)
		}
	}
}

// TestConformance_CV026_EverySpellingConvertsAlike holds CV-026 over
// generated constraints: one that admits exactly one type, however it is
// written, converts every value as Exactly of that type does, under either
// policy, diagnostics and all.
func TestConformance_CV026_EverySpellingConvertsAlike(t *testing.T) {
	conformance.Covers(t, "CV-026")
	capsule := tenon.Capsule("cap", tenon.CapsuleOps[celsius]{})
	ab := tenon.Object(map[string]tenon.Type{"a": num, "b": num})
	objAB := obj(map[string]tenon.Value{"a": n(1), "b": n(2)})
	pool := append(values.All(),
		objAB,
		obj(map[string]tenon.Value{"a": tenon.ListVal(num, n(1)), "b": s("x")}),
		tenon.MapVal(num, map[string]tenon.Value{"a": n(1), "b": n(2)}),
		tenon.MapVal(str, map[string]tenon.Value{"a": s("1")}),
		tenon.NullVal(ab),
		tenon.Unknown(ab),
		tenon.Pending(is(ab)),
		tenon.ListVal(ab, objAB),
		tenon.SetVal(num, n(1), n(2)),
		tenon.TupleVal(n(1), s("x")),
		tenon.TupleVal(s("1"), s("x")),
		tenon.CapsuleVal(capsule, &celsius{}),
		tenon.Unknown(capsule),
	)
	r := rand.New(rand.NewSource(20260919))
	sole, failed := 0, 0
	for range conformance.Iterations(t, 400) {
		c := randomConstraint(r, 3, capsule)
		one, ok := tenon.SoleType(c)
		if !ok || c.Kind() == tenon.ConstraintExactly {
			continue
		}
		sole++
		for _, v := range pool {
			for _, p := range []tenon.Policy{safe, uns} {
				want := tenon.Convert(v, is(one), p)
				if want.IsError() {
					failed++
				}
				if got := tenon.Convert(v, c, p); !tenon.Identical(got, want) {
					t.Errorf("Convert(%v, %v, %v) = %v, but to %v it is %v", v, c, p, got, is(one), want)
				}
			}
		}
	}
	// A run that met few such constraints, or whose conversions all failed,
	// would say little.
	if sole < 100 || failed < 1000 {
		t.Errorf("%d constraints admitted one type, with %d conversions failing: too few to say much", sole, failed)
	}
}

// TestConversionMessagesWithholdRedactedShape checks that a length or a key
// that a redacting mark withholds stays out of conversion messages.
func TestConversionMessagesWithholdRedactedShape(t *testing.T) {
	conformance.Covers(t, "MK-011")
	secret := stamp{id: "secret", redact: true}
	for _, tt := range []struct {
		v    tenon.Value
		c    tenon.Constraint
		hide string
	}{
		{tenon.WithMarks(tenon.ListVal(str, s("a"), s("b"), s("c")), secret), tenon.TupleOf(tenon.Any(), tenon.Any()), "3"},
		{tenon.WithMarks(tenon.SetVal(str, s("a"), s("b"), tenon.Unknown(str)), secret), tenon.TupleOf(tenon.Any()), "2 to"},
		{tenon.TupleVal(tenon.WithMarks(tenon.MapVal(num, map[string]tenon.Value{"hunter2": n(1)}), secret),
			obj(map[string]tenon.Value{"hunter2": tenon.ListVal(num)})), tenon.ListOf(tenon.ObjectWith(nil, false)), "hunter2"},
		{tenon.WithMarks(tenon.MapVal(num, map[string]tenon.Value{"": n(1)}), secret), tenon.ObjectWith(nil, false), `""`},
	} {
		r := tenon.Convert(tt.v, tt.c, uns)
		if !r.IsError() {
			t.Errorf("Convert(%v, %v) = %v, want an error value", tt.v, tt.c, r)
			continue
		}
		for _, d := range r.Diagnostics() {
			if strings.Contains(d.Message, tt.hide) {
				t.Errorf("Convert(%v, %v) says %q, which shows %s", tt.v, tt.c, d.Message, tt.hide)
			}
		}
	}
}

func TestConformance_CV027_TheTypeAConstraintGives(t *testing.T) {
	conformance.Covers(t, "CV-027", "CV-032")
	pendingAny := tenon.Pending(tenon.Any())
	server := tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(is(str)),
		"port": tenon.Optional(is(num)),
		"tags": tenon.Optional(tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{"k": tenon.Optional(is(str))}, true))),
		"none": tenon.Optional(tenon.OneOf()),
	}, true)
	serverType := tenon.Object(map[string]tenon.Type{
		"name": str, "port": num, "tags": tenon.List(tenon.Object(map[string]tenon.Type{"k": str})),
	})
	// Where a constraint gives a type, whatever a pending value turns out to
	// be converts to that type.
	for _, tt := range []struct {
		c    tenon.Constraint
		want tenon.Type
	}{
		{is(num), num},
		{tenon.ListOf(is(str)), tenon.List(str)},
		{tenon.SetOf(tenon.TupleOf(is(num), is(boo))), tenon.Set(tenon.Tuple(num, boo))},
		{server, serverType},
		{tenon.MapOf(server), tenon.Map(serverType)},
		{tenon.OneOf(tenon.ListOf(is(str)), is(tenon.List(str)), tenon.OneOf()), tenon.List(str)},
	} {
		wantValue(t, "Convert(pending, "+tt.c.String()+")", tenon.Convert(pendingAny, tt.c, uns), tenon.Unknown(tt.want))
	}
	// Where it gives none, the result is pending.
	for _, c := range []tenon.Constraint{
		tenon.ListOf(tenon.Any()),
		tenon.ObjectWith(map[string]tenon.Field{"port": tenon.Optional(is(num))}, false),
		tenon.ObjectWith(map[string]tenon.Field{"note": tenon.Optional(tenon.Any())}, true),
		tenon.OneOf(is(num), is(str)),
		tenon.TupleOf(is(num), tenon.Any()),
	} {
		wantValue(t, "Convert(pending, "+c.String()+")", tenon.Convert(pendingAny, c, uns), tenon.Pending(c))
	}

	// A known value converts to the type too, its absent optional attributes
	// null at every depth.
	got := tenon.Convert(obj(map[string]tenon.Value{"name": s("a"), "tags": tenon.TupleVal(obj(nil))}), server, uns)
	wantValue(t, "a server", got, obj(map[string]tenon.Value{
		"name": s("a"),
		"port": tenon.NullVal(num),
		"tags": tenon.ListVal(tenon.Object(map[string]tenon.Type{"k": str}), obj(map[string]tenon.Value{"k": tenon.NullVal(str)})),
	}))
	// An unknown map has a settled type where the constraint gives one, and a
	// pending one where its keys could still add an attribute.
	wantValue(t, "unknown map", tenon.Convert(tenon.Unknown(tenon.Map(str)), tenon.ObjectWith(map[string]tenon.Field{"port": tenon.Optional(is(num))}, true), uns),
		tenon.Unknown(tenon.Object(map[string]tenon.Type{"port": num})))
	loose := tenon.ObjectWith(map[string]tenon.Field{"note": tenon.Optional(tenon.Any())}, true)
	wantValue(t, "unknown map, optional any", tenon.Convert(tenon.Unknown(tenon.Map(str)), loose, uns), tenon.Pending(loose))
	// An empty tuple takes the element type the constraint gives.
	wantValue(t, "empty tuple", tenon.Convert(tenon.TupleVal(), tenon.ListOf(server), safe), tenon.ListVal(serverType))
}

func TestConformance_CV002_ValuesInFullShapeConvertToThemselves(t *testing.T) {
	conformance.Covers(t, "CV-002")
	c := tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{"name": tenon.Required(is(str)), "port": tenon.Optional(is(num))}, true))
	full := tenon.ListVal(tenon.Object(map[string]tenon.Type{"name": str, "port": num}),
		obj(map[string]tenon.Value{"name": s("a"), "port": n(80)}))
	if got := tenon.Convert(full, c, safe); !tenon.Identical(got, full) {
		t.Errorf("a value already in full shape converted to %v", got)
	}
	// A value whose type satisfies the constraint but lacks an attribute the
	// conversion adds is not yet in that shape, at any depth.
	short := tenon.ListVal(tenon.Object(map[string]tenon.Type{"name": str}), obj(map[string]tenon.Value{"name": s("a")}))
	if !tenon.Satisfies(c, short.Type()) {
		t.Fatalf("%v does not satisfy %v", short.Type(), c)
	}
	wantValue(t, "short", tenon.Convert(short, c, safe), tenon.ListVal(tenon.Object(map[string]tenon.Type{"name": str, "port": num}),
		obj(map[string]tenon.Value{"name": s("a"), "port": tenon.NullVal(num)})))
}

func TestConformance_CV032_PendingValuesConvertedToAny(t *testing.T) {
	conformance.Covers(t, "CV-032", "CV-025")
	carried := stamp{id: "carried"}
	isolated := stamp{id: "isolated", policy: tenon.Isolate}
	lists := tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.NotNull())
	wantValue(t, "pending list", tenon.Convert(tenon.WithMarks(lists, carried, isolated), tenon.Any(), safe), tenon.WithMarks(lists, carried))
	// A constraint naming one type still settles the result.
	wantValue(t, "pending number", tenon.Convert(tenon.Narrow(tenon.Pending(is(num)), tenon.Null()), tenon.Any(), safe), tenon.NullVal(num))
}

func TestConformance_CV033_FailuresCarryOnlyTheMarksTheyRead(t *testing.T) {
	conformance.Covers(t, "CV-033", "MK-003")
	prop := stamp{id: "prop"}
	// A map does not convert to a set, so none of its members is read.
	m := tenon.MapVal(num, map[string]tenon.Value{"k": tenon.WithMarks(n(1), prop)})
	if got := tenon.Convert(m, tenon.SetOf(tenon.Any()), uns); !got.IsError() || tenon.HasMark(got, prop) {
		t.Errorf("a map converted to a set gave %v, want an error value without the member's mark", got)
	}
	// A member read and failing gives the failure its mark.
	list := tenon.ListVal(num, tenon.WithMarks(n(1), prop))
	if got := tenon.Convert(list, tenon.SetOf(is(boo)), uns); !got.IsError() || !tenon.HasMark(got, prop) {
		t.Errorf("a failing member converted into a set gave %v, want an error value carrying its mark", got)
	}
	// A member read and placed in the set gives the set its mark.
	if got := tenon.Convert(list, tenon.SetOf(tenon.Any()), uns); got.IsError() || !tenon.HasMark(got, prop) {
		t.Errorf("a member converted into a set gave %v, want a set carrying its mark", got)
	}
}
