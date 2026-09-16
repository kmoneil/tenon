package tenon

import (
	"slices"
	"strconv"
)

// propagate returns the error value that an operation with these operands
// produces, and whether any operand was an error value. The diagnostics of
// every error operand appear in operand order, with exact duplicates dropped,
// so that one pass over a configuration reports every mistake, each once.
//
// Propagation never short-circuits: an operation fails even when its other
// operands would have decided the answer, because an error means the caller
// wrote something wrong and discarding it would hide a real mistake.
func propagate(operands ...Value) (Value, bool) {
	var diags []Diagnostic
	for _, v := range operands {
		if v.data().state != stateError {
			continue
		}
		for _, d := range v.n.data.([]Diagnostic) {
			if !slices.ContainsFunc(diags, d.Equal) {
				diags = append(diags, d)
			}
		}
	}
	if len(diags) == 0 {
		return Value{}, false
	}
	return errorValue(diags...), true
}

// op is one operation of the value layer. The framework around it settles the
// operands before the operation itself sees them: error operands propagate, an
// operand of the wrong type is the calling program's mistake, a null the
// operation has no answer for is bad data, and operands that are not known are
// answered from their ranges rather than from content they do not have.
//
// Every operation defined here produces a value or an error and never null, so
// the result it gives for operands that are not known excludes null. An
// operation that can produce null will have to say so.
type op struct {
	name string // names the operation in messages
	// operand is what every operand must satisfy. It is a constraint and not a
	// type, because what an operation accepts is an acceptance test: an
	// operation that takes any value says Any, rather than a type with a
	// wildcard in it or a zero value standing in for one.
	operand Constraint
	nulls   bool // whether the operation has an answer for a null operand
	// result describes what the operation produces, given the type of each
	// operand, or the zero Type for an operand that is pending and whose
	// constraint names no single type. Exactly(T) settles the result type; any
	// other constraint leaves the result pending.
	result func(types []Type) Constraint
	// known is the operation itself. Every operand is known and acceptable,
	// and the answer is a value or an error value, never an unknown one.
	known func(args []Value) Value
	// decided gives the answer that the operands force although one of them is
	// not known, as a false operand decides And. It is optional.
	decided func(args []Value) (Value, bool)
	// narrow narrows the unknown result r by what the operand ranges say. It is
	// optional, and what it returns must still hold every possible outcome.
	narrow func(args []Value, r Value) Value
}

// fixedResult returns the result function of an operation whose result type
// does not depend on the types of its operands.
func fixedResult(t Type) func([]Type) Constraint {
	c := Exactly(t)
	return func([]Type) Constraint { return c }
}

// apply runs the operation over args, settling what the operands are before the
// operation itself is asked anything.
func (o *op) apply(args ...Value) Value {
	// The type of an operand is checked before diagnostics are collected, so
	// that a mistake in the calling program is not masked by an error value it
	// was already carrying.
	for i, a := range args {
		n := a.data()
		if n.state == stateError || n.state == statePending {
			continue
		}
		if !Satisfies(o.operand, n.typ) {
			usagePanic("%s: %s is %s, which does not satisfy %s",
				o.name, operandName(i, len(args)), n.describe(), o.operand)
		}
	}
	if e, ok := propagate(args...); ok {
		return e
	}
	types := make([]Type, len(args))
	var diags []Diagnostic
	known := true
	for i, a := range args {
		switch n := a.n; n.state {
		case statePending:
			known = false
			c := n.data.(Constraint)
			if !couldSatisfy(c, o.operand) {
				diags = append(diags, o.wrongType(i, len(args), c))
				continue
			}
			if !o.nulls && n.null == nullOnly {
				diags = append(diags, o.nullOperand(i, len(args)))
				continue
			}
			if c.Kind() == ConstraintExactly {
				types[i] = c.Type()
			}
		case stateNull:
			types[i] = n.typ
			if !o.nulls {
				diags = append(diags, o.nullOperand(i, len(args)))
			}
		default:
			types[i] = n.typ
			known = known && n.isKnown()
		}
	}
	if len(diags) > 0 {
		return errorValue(diags...)
	}
	if known {
		r := o.known(args)
		if !r.n.isKnown() && r.n.state != stateError {
			internalPanic("%s: every operand was known, but the result is %s", o.name, r.n.describe())
		}
		return r
	}
	if o.decided != nil {
		if v, ok := o.decided(args); ok {
			return v
		}
	}
	c := o.result(types)
	if c.Kind() != ConstraintExactly {
		return Narrow(Pending(c), NotNull())
	}
	r := Narrow(Unknown(c.Type()), NotNull())
	if o.narrow != nil {
		r = o.narrow(args, r)
	}
	return r
}

// operandName names operand i of n for a message.
func operandName(i, n int) string {
	switch {
	case n == 1:
		return "the operand"
	case i == 0:
		return "the first operand"
	case i == 1:
		return "the second operand"
	}
	return "operand " + strconv.Itoa(i+1)
}

