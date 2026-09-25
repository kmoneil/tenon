package tenon_test

import (
	"maps"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
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
	decomposed := tenon.ObjectWith(map[string]tenon.Field{"cafe\u0301": tenon.Required(tenon.Any())}, true)
	if !tenon.Satisfies(decomposed, object(map[string]tenon.Type{"caf\u00e9": num})) {
		t.Errorf("%v is not satisfied by an attribute named in another normal form", decomposed)
	}
	if f, ok := decomposed.Field("caf\u00e9"); !ok || !f.Required {
		t.Errorf("Field lookup did not normalize the name")
	}
	mustPanicUsage(t, "must not be empty", func() {
		tenon.ObjectWith(map[string]tenon.Field{"": tenon.Required(tenon.Any())}, true)
	})
	mustPanicUsage(t, "the same name after normalization", func() {
		tenon.ObjectWith(map[string]tenon.Field{"caf\u00e9": tenon.Required(tenon.Any()), "cafe\u0301": tenon.Optional(tenon.Any())}, false)
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

// TestConformance_UN023_SharedTypeDecidesEveryConstraint holds sharedType to its answers. A type
// it finds must satisfy every constraint it was given. Where it finds none, no
// example of any of them may satisfy them all, nor any example of what they
// have in common, which meet writes out part by part: those examples reach
// the types a shared type would have to be where the constraints leave
// positions open on opposite sides. It must agree with admitsNone, which asks
// the question of one constraint, and a constraint that soleType gives a type
// for may have no other example.
func TestConformance_UN023_SharedTypeDecidesEveryConstraint(t *testing.T) {
	conformance.Covers(t, "UN-023", "EQ-005")
	capsule := tenon.Capsule("cap", tenon.CapsuleOps[celsius]{})
	open := []tenon.Type{
		boo, num, str, capsule, tenon.List(num), tenon.Set(str), tenon.Map(str), tenon.Tuple(), tenon.Tuple(num, str),
		tenon.Object(nil), tenon.Object(map[string]tenon.Type{"a": num}),
	}
	shared, none := 0, 0
	check := func(cs ...tenon.Constraint) bool {
		t.Helper()
		w, ok := tenon.SharedType(cs...)
		if ok {
			shared++
			for _, c := range cs {
				if !tenon.Satisfies(c, w) {
					t.Errorf("SharedType(%v) = %v, which does not satisfy %v", cs, w, c)
				}
			}
			return true
		}
		none++
		common := cs[0]
		for _, c := range cs[1:] {
			common = meet(common, c)
		}
		for _, c := range append(slices.Clone(cs), common) {
			for _, e := range examples(c, open) {
				if satisfiesAll(cs, e) {
					t.Errorf("SharedType(%v) found no type, but %v satisfies them all", cs, e)
					return false
				}
			}
		}
		return false
	}

	// Where the parts meet in ways the generator reaches only now and then.
	objectWith := func(closed bool, fields map[string]tenon.Field) tenon.Constraint {
		return tenon.ObjectWith(fields, closed)
	}
	for _, tt := range []struct {
		name string
		cs   []tenon.Constraint
		want bool
	}{
		{"anything", []tenon.Constraint{tenon.Any()}, true},
		{"one of nothing", []tenon.Constraint{tenon.OneOf()}, false},
		{"one of nothing and anything", []tenon.Constraint{tenon.OneOf(), tenon.Any()}, false},
		{"a list and a set", []tenon.Constraint{tenon.ListOf(tenon.Any()), tenon.SetOf(tenon.Any())}, false},
		{"tuples of two lengths", []tenon.Constraint{tenon.TupleOf(tenon.Any()), tenon.TupleOf()}, false},
		{
			"positions left open on opposite sides",
			[]tenon.Constraint{tenon.TupleOf(tenon.Any(), is(num)), tenon.TupleOf(is(str), tenon.Any())},
			true,
		},
		{
			"elements of one of two types each, one type shared",
			[]tenon.Constraint{tenon.ListOf(tenon.OneOf(is(num), is(str))), tenon.ListOf(tenon.OneOf(is(boo), is(str)))},
			true,
		},
		{
			"a field one requires and a closed one leaves out",
			[]tenon.Constraint{objectWith(false, map[string]tenon.Field{"a": tenon.Required(is(num))}), objectWith(true, nil)},
			false,
		},
		{
			"a field one requires and a closed one names as optional",
			[]tenon.Constraint{objectWith(false, map[string]tenon.Field{"a": tenon.Required(tenon.Any())}), objectWith(true, map[string]tenon.Field{"a": tenon.Optional(is(num))})},
			true,
		},
		{
			"optional fields whose constraints share no type",
			[]tenon.Constraint{objectWith(true, map[string]tenon.Field{"a": tenon.Optional(is(num))}), objectWith(true, map[string]tenon.Field{"a": tenon.Optional(is(str))})},
			true,
		},
		{
			"a required field whose constraints share no type",
			[]tenon.Constraint{objectWith(false, map[string]tenon.Field{"a": tenon.Required(is(num))}), objectWith(false, map[string]tenon.Field{"a": tenon.Optional(is(str))})},
			false,
		},
		{
			"fields that two closed objects each require and the other leaves out",
			[]tenon.Constraint{objectWith(true, map[string]tenon.Field{"a": tenon.Required(tenon.Any())}), objectWith(true, map[string]tenon.Field{"b": tenon.Required(tenon.Any())})},
			false,
		},
		{
			"three that share a type two at a time and not together",
			[]tenon.Constraint{tenon.OneOf(is(num), is(boo)), tenon.OneOf(is(str), is(boo)), tenon.OneOf(is(num), is(str))},
			false,
		},
		{"a type and a constraint it satisfies", []tenon.Constraint{tenon.OneOf(tenon.ListOf(tenon.Any()), is(num)), is(tenon.List(str))}, true},
	} {
		if got := check(tt.cs...); got != tt.want {
			t.Errorf("%s: SharedType(%v) found one: %t, want %t", tt.name, tt.cs, got, tt.want)
		}
	}

	r := rand.New(rand.NewSource(20260918))
	for range conformance.Iterations(t, 800) {
		base := randomConstraint(r, 3, capsule)
		// The examples are what the search for a missed shared type rests on,
		// so they must satisfy their constraint, and one that admits a type
		// must have some.
		own := examples(base, open)
		for _, e := range own {
			if !tenon.Satisfies(base, e) {
				t.Fatalf("the example %v does not satisfy %v", e, base)
			}
		}
		w, ok := tenon.SharedType(base)
		if ok == tenon.AdmitsNone(base) {
			t.Errorf("SharedType(%v) found one: %t, and so did AdmitsNone", base, ok)
		}
		if ok && len(own) == 0 {
			t.Fatalf("%v admits %v, but the test found no example of it", base, w)
		}
		if s, sole := tenon.SoleType(base); sole {
			if w != s {
				t.Errorf("SoleType(%v) = %v, but SharedType found %v", base, s, w)
			}
			for _, e := range own {
				if e != s {
					t.Errorf("SoleType(%v) = %v, but %v satisfies it too", base, s, e)
					break
				}
			}
		}
		check(base)
		check(base, related(r, base, capsule))
		check(base, randomConstraint(r, 3, capsule))
		check(base, related(r, base, capsule), related(r, base, capsule))
	}
	// A run that found one answer nearly always would say little of the other.
	if shared < 500 || none < 500 {
		t.Errorf("the generated constraints shared a type %d times and none %d times, too few of one to say much", shared, none)
	}
}

// satisfiesAll reports whether t satisfies every one of cs.
func satisfiesAll(cs []tenon.Constraint, t tenon.Type) bool {
	for _, c := range cs {
		if !tenon.Satisfies(c, t) {
			return false
		}
	}
	return true
}

// examples returns types that satisfy c, reaching each part of it: every
// member of a OneOf, every optional field both present and absent, an
// attribute no field names where an object is open, and the open types
// wherever c leaves a type open. A long product is thinned to a spread of it.
func examples(c tenon.Constraint, open []tenon.Type) []tenon.Type {
	switch c.Kind() {
	case tenon.ConstraintAny:
		return open
	case tenon.ConstraintExactly:
		return []tenon.Type{c.Type()}
	case tenon.ConstraintOneOf:
		var out []tenon.Type
		for _, m := range c.Members() {
			out = append(out, examples(m, open)...)
		}
		return thinned(out)
	case tenon.ConstraintListOf, tenon.ConstraintSetOf, tenon.ConstraintMapOf:
		of := map[tenon.ConstraintKind]func(tenon.Type) tenon.Type{
			tenon.ConstraintListOf: tenon.List, tenon.ConstraintSetOf: tenon.Set, tenon.ConstraintMapOf: tenon.Map,
		}[c.Kind()]
		var out []tenon.Type
		for _, e := range examples(c.Element(), open) {
			out = append(out, of(e))
		}
		return out
	case tenon.ConstraintTupleOf:
		rows := [][]tenon.Type{nil}
		for _, m := range c.Members() {
			var next [][]tenon.Type
			for _, row := range rows {
				for _, e := range examples(m, open) {
					next = append(next, append(slices.Clone(row), e))
				}
			}
			rows = thinned(next)
		}
		out := make([]tenon.Type, len(rows))
		for i, row := range rows {
			out[i] = tenon.Tuple(row...)
		}
		return out
	}
	rows := []map[string]tenon.Type{{}}
	for _, name := range c.FieldNames() {
		f, _ := c.Field(name)
		var next []map[string]tenon.Type
		for _, row := range rows {
			if !f.Required {
				next = append(next, row)
			}
			for _, e := range examples(f.Constraint, open) {
				with := maps.Clone(row)
				with[name] = e
				next = append(next, with)
			}
		}
		rows = thinned(next)
	}
	var out []tenon.Type
	for _, row := range rows {
		out = append(out, tenon.Object(row))
		if !c.Closed() {
			with := maps.Clone(row)
			with["z"] = boo
			out = append(out, tenon.Object(with))
		}
	}
	return out
}

// thinned returns xs, or a spread of 48 of them where there are more.
func thinned[T any](xs []T) []T {
	const most = 48
	if len(xs) <= most {
		return xs
	}
	out := make([]T, most)
	for i := range out {
		out[i] = xs[i*len(xs)/most]
	}
	return out
}

// meet returns a constraint that admits exactly the types both c and d admit,
// written out part by part: the test's own account of what two constraints
// have in common, reached otherwise than sharedType reaches it.
func meet(c, d tenon.Constraint) tenon.Constraint {
	none := tenon.OneOf()
	switch {
	case c.Kind() == tenon.ConstraintAny:
		return d
	case d.Kind() == tenon.ConstraintAny:
		return c
	case c.Kind() == tenon.ConstraintExactly:
		if tenon.Satisfies(d, c.Type()) {
			return c
		}
		return none
	case d.Kind() == tenon.ConstraintExactly:
		return meet(d, c)
	case c.Kind() == tenon.ConstraintOneOf:
		var ms []tenon.Constraint
		for _, m := range c.Members() {
			ms = append(ms, meet(m, d))
		}
		return tenon.OneOf(ms...)
	case d.Kind() == tenon.ConstraintOneOf:
		return meet(d, c)
	case c.Kind() != d.Kind():
		return none
	}
	switch c.Kind() {
	case tenon.ConstraintListOf:
		return tenon.ListOf(meet(c.Element(), d.Element()))
	case tenon.ConstraintSetOf:
		return tenon.SetOf(meet(c.Element(), d.Element()))
	case tenon.ConstraintMapOf:
		return tenon.MapOf(meet(c.Element(), d.Element()))
	case tenon.ConstraintTupleOf:
		cm, dm := c.Members(), d.Members()
		if len(cm) != len(dm) {
			return none
		}
		ms := make([]tenon.Constraint, len(cm))
		for i := range cm {
			ms[i] = meet(cm[i], dm[i])
		}
		return tenon.TupleOf(ms...)
	}
	fields := map[string]tenon.Field{}
	for _, name := range append(c.FieldNames(), d.FieldNames()...) {
		fc, inC := c.Field(name)
		fd, inD := d.Field(name)
		switch {
		case inC && inD:
			fields[name] = tenon.Field{Constraint: meet(fc.Constraint, fd.Constraint), Required: fc.Required || fd.Required}
		case inC && d.Closed() || inD && c.Closed():
			// A closed constraint that does not name the attribute rules it
			// out, so only an optional field survives, and nothing can fill it.
			if fc.Required || fd.Required {
				return none
			}
			fields[name] = tenon.Optional(none)
		case inC:
			fields[name] = fc
		default:
			fields[name] = fd
		}
	}
	return tenon.ObjectWith(fields, c.Closed() || d.Closed())
}
