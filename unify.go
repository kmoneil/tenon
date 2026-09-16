package tenon

import (
	"cmp"
	"slices"
	"strings"
)

// Unify returns the most specific constraint that a value satisfying any of cs
// converts to under the policy p, and true. Where there is none, it returns an
// error value with code CodeUnifyNoCommonConstraint, naming the constraints,
// and false. A frontend unifies the types of the branches of a conditional, or
// the operands of an equality, before converting each to the result.
//
// Any stands for a type not yet settled and takes whatever the others give, as
// do the attributes an open ObjectWith leaves unnamed. OneOf unifies member by
// member, leaving out members that do not unify. Otherwise constraints unify
// by kind: one type with itself; two of Bool, Number and String as String,
// under the Unsafe policy only; lists, sets and maps element by element, with a
// set and a list as a list; tuples of one length position by position, and of
// different lengths, or with a list or a set, as a list; objects field by
// field, a field optional where some object lacks it and the result open where
// any object is open; and an object with a map as a map. Exactly of a
// collection or structural type unifies as the constraint that spells out its
// structure. Every other pair fails.
//
// The result is written canonically, so that constraints that unify alike are
// Equal, and it does not depend on the order cs are given in. Unifying none
// gives Any, and unifying one gives it written canonically.
//
// Unify panics if p is not Safe or Unsafe, or if a constraint is the zero
// Constraint.
func Unify(p Policy, cs ...Constraint) (Constraint, Value, bool) {
	if p != Safe && p != Unsafe {
		usagePanic("Unify called with %s, which is neither Safe nor Unsafe", p)
	}
	given := make([]Constraint, len(cs))
	for i, c := range cs {
		c.data()
		given[i] = canonical(c)
	}
	if len(given) == 0 {
		return Any(), Value{}, true
	}
	u := given[0]
	for _, c := range given[1:] {
		var ok bool
		if u, ok = unifyPair(u, c, p); !ok {
			return Constraint{}, unifyFailure(given, p), false
		}
	}
	return u, Value{}, true
}

// unifyFailure returns the error value of constraints that do not unify. It
// names them in canonical order, each once, so that the order they were given
// in does not show.
func unifyFailure(given []Constraint, p Policy) Value {
	sorted := slices.Clone(given)
	slices.SortFunc(sorted, compareConstraints)
	sorted = slices.CompactFunc(sorted, Constraint.equal)
	names := make([]string, len(sorted))
	for i, c := range sorted {
		names[i] = c.String()
	}
	message := names[0] + " has no common constraint with itself"
	if len(names) > 1 {
		message = strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " have no common constraint"
	}
	return errorValue(Diagnostic{Code: CodeUnifyNoCommonConstraint, Message: message + " under the " + p.String() + " policy"})
}

// canonical returns c written canonically, at every depth: a constraint that
// admits no type is OneOf(), one that admits exactly one type is Exactly of it,
// an optional field no attribute can fill is left out, and a OneOf is written
// as oneOf writes it.
func canonical(c Constraint) Constraint {
	if admitsNone(c) {
		return Constraint{&constraintData{kind: ConstraintOneOf}}
	}
	if t, ok := soleType(c); ok {
		return Exactly(t)
	}
	d := c.c
	switch d.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return elementConstraint(d.kind, canonical(d.elem))
	case ConstraintTupleOf:
		members := make([]Constraint, len(d.members))
		for i, m := range d.members {
			members[i] = canonical(m)
		}
		return Constraint{&constraintData{kind: ConstraintTupleOf, members: members}}
	case ConstraintObjectWith:
		var fields []field
		for _, f := range d.fields {
			if !f.Required && admitsNone(f.Constraint) {
				continue
			}
			fields = append(fields, field{f.name, Field{Constraint: canonical(f.Constraint), Required: f.Required}})
		}
		return Constraint{&constraintData{kind: ConstraintObjectWith, fields: fields, closed: d.closed}}
	case ConstraintOneOf:
		return oneOf(d.members)
	}
	return c
}

