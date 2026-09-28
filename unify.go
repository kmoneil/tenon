package tenon

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Unify returns the most specific constraint that a value satisfying any of cs
// converts to under the policy p. Where there is none, it fails with code
// CodeUnifyNoCommonConstraint, naming the constraints. A frontend unifies the
// types of the branches of a conditional, or the operands of an equality,
// before converting each to the result.
//
// Any stands for a type not yet settled and takes whatever the others give, as
// do the attributes an open ObjectWith leaves unnamed, so a value that turns
// out to be of another type than they are given does not convert to the
// result. OneOf unifies member by member, leaving out members that do not
// unify, so a value satisfying only a member left out does not convert to the
// result either. Otherwise constraints unify
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
// fails with code CodeUnifyTooLarge instead. Unions that stay
// small, because their pairs unify alike or fail, are never refused.
//
// Where the constraints do not unify, Unify returns the zero Constraint and a
// [*Error] whose error value says why.
//
// Unify panics if p is not Safe or Unsafe, or if a constraint is the zero
// Constraint.
func Unify(cs []Constraint, p Policy) (Constraint, error) {
	c, failure, ok := unify(p, cs...)
	if !ok {
		return Constraint{}, asError(failure)
	}
	return c, nil
}

// unify is Unify, giving the error value it fails with and false.
func unify(p Policy, cs ...Constraint) (Constraint, Value, bool) {
	if p != Safe && p != Unsafe {
		usagePanic("Unify called with %s, which is neither Safe nor Unsafe", p)
	}
	un := newUnifier(p)
	given := make([]Constraint, len(cs))
	for i, c := range cs {
		c.data()
		given[i] = un.memo.canonical(c)
	}
	if len(given) == 0 {
		return Any(), Value{}, true
	}
	// Unified in canonical order, so that the pairs formed, and whether they
	// pass the bound, do not depend on the order cs are given in (CV-045).
	ordered := slices.Clone(given)
	slices.SortFunc(ordered, compareConstraints)
	var total int64
	for _, c := range ordered {
		total = saturatingAdd(total, un.size(c))
	}
	un.left = saturatingMul(unifyBound, total)
	u, ok := un.fold(ordered[0], ordered[1:])
	if !ok {
		if un.over {
			return Constraint{}, unifyTooLarge(len(given), total), false
		}
		return Constraint{}, unifyFailure(given, p), false
	}
	return u, Value{}, true
}

// fold unifies u with each of cs in turn, stopping at the first that fails.
// The union so far is held unwritten where the constraints allow it (union):
// unifying one more with the union written out rewrote the union each time,
// so 4,000 ObjectWith constraints of distinct fields took 414 ms and 2.4 GB,
// and as many held one level down, as the elements of ListOf constraints,
// 8,000 took 1.7 GB, since each was paired with the elements' union written
// out.
func (un *unifier) fold(u Constraint, cs []Constraint) (Constraint, bool) {
	a := union{u: u}
	for _, c := range cs {
		if !a.add(un, c) {
			return Constraint{}, false
		}
	}
	return a.written(un), true
}

// objectLike reports whether c is an ObjectWith or Exactly of an object type,
// which rule 3 of CV-042 spells out as one.
func objectLike(c Constraint) bool {
	d := c.c
	return d.kind == ConstraintObjectWith || d.kind == ConstraintExactly && d.typ.t.kind == KindObject
}

// union is the union of the constraints a fold has met at one position, so
// far. Where they are collections, tuples of one length or objects, it is
// held as the union of their parts, each a union in turn (collectionUnion,
// tupleUnion, objectUnion), so that unifying one more touches only its own
// parts; otherwise it is written out (u). It forms the pairs that folding the
// constraints pair by pair forms, in the order it forms them, so a OneOf is
// weighed where it would be (CV-045), and written out it is what that fold
// gives: CV-042 unifies collections element by element, tuples of one length
// position by position and objects field by field, and a pair written out
// between them would be canonical and the same, its parts being canonical
// and admitting some value, since the constraints given and every pair
// formed do. A constraint that meets the union by another rule, as a OneOf,
// Any's absence aside, or a tuple of another length does, is paired with the
// union written out.
type union struct {
	u       Constraint
	coll    *collectionUnion
	tuple   *tupleUnion
	objects *objectUnion
}

// add unifies c into the union, reporting whether it unifies.
func (a *union) add(un *unifier, c Constraint) bool {
	if c.c.kind == ConstraintAny {
		return true
	}
	s := spelledOut(c)
	switch {
	case a.coll != nil && a.coll.takes(s):
		return a.coll.add(un, s)
	case a.tuple != nil && s.c.kind == ConstraintTupleOf && len(s.c.members) == len(a.tuple.members):
		return a.tuple.add(un, s)
	case a.objects != nil && objectLike(c):
		return a.objects.add(un, s)
	}
	u := a.written(un)
	w := spelledOut(u)
	switch {
	case isCollectionOf(w.c.kind) && collectionsMeet(w.c.kind, s.c.kind):
		a.coll = &collectionUnion{kind: w.c.kind, elem: union{u: w.c.elem}}
		return a.coll.add(un, s)
	case w.c.kind == ConstraintTupleOf && s.c.kind == ConstraintTupleOf && len(w.c.members) == len(s.c.members):
		a.tuple = newTupleUnion(w)
		return a.tuple.add(un, s)
	case objectLike(u) && objectLike(c):
		a.objects = newObjectUnion(w)
		return a.objects.add(un, s)
	}
	var ok bool
	a.u, ok = un.pair(u, c)
	return ok
}

