package tenon

import (
	"cmp"
	"slices"
	"strconv"
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
// OneOf multiplies: each member of one unifies with each member of the other,
// so constraints that are each a OneOf of object types with different
// attributes unify to a OneOf of every combination of them. Unify weighs each
// pair of members it forms by their sizes, and where the pairs would weigh
// more in all than a fixed multiple of the size of cs, it forms no more and
// returns an error value with code CodeUnifyTooLarge instead. Unions that stay
// small, because their pairs unify alike or fail, are never refused.
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
	// Unified in canonical order, so that the pairs formed, and whether they
	// pass the bound, do not depend on the order cs are given in (CV-045).
	ordered := slices.Clone(given)
	slices.SortFunc(ordered, compareConstraints)
	un := newUnifier(p)
	var total int64
	for _, c := range ordered {
		total = saturatingAdd(total, un.size(c))
	}
	un.left = saturatingMul(unifyBound, total)
	u := ordered[0]
	for _, c := range ordered[1:] {
		var ok bool
		if u, ok = un.pair(u, c); !ok {
			if un.over {
				return Constraint{}, unifyTooLarge(len(given), total), false
			}
			return Constraint{}, unifyFailure(given, p), false
		}
	}
	return u, Value{}, true
}

// unifyBound is the multiple of CV-045: the pairs a unification forms may
// weigh in all at most this many times the size of the constraints it is
// given.
const unifyBound = 64

// unifier holds what one Unify call has left to form: the weight of the pairs
// it may still form, whether a OneOf has been refused its pairs, and the sizes
// it has measured, since a member is weighed again at every pair it joins.
type unifier struct {
	p     Policy
	left  int64
	over  bool
	sizes map[*constraintData]int64
	types map[*typeData]int64
}

func newUnifier(p Policy) *unifier {
	return &unifier{p: p, sizes: map[*constraintData]int64{}, types: map[*typeData]int64{}}
}

// size returns the size of c by CV-045: the constraints c is written with,
// itself among them, each counted where it appears, Exactly of a type counting
// as the types that type is written with.
func (un *unifier) size(c Constraint) int64 {
	d := c.c
	if s, ok := un.sizes[d]; ok {
		return s
	}
	s := int64(1)
	switch d.kind {
	case ConstraintExactly:
		s = un.typeSize(d.typ)
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		s = saturatingAdd(s, un.size(d.elem))
	case ConstraintTupleOf, ConstraintOneOf:
		for _, m := range d.members {
			s = saturatingAdd(s, un.size(m))
		}
	case ConstraintObjectWith:
		for _, f := range d.fields {
			s = saturatingAdd(s, un.size(f.Constraint))
		}
	}
	un.sizes[d] = s
	return s
}

// typeSize returns the number of types t is written with, itself among them,
// each counted where it appears.
func (un *unifier) typeSize(t Type) int64 {
	d := t.t
	if s, ok := un.types[d]; ok {
		return s
	}
	s := int64(1)
	switch d.kind {
	case KindList, KindSet, KindMap:
		s = saturatingAdd(s, un.typeSize(d.elem))
	case KindTuple:
		for _, e := range d.elems {
			s = saturatingAdd(s, un.typeSize(e))
		}
	case KindObject:
		for _, a := range d.attrs {
			s = saturatingAdd(s, un.typeSize(a.typ))
		}
	}
	un.types[d] = s
	return s
}

// saturated is where sizes and weights stop counting: past any bound that the
// size of constraints a program could hold allows.
const saturated = int64(1) << 60

func saturatingAdd(a, b int64) int64 {
	if a >= saturated-b {
		return saturated
	}
	return a + b
}

func saturatingMul(a, b int64) int64 {
	if a != 0 && b > saturated/a {
		return saturated
	}
	return a * b
}

