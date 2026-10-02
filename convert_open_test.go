package tenon_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// TestConformance_CV021_AnEmptyMemberTakesItsSiblingsType converts
// collections in which a member's conversion leaves a part of its type open,
// as an empty tuple converted to ListOf(Any) does, beside members that settle
// it: the member takes the type they settle in its place and is converted to
// the element type, at any depth and in any position, null or unknown, within
// a tuple, an object or a map, as a set holding a member that is not known,
// and where it converts to a structure of another shape than its siblings.
func TestConformance_CV021_AnEmptyMemberTakesItsSiblingsType(t *testing.T) {
	conformance.Covers(t, "CV-021")
	anyList := tenon.ListOf(tenon.Any())
	strs := tenon.ListType(str)
	empty, a := tenon.Tuple(), tenon.Tuple(s("a"))
	emptyStrs, aStrs := tenon.List(str), tenon.List(str, s("a"))
	withC := tenon.ObjectType(map[string]tenon.Type{"a": strs, "c": num})
	unknownEmpty := tenon.Unknown(tenon.TupleType())
	nullOfTwo := tenon.Null(tenon.TupleType(tenon.TupleType(), tenon.TupleType(str)))
	unknownOfTwo := tenon.Unknown(tenon.TupleType(tenon.TupleType(), tenon.TupleType(str)))
	setOfUnknown := tenon.Set(tenon.TupleType(), unknownEmpty)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		c    tenon.Constraint
		p    tenon.Policy
		want tenon.Value
	}{
		{"an empty tuple beside a tuple of strings", tenon.Tuple(a, empty), tenon.ListOf(anyList), safe,
			tenon.List(strs, aStrs, emptyStrs)},
		{"an empty tuple first", tenon.Tuple(empty, a), tenon.ListOf(anyList), safe,
			tenon.List(strs, emptyStrs, aStrs)},
		{"a level down", tenon.Tuple(tenon.Tuple(empty), tenon.Tuple(a)), tenon.ListOf(tenon.ListOf(anyList)), safe,
			tenon.List(tenon.ListType(strs), tenon.List(strs, emptyStrs), tenon.List(strs, aStrs))},
		{"in a set", tenon.Tuple(empty, a), tenon.SetOf(anyList), uns,
			tenon.Set(strs, emptyStrs, aStrs)},
		{"as an attribute, to a map", obj(map[string]tenon.Value{"a": a, "b": empty}), tenon.MapOf(anyList), safe,
			tenon.Map(strs, map[string]tenon.Value{"a": aStrs, "b": emptyStrs})},
		{"an empty object", tenon.Tuple(obj(nil), obj(map[string]tenon.Value{"k": n(1)})), tenon.ListOf(tenon.MapOf(tenon.Any())), safe,
			tenon.List(tenon.MapType(num), tenon.Map(num, nil), tenon.Map(num, map[string]tenon.Value{"k": n(1)}))},
		{"null", tenon.Tuple(a, tenon.Null(tenon.TupleType())), tenon.ListOf(anyList), safe,
			tenon.List(strs, aStrs, tenon.Null(strs))},
		{"unknown", tenon.Tuple(a, unknownEmpty), tenon.ListOf(anyList), safe,
			tenon.List(strs, aStrs, tenon.Convert(unknownEmpty, is(strs), safe))},
		{"a null tuple of an empty tuple type and another", nullOfTwo, tenon.ListOf(anyList), safe,
			tenon.Null(tenon.ListType(strs))},
		{"an unknown tuple of an empty tuple type and another", unknownOfTwo, tenon.ListOf(anyList), safe,
			tenon.Convert(unknownOfTwo, is(tenon.ListType(strs)), safe)},
		{"within a tuple, a part the others settle",
			tenon.Tuple(tenon.Tuple(empty, s("x")), tenon.Tuple(a, s("y"))), tenon.ListOf(tenon.TupleOf(anyList, tenon.Any())), safe,
			tenon.List(tenon.TupleType(strs, str), tenon.Tuple(emptyStrs, s("x")), tenon.Tuple(aStrs, s("y")))},
		{"within a tuple, beside a part that unifies only unsafely",
			tenon.Tuple(tenon.Tuple(empty, s("x")), tenon.Tuple(a, n(1))), tenon.ListOf(tenon.TupleOf(anyList, tenon.Any())), uns,
			tenon.List(tenon.TupleType(strs, str), tenon.Tuple(emptyStrs, s("x")), tenon.Tuple(aStrs, s("1")))},
		{"within an object holding an attribute the others lack",
			tenon.Tuple(obj(map[string]tenon.Value{"a": empty, "c": n(1)}), obj(map[string]tenon.Value{"a": a})),
			tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(anyList)}, false)), safe,
			tenon.List(withC,
				obj(map[string]tenon.Value{"a": emptyStrs, "c": n(1)}),
				obj(map[string]tenon.Value{"a": aStrs, "c": tenon.Null(num)}))},
		{"a null tuple with a part settled already",
			tenon.Tuple(tenon.Null(tenon.TupleType(tenon.TupleType(), str)), tenon.Tuple(a, s("y"))), tenon.ListOf(tenon.TupleOf(anyList, tenon.Any())), safe,
			tenon.List(tenon.TupleType(strs, str), tenon.Null(tenon.TupleType(strs, str)), tenon.Tuple(aStrs, s("y")))},
		{"a null object",
			tenon.Tuple(tenon.Null(tenon.ObjectType(map[string]tenon.Type{"a": tenon.TupleType()})), obj(map[string]tenon.Value{"a": a})),
			tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(anyList)}, true)), safe,
			tenon.List(tenon.ObjectType(map[string]tenon.Type{"a": strs}),
				tenon.Null(tenon.ObjectType(map[string]tenon.Type{"a": strs})), obj(map[string]tenon.Value{"a": aStrs}))},
		{"a set holding a member not known, to a list", tenon.Tuple(setOfUnknown, tenon.Tuple(a)), tenon.ListOf(tenon.ListOf(anyList)), safe,
			tenon.List(tenon.ListType(strs), tenon.Convert(setOfUnknown, is(tenon.ListType(strs)), safe), tenon.List(strs, aStrs))},
		{"a set holding a member not known, to a tuple", tenon.Tuple(setOfUnknown, tenon.Tuple(a)), tenon.ListOf(tenon.TupleOf(anyList)), uns,
			tenon.List(tenon.TupleType(strs), tenon.Convert(setOfUnknown, is(tenon.TupleType(strs)), uns), tenon.Tuple(aStrs))},
		{"a member of another shape than its sibling", tenon.Tuple(tenon.Tuple(empty), tenon.Tuple(a, tenon.Tuple(s("b")))),
			tenon.ListOf(tenon.OneOf(tenon.TupleOf(anyList), anyList)), safe,
			tenon.List(tenon.ListType(strs), tenon.List(strs, emptyStrs), tenon.List(strs, aStrs, tenon.List(str, s("b"))))},
	} {
		got := tenon.Convert(tt.v, tt.c, tt.p)
		wantValue(t, tt.name, got, tt.want)
		if !got.IsError() && !tenon.Satisfies(tt.c, got.Type()) {
			t.Errorf("%s: %v does not satisfy %v", tt.name, got, tt.c)
		}
	}
}

