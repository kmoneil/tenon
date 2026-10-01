package ctytenon_test

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// TestConstraints holds each kind of constraint to crossing as itself, both
// ways.
func TestConstraints(t *testing.T) {
	var b ctytenon.Bridge
	str, num := tenon.StringType(), tenon.NumberType()
	is, req, opt := tenon.Exactly, tenon.Required, tenon.Optional
	for _, c := range []struct {
		cty   cty.Type
		tenon tenon.Constraint
	}{
		{cty.DynamicPseudoType, tenon.Any()},
		{cty.String, is(str)},
		{cty.List(cty.DynamicPseudoType), tenon.ListOf(tenon.Any())},
		{cty.Set(cty.DynamicPseudoType), tenon.SetOf(tenon.Any())},
		{cty.Map(cty.List(cty.DynamicPseudoType)), tenon.MapOf(tenon.ListOf(tenon.Any()))},
		{cty.List(cty.String), is(tenon.ListType(str))},
		{cty.Map(cty.Set(cty.Number)), is(tenon.MapType(tenon.SetType(num)))},
		{cty.Tuple([]cty.Type{cty.String, cty.DynamicPseudoType}), tenon.TupleOf(is(str), tenon.Any())},
		{cty.Tuple([]cty.Type{cty.String, cty.Number}), is(tenon.TupleType(str, num))},
		{cty.EmptyTuple, is(tenon.TupleType())},
		// An object type is an open ObjectWith, whatever it holds.
		{cty.EmptyObject, tenon.ObjectWith(nil, false)},
		{
			cty.Object(map[string]cty.Type{"name": cty.String, "tags": cty.List(cty.DynamicPseudoType)}),
			tenon.ObjectWith(map[string]tenon.Field{"name": req(is(str)), "tags": req(tenon.ListOf(tenon.Any()))}, false),
		},
		{
			cty.ObjectWithOptionalAttrs(map[string]cty.Type{"name": cty.String, "port": cty.Number, "a name": cty.DynamicPseudoType}, []string{"port", "a name"}),
			tenon.ObjectWith(map[string]tenon.Field{"name": req(is(str)), "port": opt(is(num)), "a name": opt(tenon.Any())}, false),
		},
		{
			cty.List(cty.Object(map[string]cty.Type{"a": cty.String})),
			tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{"a": req(is(str))}, false)),
		},
		{
			cty.Tuple([]cty.Type{cty.Number, cty.EmptyObject}),
			tenon.TupleOf(is(num), tenon.ObjectWith(nil, false)),
		},
	} {
		got, err := b.ConstraintFromCty(c.cty)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("ConstraintFromCty(%#v) = %v, %v; want %v", c.cty, got, err, c.tenon)
		}
		back, err := b.ConstraintToCty(c.tenon)
		if err != nil || !back.Equals(c.cty) {
			t.Errorf("ConstraintToCty(%v) = %#v, %v; want %#v", c.tenon, back, err, c.cty)
		}
	}
}

