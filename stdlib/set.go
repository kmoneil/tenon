package stdlib

import (
	"github.com/kmoneil/tenon"
)

// needleFor returns the value v looked for among members of the type t:
// converted to t under the safe policy where it converts, as a tuple to a
// list or a null not typed yet to t's null, and as it is where it does not,
// so a number is never found among strings.
func needleFor(v tenon.Value, t tenon.Type) tenon.Value {
	if c := tenon.Convert(v, tenon.Exactly(t), tenon.Safe); !c.IsError() {
		return c
	}
	return v
}

// found returns whether v is among the members ms, each compared with the
// needle converted to its type: true where one is provably v, false where
// none can be, and unknown otherwise.
func found(ms []tenon.Value, v tenon.Value) tenon.Value {
	answer := tenon.Bool(false)
	var t tenon.Type
	var needle tenon.Value
	for _, m := range ms {
		n := v
		if !m.IsPending() {
			if !m.Type().Equal(t) {
				t, needle = m.Type(), needleFor(v, m.Type())
			}
			n = needle
		}
		switch e := tenon.Equals(m, n); {
		case !e.IsKnown():
			answer = tenon.Unknown(tenon.BoolType())
		case e.AsBool():
			return tenon.Bool(true)
		}
	}
	return answer
}

// ContainsFunc reports whether a list, a tuple or a set holds a value: true
// where a member is provably equal to it, false where every member provably
// is not, and unknown otherwise. The value is converted to the type of the
// members under the safe policy where it converts, whatever the call's, so
// a tuple is found among lists and a language's untyped null among nulls,
// and is compared as it is where it does not, so a number is never found
// among strings. An empty collection holds nothing, whatever the value; one
// not known yet answers from its lengths and its members' type. go-cty's
// answers an untyped null with an unknown of no type (#221), and never
// finds a tuple in a list of lists.
var ContainsFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Contains",
	Description: "Returns true if the given value is a member of the given list, tuple or set.",
	Params: []tenon.Param{
		{Name: "list", Description: "The list, tuple or set.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
		{Name: "value", Description: "The value to look for.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if !mayBe(args[0], tenon.KindList, tenon.KindTuple, tenon.KindSet) {
			return tenon.Constraint{}, wrongKind("Contains", 0, args[0], "a list, a tuple or a set")
		}
		return boolean, nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return contains(args[0], args[1]), nil
	},
})

// contains answers Contains of the collection c and the value v.
func contains(c, v tenon.Value) tenon.Value {
	k, resolved := kindOf(c)
	switch {
	case resolved && k == tenon.KindSet:
		return tenon.Contains(c, needleFor(v, c.Type().ElementType()))
	case c.HasMembers():
		return found(c.Elements(), v)
	case !resolved:
		return tenon.Unknown(tenon.BoolType())
	}
	if _, hi, bounded := lengthOf(c); bounded && hi == 0 {
		return tenon.Bool(false)
	}
	// Not known yet, its members may be any of their type's values: the
	// value is found among them only where it may be of that type.
	var ms []tenon.Value
	if k == tenon.KindTuple {
		for _, t := range c.Type().TupleElementTypes() {
			ms = append(ms, tenon.Unknown(t))
		}
	} else {
		ms = []tenon.Value{tenon.Unknown(c.Type().ElementType())}
	}
	if f := found(ms, v); f.IsKnown() && !f.AsBool() {
		return f
	}
	return tenon.Unknown(tenon.BoolType())
}

// SetHasElementFunc reports whether a set holds a value, as ContainsFunc
// does: the value converted to the members' type under the safe policy
// where it converts. The set is read as the set operations read theirs: a
// list or a tuple converts to one under the call's policy.
var SetHasElementFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "SetHasElement",
	Description: "Returns true if the given set contains the given element.",
	Params: []tenon.Param{
		{Name: "set", Description: "The set.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
		{Name: "elem", Description: "The value to look for.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true},
	},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		if _, _, err := asSet("SetHasElement", 0, args[0], p); err != nil {
			return tenon.Constraint{}, err
		}
		return boolean, nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		s, empty, _ := asSet("SetHasElement", 0, args[0], p)
		if empty {
			return tenon.Bool(false), nil
		}
		return contains(s, args[1]), nil
	},
})