// TestConformance_CV021_WhatNothingSettlesFailsWhereItIsLeftOpen converts
// values whose conversion leaves a part of the result's type open with
// nothing to settle it: the conversion fails with convert.no_common_type,
// located at each innermost value that left a part open which nothing
// settles, and not at one whose open part a sibling settles; at the value
// itself where its type alone leaves the part open; and at a value carrying a
// redacting mark, where the part is left open within it.
// TestConformance_CV021_APendingNullTakesItsSiblingsType holds a member that
// converts to a pending value known to be null, as JSON's null is read, to
// taking the element type the other members of its collection settle, here or
// a level above, and becoming its null; and to leaving the collection pending
// where nothing settles one, as it did before (CV-031).
func TestConformance_CV021_APendingNullTakesItsSiblingsType(t *testing.T) {
	conformance.Covers(t, "CV-021", "CV-031", "CV-033")
	prop := stamp{id: "prop"}
	anyC, anyList := tenon.Any(), tenon.ListOf(tenon.Any())
	pn := tenon.Narrow(tenon.Pending(anyC), tenon.NullOnly())
	numbers := tenon.ListType(num)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		c    tenon.Constraint
		want tenon.Value
	}{
		// JSON's {"a": null, "b": 1} under map(any), as go-cty reads it.
		{"beside a number in a map", obj(map[string]tenon.Value{"a": pn, "b": n(1)}), tenon.MapOf(anyC),
			tenon.Map(num, map[string]tenon.Value{"a": tenon.Null(num), "b": n(1)})},
		{"beside a number in a list", tenon.Tuple(pn, n(1)), anyList, tenon.List(num, tenon.Null(num), n(1))},
		{"a level down, settled by a sibling's members", tenon.Tuple(tenon.Tuple(pn), tenon.Tuple(n(1))), tenon.ListOf(anyList),
			tenon.List(numbers, tenon.List(num, tenon.Null(num)), tenon.List(num, n(1)))},
		{"in a set", tenon.Tuple(pn, s("a")), tenon.SetOf(anyC), tenon.Set(str, tenon.Null(str), s("a"))},
		{"keeping a Propagate mark", tenon.Tuple(tenon.WithMarks(pn, prop), n(1)), anyList,
			tenon.List(num, tenon.WithMarks(tenon.Null(num), prop), n(1))},
		// Where nothing settles a type, the collection is pending, as before.
		{"alone", tenon.Tuple(pn, pn), anyList,
			tenon.Narrow(tenon.Pending(anyList), tenon.NotNull(), tenon.LengthMin(2), tenon.LengthMax(2))},
		{"alone a level down", tenon.Tuple(tenon.Tuple(pn)), tenon.ListOf(anyList),
			tenon.Narrow(tenon.Pending(tenon.ListOf(anyList)), tenon.NotNull(), tenon.LengthMin(1), tenon.LengthMax(1))},
		{"beside another pending value", tenon.Tuple(pn, tenon.Pending(anyC)), anyList,
			tenon.Narrow(tenon.Pending(anyList), tenon.NotNull(), tenon.LengthMin(2), tenon.LengthMax(2))},
		{"alone in a structure's part", obj(map[string]tenon.Value{"x": tenon.Tuple(pn)}),
			tenon.ObjectWith(map[string]tenon.Field{"x": tenon.Required(anyList)}, true),
			obj(map[string]tenon.Value{"x": tenon.Narrow(tenon.Pending(anyList), tenon.NotNull(), tenon.LengthMin(1), tenon.LengthMax(1))})},
	} {
		wantValue(t, tt.name, tenon.Convert(tt.v, tt.c, tenon.Unsafe), tt.want)
	}
	// What an empty member leaves open still fails where nothing settles it,
	// and the null takes no blame.
	wantErrors(t, "beside an empty tuple", tenon.Convert(tenon.Tuple(tenon.Tuple(pn), tenon.Tuple()), tenon.ListOf(anyList), tenon.Unsafe),
		wantDiag{tenon.CodeConvertNoCommonType, ".[1]"})
}

