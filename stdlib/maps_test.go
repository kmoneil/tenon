package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// numbers returns the map of numbers given.
func numbers(entries map[string]tenon.Value) tenon.Value {
	return tenon.Map(tenon.NumberType(), entries)
}

func TestConformance_LC030_Keys(t *testing.T) {
	conformance.Covers(t, "LC-030")
	m := numbers(map[string]tenon.Value{"b": num("2"), "a": num("1"), "B": num("3")})
	if got := call(stdlib.KeysFunc, m); !got.Equal(list(tenon.String("B"), a, b)) {
		t.Errorf("Keys(%v) = %v, want [B, a, b]", m, got)
	}
	unknownObj := tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"b": str, "a": str}))
	if got := call(stdlib.KeysFunc, unknownObj); !got.Equal(tenon.Tuple(a, b)) {
		t.Errorf("Keys(%v) = %v, want the tuple (a, b)", unknownObj, got)
	}
	unknownMap := tenon.Narrow(tenon.Unknown(tenon.MapType(str)), tenon.LengthMin(1), tenon.LengthMax(3))
	if got := call(stdlib.KeysFunc, unknownMap); !lengthBetween(got, 1, 3) {
		t.Errorf("Keys(%v) = %v, want an unknown list of 1 to 3", unknownMap, got)
	}
	failsWith(t, "Keys of a list", call(stdlib.KeysFunc, list(a)), tenon.CodeOperationWrongType, at(0))
}

func TestConformance_LC031_Values(t *testing.T) {
	conformance.Covers(t, "LC-031")
	m := numbers(map[string]tenon.Value{"b": num("2"), "a": num("1")})
	if got := call(stdlib.ValuesFunc, m); !got.Equal(tenon.List(tenon.NumberType(), num("1"), num("2"))) {
		t.Errorf("Values(%v) = %v, want [1, 2]", m, got)
	}
	obj := tenon.Object(map[string]tenon.Value{"b": num("2"), "a": tenon.String("x")})
	if got := call(stdlib.ValuesFunc, obj); !got.Equal(tenon.Tuple(tenon.String("x"), num("2"))) {
		t.Errorf("Values(%v) = %v", obj, got)
	}
	unknownMap := tenon.Narrow(tenon.Unknown(tenon.MapType(str)), tenon.LengthMin(2), tenon.LengthMax(2))
	if got := call(stdlib.ValuesFunc, unknownMap); !lengthBetween(got, 2, 2) {
		t.Errorf("Values(%v) = %v, want an unknown list of 2", unknownMap, got)
	}
}

func TestConformance_LC032_Zipmap(t *testing.T) {
	conformance.Covers(t, "LC-032")
	nums := func(vs ...tenon.Value) tenon.Value { return tenon.List(tenon.NumberType(), vs...) }
	one, two := num("1"), num("2")
	if got := call(stdlib.ZipmapFunc, list(a, b), nums(one, two)); !got.Equal(numbers(map[string]tenon.Value{"a": one, "b": two})) {
		t.Errorf("Zipmap([a, b], [1, 2]) = %v", got)
	}
	if got := call(stdlib.ZipmapFunc, list(a, b), tenon.Tuple(one, tenon.String("x"))); !got.Equal(tenon.Object(map[string]tenon.Value{"a": one, "b": tenon.String("x")})) {
		t.Errorf("Zipmap([a, b], tuple(1, x)) = %v", got)
	}
	if got := call(stdlib.ZipmapFunc, list(a, a), nums(one, two)); !got.Equal(numbers(map[string]tenon.Value{"a": two})) {
		t.Errorf("Zipmap([a, a], [1, 2]) = %v, want the later to win", got)
	}
	failsWith(t, "Zipmap([null], [1])", call(stdlib.ZipmapFunc, tenon.List(str, tenon.Null(str)), nums(one)), tenon.CodeOperationNullOperand, at(0).Index(tenon.NumberFromInt(0)))
	failsWith(t, "Zipmap([a], [1, 2])", call(stdlib.ZipmapFunc, list(a), nums(one, two)), tenon.CodeFunctionInvalidArgument, at(1))
	failsWith(t, "Zipmap([a, unknown], [1])", call(stdlib.ZipmapFunc, list(a, tenon.Unknown(str)), nums(one)), tenon.CodeFunctionInvalidArgument, at(1))
	got := call(stdlib.ZipmapFunc, list(a, tenon.Unknown(str)), nums(one, two))
	if got.IsKnown() || got.Range().LengthMin() != 1 {
		t.Errorf("Zipmap([a, unknown], [1, 2]) = %v, want an unknown map of 1 to 2", got)
	}
	// Every key's marks reach the map.
	if got := call(stdlib.ZipmapFunc, list(tenon.WithMarks(a, bare("key"))), nums(one)); !tenon.HasMark(got, bare("key")) {
		t.Errorf("Zipmap with a marked key = %v, want the mark on the map", got)
	}
}