// asSet returns argument i of a function taking sets, unmarked: a set as it
// is, and a list or a tuple converted to a set under the policy p, as a
// language converts one where a set is wanted, so the safe policy refuses
// it; empty reports the empty tuple, the empty collection a language writes
// before it knows a type, which has no element type.
func asSet(fn string, i int, v tenon.Value, p tenon.Policy) (s tenon.Value, empty bool, err error) {
	c, _ := tenon.Unmark(v)
	if !mayBe(c, tenon.KindSet, tenon.KindList, tenon.KindTuple) {
		return tenon.Value{}, false, wrongKind(fn, i, v, "sets")
	}
	if c.IsPending() {
		if k := c.Constraint(); k.Kind() == tenon.ConstraintSetOf {
			return tenon.Pending(k), false, nil
		}
		return tenon.Pending(tenon.SetOf(tenon.Any())), false, nil
	}
	t := c.Type()
	switch {
	case t.Kind() == tenon.KindSet:
		return c, false, nil
	case t.Kind() == tenon.KindTuple && t.TupleLength() == 0:
		return tenon.Value{}, true, nil
	}
	if s = tenon.Convert(c, tenon.SetOf(tenon.Any()), p); s.IsError() {
		return tenon.Value{}, false, tenon.NewError(at(i, s))
	}
	return s, false, nil
}

// setsOf reads the arguments of a set operation: each as asSet reads it,
// the empty tuple as the zero Value, and the constraint of the answer, a
// set of the type the element types unify to under the policy p.
func setsOf(fn string, args []tenon.Value, p tenon.Policy) ([]tenon.Value, tenon.Constraint, error) {
	sets := make([]tenon.Value, len(args))
	var cs []tenon.Constraint
	for i, a := range args {
		s, empty, err := asSet(fn, i, a, p)
		switch {
		case err != nil:
			return nil, tenon.Constraint{}, err
		case empty:
			continue
		case s.IsPending():
			cs = append(cs, s.Constraint().Element())
		default:
			cs = append(cs, tenon.Exactly(s.Type().ElementType()))
		}
		sets[i] = s
	}
	if len(cs) == 0 {
		return nil, tenon.Constraint{}, tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{
			Code:    tenon.CodeConvertNoCommonType,
			Message: fn + ": every argument is the empty tuple, which gives the sets no element type",
			Path:    argument(0),
		}))
	}
	u, err := tenon.Unify(cs, p)
	if err != nil {
		return nil, tenon.Constraint{}, err
	}
	if u.Kind() == tenon.ConstraintExactly {
		return sets, tenon.Exactly(tenon.SetType(u.Type())), nil
	}
	return sets, tenon.SetOf(u), nil
}

// setArgs returns the arguments of a set operation converted to the set
// type t, the empty tuple as the empty set, or the failure of a conversion,
// located at its argument.
func setArgs(fn string, args []tenon.Value, t tenon.Type, p tenon.Policy) ([]tenon.Value, tenon.Value) {
	sets, _, _ := setsOf(fn, args, p)
	for i, s := range sets {
		if s.IsZero() {
			sets[i] = tenon.Set(t.ElementType())
			continue
		}
		if sets[i] = tenon.Convert(s, tenon.Exactly(t), p); sets[i].IsError() {
			return nil, at(i, sets[i])
		}
	}
	return sets, tenon.Value{}
}

// candidate is a member a set operation may answer with, and the argument
// it was read from.
type candidate struct {
	v    tenon.Value
	from int
}

// listed returns the members the set s is known to hold: all of them where
// it holds its members, and complete is true, and otherwise those its range
// lists.
func listed(s tenon.Value) (ms []tenon.Value, complete bool) {
	if s.HasMembers() {
		return s.Elements(), true
	}
	return s.Range().Members(), false
}

// candidates returns the members the sets at the indexes given are known to
// hold, each wholly known one once, and whether every such set holds its
// members.
func candidates(sets []tenon.Value, from ...int) ([]candidate, bool) {
	var out []candidate
	var seen members
	complete := true
	for _, i := range from {
		ms, all := listed(sets[i])
		complete = complete && all
		for _, m := range ms {
			if m.IsKnown() {
				if equal, _ := seen.meet(m); equal {
					continue
				}
				seen.add(m)
			}
			out = append(out, candidate{m, i})
		}
	}
	return out, complete
}

// verdict is whether a member is in the answer of a set operation.
type verdict int

const (
	isIn verdict = iota
	isOut
	isOpen
)

// asked answers whether each of a set operation's sets holds one value
// after another, as Contains answers: a set holding its members from an
// index of them made once, so a known value is compared only with the
// members of its hash and those not known, and one not holding them from
// its range.
type asked struct {
	sets []tenon.Value
	held []*members
}