func TestConformance_CV021_WhatNothingSettlesFailsWhereItIsLeftOpen(t *testing.T) {
	conformance.Covers(t, "CV-021", "CV-050", "CV-032")
	anyList := tenon.ListOf(tenon.Any())
	empty, a := tenon.Tuple(), tenon.Tuple(s("a"))
	at := func(path string) wantDiag { return wantDiag{tenon.CodeConvertNoCommonType, path} }
	crossing := tenon.TupleType(tenon.TupleType(tenon.TupleType(), tenon.TupleType()), tenon.TupleType(tenon.TupleType(str), tenon.TupleType()))
	for _, tt := range []struct {
		name string
		v    tenon.Value
		c    tenon.Constraint
		want []wantDiag
	}{
		{"an empty tuple", empty, anyList, []wantDiag{at(".")}},
		{"a null empty tuple", tenon.Null(tenon.TupleType()), anyList, []wantDiag{at(".")}},
		{"an empty list of empty tuples", tenon.List(tenon.TupleType()), tenon.ListOf(anyList), []wantDiag{at(".")}},
		{"an empty tuple alone", tenon.Tuple(empty), tenon.ListOf(anyList), []wantDiag{at(".[0]")}},
		{"empty tuples alone", tenon.Tuple(empty, empty), tenon.ListOf(anyList), []wantDiag{at(".[0]"), at(".[1]")}},
		{"a level down", tenon.Tuple(tenon.Tuple(empty), tenon.Tuple(empty)), tenon.ListOf(tenon.ListOf(anyList)),
			[]wantDiag{at(".[0][0]"), at(".[1][0]")}},
		{"as attributes", obj(map[string]tenon.Value{"b": empty, "a": empty}), tenon.MapOf(anyList),
			[]wantDiag{at(".a"), at(".b")}},
		{"a set holding a member not known", tenon.Set(tenon.TupleType(), tenon.Unknown(tenon.TupleType())), tenon.ListOf(anyList),
			[]wantDiag{at(".[0]")}},
		{"a part a sibling settles and a part nothing settles",
			tenon.Tuple(tenon.Tuple(empty, empty), tenon.Tuple(a, empty)), tenon.ListOf(tenon.TupleOf(anyList, anyList)),
			[]wantDiag{at(".[0][1]"), at(".[1][1]")}},
		{"the same, as a null of their type", tenon.Null(crossing), tenon.ListOf(tenon.TupleOf(anyList, anyList)),
			[]wantDiag{at(".")}},
	} {
		got := tenon.Convert(tt.v, tt.c, tenon.Unsafe)
		wantErrors(t, tt.name, got, tt.want...)
		for _, d := range got.Diagnostics() {
			if d.Message != "nothing settles an element type for any: there are no members, and the constraint admits more than one type" {
				t.Errorf("%s: %v, want each message to say that nothing settles the element type", tt.name, got)
			}
		}
	}

	// A pending value is in no container, so nothing settles what the one
	// type it allows leaves open (CV-032).
	wantErrors(t, "a pending value of one type",
		tenon.Convert(tenon.Pending(is(tenon.TupleType(tenon.TupleType()))), tenon.ListOf(anyList), safe),
		wantDiag{tenon.CodeOperationWrongType, "."})

	secret := stamp{id: "secret", redact: true}
	got := tenon.Convert(tenon.Tuple(tenon.Tuple(a, empty), tenon.WithMarks(tenon.Tuple(empty, empty), secret)),
		tenon.ListOf(tenon.TupleOf(anyList, anyList)), safe)
	wantErrors(t, "within a redacted member", got, at(".[0][1]"), at(".[1]"))
	if m := got.Diagnostics()[1].Message; !strings.Contains(m, `redacted("secret")`) {
		t.Errorf("within a redacted member: %q, want it to name the member by the placeholder", m)
	}
}