// TestConstraintsThatWiden holds the tenon constraints that cross to cty as
// one accepting more, or as one written otherwise, to coming back as that.
func TestConstraintsThatWiden(t *testing.T) {
	var b ctytenon.Bridge
	str := tenon.StringType()
	is, req := tenon.Exactly, tenon.Required
	open := tenon.ObjectWith(map[string]tenon.Field{"a": req(is(str))}, false)
	for _, c := range []struct {
		tenon tenon.Constraint
		cty   cty.Type
		back  tenon.Constraint
	}{
		{tenon.ObjectWith(map[string]tenon.Field{"a": req(is(str))}, true), cty.Object(map[string]cty.Type{"a": cty.String}), open},
		{is(tenon.ObjectType(map[string]tenon.Type{"a": str})), cty.Object(map[string]cty.Type{"a": cty.String}), open},
		{tenon.ObjectWith(nil, true), cty.EmptyObject, tenon.ObjectWith(nil, false)},
		{tenon.ListOf(is(tenon.ObjectType(nil))), cty.List(cty.EmptyObject), tenon.ListOf(tenon.ObjectWith(nil, false))},
		// Written as Unify writes them: each is satisfied by one type.
		{tenon.ListOf(is(str)), cty.List(cty.String), is(tenon.ListType(str))},
		{tenon.TupleOf(), cty.EmptyTuple, is(tenon.TupleType())},
	} {
		got, err := b.ConstraintToCty(c.tenon)
		if err != nil || !got.Equals(c.cty) {
			t.Errorf("ConstraintToCty(%v) = %#v, %v; want %#v", c.tenon, got, err, c.cty)
			continue
		}
		if back, err := b.ConstraintFromCty(got); err != nil || !back.Equal(c.back) {
			t.Errorf("%v crossed to %#v and back as %v, %v; want %v", c.tenon, got, back, err, c.back)
		}
	}
}

// TestConstraintsRoundTripFromTenon carries the random constraints Unify's
// and Convert's tests run over to cty and back. One holding a OneOf, or a
// capsule type, which the zero Bridge pairs with none, does not cross. Any
// other comes back written as Unify writes
// it, its object constraints opened, and accepts every type it accepted; and
// cty can convert to its cty constraint from every such type.
func TestConstraintsRoundTripFromTenon(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261001))
	capsule := values.Opaque.Type()
	crossed, refused := 0, 0
	for range conformance.Iterations(t, 2000) {
		c := values.RandomConstraint(r, 3, capsule)
		ct, err := b.ConstraintToCty(c)
		if holdsOneOfOrCapsule(c) {
			if err == nil || !strings.Contains(err.Error(), "which no cty type constraint says") && !strings.Contains(err.Error(), "pairs with no cty type") {
				t.Fatalf("ConstraintToCty(%v) = %#v, %v; want it refused", c, ct, err)
			}
			refused++
			continue
		}
		if err != nil {
			t.Fatalf("ConstraintToCty(%v): %v", c, err)
		}
		back, err := b.ConstraintFromCty(ct)
		if err != nil {
			t.Fatalf("%v crossed to %#v and failed back: %v", c, ct, err)
		}
		if want := written(t, opened(c)); !back.Equal(want) {
			t.Fatalf("%v crossed to %#v and back as %v, want %v", c, ct, back, want)
		}
		for range 5 {
			typ := instance(r, c)
			if !tenon.Satisfies(c, typ) {
				t.Fatalf("instance(%v) = %s, which does not satisfy it", c, typ)
			}
			if !tenon.Satisfies(back, typ) {
				t.Fatalf("%v crossed to %#v and back as %v, which %s does not satisfy", c, ct, back, typ)
			}
			from, err := b.TypeToCty(typ)
			if err != nil {
				t.Fatalf("TypeToCty(%s): %v", typ, err)
			}
			if !ctyConverts(from, ct) {
				t.Fatalf("%s satisfies %v, but cty has no conversion from %#v to %#v", typ, c, from, ct)
			}
		}
		crossed++
	}
	if crossed < 400 || refused < 400 {
		t.Errorf("crossed %d constraints and refused %d; want many of each", crossed, refused)
	}
}

