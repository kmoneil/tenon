package tenon_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

// mustPanicUsage runs f and fails t unless f panics with a usage error whose
// message contains want.
func mustPanicUsage(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		msg, ok := r.(string)
		if !ok || !strings.HasPrefix(msg, "tenon: usage: ") || !strings.Contains(msg, want) {
			t.Errorf("recovered %#v, want a usage error panic containing %q", r, want)
		}
	}()
	f()
}

func TestConformance_TY010_TypeKinds(t *testing.T) {
	conformance.Covers(t, "TY-010")
	str := tenon.StringType()
	tests := []struct {
		typ  tenon.Type
		kind tenon.Kind
		name string
	}{
		{tenon.BoolType(), tenon.KindBool, "Bool"},
		{tenon.NumberType(), tenon.KindNumber, "Number"},
		{str, tenon.KindString, "String"},
		{tenon.List(str), tenon.KindList, "List"},
		{tenon.Set(str), tenon.KindSet, "Set"},
		{tenon.Map(str), tenon.KindMap, "Map"},
		{tenon.Object(map[string]tenon.Type{"a": str}), tenon.KindObject, "Object"},
		{tenon.Tuple(str), tenon.KindTuple, "Tuple"},
		{sampleCapsule, tenon.KindCapsule, "Capsule"},
	}
	for _, tt := range tests {
		if got := tt.typ.Kind(); got != tt.kind || got.String() != tt.name {
			t.Errorf("%v: Kind() = %v, want %s", tt.typ, got, tt.name)
		}
	}

	// There are no other kinds.
	var names []string
	for k := range tenon.Kind(32) {
		if name := k.String(); !strings.HasPrefix(name, "Kind(") {
			names = append(names, name)
		}
	}
	want := []string{"Bool", "Number", "String", "List", "Set", "Map", "Tuple", "Object", "Capsule"}
	if !slices.Equal(names, want) {
		t.Errorf("named kinds = %q, want %q", names, want)
	}
}

func TestConformance_TY011_CollectionTypes(t *testing.T) {
	conformance.Covers(t, "TY-011")
	elem := tenon.List(tenon.NumberType())
	for _, typ := range []tenon.Type{tenon.List(elem), tenon.Set(elem), tenon.Map(elem)} {
		if !typ.IsCollection() || typ.IsStructural() {
			t.Errorf("%v: IsCollection() = %t, IsStructural() = %t; want a collection type", typ, typ.IsCollection(), typ.IsStructural())
		}
		if got := typ.ElementType(); !got.Equals(elem) {
			t.Errorf("%v: ElementType() = %v, want %v", typ, got, elem)
		}
	}
	for _, typ := range []tenon.Type{tenon.BoolType(), tenon.StringType(), tenon.Object(nil), tenon.Tuple(elem)} {
		if typ.IsCollection() {
			t.Errorf("%v: IsCollection() = true", typ)
		}
		mustPanicUsage(t, "not a collection kind", func() { typ.ElementType() })
	}
}

func TestConformance_TY012_StructuralTypes(t *testing.T) {
	conformance.Covers(t, "TY-012")
	str, num, tags := tenon.StringType(), tenon.NumberType(), tenon.Set(tenon.StringType())
	obj := tenon.Object(map[string]tenon.Type{"name": str, "count": num, "tags": tags})
	tup := tenon.Tuple(str, num, tags)
	for _, typ := range []tenon.Type{obj, tup} {
		if !typ.IsStructural() || typ.IsCollection() {
			t.Errorf("%v: IsStructural() = %t, IsCollection() = %t; want a structural type", typ, typ.IsStructural(), typ.IsCollection())
		}
	}

	// Members have differing types.
	for name, want := range map[string]tenon.Type{"name": str, "count": num, "tags": tags} {
		if got := obj.AttributeType(name); !got.Equals(want) {
			t.Errorf("AttributeType(%q) = %v, want %v", name, got, want)
		}
	}
	if got, want := obj.AttributeNames(), []string{"count", "name", "tags"}; !slices.Equal(got, want) {
		t.Errorf("AttributeNames() = %q, want %q", got, want)
	}
	if got := tup.TupleLength(); got != 3 {
		t.Errorf("TupleLength() = %d, want 3", got)
	}
	for i, want := range []tenon.Type{str, num, tags} {
		if got := tup.TupleElementType(i); !got.Equals(want) {
			t.Errorf("TupleElementType(%d) = %v, want %v", i, got, want)
		}
	}

	mustPanicUsage(t, "whose kind is Tuple, not Object", func() { tup.AttributeNames() })
	mustPanicUsage(t, "whose kind is Object, not Tuple", func() { obj.TupleLength() })
	mustPanicUsage(t, "whose kind is List, not Object", func() { tenon.List(str).HasAttribute("name") })
	mustPanicUsage(t, `has no attribute "missing"`, func() { obj.AttributeType("missing") })
	mustPanicUsage(t, "which has 3 elements", func() { tup.TupleElementType(3) })
}