// nullOperand returns the diagnostic for an operand that is null where the
// operation has no answer for null.
func (o *op) nullOperand(i, n int) Diagnostic {
	return Diagnostic{
		Code:    CodeOperationNullOperand,
		Message: operandName(i, n) + " of " + o.name + " is null, which " + o.name + " cannot use",
	}
}

// couldSatisfy reports whether some type satisfying c satisfies the operand
// constraint too, so that the operation could still apply once the type of a
// pending value is settled.
//
// Every operation states Any or Exactly, which is what the TY-003 test holds
// them to. Deciding a richer operand constraint would mean comparing two
// constraints, which nothing needs yet; assuming it could apply leaves the
// answer to the value rather than inventing one here.
func couldSatisfy(c, operand Constraint) bool {
	if operand.Kind() == ConstraintExactly {
		return Satisfies(c, operand.Type())
	}
	return true
}

// wrongType returns the diagnostic for a pending operand that can never have a
// type the operation accepts.
func (o *op) wrongType(i, n int, c Constraint) Diagnostic {
	return Diagnostic{
		Code: CodeOperationWrongType,
		Message: operandName(i, n) + " of " + o.name + " is pending with constraint " +
			c.String() + ", and no type it allows satisfies " + o.operand.String(),
	}
}

// unknownBool is the answer to a test that nothing has settled: a Bool that
// could be either, and that is not null, because a test does have an answer.
var unknownBool = Narrow(Unknown(Type{boolType}), NotNull())

// boolOperand is what the logical operations accept.
var boolOperand = Exactly(Type{boolType})

// And returns the conjunction of two Bool values. An operand that is false
// decides the answer, whatever the other one turns out to be.
//
// If either operand is an error value the result is an error value, even when
// the other operand is false. A null operand is an error value too, with code
// CodeOperationNullOperand, since a conjunction has no answer for null.
//
// And panics if an operand is a value of another type.
func And(a, b Value) Value { return andOp.apply(a, b) }

// Or returns the disjunction of two Bool values. An operand that is true
// decides the answer, whatever the other one turns out to be. It treats error
// and null operands as And does, and panics on the same operands.
func Or(a, b Value) Value { return orOp.apply(a, b) }

// Not returns the negation of a Bool value. It treats an error or null operand
// as And does, and panics on the same operands.
func Not(a Value) Value { return notOp.apply(a) }

var (
	andOp = &op{
		name:    "And",
		operand: boolOperand,
		result:  fixedResult(Type{boolType}),
		known: func(args []Value) Value {
			return Bool(args[0].n.data.(bool) && args[1].n.data.(bool))
		},
		decided: func(args []Value) (Value, bool) { return decidedBy(args, false) },
	}
	orOp = &op{
		name:    "Or",
		operand: boolOperand,
		result:  fixedResult(Type{boolType}),
		known: func(args []Value) Value {
			return Bool(args[0].n.data.(bool) || args[1].n.data.(bool))
		},
		decided: func(args []Value) (Value, bool) { return decidedBy(args, true) },
	}
	notOp = &op{
		name:    "Not",
		operand: boolOperand,
		result:  fixedResult(Type{boolType}),
		known:   func(args []Value) Value { return Bool(!args[0].n.data.(bool)) },
	}
)

// decidedBy answers with b when an operand is already known to be b, which
// leaves nothing for the other operand to decide.
func decidedBy(args []Value, b bool) (Value, bool) {
	for _, a := range args {
		if a.n.state == stateKnown && a.n.data.(bool) == b {
			return Bool(b), true
		}
	}
	return Value{}, false
}

// IsNull returns whether v is null, as a Bool value: known true for the null
// value of a type, known false for a value whose range no longer holds null,
// and an unknown Bool while the range holds null and something else. A pending
// value answers from the nullness fact it carries, which it has whether or not
// its type is settled.
//
// IsNull returns an error value if v is one.
func IsNull(v Value) Value { return isNullOp.apply(v) }

var isNullOp = &op{
	name:    "IsNull",
	operand: Any(),
	nulls:   true,
	result:  fixedResult(Type{boolType}),
	known:   func(args []Value) Value { return Bool(args[0].n.state == stateNull) },
	decided: func(args []Value) (Value, bool) {
		switch n := args[0].n; n.state {
		case stateUnknown:
			if null := n.data.(*rangeData).null; null != nullMaybe {
				return Bool(null == nullOnly), true
			}
		case statePending:
			if n.null != nullMaybe {
				return Bool(n.null == nullOnly), true
			}
		default:
			// Content that a member leaves open is content all the same: a
			// list holding an unknown is a list, and no list is null.
			return Bool(n.state == stateNull), true
		}
		return Value{}, false
	},
}
