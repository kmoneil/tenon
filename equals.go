package tenon

import (
	"slices"
	"strings"

	"github.com/kmoneil/tenon/internal/decimal"
)

// Equals returns whether a and b are the same value, as a Bool value. Values of
// different types are never equal, which is an answer and not a mistake, and
// null is a value like any other: it equals null of its own type and nothing
// else.
//
// Where an operand is not known, the answer is known false if the two cannot
// meet whatever they turn out to be, known true if both are the same known
// value, and an unknown Bool otherwise. A pending operand counts for what it
// already says. One known to be null never equals a value that cannot be
// null, and equals the null of the one type its constraint admits. Two
// operands that can never have one type between them, as a pending list and a
// pending set cannot, are never equal. An error operand gives an error value.
//
// Equals compares what values are rather than how they are held: a number
// written two ways is one number, and a string is compared in the normalized
// form every string value has.
func Equals(a, b Value) Value { return equalsOp.apply(a, b) }

var equalsOp = register(&op{
	name:     "Equals",
	operands: reading(alike(2, Any(), true)),
	result:   fixedResult(Type{boolType}),
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
})

// equality says whether a and b are the same value, and whether that is
// settled. It answers only where the answer cannot change: what it leaves open
// is reported as an unknown Bool.
//
// A pending operand has no type yet, but it may already say enough. Null
// equals null and nothing else, so a value known to be null differs from one
// that cannot be null whatever types the two turn out to have; and where the
// constraints of the two share no type, a type in hand counting as the
// constraint that names it, they will never have one type.
//
// Null is decided here and nowhere else. Two operands that could both still
// be null could both turn out to be null, which is one value, so nothing they
// say about the values they hold otherwise can settle the answer.
func equality(a, b *node) (eq, settled bool) {
	if a.state == stateError || b.state == stateError {
		// An error value has no type and no value, and settles nothing.
		return false, false
	}
	nullA, nullB := knownNull(a), knownNull(b)
	if nullA && !mayBeNull(b) || nullB && !mayBeNull(a) {
		return false, true
	}
	ta, oka := settledType(a)
	tb, okb := settledType(b)
	switch {
	case !oka || !okb:
		// A type that is not settled could still turn out to be the other's,
		// unless no type satisfies what both say of theirs.
		_, share := sharedType(constraintOf(a), constraintOf(b))
		return false, !share
	case ta != tb:
		return false, true
	case nullA && nullB:
		// Two nulls of one type are one value, whether or not either has
		// resolved to it yet.
		return true, true
	case a.isKnown() && b.isKnown():
		return sameValue(a, b), true
	case mayBeNull(a) && mayBeNull(b):
		// Both could still be null, and two nulls of one type are the same
		// value. What the two say about anything else rules nothing out.
		return false, false
	case disjoint(a, b):
		return false, true
	}
	return false, false
}

// knownNull reports whether n is known to be null: the null value of a type,
// or a pending value whose nullness fact says it will be one.
func knownNull(n *node) bool {
	return n.state == stateNull || n.state == statePending && n.null == nullOnly
}

// settledType returns the type that a value has or will have, and whether it
// has one. A pending value has one where its constraint admits exactly one
// type, however the constraint is written.
func settledType(n *node) (Type, bool) {
	switch n.state {
	case stateError:
		// An error value has no type and never will have one.
		return Type{}, false
	case statePending:
		return soleType(n.data.(Constraint))
	}
	return n.typ, true
}

// constraintOf returns the constraint that the type of n satisfies: the one a
// pending value carries, and Exactly of its type for any other value but an
// error value, which has no type and is never asked about.
func constraintOf(n *node) Constraint {
	if n.state == statePending {
		return n.data.(Constraint)
	}
	return Exactly(n.typ)
}