// oneOf returns the OneOf of members written canonically: members that are
// OneOf are flattened into it, identical members appear once, in canonical
// order, and a lone member stands for the OneOf.
func oneOf(members []Constraint) Constraint {
	var flat []Constraint
	for _, m := range members {
		if cm := canonical(m); cm.c.kind == ConstraintOneOf {
			flat = append(flat, cm.c.members...)
		} else {
			flat = append(flat, cm)
		}
	}
	slices.SortFunc(flat, compareConstraints)
	flat = slices.CompactFunc(flat, Constraint.equal)
	if len(flat) == 1 {
		return flat[0]
	}
	return Constraint{&constraintData{kind: ConstraintOneOf, members: flat}}
}

// constraintOrder is the place of each kind of constraint in the canonical
// order, which is not the order the kinds are declared in.
var constraintOrder = [...]int{
	ConstraintExactly:    0,
	ConstraintAny:        1,
	ConstraintListOf:     2,
	ConstraintSetOf:      3,
	ConstraintMapOf:      4,
	ConstraintTupleOf:    5,
	ConstraintObjectWith: 6,
	ConstraintOneOf:      7,
}

// compareConstraints orders two constraints canonically. It returns zero
// exactly for constraints that are Equal.
func compareConstraints(a, b Constraint) int {
	x, y := a.c, b.c
	if c := cmp.Compare(constraintOrder[x.kind], constraintOrder[y.kind]); c != 0 {
		return c
	}
	switch x.kind {
	case ConstraintExactly:
		return compareTypes(x.typ, y.typ)
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return compareConstraints(x.elem, y.elem)
	case ConstraintTupleOf, ConstraintOneOf:
		return slices.CompareFunc(x.members, y.members, compareConstraints)
	case ConstraintObjectWith:
		if c := slices.CompareFunc(x.fields, y.fields, compareFields); c != 0 {
			return c
		}
		return cmp.Compare(boolOrder(x.closed), boolOrder(y.closed))
	}
	return 0
}

// compareFields orders two fields of ObjectWith constraints: by name, then an
// optional field before a required one, then by constraint.
func compareFields(f, g field) int {
	if c := strings.Compare(f.name, g.name); c != 0 {
		return c
	}
	if c := cmp.Compare(boolOrder(f.Required), boolOrder(g.Required)); c != 0 {
		return c
	}
	return compareConstraints(f.Constraint, g.Constraint)
}

// unifyPair unifies two canonical constraints, returning the canonical result
// and whether there is one.
func unifyPair(a, b Constraint, p Policy) (Constraint, bool) {
	switch {
	case a.c.kind == ConstraintAny:
		return b, true
	case b.c.kind == ConstraintAny:
		return a, true
	case a.c.kind == ConstraintOneOf || b.c.kind == ConstraintOneOf:
		if a.c.kind != ConstraintOneOf {
			a, b = b, a
		}
		var out []Constraint
		for _, m := range a.c.members {
			if u, ok := unifyPair(m, b, p); ok {
				out = append(out, u)
			}
		}
		if len(out) == 0 {
			return Constraint{}, false
		}
		return oneOf(out), true
	}
	r, ok := unifyStructures(spelledOut(a), spelledOut(b), p)
	if !ok {
		return Constraint{}, false
	}
	return canonical(r), true
}

// spelledOut returns Exactly of a collection or structural type as the
// constraint that spells out its structure, and any other constraint as it is.
func spelledOut(c Constraint) Constraint {
	if d := c.c; d.kind == ConstraintExactly && !isPrimitive(d.typ.t.kind) && d.typ.t.kind != KindCapsule {
		return structural(d.typ)
	}
	return c
}