// TestConformance_CV044_AnOpenPartUnifiesAsAny unifies the types of members
// whose conversions leave a part open with the types of others: the open part
// takes the type the others have in its place, whatever kind it is, under the
// rules that unify them, so members that would not unify still fail, and the
// order of the members does not change the result.
func TestConformance_CV044_AnOpenPartUnifiesAsAny(t *testing.T) {
	conformance.Covers(t, "CV-044", "CV-041")
	anyList := tenon.ListOf(tenon.ListOf(tenon.Any()))
	empty := tenon.Tuple()
	for _, tt := range []struct {
		name  string
		elems []tenon.Value
		p     tenon.Policy
		want  tenon.Type
	}{
		{"a number", []tenon.Value{empty, tenon.Tuple(n(1))}, safe, num},
		{"a number and a string, unsafe", []tenon.Value{empty, tenon.Tuple(n(1)), tenon.Tuple(s("a"))}, uns, str},
		{"a tuple", []tenon.Value{empty, tenon.Tuple(tenon.Tuple(n(1), s("a")))}, safe, tenon.TupleType(num, str)},
		{"objects of two shapes", []tenon.Value{
			tenon.Tuple(obj(map[string]tenon.Value{"a": n(1)})), empty, tenon.Tuple(obj(map[string]tenon.Value{"b": n(2)})),
		}, safe, tenon.ObjectType(map[string]tenon.Type{"a": num, "b": num})},
	} {
		r := tenon.Convert(tenon.Tuple(tt.elems...), anyList, tt.p)
		if r.IsError() || r.Type() != tenon.ListType(tenon.ListType(tt.want)) {
			t.Errorf("%s: %v, want a list of lists of %v", tt.name, r, tt.want)
		}
	}
	wantErrors(t, "a number and a string, safe",
		tenon.Convert(tenon.Tuple(empty, tenon.Tuple(n(1)), tenon.Tuple(s("a"))), anyList, safe),
		wantDiag{tenon.CodeConvertNoCommonType, "."})

	pool := []tenon.Value{empty, tenon.Tuple(n(1)), tenon.Tuple(s("b"), tenon.Bool(true)), empty, tenon.Tuple(n(2))}
	for _, perm := range permutations(len(pool)) {
		elems := make([]tenon.Value, len(pool))
		for i, j := range perm {
			elems[i] = pool[j]
		}
		r := tenon.Convert(tenon.Tuple(elems...), tenon.ListOf(tenon.ListOf(tenon.Any())), uns)
		if r.IsError() || r.Type() != tenon.ListType(tenon.ListType(str)) {
			t.Errorf("members %v gave %v, want a list of lists of strings in every order", elems, r)
		}
	}
}