// sameValue reports whether two known values of one type are the same value.
// Marks, when they arrive, are not part of this: equality compares values, not
// what has been attached to them.
func sameValue(a, b *node) bool {
	if a == b {
		// One node is one known value, which is equal to itself, so nothing
		// within it is compared: not for a value compared with itself, and not
		// for a part two values share, as a planned tree shares with the prior
		// one every part that did not change. A capsule type's equality is an
		// equivalence relation (TY-041), so this is the answer it would give.
		return true
	}
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

// sameMembers reports whether two known sets hold the same members. A set
// holds its members in the canonical order, which ties exactly the ones that
// are equal (EQ-045), and holds each of them once, so two sets with the same
// members hold them in the same order and one walk decides it, as
// compareContent's walk of the same members already does. Membership each way
// would compare every pair.
func sameMembers(x, y []Value) bool {
	return slices.EqualFunc(x, y, func(a, b Value) bool { return sameValue(a.n, b.n) })
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
//
// Null is settled before this is called: equality, its only caller, has
// already answered both the operands that could each still be null and the
// null operand facing one that cannot be null. So neither operand here is
// null, and at most one of them could still become it.
func disjoint(a, b *node) bool {
	ra, oka := unknownRange(a)
	rb, okb := unknownRange(b)
	switch {
	case oka && okb:
		return rangesDisjoint(ra, rb)
	case oka && b.isKnown():
		return !ra.holds(b)
	case okb && a.isKnown():
		return !rb.holds(a)
	case oka:
		// The other is a container holding a member that is not known, or is
		// pending, which has no members to compare.
		return b.state == stateKnown && ra.excludesPartial(b)
	case okb:
		return a.state == stateKnown && rb.excludesPartial(a)
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
	// A set in the range holds every recorded member, so a set that provably
	// does not hold one of them is outside it.
	for _, m := range r.members {
		if found, settled := membership(n, m); settled && !found {
			return false
		}
	}
	return true
}

// excludesPartial reports whether r rules out the container n, which holds a
// member that is not known: every length n could have lies outside the
// lengths r allows, or r records a member that n provably does not hold. A
// range says nothing about the members of a tuple or an object that a member
// could contradict.
func (r *rangeData) excludesPartial(n *node) bool {
	var lo, hi int
	switch n.typ.t.kind {
	case KindList:
		lo = len(n.data.([]Value))
		hi = lo
	case KindMap:
		lo = len(n.data.([]mapEntry))
		hi = lo
	case KindSet:
		lo, hi = setLengthBounds(n)
	default:
		return false
	}
	if int64(hi) < r.lenLo || r.lenHi.set && int64(lo) > r.lenHi.n {
		return true
	}
	return lacksSome(n, r.members)
}

// lacksSome reports whether some of these values is provably not a member of
// the set. The set's members are indexed once, since every value asks the
// same set the same question.
func lacksSome(set *node, values []Value) bool {
	if len(values) == 0 {
		return false
	}
	held := indexMembers(set.data.([]Value))
	for _, v := range values {
		if found, settled := held.membership(v); settled && !found {
			return true
		}
	}
	return false
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
	// At most one of these ranges still holds null, since equality settles two
	// that both do. That one can meet the other anywhere else, so null rules
	// nothing out here.
	return false
}

// membersDisjoint reports whether two containers cannot be the same value
// because of what their members already say: a different number of them, or a
// pair that cannot be equal. A set holding members that are not known has a
// length that is a range, and two sets differ where those ranges cannot meet
// or where a member of one is provably no member of the other.
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
	case KindSet:
		// One of the two holds members that are not known, two known sets
		// having been settled by their values already. Where the other is
		// known, the matching decides it: its members must be what the members
		// of the one could turn out to be, each its own.
		switch {
		case !a.partial:
			return !couldEqual(a, b)
		case !b.partial:
			return !couldEqual(b, a)
		}
		x, y := a.data.([]Value), b.data.([]Value)
		xlo, xhi := setLengthBounds(a)
		ylo, yhi := setLengthBounds(b)
		return xhi < ylo || yhi < xlo || lacksSome(b, x) || lacksSome(a, y)
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