// unifyStructures unifies two constraints that are neither Any nor OneOf, and
// of which an Exactly names a primitive or capsule type.
func unifyStructures(x, y Constraint, p Policy) (Constraint, bool) {
	if constraintOrder[x.c.kind] > constraintOrder[y.c.kind] {
		x, y = y, x
	}
	kx, ky := x.c.kind, y.c.kind
	switch {
	case kx == ConstraintExactly && ky == ConstraintExactly:
		switch tx, ty := x.c.typ, y.c.typ; {
		case tx == ty:
			return x, true
		case p == Unsafe && isPrimitive(tx.t.kind) && isPrimitive(ty.t.kind):
			// Two primitive types meet only as text.
			return Exactly(Type{stringType}), true
		}
	case kx == ky && (kx == ConstraintListOf || kx == ConstraintSetOf || kx == ConstraintMapOf):
		if e, ok := unifyPair(x.c.elem, y.c.elem, p); ok {
			return elementConstraint(kx, e), true
		}
	case kx == ConstraintListOf && ky == ConstraintSetOf:
		if e, ok := unifyPair(x.c.elem, y.c.elem, p); ok {
			return ListOf(e), true
		}
	case (kx == ConstraintListOf || kx == ConstraintSetOf) && ky == ConstraintTupleOf:
		if e, ok := unifyAll(append([]Constraint{x.c.elem}, y.c.members...), p); ok {
			return ListOf(e), true
		}
	case kx == ConstraintTupleOf && ky == ConstraintTupleOf:
		return unifyTupleOf(x, y, p)
	case kx == ConstraintMapOf && ky == ConstraintObjectWith:
		members := []Constraint{x.c.elem}
		for _, f := range y.c.fields {
			members = append(members, f.Constraint)
		}
		if e, ok := unifyAll(members, p); ok {
			return MapOf(e), true
		}
	case kx == ConstraintObjectWith && ky == ConstraintObjectWith:
		return unifyObjectWith(x, y, p)
	}
	return Constraint{}, false
}

// unifyAll unifies canonical constraints pair by pair. With none it gives Any.
func unifyAll(cs []Constraint, p Policy) (Constraint, bool) {
	u := Any()
	for _, c := range cs {
		var ok bool
		if u, ok = unifyPair(u, c, p); !ok {
			return Constraint{}, false
		}
	}
	return u, true
}

// unifyTupleOf unifies two TupleOf constraints: position by position where
// they are of one length, and otherwise as a list of every member of both.
func unifyTupleOf(x, y Constraint, p Policy) (Constraint, bool) {
	xs, ys := x.c.members, y.c.members
	if len(xs) != len(ys) {
		e, ok := unifyAll(slices.Concat(xs, ys), p)
		if !ok {
			return Constraint{}, false
		}
		return ListOf(e), true
	}
	members := make([]Constraint, len(xs))
	for i := range xs {
		var ok bool
		if members[i], ok = unifyPair(xs[i], ys[i], p); !ok {
			return Constraint{}, false
		}
	}
	return Constraint{&constraintData{kind: ConstraintTupleOf, members: members}}, true
}

// unifyObjectWith unifies two ObjectWith constraints field by field: a field
// for each name in either, required where both require it, and closed where
// both are.
func unifyObjectWith(x, y Constraint, p Policy) (Constraint, bool) {
	var fields []field
	fx, fy := x.c.fields, y.c.fields
	for len(fx) > 0 || len(fy) > 0 {
		switch {
		case len(fy) == 0 || len(fx) > 0 && fx[0].name < fy[0].name:
			fields = append(fields, field{fx[0].name, Optional(fx[0].Constraint)})
			fx = fx[1:]
		case len(fx) == 0 || fy[0].name < fx[0].name:
			fields = append(fields, field{fy[0].name, Optional(fy[0].Constraint)})
			fy = fy[1:]
		default:
			u, ok := unifyPair(fx[0].Constraint, fy[0].Constraint, p)
			if !ok {
				return Constraint{}, false
			}
			fields = append(fields, field{fx[0].name, Field{Constraint: u, Required: fx[0].Required && fy[0].Required}})
			fx, fy = fx[1:], fy[1:]
		}
	}
	return Constraint{&constraintData{kind: ConstraintObjectWith, fields: fields, closed: x.c.closed && y.c.closed}}, true
}
