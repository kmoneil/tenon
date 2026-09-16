package tenon

import (
	"slices"
	"strings"

	"tenon/internal/decimal"
)

// Equals returns whether a and b are the same value, as a Bool value. Values of
// different types are never equal, which is an answer and not a mistake, and
// null is a value like any other: it equals null of its own type and nothing
// else.
//
// Where an operand is not known, the answer is known false if the two cannot
// meet whatever they turn out to be, known true if both are the same known
// value, and an unknown Bool otherwise. An error operand gives an error value.
//
// Equals compares what values are rather than how they are held: a number
// written two ways is one number, and a string is compared in the normalized
// form every string value has.
func Equals(a, b Value) Value { return equalsOp.apply(a, b) }

var equalsOp = &op{
	name:    "Equals",
	operand: Any(),
	nulls:   true,
	result:  fixedResult(Type{boolType}),
	known: func(args []Value) Value {
		eq, settled := equality(args[0].n, args[1].n)
		if !settled {
			// Two known values always settle it. Leaving the answer open is a
			// defect here, and the framework catches it rather than letting a
			// false answer through.
			return unknownBool
		}
		return Bool(eq)
	},
	decided: func(args []Value) (Value, bool) {
		if eq, settled := equality(args[0].n, args[1].n); settled {
			return Bool(eq), true
		}
		return Value{}, false
	},
}

// equality says whether a and b are the same value, and whether that is
// settled. It answers only where the answer cannot change: what it leaves open
// is reported as an unknown Bool.
func equality(a, b *node) (eq, settled bool) {
	ta, oka := settledType(a)
	tb, okb := settledType(b)
	switch {
	case !oka || !okb:
		// An operand that could still be of any type settles nothing.
		return false, false
	case ta != tb:
		return false, true
	case a.isKnown() && b.isKnown():
		return sameValue(a, b), true
	case disjoint(a, b):
		return false, true
	}
	return false, false
}

// settledType returns the type that a value has or will have, and whether it
// has one. A pending value has one only where its constraint names it.
func settledType(n *node) (Type, bool) {
	switch n.state {
	case stateError:
		// An error value has no type and never will have one.
		return Type{}, false
	case statePending:
		if c := n.data.(Constraint); c.Kind() == ConstraintExactly {
			return c.Type(), true
		}
		return Type{}, false
	}
	return n.typ, true
}

// sameValue reports whether two known values of one type are the same value.
// Marks, when they arrive, are not part of this: equality compares values, not
// what has been attached to them.
func sameValue(a, b *node) bool {
	if a.state == stateNull || b.state == stateNull {
		return a.state == b.state
	}
	switch a.typ.t.kind {
	case KindBool:
		return a.data.(bool) == b.data.(bool)
	case KindNumber:
		return a.data.(decimal.Dec).Equal(b.data.(decimal.Dec))
	case KindString:
		return a.data.(string) == b.data.(string)
	case KindCapsule:
		return a.typ.t.capsule.equal(a.data, b.data)
	case KindSet:
		return sameMembers(a.data.([]Value), b.data.([]Value))
	case KindMap:
		return sameEntries(a.data.([]mapEntry), b.data.([]mapEntry))
	}
	// Lists, tuples and objects line up member by member. The type fixes the
	// order for the last two, and a list of another length is another value.
	x, y := a.data.([]Value), b.data.([]Value)
	return slices.EqualFunc(x, y, func(p, q Value) bool { return sameValue(p.n, q.n) })
}

// sameMembers reports whether two sets hold the same members. Until a set drops
// the members that equal one another, one set can be held as different slices,
// in different orders and with repetitions, so the comparison is membership
// each way rather than position by position.
func sameMembers(x, y []Value) bool {
	return sameMembersFunc(x, y, func(a, b Value) bool { return sameValue(a.n, b.n) })
}

// sameMembersFunc reports whether every member of each set is a member of the
// other, by whichever comparison the caller holds members to. That comparison
// must be an equivalence relation, or this is not one either.
func sameMembersFunc(x, y []Value, same func(a, b Value) bool) bool {
	return holdsEvery(x, y, same) && holdsEvery(y, x, same)
}

// holdsEvery reports whether every member of want matches some member of have.
func holdsEvery(have, want []Value, same func(a, b Value) bool) bool {
	for _, w := range want {
		if !slices.ContainsFunc(have, func(h Value) bool { return same(h, w) }) {
			return false
		}
	}
	return true
}

