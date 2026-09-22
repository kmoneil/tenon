package tenon

// Length returns how many members a value has, as a Number value: the count of
// extended grapheme clusters of a String, the elements of a list, the members
// of a set or the entries of a map. Those are the kinds a length narrowing
// applies to; a tuple and an object have the length their type gives them, and
// asking this for one is a mistake in the calling program, as is asking it for
// a scalar.
//
// A set whose members are not all known has a length that is a range rather
// than a count: as few as the members that are provably distinct and as many as
// all of them. Every other container has the length it has, whether or not its
// members are known.
//
// A null operand gives an error value, since null has no members, and an error
// operand carries forward.
func Length(v Value) Value { return lengthOp.apply(v) }

// Contains returns whether v is a member of a set, as a Bool value. It is known
// true where a member is provably v, known false where every member is provably
// not v, and unknown in between, which is what a set holding members that are
// not known leaves. Membership is equality, so v may be pending and still
// settle it: a pending value known to be null is no member of a set whose
// members cannot be null.
//
// An unknown set answers from its range: a member recorded there by the
// Members narrowing settles containment as a held member would, once the
// range excludes null, because were the set to turn out null the answer
// would be an error rather than true.
//
// The value looked for may be of any type: a value of another type than the
// set's members is simply not one of them. It may also be null, which is a
// member like any other. A null set gives an error value, since null holds
// nothing, and an error operand carries forward.
func Contains(set, v Value) Value { return containsOp.apply(set, v) }

// lengthOperand is what Length accepts: the kinds that have a length.
var lengthOperand = OneOf(
	Exactly(Type{stringType}),
	ListOf(Any()),
	SetOf(Any()),
	MapOf(Any()),
)

var lengthOp = register(&op{
	name:     "Length",
	operands: alike(1, lengthOperand, false),
	result:   fixedResult(Type{numberType}),
	known:    func(args []Value) Value { return NumberFromInt(args[0].n.length()) },
	decided: func(args []Value) (Value, bool) {
		// A container that holds its members has the number of them it holds,
		// known or not, unless it is a set, where two of them may turn out to
		// be one member.
		n := args[0].n
		if n.state == stateKnown && n.typ.t.kind != KindSet {
			return NumberFromInt(n.length()), true
		}
		return Value{}, false
	},
	narrow: func(args []Value, r Value) Value {
		// However little is known, a length is a count: never negative, and
		// bounded by whatever the operand's own range or members say.
		ns := []Narrowing{NumberMin(NumberFromInt(0), true)}
		switch n := args[0].n; {
		case n.state == stateKnown:
			low, high := setLengthBounds(n.typ, n.data.([]Value))
			ns = append(ns,
				NumberMin(NumberFromInt(int64(low)), true),
				NumberMax(NumberFromInt(int64(high)), true))
		case n.state == stateUnknown:
			rd := n.data.(*rangeData)
			if rd.lenLo > 0 {
				ns = append(ns, NumberMin(NumberFromInt(rd.lenLo), true))
			}
			// A set of an element type holding few values is no longer than
			// the values it could hold, whether or not its range says so.
			hi := setCeiling(n.typ)
			if rd.lenHi.set && (!hi.set || rd.lenHi.n < hi.n) {
				hi = rd.lenHi
			}
			if hi.set {
				ns = append(ns, NumberMax(NumberFromInt(hi.n), true))
			}
		}
		return Narrow(r, ns...)
	},
})

// setLengthBounds returns how few and how many members a set of type t holding
// these members could turn out to have: the members that are provably distinct
// from every member counted before them, and all of them, or as many as the
// element type has values, null among them, where it has fewer.
func setLengthBounds(t Type, members []Value) (low, high int) {
	high = len(members)
	// Every element type has at least one value, so it bounds nothing until a
	// set holds two members, and equality asks this of every set it compares.
	if high > 1 {
		if c := setCeiling(t); c.set && c.n < int64(high) {
			high = int(c.n)
		}
	}
	return provablyDistinct(members), high
}

