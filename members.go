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
// not known leaves.
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

var lengthOp = &op{
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
			low, high := setLengthBounds(n.data.([]Value))
			ns = append(ns,
				NumberMin(NumberFromInt(int64(low)), true),
				NumberMax(NumberFromInt(int64(high)), true))
		case n.state == stateUnknown:
			rd := n.data.(*rangeData)
			if rd.lenLo > 0 {
				ns = append(ns, NumberMin(NumberFromInt(rd.lenLo), true))
			}
			if rd.lenHi.set {
				ns = append(ns, NumberMax(NumberFromInt(rd.lenHi.n), true))
			}
		}
		return Narrow(r, ns...)
	},
}

// setLengthBounds returns how few and how many members a set could turn out to
// have: the members that are provably distinct from every member counted before
// them, and all of them.
func setLengthBounds(members []Value) (low, high int) {
	return provablyDistinct(members), len(members)
}

// provablyDistinct returns how many of these members are provably distinct
// from every member counted before them, taken in the order they are held,
// which is canonical for a set and for the members recorded in a range alike.
func provablyDistinct(members []Value) int {
	var counted []Value
	for _, m := range members {
		if distinctFromAll(counted, m) {
			counted = append(counted, m)
		}
	}
	return len(counted)
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

var containsOp = &op{
	name: "Contains",
	operands: []operand{
		{constraint: SetOf(Any())},
		{constraint: Any(), nulls: true},
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
}

// membership says whether v is a member of the set, and whether that is
// settled: a member that is provably v settles it, and so does every member
// being provably not v. Anything else leaves it open.
func membership(set *node, v Value) (found, settled bool) {
	settled = true
	for _, m := range set.data.([]Value) {
		switch eq, ok := equality(m.n, v.n); {
		case ok && eq:
			return true, true
		case !ok:
			settled = false
		}
	}
	return false, settled
}