// TestConstraintsRoundTripFromCty carries random cty type constraints to
// tenon and back, which gives each as it was, written as Unify writes it. The
// tenon constraint accepts every type cty can convert to the cty one from
// without converting a primitive, extra attributes and absent optional ones
// included.
func TestConstraintsRoundTripFromCty(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261002))
	for range conformance.Iterations(t, 2000) {
		ct := randomCtyConstraint(r, 4)
		c, err := b.ConstraintFromCty(ct)
		if err != nil {
			t.Fatalf("ConstraintFromCty(%#v): %v", ct, err)
		}
		if want := written(t, c); !c.Equal(want) {
			t.Fatalf("ConstraintFromCty(%#v) = %v, which Unify writes %v", ct, c, want)
		}
		if back, err := b.ConstraintToCty(c); err != nil || !back.Equals(ct) {
			t.Fatalf("%#v crossed to %v and back as %#v, %v", ct, c, back, err)
		}
		for range 5 {
			from := ctyInstance(r, ct)
			if !ctyConverts(from, ct) {
				t.Fatalf("ctyInstance(%#v) = %#v, which cty does not convert to it", ct, from)
			}
			typ, err := b.TypeFromCty(from)
			if err != nil {
				t.Fatalf("TypeFromCty(%#v): %v", from, err)
			}
			if !tenon.Satisfies(c, typ) {
				t.Fatalf("cty converts %#v to %#v, but %s does not satisfy %v", from, ct, typ, c)
			}
		}
	}
}

// TestWhatIsNotAConstraint holds each cty type constraint tenon cannot hold,
// and each tenon constraint cty cannot, to failing with an error that says
// why, naming the whole constraint.
func TestWhatIsNotAConstraint(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		cty  cty.Type
		want string
	}{
		{cty.Object(map[string]cty.Type{"": cty.DynamicPseudoType}), `ctytenon: cty.Object(map[string]cty.Type{"":cty.DynamicPseudoType}) has an object type whose attribute names tenon refuses: object.empty_name`},
		{cty.List(cty.Object(map[string]cty.Type{"a": cty.Object(map[string]cty.Type{"": cty.String})})), "ctytenon: cty.List(cty.Object("},
		{cty.Map(cty.Capsule("thing", reflect.TypeOf(0))), "ctytenon: cty.Map(cty.Capsule(\"thing\", reflect.TypeOf(0))) holds the capsule type thing, which the Bridge pairs with no tenon type"},
	} {
		got, err := b.ConstraintFromCty(c.cty)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("ConstraintFromCty(%#v) = %v, %v; want an error beginning %q", c.cty, got, err, c.want)
		}
	}

	str, num := tenon.Exactly(tenon.StringType()), tenon.Exactly(tenon.NumberType())
	type thing struct{}
	capsule := tenon.NewCapsule("thing", tenon.CapsuleOps[thing]{}).Type()
	for _, c := range []struct {
		tenon tenon.Constraint
		want  string
	}{
		{tenon.OneOf(str, num), "ctytenon: one_of([exactly(string), exactly(number)]) holds one_of([exactly(string), exactly(number)]), a OneOf, which no cty type constraint says"},
		{tenon.ListOf(tenon.OneOf()), "ctytenon: list_of(one_of([])) holds one_of([]), a OneOf, which no cty type constraint says"},
		{tenon.OneOf(str), "ctytenon: one_of([exactly(string)]) holds one_of([exactly(string)]), a OneOf"},
		{tenon.TupleOf(str, tenon.Exactly(tenon.ListType(capsule))), `ctytenon: tuple_of([exactly(string), exactly(list(capsule("thing")))]) holds the capsule type "thing", which the Bridge pairs with no cty type`},
	} {
		got, err := b.ConstraintToCty(c.tenon)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("ConstraintToCty(%v) = %#v, %v; want an error beginning %q", c.tenon, got, err, c.want)
		}
	}
}

// ctyConverts reports whether cty converts a value of type from to the type
// constraint to, without converting a primitive, as cty's Convert decides:
// a type it is already, or one GetConversion, which gives no conversion
// between equal types, gives a conversion from.
func ctyConverts(from, to cty.Type) bool {
	return from.Equals(to.WithoutOptionalAttributesDeep()) || convert.GetConversion(from, to) != nil
}