func TestConformance_TY013_AttributeNames(t *testing.T) {
	conformance.Covers(t, "TY-013")
	num := tenon.NumberType()
	composed, decomposed := "café", "café"

	// A name that is not in NFC and its NFC form construct equal types, and
	// the name is kept in its normalized form.
	fromDecomposed := tenon.Object(map[string]tenon.Type{decomposed: num})
	fromComposed := tenon.Object(map[string]tenon.Type{composed: num})
	if !fromDecomposed.Equals(fromComposed) {
		t.Errorf("%v and %v are not equal", fromDecomposed, fromComposed)
	}
	if got, want := fromDecomposed.AttributeNames(), []string{composed}; !slices.Equal(got, want) {
		t.Errorf("AttributeNames() = %+q, want %+q", got, want)
	}

	// Lookups normalize the name they are given.
	if !fromComposed.HasAttribute(decomposed) || !fromComposed.AttributeType(decomposed).Equals(num) {
		t.Errorf("%v: looking up %+q failed", fromComposed, decomposed)
	}
	if fromComposed.HasAttribute("cafe") || fromComposed.HasAttribute("\xff") {
		t.Errorf("%v: found an attribute that it does not have", fromComposed)
	}

	// Names are non-empty strings of Unicode scalar values, and one object
	// type cannot have two attributes with the same name.
	mustPanicUsage(t, "must not be empty", func() { tenon.Object(map[string]tenon.Type{"": num}) })
	mustPanicUsage(t, "not valid UTF-8", func() { tenon.Object(map[string]tenon.Type{"a\xff": num}) })
	mustPanicUsage(t, "not valid UTF-8", func() { tenon.Object(map[string]tenon.Type{"\xed\xa0\x80": num}) }) // a surrogate
	mustPanicUsage(t, "the same name after normalization", func() {
		tenon.Object(map[string]tenon.Type{composed: num, decomposed: tenon.StringType()})
	})
}

func TestConformance_TY022_TypesImmutable(t *testing.T) {
	conformance.Covers(t, "TY-022")
	str, num := tenon.StringType(), tenon.NumberType()

	attrs := map[string]tenon.Type{"a": str}
	obj := tenon.Object(attrs)
	attrs["a"], attrs["b"] = num, num
	if want := tenon.Object(map[string]tenon.Type{"a": str}); !obj.Equals(want) {
		t.Errorf("changing the map passed to Object changed the type to %v", obj)
	}
	names := obj.AttributeNames()
	names[0] = "z"
	if !obj.HasAttribute("a") || obj.HasAttribute("z") {
		t.Errorf("changing the slice from AttributeNames changed the type to %v", obj)
	}

	elems := []tenon.Type{str, num}
	tup := tenon.Tuple(elems...)
	elems[0] = num
	if !tup.TupleElementType(0).Equals(str) {
		t.Errorf("changing the slice passed to Tuple changed the type to %v", tup)
	}
	got := tup.TupleElementTypes()
	got[1] = str
	if !tup.TupleElementType(1).Equals(num) {
		t.Errorf("changing the slice from TupleElementTypes changed the type to %v", tup)
	}

	// Type has no exported fields through which it could be changed.
	for f := range reflect.TypeFor[tenon.Type]().Fields() {
		if f.IsExported() {
			t.Errorf("tenon.Type has an exported field %s", f.Name)
		}
	}
}

func TestTypeEquals(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	object := func(attrs map[string]tenon.Type) tenon.Type { return tenon.Object(attrs) }
	equal := [][2]tenon.Type{
		{str, tenon.StringType()},
		{tenon.List(tenon.Map(num)), tenon.List(tenon.Map(num))},
		{object(map[string]tenon.Type{"a": str, "b": num}), object(map[string]tenon.Type{"b": num, "a": str})},
		{tenon.Tuple(str, num), tenon.Tuple(str, num)},
		{tenon.Tuple(), tenon.Tuple()},
		{object(nil), object(map[string]tenon.Type{})},
	}
	unequal := [][2]tenon.Type{
		{str, num},
		{tenon.List(str), tenon.Set(str)},
		{tenon.List(str), tenon.List(num)},
		{object(map[string]tenon.Type{"a": str}), object(map[string]tenon.Type{"a": num})},
		{object(map[string]tenon.Type{"a": str}), object(map[string]tenon.Type{"a": str, "b": str})},
		{tenon.Tuple(str, num), tenon.Tuple(num, str)},
		{tenon.Tuple(str), tenon.Tuple(str, str)},
		{tenon.Tuple(), object(nil)},
	}
	for _, p := range equal {
		if !p[0].Equals(p[1]) || !p[1].Equals(p[0]) || p[0] != p[1] {
			t.Errorf("%v and %v are not equal", p[0], p[1])
		}
	}
	for _, p := range unequal {
		if p[0].Equals(p[1]) || p[1].Equals(p[0]) || p[0] == p[1] {
			t.Errorf("%v and %v are equal", p[0], p[1])
		}
	}
}

func TestTypeString(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	tests := []struct {
		typ  tenon.Type
		want string
	}{
		{tenon.BoolType(), "bool"},
		{tenon.Map(tenon.Set(num)), "map(set(number))"},
		{tenon.Tuple(), "tuple([])"},
		{tenon.Object(nil), "object({})"},
		{
			tenon.Object(map[string]tenon.Type{"b": tenon.Tuple(tenon.BoolType(), num), "a": tenon.List(str)}),
			`object({"a": list(string), "b": tuple([bool, number])})`,
		},
		{tenon.Type{}, "<zero Type>"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("String() = %s, want %s", got, tt.want)
		}
	}
}

func TestZeroType(t *testing.T) {
	var zero tenon.Type
	str := tenon.StringType()
	mustPanicUsage(t, "zero Type", func() { zero.Kind() })
	mustPanicUsage(t, "zero Type", func() { zero.Equals(str) })
	mustPanicUsage(t, "zero Type", func() { str.Equals(zero) })
	mustPanicUsage(t, "zero Type", func() { tenon.List(zero) })
	mustPanicUsage(t, "zero Type", func() { tenon.Tuple(str, zero) })
	mustPanicUsage(t, "zero Type", func() { tenon.Object(map[string]tenon.Type{"a": zero}) })
}