// ask indexes the sets.
func ask(sets []tenon.Value) asked {
	a := asked{sets: sets, held: make([]*members, len(sets))}
	for j, s := range sets {
		if s.HasMembers() {
			a.held[j] = &members{}
			for _, m := range s.Elements() {
				a.held[j].add(m)
			}
		}
	}
	return a
}

// holds returns whether set j holds v.
func (a asked) holds(j int, v tenon.Value) tenon.Value {
	if a.held[j] == nil {
		return tenon.Contains(a.sets[j], v)
	}
	switch equal, open := a.held[j].meet(v); {
	case equal:
		return tenon.Bool(true)
	case open:
		return tenon.Unknown(tenon.BoolType())
	}
	return tenon.Bool(false)
}

// holders returns, for each set, whether it holds the candidate c: true for
// the set it was read from, and otherwise what Contains would say.
func (a asked) holders(c candidate) []tenon.Value {
	hs := make([]tenon.Value, len(a.sets))
	for j := range a.sets {
		if j == c.from {
			hs[j] = tenon.Bool(true)
		} else {
			hs[j] = a.holds(j, c.v)
		}
	}
	return hs
}

// setAnswer returns the answer of a set operation over sets of the type t
// from the members known to be in it and those undecided: the set of them
// where every member is decided and no argument holds members not read,
// and otherwise the unknown set listing those known in, at least lo long
// and at most as many as are in and undecided, or as hi where more may be.
func setAnswer(t tenon.Type, ins, open []tenon.Value, more bool, lo, hi int64, bounded bool) tenon.Value {
	if !more && len(open) == 0 {
		return tenon.Set(t.ElementType(), ins...)
	}
	if !more {
		hi, bounded = int64(len(ins)+len(open)), true
	}
	ns := []tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(lo)}
	if len(ins) > 0 {
		ns = append(ns, tenon.Members(ins...))
	}
	if bounded {
		ns = append(ns, tenon.LengthMax(hi))
	}
	return tenon.Narrow(tenon.Unknown(t), ns...)
}

// sift returns the members of cs that are in, and those undecided, by what
// decide says of the sets holding each.
func sift(sets []tenon.Value, cs []candidate, decide func(hs []tenon.Value) verdict) (ins, open []tenon.Value) {
	a := ask(sets)
	for _, c := range cs {
		switch decide(a.holders(c)) {
		case isIn:
			ins = append(ins, c.v)
		case isOpen:
			open = append(open, c.v)
		}
	}
	return ins, open
}

// lengths returns the least and greatest lengths of the sets at the indexes
// given, the greatest summed or the least of them, and whether every one
// records a greatest.
func lengths(sets []tenon.Value, sum bool, from ...int) (lo, hi int64, bounded bool) {
	bounded = true
	first := true
	for _, i := range from {
		l, h, ok := lengthOf(sets[i])
		lo = max(lo, l)
		switch {
		case sum:
			hi += h
			bounded = bounded && ok
		case ok && (first || h < hi):
			hi, first = h, false
		}
	}
	if !sum {
		bounded = !first
	}
	return lo, hi, bounded
}

// setOperation returns a set operation's function of the name given, which
// answers by answer from its arguments converted to the set type t.
func setOperation(name, description string, params []tenon.Param, variadic *tenon.Param, answer func(sets []tenon.Value, t tenon.Type) tenon.Value) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:        name,
		Description: description,
		Params:      params,
		VarParam:    variadic,
		ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
			_, rc, err := setsOf(name, args, p)
			return rc, err
		},
		NotNull: true,
		Impl: func(args []tenon.Value, rc tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
			if rc.Kind() != tenon.ConstraintExactly {
				return unknownOf(rc), nil
			}
			sets, failure := setArgs(name, args, rc.Type(), p)
			if !failure.IsZero() {
				return failure, nil
			}
			return answer(sets, rc.Type()), nil
		},
	})
}

// set returns a parameter taking a set.
func set(name, description string) tenon.Param {
	return tenon.Param{Name: name, Description: description, Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true}
}