// holdsOneOfOrCapsule reports whether c holds a OneOf or a capsule type at
// any depth.
func holdsOneOfOrCapsule(c tenon.Constraint) bool {
	switch c.Kind() {
	case tenon.ConstraintOneOf:
		return true
	case tenon.ConstraintExactly:
		return holdsCapsule(c.Type())
	case tenon.ConstraintListOf, tenon.ConstraintSetOf, tenon.ConstraintMapOf:
		return holdsOneOfOrCapsule(c.Element())
	case tenon.ConstraintTupleOf:
		for _, m := range c.Members() {
			if holdsOneOfOrCapsule(m) {
				return true
			}
		}
	case tenon.ConstraintObjectWith:
		for _, name := range c.FieldNames() {
			if f, _ := c.LookupField(name); holdsOneOfOrCapsule(f.Constraint) {
				return true
			}
		}
	}
	return false
}

// holdsCapsule reports whether t is or holds a capsule type.
func holdsCapsule(t tenon.Type) bool {
	switch t.Kind() {
	case tenon.KindCapsule:
		return true
	case tenon.KindList, tenon.KindSet, tenon.KindMap:
		return holdsCapsule(t.ElementType())
	case tenon.KindTuple:
		for _, e := range t.TupleElementTypes() {
			if holdsCapsule(e) {
				return true
			}
		}
	case tenon.KindObject:
		for _, name := range t.AttributeNames() {
			if holdsCapsule(t.AttributeType(name)) {
				return true
			}
		}
	}
	return false
}

// opened returns c with every object constraint in it open, and Exactly of a
// type holding an object type spelled out as the constraints of its parts:
// what cty's object types, which accept attributes they do not name, make of
// it.
func opened(c tenon.Constraint) tenon.Constraint {
	switch c.Kind() {
	case tenon.ConstraintExactly:
		return openedType(c.Type())
	case tenon.ConstraintListOf:
		return tenon.ListOf(opened(c.Element()))
	case tenon.ConstraintSetOf:
		return tenon.SetOf(opened(c.Element()))
	case tenon.ConstraintMapOf:
		return tenon.MapOf(opened(c.Element()))
	case tenon.ConstraintTupleOf:
		members := c.Members()
		for i, m := range members {
			members[i] = opened(m)
		}
		return tenon.TupleOf(members...)
	case tenon.ConstraintObjectWith:
		fields := map[string]tenon.Field{}
		for _, name := range c.FieldNames() {
			f, _ := c.LookupField(name)
			fields[name] = tenon.Field{Constraint: opened(f.Constraint), Required: f.Required}
		}
		return tenon.ObjectWith(fields, false)
	}
	return c
}

// openedType returns the constraint of t's parts, each object type in it an
// open ObjectWith of its attributes.
func openedType(t tenon.Type) tenon.Constraint {
	switch t.Kind() {
	case tenon.KindList:
		return tenon.ListOf(openedType(t.ElementType()))
	case tenon.KindSet:
		return tenon.SetOf(openedType(t.ElementType()))
	case tenon.KindMap:
		return tenon.MapOf(openedType(t.ElementType()))
	case tenon.KindTuple:
		var members []tenon.Constraint
		for _, e := range t.TupleElementTypes() {
			members = append(members, openedType(e))
		}
		return tenon.TupleOf(members...)
	case tenon.KindObject:
		fields := map[string]tenon.Field{}
		for _, name := range t.AttributeNames() {
			fields[name] = tenon.Required(openedType(t.AttributeType(name)))
		}
		return tenon.ObjectWith(fields, false)
	}
	return tenon.Exactly(t)
}

// written returns c as Unify writes it.
func written(t *testing.T, c tenon.Constraint) tenon.Constraint {
	t.Helper()
	w, err := tenon.Unify([]tenon.Constraint{c}, tenon.Safe)
	if err != nil {
		t.Fatalf("Unify(%v): %v", c, err)
	}
	return w
}

