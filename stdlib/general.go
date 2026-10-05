package stdlib

import "github.com/kmoneil/tenon"

// equalityOperand returns a parameter of an equality, which has an answer for
// every value: null, values not known yet, pending ones, marked ones.
func equalityOperand(name, description string) tenon.Param {
	return tenon.Param{
		Name: name, Description: description, Constraint: tenon.Any(),
		AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true,
	}
}

// EqualFunc reports whether two values are equal: tenon's Equals, after a
// language's untyped null is settled. A pending operand whose constraint
// admits the other operand's type is resolved to it first, so a null whose
// type was never given compares with a null of any type as equal, and with
// any other value as not; and two such nulls are equal. That is what x ==
// null asks of x. Two tuples or objects whose types wait on such nulls are
// compared member by member by the same rules, so [null] == [null] is true,
// as null == null is.
var EqualFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Equal",
	Description: "Returns true if the two given values are equal, or false otherwise.",
	Params:      []tenon.Param{equalityOperand("a", "The first value."), equalityOperand("b", "The second value.")},
	Result:      boolean,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return equal(args[0], args[1]), nil
	},
})

// NotEqualFunc reports whether two values differ: EqualFunc's answer,
// negated.
var NotEqualFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "NotEqual",
	Description: "Returns false if the two given values are equal, or true otherwise.",
	Params:      []tenon.Param{equalityOperand("a", "The first value."), equalityOperand("b", "The second value.")},
	Result:      boolean,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return tenon.Not(equal(args[0], args[1])), nil
	},
})

// equal is EqualFunc's answer (LN-011): Equals, once a pending operand is
// resolved to the type of the other where its constraint admits it; true of
// two pending values both known to be null; and, of two pending tuples or
// objects holding their members, their members compared pair by pair by
// these rules, the answers joined as And joins them.
func equal(a, b tenon.Value) tenon.Value {
	ua, _ := tenon.Unmark(a)
	ub, _ := tenon.Unmark(b)
	switch {
	case ua.IsPending() && ub.IsPending():
		if ua.IsNull() && ub.IsNull() {
			return tenon.WithMarks(tenon.Bool(true), propagating(a, b)...)
		}
		if pairs, ok := heldPairs(ua, ub); ok {
			answer := tenon.Bool(true)
			for _, p := range pairs {
				answer = tenon.And(answer, equal(p[0], p[1]))
			}
			return tenon.WithMarks(answer, propagating(a, b)...)
		}
	case ua.IsPending() && ub.IsResolved():
		if tenon.Satisfies(ua.Constraint(), ub.Type()) {
			a = tenon.Resolve(a, ub.Type())
		}
	case ub.IsPending() && ua.IsResolved():
		if tenon.Satisfies(ub.Constraint(), ua.Type()) {
			b = tenon.Resolve(b, ua.Type())
		}
	}
	return tenon.Equals(a, b)
}

// heldPairs returns the members of a and b paired, where both are pending
// tuples holding their elements, of one length, or pending objects holding
// their attributes, of the same names, and whether they are (UN-025).
func heldPairs(a, b tenon.Value) ([][2]tenon.Value, bool) {
	if !a.HasMembers() || !b.HasMembers() || a.Constraint().Kind() != b.Constraint().Kind() {
		return nil, false
	}
	var pairs [][2]tenon.Value
	switch a.Constraint().Kind() {
	case tenon.ConstraintTupleOf:
		ea, eb := a.Elements(), b.Elements()
		if len(ea) != len(eb) {
			return nil, false
		}
		for i := range ea {
			pairs = append(pairs, [2]tenon.Value{ea[i], eb[i]})
		}
	case tenon.ConstraintObjectWith:
		if a.Len() != b.Len() {
			return nil, false
		}
		for name, va := range a.Attributes() {
			vb, ok := b.LookupAttribute(name)
			if !ok {
				return nil, false
			}
			pairs = append(pairs, [2]tenon.Value{va, vb})
		}
	default:
		return nil, false
	}
	return pairs, true
}

// propagating returns the marks of vs that reach what is derived from them:
// those that propagate, and redacting ones, whatever they say.
func propagating(vs ...tenon.Value) []tenon.Mark {
	var out []tenon.Mark
	for _, v := range vs {
		_, ms := tenon.Unmark(v)
		for _, m := range ms {
			if m.Propagation() == tenon.Propagate || m.Redacting() {
				out = append(out, m)
			}
		}
	}
	return out
}

// anything returns a parameter of a function that has an answer for any
// value: null, values not known yet, pending ones, marked ones.
func anything(name, description string) tenon.Param {
	return tenon.Param{
		Name: name, Description: description, Constraint: tenon.Any(),
		AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true,
	}
}

// CoalesceFunc returns the first of its arguments that is not null,
// converted to the type the arguments unify to under the call's policy. A
// null argument is passed over; one that may still be null, not known yet,
// leaves the answer unknown, never null, since a later one may stand in for
// it; and where every argument is null the call fails with
// tenon.CodeFunctionInvalidArgument. The answer carries the marks of what
// the function read: the nulls it passed over and the argument it chose,
// not those it never reached.
var CoalesceFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Coalesce",
	Description: "Returns the first of the given arguments that is not null.",
	Params:      []tenon.Param{anything("first", "The first value to consider.")},
	VarParam:    &tenon.Param{Name: "vals", Description: "The further values to consider, in order.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		cs := make([]tenon.Constraint, len(args))
		for i, a := range args {
			cs[i] = typeOf(a)
		}
		return tenon.Unify(cs, p)
	},
	NotNull: true,
	Impl: func(args []tenon.Value, result tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		var read []tenon.Mark
		for i, a := range args {
			u, _ := tenon.Unmark(a)
			switch n := tenon.IsNull(u); {
			case n.IsKnown() && n.AsBool():
				read = append(read, propagating(a)...)
			case !n.IsKnown():
				return tenon.WithMarks(unknownOf(result), append(read, propagating(a)...)...), nil
			default:
				return tenon.WithMarks(at(i, tenon.Convert(a, result, p)), read...), nil
			}
		}
		return tenon.ErrorVal(tenon.Diagnostic{
			Code:    tenon.CodeFunctionInvalidArgument,
			Message: "every argument of Coalesce is null, and it has no answer without one that is not",
		}), nil
	},
})

// unknownOf returns the value not known yet that the constraint c gives:
// the unknown of its type where it names one, and pending otherwise.
func unknownOf(c tenon.Constraint) tenon.Value {
	if c.Kind() == tenon.ConstraintExactly {
		return tenon.Unknown(c.Type())
	}
	return tenon.Pending(c)
}