// sparse returns a tuple of up to four values of one shape, depth levels
// deep: strings, numbers or bools; tuples of any number of values of one
// shape below, often none; and objects of the same attributes. So the
// members' empty tuples leave their element types open, and their siblings
// settle them. Values within are null or unknown now and then, and carry
// marks of every kind, a redacting one among them.
func sparse(r *rand.Rand, depth int) tenon.Value {
	secret := stamp{id: "secret", redact: true}
	marks := []tenon.Mark{markPlain, markDeep, markIsolated, secret}
	vary := func(v tenon.Value) tenon.Value {
		if v.IsPending() {
			return v
		}
		switch r.Intn(13) {
		case 0:
			return tenon.Null(v.Type())
		case 1:
			return tenon.Unknown(v.Type())
		case 2:
			return tenon.WithMarks(v, marks[r.Intn(len(marks))])
		case 3:
			// A value whose type is not settled yet, which makes a tuple or
			// object holding it the pending one holding its members.
			return tenon.Pending(tenon.Exactly(v.Type()))
		case 4:
			// A null whose type nothing gives, as JSON's null is read, which
			// takes the type its siblings settle in a collection (CV-021).
			pn := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
			if r.Intn(3) == 0 {
				pn = tenon.WithMarks(pn, marks[r.Intn(len(marks))])
			}
			return pn
		}
		return v
	}
	var shape func(depth int) func() tenon.Value
	shape = func(depth int) func() tenon.Value {
		switch k := r.Intn(4); {
		case depth == 0 || k == 0:
			prim := []tenon.Value{s("x"), n(1), tenon.Bool(true)}[r.Intn(3)]
			return func() tenon.Value { return prim }
		case k < 3:
			elem := shape(depth - 1)
			return func() tenon.Value {
				members := make([]tenon.Value, []int{0, 0, 1, 2, 3}[r.Intn(5)])
				for i := range members {
					members[i] = vary(elem())
				}
				return tenon.Tuple(members...)
			}
		}
		attrs := map[string]func() tenon.Value{}
		for i := range 1 + r.Intn(2) {
			attrs[fmt.Sprintf("a%d", i)] = shape(depth - 1)
		}
		return func() tenon.Value {
			v := map[string]tenon.Value{}
			for name, attr := range attrs {
				v[name] = vary(attr())
			}
			return tenon.Object(v)
		}
	}
	top := shape(depth)
	members := make([]tenon.Value, 1+r.Intn(4))
	for i := range members {
		members[i] = vary(top())
	}
	return tenon.Tuple(members...)
}