func TestConformance_LC033_Lookup(t *testing.T) {
	conformance.Covers(t, "LC-033")
	m := numbers(map[string]tenon.Value{"a": num("1"), "b": num("2")})
	zero := num("0")
	for _, tt := range []struct {
		key  string
		def  []tenon.Value
		want tenon.Value
	}{
		{"a", []tenon.Value{zero}, num("1")},
		{"c", []tenon.Value{zero}, zero},
		{"a", nil, num("1")},
		{"c", []tenon.Value{tenon.Null(tenon.NumberType())}, tenon.Null(tenon.NumberType())},
		{"a", []tenon.Value{tenon.Unknown(tenon.NumberType())}, num("1")},
	} {
		args := append([]tenon.Value{m, tenon.String(tt.key)}, tt.def...)
		if got := call(stdlib.LookupFunc, args...); !got.Equal(tt.want) {
			t.Errorf("Lookup(%v) = %v, want %v", args, got, tt.want)
		}
	}
	if got := tenon.Call(stdlib.LookupFunc, []tenon.Value{m, tenon.String("c"), tenon.String("7")}, tenon.Unsafe); !got.Equal(num("7")) {
		t.Errorf("Lookup(m, c, \"7\") under Unsafe = %v, want 7", got)
	}
	failsWith(t, "Lookup(m, c)", call(stdlib.LookupFunc, m, tenon.String("c")), tenon.CodeFunctionInvalidArgument, at(1))
	failsWith(t, "Lookup(m, c, \"x\")", tenon.Call(stdlib.LookupFunc, []tenon.Value{m, tenon.String("c"), tenon.String("x")}, tenon.Unsafe), tenon.CodeNumberInvalidSyntax, at(2))
	if got := call(stdlib.LookupFunc, m, tenon.String("a"), zero, zero); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionArity {
		t.Errorf("Lookup with two defaults = %v, want %s", got, tenon.CodeFunctionArity)
	}
	// A map holding a member not known yet still answers a known key.
	partly := numbers(map[string]tenon.Value{"a": num("1"), "b": tenon.Unknown(tenon.NumberType())})
	if got := call(stdlib.LookupFunc, partly, tenon.String("a"), zero); !got.Equal(num("1")) {
		t.Errorf("Lookup(%v, a, 0) = %v, want 1", partly, got)
	}
	// An object: the attribute's type, or the default's.
	obj := tenon.Object(map[string]tenon.Value{"a": num("1"), "b": tenon.String("x")})
	if got := call(stdlib.LookupFunc, obj, tenon.String("c"), tenon.Bool(true)); !got.Equal(tenon.Bool(true)) {
		t.Errorf("Lookup(%v, c, true) = %v", obj, got)
	}
	// The default's marks only where it is the answer.
	def := tenon.WithMarks(zero, bare("def"))
	if got := call(stdlib.LookupFunc, m, tenon.String("a"), def); tenon.HasMark(got, bare("def")) {
		t.Errorf("Lookup(m, a, marked default) = %v, carrying the unread default's mark", got)
	}
	if got := call(stdlib.LookupFunc, m, tenon.String("c"), def); !tenon.HasMark(got, bare("def")) {
		t.Errorf("Lookup(m, c, marked default) = %v, want the default's mark", got)
	}
}