// provablyDistinct returns how many of these members are provably distinct
// from every member counted before them, taken in the order they are held,
// which is canonical for a set and for the members recorded in a range alike.
//
// Two known members are told apart by being unequal, and that order holds them
// first and ties exactly the ones that are equal (EQ-045), so a known member
// is one already counted exactly when it is the one counted last. A member
// that is not known is compared with every member counted, since what tells it
// apart is what it could still turn out to be.
func provablyDistinct(members []Value) int {
	var known, others []Value
	for _, m := range members {
		if !m.n.isKnown() {
			if distinctFromAll(known, m) && distinctFromAll(others, m) {
				others = append(others, m)
			}
			continue
		}
		if last := len(known) - 1; last >= 0 && sameValue(known[last].n, m.n) {
			continue
		}
		known = append(known, m)
	}
	return len(known) + len(others)
}

// distinctFromAll reports whether equality settles that m differs from every
// one of these members.
func distinctFromAll(members []Value, m Value) bool {
	for _, k := range members {
		if eq, settled := equality(k.n, m.n); !settled || eq {
			return false
		}
	}
	return true
}

var containsOp = register(&op{
	name: "Contains",
	operands: []operand{
		{constraint: SetOf(Any())},
		{constraint: Any(), nulls: true, within: true},
	},
	result: fixedResult(Type{boolType}),
	known: func(args []Value) Value {
		found, settled := membership(args[0].n, args[1])
		if !settled {
			// Every member of a known set is known, and equality settles every
			// pair of those, so this cannot happen. The framework catches it
			// rather than letting a false answer through.
			return unknownBool
		}
		return Bool(found)
	},
	decided: func(args []Value) (Value, bool) {
		switch set := args[0].n; set.state {
		case stateKnown:
			if found, settled := membership(set, args[1]); settled {
				return Bool(found), true
			}
		case stateUnknown:
			// A member recorded in the set's range settles membership as a
			// held member would, once null is excluded: a set that could
			// still turn out null could still have an error for an answer.
			if rd := set.data.(*rangeData); rd.null == nullNo {
				for _, m := range rd.members {
					if eq, settled := equality(m.n, args[1].n); settled && eq {
						return Bool(true), true
					}
				}
			}
		}
		return Value{}, false
	},
})

// membership says whether v is a member of the set, and whether that is
// settled: a member that is provably v settles it, and so does every member
// being provably not v. Anything else leaves it open.
func membership(set *node, v Value) (found, settled bool) {
	return among(set.data.([]Value), v)
}

// among says whether v is one of these members, and whether that is settled.
func among(members []Value, v Value) (found, settled bool) {
	settled = true
	for _, m := range members {
		switch eq, ok := equality(m.n, v.n); {
		case ok && eq:
			return true, true
		case !ok:
			settled = false
		}
	}
	return false, settled
}

// knownMembers returns how many of a set's members are known, which are the
// ones it holds first (EQ-044).
func knownMembers(members []Value) int {
	n := 0
	for n < len(members) && members[n].n.isKnown() {
		n++
	}
	return n
}

// memberIndex answers membership of one set for value after value. Its known
// members are bucketed by hash, since a known value is equal to a known
// member only where their hashes agree (EQ-030), as a set's own construction
// tells them apart; the rest are kept as they are, since what tells a member
// that is not known apart from a value is what it could still turn out to be,
// which no hash holds.
type memberIndex struct {
	known   []Value
	buckets map[uint64][]Value
	rest    []Value
}

// indexMembers indexes the members of a known set.
func indexMembers(set *node) memberIndex {
	members := set.data.([]Value)
	k := knownMembers(members)
	x := memberIndex{known: members[:k], rest: members[k:]}
	x.buckets = make(map[uint64][]Value, len(x.known))
	for _, m := range x.known {
		h := hashNode(m.n)
		x.buckets[h] = append(x.buckets[h], m)
	}
	return x
}

// membership says whether v is a member of the set indexed, and gives the
// answer membership gives.
func (x memberIndex) membership(v Value) (found, settled bool) {
	candidates := x.known
	if v.n.isKnown() {
		// Equal known values have equal hashes, so the known members in other
		// buckets are provably not v and say nothing about the answer.
		candidates = x.buckets[hashNode(v.n)]
	}
	if found, settled = among(candidates, v); found {
		return true, true
	}
	restFound, restSettled := among(x.rest, v)
	return restFound, settled && restSettled
}