// written returns the union written out, canonically, and holds it so.
func (a *union) written(un *unifier) Constraint {
	switch {
	case a.coll != nil:
		a.u, a.coll = a.coll.constraint(un), nil
	case a.tuple != nil:
		a.u, a.tuple = a.tuple.constraint(un), nil
	case a.objects != nil:
		a.u, a.objects = a.objects.constraint(un), nil
	}
	return a.u
}

// isCollectionOf reports whether k is ListOf, SetOf or MapOf.
func isCollectionOf(k ConstraintKind) bool {
	return k == ConstraintListOf || k == ConstraintSetOf || k == ConstraintMapOf
}

// collectionsMeet reports whether CV-042 unifies collections of kinds a and b
// element by element: a list or a set with a list or a set, a map with a map.
func collectionsMeet(a, b ConstraintKind) bool {
	if a == ConstraintMapOf || b == ConstraintMapOf {
		return a == b
	}
	return isCollectionOf(a) && isCollectionOf(b)
}

// collectionUnion is the union of ListOf, SetOf or MapOf constraints: the
// union of their elements, a list where a list has met a set.
type collectionUnion struct {
	kind ConstraintKind
	elem union
}

// takes reports whether the union unifies the collection s element by
// element.
func (u *collectionUnion) takes(s Constraint) bool {
	return collectionsMeet(u.kind, s.c.kind)
}

// add unifies the collection s into the union.
func (u *collectionUnion) add(un *unifier, s Constraint) bool {
	if s.c.kind == ConstraintListOf {
		u.kind = ConstraintListOf
	}
	return u.elem.add(un, s.c.elem)
}

// constraint writes the union out, canonically.
func (u *collectionUnion) constraint(un *unifier) Constraint {
	return un.memo.canonical(elementConstraint(u.kind, u.elem.written(un)))
}

// tupleUnion is the union of TupleOf constraints of one length, position by
// position.
type tupleUnion struct {
	members []union
}

func newTupleUnion(first Constraint) *tupleUnion {
	u := &tupleUnion{members: make([]union, len(first.c.members))}
	for i, m := range first.c.members {
		u.members[i] = union{u: m}
	}
	return u
}

// add unifies the TupleOf s, of the union's length, into the union, position
// by position, stopping at the first that fails.
func (u *tupleUnion) add(un *unifier, s Constraint) bool {
	for i, m := range s.c.members {
		if !u.members[i].add(un, m) {
			return false
		}
	}
	return true
}

// constraint writes the union out, canonically.
func (u *tupleUnion) constraint(un *unifier) Constraint {
	members := make([]Constraint, len(u.members))
	for i := range u.members {
		members[i] = u.members[i].written(un)
	}
	return un.memo.canonical(Constraint{c: &constraintData{kind: ConstraintTupleOf, members: members}})
}

// objectUnion is the union of ObjectWith constraints: rule 7 of CV-042 gives a
// field for each name in either, its constraint the fields of that name
// unified, required where both require it, closed where both are. So a field
// is required where every object so far holds it and requires it, which a
// count tells, and one more object unifies its fields with the union's of the
// same names, in name order, stopping at the first that fails, which is what
// unifying the union written out with it pairs.
type objectUnion struct {
	fields  map[string]*unionField
	objects int
	closed  bool
}

// unionField is one field of an objectUnion.
type unionField struct {
	c        union
	required bool // required by every object that holds it
	held     int  // how many of the objects hold it
}

func newObjectUnion(first Constraint) *objectUnion {
	u := &objectUnion{fields: make(map[string]*unionField, len(first.c.fields)), objects: 1, closed: first.c.closed}
	for _, f := range first.c.fields {
		u.fields[f.name] = &unionField{c: union{u: f.Constraint}, required: f.Required, held: 1}
	}
	return u
}

// add unifies the ObjectWith constraint o into the union, reporting whether
// its fields unify with the union's.
func (u *objectUnion) add(un *unifier, o Constraint) bool {
	for _, f := range o.c.fields {
		have, ok := u.fields[f.name]
		if !ok {
			u.fields[f.name] = &unionField{c: union{u: f.Constraint}, required: f.Required, held: 1}
			continue
		}
		if !have.c.add(un, f.Constraint) {
			return false
		}
		have.required, have.held = have.required && f.Required, have.held+1
	}
	u.objects++
	u.closed = u.closed && o.c.closed
	return true
}