// sameEntries reports whether two maps hold the same entries. Entries are kept
// in key order, so one pass compares them and nothing depends on the order a Go
// map would have given.
func sameEntries(x, y []mapEntry) bool {
	return slices.EqualFunc(x, y, func(p, q mapEntry) bool {
		return p.key == q.key && sameValue(p.val.n, q.val.n)
	})
}

// disjoint reports whether nothing in the range of a is in the range of b, so
// that the two cannot be the same value however they settle. It answers false
// whenever it cannot tell.
func disjoint(a, b *node) bool {
	// Null is a value like any other: either the other could be null too, or
	// the two can never meet.
	if a.state == stateNull || b.state == stateNull {
		other := b
		if b.state == stateNull {
			other = a
		}
		return !mayBeNull(other)
	}
	ra, oka := unknownRange(a)
	rb, okb := unknownRange(b)
	switch {
	case oka && okb:
		return rangesDisjoint(ra, rb)
	case oka && b.isKnown():
		return !ra.holds(b)
	case okb && a.isKnown():
		return !rb.holds(a)
	case oka || okb:
		// One carries a range while the other is a container holding a member
		// that is not known, or is pending. Nothing lines up to compare.
		return false
	}
	return membersDisjoint(a, b)
}

// unknownRange returns the range of an unknown value, and whether it has one.
func unknownRange(n *node) (*rangeData, bool) {
	if n.state == stateUnknown {
		return n.data.(*rangeData), true
	}
	return nil, false
}

// mayBeNull reports whether null is still one of the things a value could be.
func mayBeNull(n *node) bool {
	switch n.state {
	case stateNull:
		return true
	case stateUnknown:
		return n.data.(*rangeData).null != nullNo
	case statePending:
		return n.null != nullNo
	}
	return false
}

// holds reports whether the known value n is one of the values that r describes.
func (r *rangeData) holds(n *node) bool {
	if n.state == stateNull {
		return r.null != nullNo
	}
	if r.null == nullOnly {
		return false
	}
	switch n.typ.t.kind {
	case KindNumber:
		d := n.data.(decimal.Dec)
		if !r.lo.holdsLower(d) || !r.hi.holdsUpper(d) {
			return false
		}
	case KindString:
		if !strings.HasPrefix(n.data.(string), r.pfx) {
			return false
		}
	}
	if r.lenLo > 0 || r.lenHi.set {
		length := n.length()
		if length < r.lenLo || (r.lenHi.set && length > r.lenHi.n) {
			return false
		}
	}
	return true
}

// rangesDisjoint reports whether no value at all is in both ranges.
func rangesDisjoint(a, b *rangeData) bool {
	switch {
	case crosses(a.lo, b.hi), crosses(b.lo, a.hi):
		// Number bounds that do not overlap.
		return true
	case !strings.HasPrefix(a.pfx, b.pfx) && !strings.HasPrefix(b.pfx, a.pfx):
		// Prefixes that diverge: no string begins with both.
		return true
	case a.lenHi.set && a.lenHi.n < b.lenLo, b.lenHi.set && b.lenHi.n < a.lenLo:
		// Lengths that do not overlap.
		return true
	}
	// Null is no help here: a range that holds it and a range that does not can
	// still meet anywhere else.
	return false
}

// membersDisjoint reports whether two containers cannot be the same value
// because of what their members already say: a different number of them, or a
// pair that cannot be equal.
//
// Sets are left out until the members that equal one another are dropped: two
// sets of different lengths may still be the same set.
func membersDisjoint(a, b *node) bool {
	if a.state != stateKnown || b.state != stateKnown {
		return false // A pending value has no members to compare.
	}
	switch a.typ.t.kind {
	case KindList:
		x, y := a.data.([]Value), b.data.([]Value)
		return len(x) != len(y) || anyDisjoint(x, y)
	case KindTuple, KindObject:
		// The type fixes how many there are and what order they are in.
		return anyDisjoint(a.data.([]Value), b.data.([]Value))
	case KindMap:
		x, y := a.data.([]mapEntry), b.data.([]mapEntry)
		if len(x) != len(y) {
			return true
		}
		for i := range x {
			if x[i].key != y[i].key {
				return true
			}
			if eq, settled := equality(x[i].val.n, y[i].val.n); settled && !eq {
				return true
			}
		}
		return false
	}
	return false
}

// anyDisjoint reports whether some pair of members settles that two containers
// hold different values.
func anyDisjoint(x, y []Value) bool {
	for i := range x {
		if eq, settled := equality(x[i].n, y[i].n); settled && !eq {
			return true
		}
	}
	return false
}
