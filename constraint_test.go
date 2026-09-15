package tenon_test

import (
	"slices"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

// sampleTypes returns distinct types of every kind, nested ones included.
func sampleTypes() []tenon.Type {
	str, num, boolean := tenon.StringType(), tenon.NumberType(), tenon.BoolType()
	return []tenon.Type{
		boolean, num, str,
		tenon.List(str), tenon.List(num), tenon.List(tenon.List(str)),
		tenon.Set(str), tenon.Set(num),
		tenon.Map(str), tenon.Map(tenon.Set(num)),
		tenon.Tuple(), tenon.Tuple(str), tenon.Tuple(str, num), tenon.Tuple(num, str),
		tenon.Object(nil),
		tenon.Object(map[string]tenon.Type{"name": str}),
		tenon.Object(map[string]tenon.Type{"name": num}),
		tenon.Object(map[string]tenon.Type{"name": str, "tags": tenon.List(str)}),
		tenon.Object(map[string]tenon.Type{"name": str, "extra": boolean}),
		tenon.Object(map[string]tenon.Type{"tags": tenon.List(str)}),
		sampleCapsule, tenon.List(sampleCapsule),
	}
}

// sampleConstraints returns constraints of every kind, nested ones included.
func sampleConstraints() []tenon.Constraint {
	str, num := tenon.StringType(), tenon.NumberType()
	cs := []tenon.Constraint{tenon.Any(), tenon.OneOf()}
	for _, typ := range sampleTypes() {
		cs = append(cs, tenon.Exactly(typ))
	}
	return append(cs,
		tenon.ListOf(tenon.Any()),
		tenon.ListOf(tenon.Exactly(str)),
		tenon.SetOf(tenon.Any()),
		tenon.MapOf(tenon.SetOf(tenon.Exactly(num))),
		tenon.TupleOf(),
		tenon.TupleOf(tenon.Any(), tenon.Exactly(num)),
		tenon.ObjectWith(nil, true),
		tenon.ObjectWith(nil, false),
		tenon.ObjectWith(map[string]tenon.Field{
			"name": tenon.Required(tenon.Exactly(str)),
			"tags": tenon.Optional(tenon.ListOf(tenon.Any())),
		}, true),
		tenon.ObjectWith(map[string]tenon.Field{"name": tenon.Required(tenon.Exactly(str))}, false),
		tenon.OneOf(tenon.Exactly(str), tenon.ListOf(tenon.Any())),
		tenon.OneOf(tenon.OneOf(), tenon.TupleOf()),
	)
}

func TestConformance_TY030_ConstraintKinds(t *testing.T) {
	conformance.Covers(t, "TY-030")
	str := tenon.StringType()
	exactly := tenon.Exactly(str)
	fields := map[string]tenon.Field{"name": tenon.Required(exactly), "tags": tenon.Optional(tenon.Any())}
	object := tenon.ObjectWith(fields, true)
	elements := []tenon.Constraint{tenon.ListOf(exactly), tenon.SetOf(exactly), tenon.MapOf(exactly)}
	memberLists := []tenon.Constraint{tenon.TupleOf(exactly), tenon.OneOf(exactly)}

	all := []tenon.Constraint{exactly, tenon.Any(), elements[0], elements[1], elements[2], object, memberLists[0], memberLists[1]}
	want := []string{"Exactly", "Any", "ListOf", "SetOf", "MapOf", "ObjectWith", "TupleOf", "OneOf"}
	for i, c := range all {
		if got := c.Kind().String(); got != want[i] {
			t.Errorf("%v: Kind() = %s, want %s", c, got, want[i])
		}
	}
	var names []string
	for k := range tenon.ConstraintKind(32) {
		if name := k.String(); !strings.HasPrefix(name, "ConstraintKind(") {
			names = append(names, name)
		}
	}
	if !slices.Equal(names, want) {
		t.Errorf("named constraint kinds = %q, want %q", names, want)
	}

	// Each constraint exposes what it was built from.
	if got := exactly.Type(); got != str {
		t.Errorf("Type() = %v, want %v", got, str)
	}
	for _, c := range elements {
		if got := c.Element(); got.Type() != str {
			t.Errorf("%v: Element() = %v", c, got)
		}
	}
	if got := object.FieldNames(); !slices.Equal(got, []string{"name", "tags"}) {
		t.Errorf("FieldNames() = %q", got)
	}
	if f, ok := object.Field("name"); !ok || !f.Required || f.Constraint.Type() != str {
		t.Errorf("Field(%q) = %v, %t", "name", f, ok)
	}
	if f, ok := object.Field("tags"); !ok || f.Required || f.Constraint.Kind() != tenon.ConstraintAny {
		t.Errorf("Field(%q) = %v, %t", "tags", f, ok)
	}
	if _, ok := object.Field("missing"); ok {
		t.Errorf("Field(%q) found a field", "missing")
	}
	if !object.Closed() || tenon.ObjectWith(fields, false).Closed() {
		t.Error("Closed() does not report how the constraint was built")
	}
	for _, c := range memberLists {
		if got := c.Members(); len(got) != 1 || got[0].Type() != str {
			t.Errorf("%v: Members() = %v", c, got)
		}
	}

	mustPanicUsage(t, "whose kind is Any", func() { tenon.Any().Type() })
	mustPanicUsage(t, "whose kind is Exactly", func() { exactly.Members() })
	mustPanicUsage(t, "whose kind is ListOf", func() { elements[0].Closed() })
}

func TestConformance_TY031_Any(t *testing.T) {
	conformance.Covers(t, "TY-031")
	for _, typ := range sampleTypes() {
		if !tenon.Satisfies(tenon.Any(), typ) {
			t.Errorf("%v does not satisfy Any", typ)
		}
	}
}

func TestConformance_TY032_ObjectWith(t *testing.T) {
	conformance.Covers(t, "TY-032")
	str, num := tenon.StringType(), tenon.NumberType()
	object := func(attrs map[string]tenon.Type) tenon.Type { return tenon.Object(attrs) }
	fields := map[string]tenon.Field{
		"name": tenon.Required(tenon.Exactly(str)),
		"tags": tenon.Optional(tenon.ListOf(tenon.Any())),
	}
	closed, open := tenon.ObjectWith(fields, true), tenon.ObjectWith(fields, false)
	tests := []struct {
		name         string
		typ          tenon.Type
		closed, open bool
	}{
		{"the required attribute", object(map[string]tenon.Type{"name": str}), true, true},
		{"required and optional attributes", object(map[string]tenon.Type{"name": str, "tags": tenon.List(num)}), true, true},
		{"an extra attribute", object(map[string]tenon.Type{"name": str, "extra": num}), false, true},
		{"optional and extra attributes", object(map[string]tenon.Type{"name": str, "tags": tenon.List(str), "zzz": num}), false, true},
		{"no required attribute", object(map[string]tenon.Type{"tags": tenon.List(str)}), false, false},
		{"no attributes", object(nil), false, false},
		{"a required attribute of the wrong type", object(map[string]tenon.Type{"name": num}), false, false},
		{"an optional attribute of the wrong type", object(map[string]tenon.Type{"name": str, "tags": tenon.Set(str)}), false, false},
		{"not an object type", tenon.Map(str), false, false},
	}
	for _, tt := range tests {
		if got := tenon.Satisfies(closed, tt.typ); got != tt.closed {
			t.Errorf("%s: Satisfies(%v, %v) = %t", tt.name, closed, tt.typ, got)
		}
		if got := tenon.Satisfies(open, tt.typ); got != tt.open {
			t.Errorf("%s: Satisfies(%v, %v) = %t", tt.name, open, tt.typ, got)
		}
	}

	// Without fields, a closed constraint accepts only the empty object type,
	// and an open one accepts every object type.
	some := object(map[string]tenon.Type{"a": str})
	if !tenon.Satisfies(tenon.ObjectWith(nil, true), object(nil)) || tenon.Satisfies(tenon.ObjectWith(nil, true), some) {
		t.Error("a closed constraint without fields is not satisfied by exactly the empty object type")
	}
	if !tenon.Satisfies(tenon.ObjectWith(nil, false), some) || tenon.Satisfies(tenon.ObjectWith(nil, false), tenon.Tuple()) {
		t.Error("an open constraint without fields is not satisfied by exactly the object types")
	}

	// Field names are normalized as attribute names are.
	decomposed := tenon.ObjectWith(map[string]tenon.Field{"café": tenon.Required(tenon.Any())}, true)
	if !tenon.Satisfies(decomposed, object(map[string]tenon.Type{"café": num})) {
		t.Errorf("%v is not satisfied by an attribute named in another normal form", decomposed)
	}
	if f, ok := decomposed.Field("café"); !ok || !f.Required {
		t.Errorf("Field lookup did not normalize the name")
	}
	mustPanicUsage(t, "must not be empty", func() {
		tenon.ObjectWith(map[string]tenon.Field{"": tenon.Required(tenon.Any())}, true)
	})
	mustPanicUsage(t, "the same name after normalization", func() {
		tenon.ObjectWith(map[string]tenon.Field{"café": tenon.Required(tenon.Any()), "café": tenon.Optional(tenon.Any())}, false)
	})
}

func TestConformance_TY033_OneOf(t *testing.T) {
	conformance.Covers(t, "TY-033")
	str := tenon.StringType()
	either := tenon.OneOf(tenon.Exactly(str), tenon.ListOf(tenon.Any()))
	for _, typ := range sampleTypes() {
		want := typ == str || typ.Kind() == tenon.KindList
		if got := tenon.Satisfies(either, typ); got != want {
			t.Errorf("Satisfies(%v, %v) = %t, want %t", either, typ, got, want)
		}
		for _, none := range []tenon.Constraint{tenon.OneOf(), tenon.OneOf(tenon.OneOf())} {
			if tenon.Satisfies(none, typ) {
				t.Errorf("%v satisfies %v", typ, none)
			}
		}
	}
}

func TestConformance_TY034_SatisfiesIsTotal(t *testing.T) {
	conformance.Covers(t, "TY-034")
	// Every pair has an answer, and the same answer every time.
	for _, c := range sampleConstraints() {
		for _, typ := range sampleTypes() {
			if first, again := tenon.Satisfies(c, typ), tenon.Satisfies(c, typ); first != again {
				t.Errorf("Satisfies(%v, %v) answered %t, then %t", c, typ, first, again)
			}
		}
	}

	// Satisfaction is decided however deeply the inputs are nested.
	deepType, deepConstraint := tenon.StringType(), tenon.Exactly(tenon.StringType())
	for range 5000 {
		deepType, deepConstraint = tenon.List(deepType), tenon.ListOf(deepConstraint)
	}
	if !tenon.Satisfies(deepConstraint, deepType) || tenon.Satisfies(deepConstraint, tenon.List(deepType)) {
		t.Error("satisfaction of deeply nested inputs was decided wrongly")
	}
}

func TestConformance_TY035_Exactly(t *testing.T) {
	conformance.Covers(t, "TY-035")
	types := sampleTypes()
	for _, want := range types {
		exactly := tenon.Exactly(want)
		for _, typ := range types {
			if got := tenon.Satisfies(exactly, typ); got != (typ == want) {
				t.Errorf("Satisfies(%v, %v) = %t", exactly, typ, got)
			}
		}
	}
}

func TestConstraintString(t *testing.T) {
	str := tenon.StringType()
	tests := []struct {
		c    tenon.Constraint
		want string
	}{
		{tenon.Any(), "any"},
		{tenon.Exactly(tenon.List(str)), "exactly(list(string))"},
		{tenon.MapOf(tenon.SetOf(tenon.ListOf(tenon.Any()))), "map_of(set_of(list_of(any)))"},
		{tenon.TupleOf(), "tuple_of([])"},
		{tenon.OneOf(tenon.Any(), tenon.TupleOf(tenon.Any())), "one_of([any, tuple_of([any])])"},
		{
			tenon.ObjectWith(map[string]tenon.Field{"tags": tenon.Optional(tenon.Any()), "name": tenon.Required(tenon.Exactly(str))}, true),
			`object_with({"name": exactly(string), "tags"?: any}, closed)`,
		},
		{tenon.ObjectWith(nil, false), "object_with({}, open)"},
		{tenon.Constraint{}, "<zero Constraint>"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("String() = %s, want %s", got, tt.want)
		}
	}
}

func TestConstraintsImmutable(t *testing.T) {
	fields := map[string]tenon.Field{"a": tenon.Required(tenon.Any())}
	object := tenon.ObjectWith(fields, true)
	fields["a"], fields["b"] = tenon.Optional(tenon.Any()), tenon.Required(tenon.Any())
	if f, ok := object.Field("a"); !ok || !f.Required || len(object.FieldNames()) != 1 {
		t.Errorf("changing the map passed to ObjectWith changed the constraint to %v", object)
	}
	names := object.FieldNames()
	names[0] = "z"
	if _, ok := object.Field("a"); !ok {
		t.Errorf("changing the slice from FieldNames changed the constraint to %v", object)
	}

	members := []tenon.Constraint{tenon.Exactly(tenon.StringType()), tenon.Any()}
	tuple := tenon.TupleOf(members...)
	members[0] = tenon.Any()
	if tuple.Members()[0].Kind() != tenon.ConstraintExactly {
		t.Errorf("changing the slice passed to TupleOf changed the constraint to %v", tuple)
	}
	got := tuple.Members()
	got[1] = tenon.Exactly(tenon.StringType())
	if tuple.Members()[1].Kind() != tenon.ConstraintAny {
		t.Errorf("changing the slice from Members changed the constraint to %v", tuple)
	}
}

func TestZeroConstraint(t *testing.T) {
	var zero tenon.Constraint
	mustPanicUsage(t, "zero Constraint", func() { zero.Kind() })
	mustPanicUsage(t, "zero Constraint", func() { tenon.Satisfies(zero, tenon.StringType()) })
	mustPanicUsage(t, "zero Type", func() { tenon.Satisfies(tenon.Any(), tenon.Type{}) })
	mustPanicUsage(t, "zero Type", func() { tenon.Exactly(tenon.Type{}) })
	mustPanicUsage(t, "zero Constraint", func() { tenon.ListOf(zero) })
	mustPanicUsage(t, "zero Constraint", func() { tenon.OneOf(tenon.Any(), zero) })
	mustPanicUsage(t, "zero Constraint", func() { tenon.ObjectWith(map[string]tenon.Field{"a": {}}, true) })
}