// constraint writes the union out, canonically.
func (u *objectUnion) constraint(un *unifier) Constraint {
	fields := make([]field, 0, len(u.fields))
	for name, f := range u.fields {
		fields = append(fields, field{name, Field{Constraint: f.c.written(un), Required: f.required && f.held == u.objects}})
	}
	slices.SortFunc(fields, func(a, b field) int { return strings.Compare(a.name, b.name) })
	return un.memo.canonical(Constraint{c: &constraintData{kind: ConstraintObjectWith, fields: fields, closed: u.closed}})
}

// unifyBound is the multiple of CV-045: the pairs a unification forms may
// weigh in all at most this many times the size of the constraints it is
// given.
const unifyBound = 64

// unifier holds what one Unify call has left to form: the weight of the pairs
// it may still form, whether a OneOf has been refused its pairs, the sizes it
// has measured, since a member is weighed again at every pair it joins, and
// the canonical forms it has written.
type unifier struct {
	p     Policy
	left  int64
	over  bool
	sizes map[*constraintData]int64
	types map[*typeData]int64
	memo  *canonMemo
}

func newUnifier(p Policy) *unifier {
	return &unifier{p: p, sizes: map[*constraintData]int64{}, types: map[*typeData]int64{}, memo: newCanonMemo()}
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
// as canonMemo.oneOf writes it.
func canonical(c Constraint) Constraint { return (*canonMemo)(nil).canonical(c) }

// canonMemo remembers, for one unification, the canonical form of each
// constraint it has written and the facts that form rests on: the type a
// constraint admits alone (soleType) and whether it admits none (admitsNone).
// Every pair a unification forms is written canonically, and each is made of
// parts written so already, so remembering them makes that the work of the
// pair alone, where it was the work of everything within it: unifying list
// constraints nested 800 deep wrote each level out again from the bottom.
// A conversion keeps one for the soleType it asks (convertMemo). A nil
// *canonMemo remembers nothing.
type canonMemo struct {
	forms map[*constraintData]Constraint
	sole  map[*constraintData]soleResult
	none  map[*constraintData]bool
}

// soleResult is what soleType gives.
type soleResult struct {
	t  Type
	ok bool
}

func newCanonMemo() *canonMemo {
	return &canonMemo{
		forms: map[*constraintData]Constraint{},
		sole:  map[*constraintData]soleResult{},
		none:  map[*constraintData]bool{},
	}
}

// canonical is canonical, remembered in m where m is not nil, and a
// constraint it gives is remembered as its own canonical form.
func (m *canonMemo) canonical(c Constraint) Constraint {
	if m != nil {
		if r, ok := m.forms[c.c]; ok {
			return r
		}
	}
	r := m.canonicalOf(c)
	if m != nil {
		m.forms[c.c], m.forms[r.c] = r, r
	}
	return r
}

// canonicalOf writes c canonically, asking m of its parts.
func (m *canonMemo) canonicalOf(c Constraint) Constraint {
	if m.admitsNone(c) {
		return Constraint{c: &constraintData{kind: ConstraintOneOf}}
	}
	if t, ok := m.soleType(c); ok {
		return Exactly(t)
	}
	d := c.c
	switch d.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return elementConstraint(d.kind, m.canonical(d.elem))
	case ConstraintTupleOf:
		members := make([]Constraint, len(d.members))
		for i, member := range d.members {
			members[i] = m.canonical(member)
		}
		return Constraint{c: &constraintData{kind: ConstraintTupleOf, members: members}}
	case ConstraintObjectWith:
		var fields []field
		for _, f := range d.fields {
			if !f.Required && m.admitsNone(f.Constraint) {
				continue
			}
			fields = append(fields, field{f.name, Field{Constraint: m.canonical(f.Constraint), Required: f.Required}})
		}
		return Constraint{c: &constraintData{kind: ConstraintObjectWith, fields: fields, closed: d.closed}}
	case ConstraintOneOf:
		return m.oneOf(d.members)
	}
	return c
}

// oneOf returns the OneOf of members written canonically: members that are
// OneOf are flattened into it, identical members appear once, in canonical
// order, and a lone member stands for the OneOf. m, which may be nil, gives
// the members' canonical forms.
func (m *canonMemo) oneOf(members []Constraint) Constraint {
	var flat []Constraint
	for _, member := range members {
		if cm := m.canonical(member); cm.c.kind == ConstraintOneOf {
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
	return Constraint{c: &constraintData{kind: ConstraintOneOf, members: flat}}
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
		return un.memo.oneOf(out), true
	}
	r, ok := un.structures(spelledOut(a), spelledOut(b))
	if !ok {
		return Constraint{}, false
	}
	return un.memo.canonical(r), true
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
	return un.fold(Any(), ordered)
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
	return Constraint{c: &constraintData{kind: ConstraintTupleOf, members: members}}, true
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
	return Constraint{c: &constraintData{kind: ConstraintObjectWith, fields: fields, closed: x.c.closed && y.c.closed}}, true
}
