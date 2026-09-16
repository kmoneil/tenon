package tenon

import "slices"

// Identical reports whether a and b are the same value in every respect the
// value system holds: the same state, the same type, the same range, the same
// diagnostics, the same marks. It is the host language's comparison, not the
// language's own: it answers with a plain bool, never unknown and never an
// error, and two unknown values with the same range are identical though
// comparing them with Equals cannot say. Equals ignores marks; Identical
// does not, so a marked value and its unmarked twin are two things here.
//
// It compares values rather than representations, so a number written two ways
// is identical to itself, a string is compared in the normalized form every
// string value has, and a set is its members however they were given. Nothing
// it does depends on the order a Go map would iterate in.
//
// Identical panics only on the zero Value, which is not a value.
func Identical(a, b Value) bool {
	na, nb := a.data(), b.data()
	if na == nb {
		// One node is the same value as itself, which saves the walk.
		return true
	}
	if na.state != nb.state || !sameMarks(na, nb) {
		return false
	}
	switch na.state {
	case stateError:
		x, y := na.data.([]Diagnostic), nb.data.([]Diagnostic)
		return slices.EqualFunc(x, y, Diagnostic.Equal)
	case statePending:
		// A pending value is its constraint and what it says about null.
		return na.null == nb.null && na.data.(Constraint).equal(nb.data.(Constraint))
	}
	if na.typ != nb.typ {
		return false
	}
	switch na.state {
	case stateNull:
		return true
	case stateUnknown:
		return na.data.(*rangeData).equal(nb.data.(*rangeData))
	}
	return identicalContent(na, nb)
}

// identicalContent reports whether two known values of one type hold the same
// thing. A member that is not known is compared as a value in its own right,
// by its range, which is what makes the whole comparison an equivalence
// relation over every state a member can be in.
func identicalContent(a, b *node) bool {
	switch a.typ.t.kind {
	case KindSet:
		// A set can hold one unknown member twice (EQ-041), and holding it
		// twice is a different range from holding it once, so members are
		// matched with their repetitions, not only as a set.
		return sameMultiset(a.data.([]Value), b.data.([]Value))
	case KindMap:
		x, y := a.data.([]mapEntry), b.data.([]mapEntry)
		return slices.EqualFunc(x, y, func(p, q mapEntry) bool {
			return p.key == q.key && Identical(p.val, q.val)
		})
	case KindList, KindTuple, KindObject:
		x, y := a.data.([]Value), b.data.([]Value)
		return slices.EqualFunc(x, y, Identical)
	}
	// A scalar and a capsule are the same value or they are not, and for a
	// capsule that is what the capsule type says it is.
	return sameValue(a, b)
}

// sameMarks reports whether two values carry the same marks. Marks are a
// set: what is there matters, the order it was attached in does not.
func sameMarks(a, b *node) bool {
	x, y := a.markList(), b.markList()
	if len(x) != len(y) {
		return false
	}
	for _, m := range x {
		if !slices.Contains(y, m) {
			return false
		}
	}
	return true
}

// equal reports whether two constraints are the same constraint. Constraints
// are not interned, so this walks them.
func (c Constraint) equal(d Constraint) bool {
	a, b := c.data(), d.data()
	if a == b {
		return true
	}
	if a.kind != b.kind || a.typ != b.typ || a.closed != b.closed {
		return false
	}
	if (a.elem.c == nil) != (b.elem.c == nil) {
		return false
	}
	if a.elem.c != nil && !a.elem.equal(b.elem) {
		return false
	}
	if !slices.EqualFunc(a.members, b.members, Constraint.equal) {
		return false
	}
	return slices.EqualFunc(a.fields, b.fields, func(x, y field) bool {
		return x.name == y.name && x.Required == y.Required && x.Constraint.equal(y.Constraint)
	})
}

// sameMultiset reports whether two sets hold identical members, each as many
// times in one as in the other.
func sameMultiset(x, y []Value) bool {
	if len(x) != len(y) {
		return false
	}
	count := func(members []Value, m Value) int {
		n := 0
		for _, k := range members {
			if Identical(k, m) {
				n++
			}
		}
		return n
	}
	for _, m := range x {
		if count(x, m) != count(y, m) {
			return false
		}
	}
	return true
}