// indexes returns 0 to n-1.
func indexes(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// SetUnionFunc is the set of every member of its sets. An argument not known
// yet leaves the answer unknown, listing the members the others hold, at
// least as long as each argument and at most as long as all of them.
var SetUnionFunc = setOperation("SetUnion",
	"Returns the union of all given sets.",
	[]tenon.Param{set("first_set", "The first set.")},
	&tenon.Param{Name: "other_sets", Description: "The further sets.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
	func(sets []tenon.Value, t tenon.Type) tenon.Value {
		cs, complete := candidates(sets, indexes(len(sets))...)
		if complete {
			var all []tenon.Value
			for _, s := range sets {
				all = append(all, s.Elements()...)
			}
			return tenon.Set(t.ElementType(), all...)
		}
		ins := make([]tenon.Value, len(cs))
		for i, c := range cs {
			ins[i] = c.v
		}
		var unread []int
		for i, s := range sets {
			if !s.HasMembers() {
				unread = append(unread, i)
			}
		}
		lo, hi, bounded := lengths(sets, true, unread...)
		return setAnswer(t, ins, nil, true, lo, int64(len(cs))+hi, bounded)
	})

// SetIntersectionFunc is the set of the members every one of its sets
// holds: a member of one is in where every other provably holds it, out
// where one provably does not, and undecided otherwise, which leaves the
// answer unknown. Where no set holds its members, the answer is the unknown
// set no longer than the shortest.
var SetIntersectionFunc = setOperation("SetIntersection",
	"Returns the intersection of all given sets.",
	[]tenon.Param{set("first_set", "The first set.")},
	&tenon.Param{Name: "other_sets", Description: "The further sets.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
	func(sets []tenon.Value, t tenon.Type) tenon.Value {
		base := 0
		for i, s := range sets {
			if s.HasMembers() {
				base = i
				break
			}
		}
		cs, complete := candidates(sets, base)
		ins, open := sift(sets, cs, func(hs []tenon.Value) verdict {
			v := isIn
			for _, h := range hs {
				switch {
				case !h.IsKnown():
					v = isOpen
				case !h.AsBool():
					return isOut
				}
			}
			return v
		})
		_, hi, bounded := lengths(sets, false, indexes(len(sets))...)
		return setAnswer(t, ins, open, !complete, 0, hi, bounded)
	})

// SetSubtractFunc is the set of the members of a that b does not hold: a
// member of a is in where b provably does not hold it, out where it
// provably does, and undecided otherwise. Where a does not hold its
// members, the answer is the unknown set no longer than a, and at least as
// long as a's least length less b's greatest.
var SetSubtractFunc = setOperation("SetSubtract",
	"Returns the relative complement of the two given sets.",
	[]tenon.Param{set("a", "The set to take members from."), set("b", "The set whose members are taken.")},
	nil,
	func(sets []tenon.Value, t tenon.Type) tenon.Value {
		a, b := sets[0], sets[1]
		if bl, bh, ok := lengthOf(b); ok && bh == 0 && bl == 0 {
			return a
		}
		cs, complete := candidates(sets, 0)
		ins, open := sift(sets, cs, func(hs []tenon.Value) verdict {
			switch h := hs[1]; {
			case !h.IsKnown():
				return isOpen
			case h.AsBool():
				return isOut
			}
			return isIn
		})
		lo, hi, bounded := lengthOf(a)
		if _, bh, ok := lengthOf(b); ok {
			lo = max(0, lo-bh)
		} else {
			lo = 0
		}
		return setAnswer(t, ins, open, !complete, lo, hi, bounded)
	})

// SetSymmetricDifferenceFunc is the set of the members an odd number of its
// sets hold: a member is in or out where every set provably holds it or
// provably does not, and undecided otherwise. An argument not known yet
// leaves the answer unknown, no longer than the members listed and every
// such argument together.
var SetSymmetricDifferenceFunc = setOperation("SetSymmetricDifference",
	"Returns the symmetric difference of the two given sets.",
	[]tenon.Param{set("first_set", "The first set.")},
	&tenon.Param{Name: "other_sets", Description: "The further sets.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
	func(sets []tenon.Value, t tenon.Type) tenon.Value {
		cs, complete := candidates(sets, indexes(len(sets))...)
		ins, open := sift(sets, cs, func(hs []tenon.Value) verdict {
			odd := false
			for _, h := range hs {
				if !h.IsKnown() {
					return isOpen
				}
				odd = odd != h.AsBool()
			}
			if odd {
				return isIn
			}
			return isOut
		})
		var unread []int
		for i, s := range sets {
			if !s.HasMembers() {
				unread = append(unread, i)
			}
		}
		_, hi, bounded := lengths(sets, true, unread...)
		return setAnswer(t, ins, open, !complete, 0, int64(len(ins)+len(open))+hi, bounded)
	})