// unifyTooLarge returns the error value of a unification refused by CV-045. It
// names neither the constraints nor their order, only how many there were and
// the weight their size allowed, which do not depend on the order either.
func unifyTooLarge(n int, size int64) Value {
	return errorValue(Diagnostic{Code: CodeUnifyTooLarge, Message: "unifying " + count(n, "constraint") + " of size " +
		strconv.FormatInt(size, 10) + " forms pairs weighing more than the " +
		strconv.FormatInt(saturatingMul(unifyBound, size), 10) + " that size allows"})
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

// pair unifies two canonical constraints, returning the canonical result and
// whether there is one. Where it fails because a OneOf was refused its pairs,
// un.over says so, and every caller stops.
func (un *unifier) pair(a, b Constraint) (Constraint, bool) {
	switch {
	case a.c.kind == ConstraintAny:
		return b, true
	case b.c.kind == ConstraintAny:
		return a, true
	case a.c.kind == ConstraintOneOf || b.c.kind == ConstraintOneOf:
		// Each member of one unifies with each member of the other, a
		// constraint that is no OneOf being its own one member. Taking the
		// members of only one side would make OneOf() fail or not by which
		// side it is on, since a member Any unifies with it and nothing else
		// does.
		xs, ys := []Constraint{a}, []Constraint{b}
		if a.c.kind == ConstraintOneOf {
			xs = a.c.members
		}
		if b.c.kind == ConstraintOneOf {
			ys = b.c.members
		}
		// Every member of xs joins a pair with each member of ys, and a pair
		// weighs its two members' sizes, so the pairs weigh this in all,
		// which is judged before any of them is formed (CV-045).
		var sx, sy int64
		for _, x := range xs {
			sx = saturatingAdd(sx, un.size(x))
		}
		for _, y := range ys {
			sy = saturatingAdd(sy, un.size(y))
		}
		weight := saturatingAdd(saturatingMul(int64(len(ys)), sx), saturatingMul(int64(len(xs)), sy))
		if weight > un.left {
			un.over = true
			return Constraint{}, false
		}
		un.left -= weight
		var out []Constraint
		for _, x := range xs {
			for _, y := range ys {
				u, ok := un.pair(x, y)
				if un.over {
					return Constraint{}, false
				}
				if ok {
					out = append(out, u)
				}
			}
		}
		if len(out) == 0 {
			return Constraint{}, false
		}
		return oneOf(out), true
	}
	r, ok := un.structures(spelledOut(a), spelledOut(b))
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

// structures unifies two constraints that are neither Any nor OneOf, and of
// which an Exactly names a primitive or capsule type.
func (un *unifier) structures(x, y Constraint) (Constraint, bool) {
	if constraintOrder[x.c.kind] > constraintOrder[y.c.kind] {
		x, y = y, x
	}
	kx, ky := x.c.kind, y.c.kind
	switch {
	case kx == ConstraintExactly && ky == ConstraintExactly:
		switch tx, ty := x.c.typ, y.c.typ; {
		case tx == ty:
			return x, true
		case un.p == Unsafe && isPrimitive(tx.t.kind) && isPrimitive(ty.t.kind):
			// Two primitive types meet only as text.
			return Exactly(Type{stringType}), true
		}
	case kx == ky && (kx == ConstraintListOf || kx == ConstraintSetOf || kx == ConstraintMapOf):
		if e, ok := un.pair(x.c.elem, y.c.elem); ok {
			return elementConstraint(kx, e), true
		}
	case kx == ConstraintListOf && ky == ConstraintSetOf:
		if e, ok := un.pair(x.c.elem, y.c.elem); ok {
			return ListOf(e), true
		}
	case (kx == ConstraintListOf || kx == ConstraintSetOf) && ky == ConstraintTupleOf:
		if e, ok := un.all(append([]Constraint{x.c.elem}, y.c.members...)); ok {
			return ListOf(e), true
		}
	case kx == ConstraintTupleOf && ky == ConstraintTupleOf:
		return un.tuples(x, y)
	case kx == ConstraintMapOf && ky == ConstraintObjectWith:
		members := []Constraint{x.c.elem}
		for _, f := range y.c.fields {
			members = append(members, f.Constraint)
		}
		if e, ok := un.all(members); ok {
			return MapOf(e), true
		}
	case kx == ConstraintObjectWith && ky == ConstraintObjectWith:
		return un.objects(x, y)
	}
	return Constraint{}, false
}

// all unifies canonical constraints pair by pair, in canonical order, so that
// the pairs formed do not depend on the order the rule gathered them in
// (CV-045). With none it gives Any.
func (un *unifier) all(cs []Constraint) (Constraint, bool) {
	ordered := slices.Clone(cs)
	slices.SortFunc(ordered, compareConstraints)
	u := Any()
	for _, c := range ordered {
		var ok bool
		if u, ok = un.pair(u, c); !ok {
			return Constraint{}, false
		}
	}
	return u, true
}

// tuples unifies two TupleOf constraints: position by position where they are
// of one length, and otherwise as a list of every member of both.
func (un *unifier) tuples(x, y Constraint) (Constraint, bool) {
	xs, ys := x.c.members, y.c.members
	if len(xs) != len(ys) {
		e, ok := un.all(slices.Concat(xs, ys))
		if !ok {
			return Constraint{}, false
		}
		return ListOf(e), true
	}
	members := make([]Constraint, len(xs))
	for i := range xs {
		var ok bool
		if members[i], ok = un.pair(xs[i], ys[i]); !ok {
			return Constraint{}, false
		}
	}
	return Constraint{&constraintData{kind: ConstraintTupleOf, members: members}}, true
}

// objects unifies two ObjectWith constraints field by field, in name order: a
// field for each name in either, required where both require it, and closed
// where both are.
func (un *unifier) objects(x, y Constraint) (Constraint, bool) {
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
			u, ok := un.pair(fx[0].Constraint, fy[0].Constraint)
			if !ok {
				return Constraint{}, false
			}
			fields = append(fields, field{fx[0].name, Field{Constraint: u, Required: fx[0].Required && fy[0].Required}})
			fx, fy = fx[1:], fy[1:]
		}
	}
	return Constraint{&constraintData{kind: ConstraintObjectWith, fields: fields, closed: x.c.closed && y.c.closed}}, true
}