// openShaped returns a constraint of much the shape of t that converts its
// tuples and objects to collections of an element type left to the members
// more often than not, so that an empty one leaves its element type open.
func openShaped(r *rand.Rand, t tenon.Type) tenon.Constraint {
	collection := func(elem tenon.Constraint) tenon.Constraint {
		if r.Intn(3) == 0 {
			return tenon.SetOf(elem)
		}
		return tenon.ListOf(elem)
	}
	switch t.Kind() {
	case tenon.KindTuple:
		elems := t.TupleElementTypes()
		switch {
		case len(elems) == 0 || r.Intn(3) > 0:
			if len(elems) == 0 || r.Intn(2) == 0 {
				return collection(tenon.Any())
			}
			return collection(openShaped(r, elems[r.Intn(len(elems))]))
		}
		members := make([]tenon.Constraint, len(elems))
		for i, e := range elems {
			members[i] = openShaped(r, e)
		}
		return tenon.TupleOf(members...)
	case tenon.KindObject:
		names := t.AttributeNames()
		if len(names) == 0 || r.Intn(3) == 0 {
			return tenon.MapOf(tenon.Any())
		}
		fields := map[string]tenon.Field{}
		for _, name := range names {
			if r.Intn(4) > 0 {
				fields[name] = tenon.Field{Constraint: openShaped(r, t.AttributeType(name)), Required: r.Intn(2) == 0}
			}
		}
		return tenon.ObjectWith(fields, false)
	case tenon.KindList, tenon.KindSet:
		return collection(openShaped(r, t.ElementType()))
	}
	return tenon.Any()
}

// TestConformance_CV021_OpenPartsBuildAsTheReferenceFitsThem holds the
// conversion of values in which many members leave a part of their type
// open, converted to lists and sets of Any or to constraints of much their
// shape, to the reference that makes such a member as a value of a type left
// open and fits it to what the levels above settle (CV-021, CV-044): the same
// result, identical in its type, its contents, its marks and its failures,
// under both policies, and a result whose type satisfies the constraint.
func TestConformance_CV021_OpenPartsBuildAsTheReferenceFitsThem(t *testing.T) {
	conformance.Covers(t, "CV-021", "CV-044", "CV-033")
	r := rand.New(rand.NewSource(20261002))
	policies := []tenon.Policy{tenon.Safe, tenon.Unsafe}
	var cases, settled int
	for range conformance.Iterations(t, 1500) {
		v := sparse(r, 1+r.Intn(3))
		p := policies[r.Intn(2)]
		shaped := nestedCollections(r, 1+r.Intn(3))
		if v.IsResolved() {
			shaped = openShaped(r, v.Type())
		}
		for _, c := range []tenon.Constraint{nestedCollections(r, 1+r.Intn(3)), shaped} {
			cases++
			converted, fitted := tenon.ConvertBothWays(v, c, p)
			if !tenon.Identical(converted, fitted) {
				t.Errorf("%v converted to %v under the %v policy gives %v, where the reference gives %v", v, c, p, converted, fitted)
				continue
			}
			if converted.IsError() || converted.IsPending() {
				continue
			}
			settled++
			if !tenon.Satisfies(c, converted.Type()) {
				t.Errorf("%v converted to %v under the %v policy gives %v, which does not satisfy it", v, c, p, converted)
			}
		}
	}
	if settled < cases/4 {
		t.Errorf("%d of %d cases converted; want a quarter at least", settled, cases)
	}
}