// instance returns a random type that satisfies c, which holds no OneOf:
// some type for Any, an object type for an ObjectWith that has each required
// field, some optional ones, and attributes no field names where c is open.
func instance(r *rand.Rand, c tenon.Constraint) tenon.Type {
	switch c.Kind() {
	case tenon.ConstraintAny:
		return randomTenonType(r, 2)
	case tenon.ConstraintExactly:
		return c.Type()
	case tenon.ConstraintListOf:
		return tenon.ListType(instance(r, c.Element()))
	case tenon.ConstraintSetOf:
		return tenon.SetType(instance(r, c.Element()))
	case tenon.ConstraintMapOf:
		return tenon.MapType(instance(r, c.Element()))
	case tenon.ConstraintTupleOf:
		var elems []tenon.Type
		for _, m := range c.Members() {
			elems = append(elems, instance(r, m))
		}
		return tenon.TupleType(elems...)
	}
	attrs := map[string]tenon.Type{}
	for _, name := range c.FieldNames() {
		if f, _ := c.LookupField(name); f.Required || r.Intn(2) == 0 {
			attrs[name] = instance(r, f.Constraint)
		}
	}
	if !c.Closed() {
		for range r.Intn(3) {
			name := names[r.Intn(len(names))]
			if _, named := c.LookupField(name); !named {
				attrs[name] = randomTenonType(r, 1)
			}
		}
	}
	return tenon.ObjectType(attrs)
}

// randomCtyConstraint returns a random cty type constraint nested at most
// depth deep: cty.DynamicPseudoType at any depth, and object types with
// optional attributes among the rest.
func randomCtyConstraint(r *rand.Rand, depth int) cty.Type {
	n := 4
	if depth > 0 {
		n = 9
	}
	switch r.Intn(n) {
	case 0:
		return cty.DynamicPseudoType
	case 1:
		return cty.Bool
	case 2:
		return cty.Number
	case 3:
		return cty.String
	case 4:
		return cty.List(randomCtyConstraint(r, depth-1))
	case 5:
		return cty.Set(randomCtyConstraint(r, depth-1))
	case 6:
		return cty.Map(randomCtyConstraint(r, depth-1))
	case 7:
		elems := make([]cty.Type, r.Intn(4))
		for i := range elems {
			elems[i] = randomCtyConstraint(r, depth-1)
		}
		return cty.Tuple(elems)
	}
	attrs := map[string]cty.Type{}
	var optional []string
	for range r.Intn(4) {
		name := names[r.Intn(len(names))]
		if _, ok := attrs[name]; !ok && r.Intn(3) == 0 {
			optional = append(optional, name)
		}
		attrs[name] = randomCtyConstraint(r, depth-1)
	}
	return cty.ObjectWithOptionalAttrs(attrs, optional)
}

// ctyInstance returns a random cty type that cty converts to the type
// constraint c without converting a primitive: some type for
// cty.DynamicPseudoType, and an object type for an object type that has each
// attribute but some optional ones, and some attributes c does not name.
func ctyInstance(r *rand.Rand, c cty.Type) cty.Type {
	switch {
	case c == cty.DynamicPseudoType:
		return randomCtyType(r, 2)
	case c.IsListType():
		return cty.List(ctyInstance(r, c.ElementType()))
	case c.IsSetType():
		return cty.Set(ctyInstance(r, c.ElementType()))
	case c.IsMapType():
		return cty.Map(ctyInstance(r, c.ElementType()))
	case c.IsTupleType():
		var elems []cty.Type
		for _, e := range c.TupleElementTypes() {
			elems = append(elems, ctyInstance(r, e))
		}
		return cty.Tuple(elems)
	case c.IsObjectType():
		attrs := map[string]cty.Type{}
		for name, a := range c.AttributeTypes() {
			if !c.AttributeOptional(name) || r.Intn(2) == 0 {
				attrs[name] = ctyInstance(r, a)
			}
		}
		for range r.Intn(3) {
			if name := names[r.Intn(len(names))]; !c.HasAttribute(name) {
				attrs[name] = randomCtyType(r, 1)
			}
		}
		return cty.Object(attrs)
	}
	return c
}
