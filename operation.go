package tenon

import (
	"slices"
	"strconv"
)

// propagate returns the error value that an operation with these operands
// produces, and whether any operand was an error value. The diagnostics of
// every error operand appear in operand order, with exact duplicates dropped,
// so that one pass over a configuration reports every mistake, each once. The
// error value carries no marks yet: the caller puts on it the marks its own
// rules call for.
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
	// operands is what the operation accepts in each position. Most operations
	// take their operands on the same terms, and some do not: membership takes
	// a set and then anything at all, and answers for a null member but not
	// for a null set.
	operands []operand
	// agree requires the operands to have one type between them. Equality
	// takes two of different types and answers false; ordering does not, and
	// says so rather than inventing an order across types.
	agree bool
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
	// registered is set by register, and apply refuses an operation without
	// it, so that no operation escapes the operand matrix.
	registered bool
	// bind is set on an operation that takes parameters besides its operands,
	// as a conversion takes the constraint it converts to and a policy. The
	// registered operation is a template, which apply refuses; with makes a
	// copy of it for one choice of parameters, and bind installs in that copy
	// whatever depends on them.
	bind func(o *op, param opParam)
	// samples are the choices of parameters that the operand matrix checks a
	// template with.
	samples []opParam
	// param is the choice of parameters a copy made by with was bound to.
	param opParam
}

// opParam is a choice of parameters for an operation that takes them.
type opParam interface {
	// String describes the choice for messages, as in (list_of(any), safe).
	String() string
}

// with returns the template o bound to one choice of parameters.
func (o *op) with(param opParam) *op {
	if o.bind == nil {
		internalPanic("%s takes no parameters", o.name)
	}
	b := *o
	b.bind, b.samples, b.param = nil, nil, param
	b.operands = slices.Clone(o.operands)
	o.bind(&b, param)
	return &b
}

// operations holds every registered operation, in the order they were
// declared, for the operand matrix that checks what must hold of all of them.
var operations []*op

// register records o among the operations and returns it. Every operation is
// declared through it: apply panics on one that is not.
func register(o *op) *op {
	o.registered = true
	operations = append(operations, o)
	return o
}

// fixedResult returns the result function of an operation whose result type
// does not depend on the types of its operands.
func fixedResult(t Type) func([]Type) Constraint {
	c := Exactly(t)
	return func([]Type) Constraint { return c }
}

// apply runs the operation over args and puts the union of the operands'
// Propagate marks on the result, so an operation says what its result is
// and inherits how marks travel. An error result carries them as any result
// does: it stands where the result would have, the marks of an error operand
// survive into it, and what it says may come from any operand.
func (o *op) apply(args ...Value) Value {
	if !o.registered {
		internalPanic("%s is not registered, so the operand matrix does not check it", o.name)
	}
	if o.bind != nil {
		internalPanic("%s takes parameters, and was applied without them", o.name)
	}
	r := o.applyValue(args)
	if ms := o.propagated(args); len(ms) != 0 {
		r = WithMarks(r, ms...)
	}
	return r
}

// applyValue runs the operation over args, settling what the operands are
// before the operation itself is asked anything.
func (o *op) applyValue(args []Value) Value {
	if len(args) != len(o.operands) {
		internalPanic("%s: %d operands were given to an operation that takes %d",
			o.name, len(args), len(o.operands))
	}
	// The type of an operand is checked before diagnostics are collected, so
	// that a mistake in the calling program is not masked by an error value it
	// was already carrying.
	for i, a := range args {
		n := a.data()
		if n.state == stateError || n.state == statePending {
			continue
		}
		if !Satisfies(o.operands[i].constraint, n.typ) {
			usagePanic("%s: %s is %s, which does not satisfy %s",
				o.name, operandName(i, len(args)), n.describe(), o.operands[i].constraint)
		}
	}
	if o.agree {
		if i, j, ok := disagreeing(args, false); ok {
			usagePanic("%s: %s is %s and %s is %s, but %s takes operands of one type",
				o.name, operandName(i, len(args)), args[i].n.describe(),
				operandName(j, len(args)), args[j].n.describe(), o.name)
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
			if !couldSatisfy(c, o.operands[i].constraint) {
				diags = append(diags, o.wrongType(i, len(args), c))
				continue
			}
			if !o.operands[i].nulls && n.null == nullOnly {
				diags = append(diags, o.nullOperand(i, len(args)))
				continue
			}
			if c.Kind() == ConstraintExactly {
				types[i] = c.Type()
			}
		case stateNull:
			types[i] = n.typ
			if !o.operands[i].nulls {
				diags = append(diags, o.nullOperand(i, len(args)))
			}
		default:
			types[i] = n.typ
			known = known && n.isKnown()
		}
	}
	if o.agree && len(diags) == 0 {
		// A pending operand whose constraint puts it at a type another operand
		// rules out is bad data and not a bad call: nothing was wrong with the
		// call when it was made, and the type it will have is what rules it
		// out. It is only worth saying when nothing else about the operands
		// was wrong already.
		if i, j, ok := disagreeing(args, true); ok {
			diags = append(diags, o.disagreement(args, i, j))
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

// operand is what an operation accepts in one position: the constraint that the
// type of the value there must satisfy, and whether the operation has an answer
// for null there.
type operand struct {
	constraint Constraint
	nulls      bool
	// within says the operation reads the values within the operand, as
	// equality reads the members of what it compares, rather than only its
	// shape, as a length does. A value it reads is consumed along with the
	// operand, so its Propagate marks reach the result.
	within bool
	// marksWithin says the operation reads within the operand as within
	// does, but puts the marks of what it read on the result itself, because
	// which values it reads depends on the operand: a conversion to a set
	// reads no member where it fails first. The framework adds nothing for
	// it, and the operand matrix expects what within would give.
	marksWithin bool
}

// alike returns the operands of an operation that takes n of them on the same
// terms.
func alike(n int, c Constraint, nulls bool) []operand {
	list := make([]operand, n)
	for i := range list {
		list[i] = operand{constraint: c, nulls: nulls}
	}
	return list
}

// reading returns operands that the operation reads within.
func reading(operands []operand) []operand {
	for i := range operands {
		operands[i].within = true
	}
	return operands
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

// disagreeing returns two operands whose types differ, and whether there are
// any. An operand whose type nothing has settled agrees with everything, since
// nothing it could turn out to be is ruled out yet. A pending operand counts
// only when pending is true: the type it will have follows from its constraint
// rather than being in hand, so a disagreement there is a different thing from
// one between two values.
func disagreeing(args []Value, pending bool) (int, int, bool) {
	first, at := Type{}, 0
	for i, a := range args {
		if !pending && a.n.state == statePending {
			continue
		}
		t, ok := settledType(a.n)
		switch {
		case !ok:
		case first.t == nil:
			first, at = t, i
		case t != first:
			return at, i, true
		}
	}
	return 0, 0, false
}

// disagreement returns the diagnostic for operands whose types will not agree.
func (o *op) disagreement(args []Value, i, j int) Diagnostic {
	n := len(args)
	return Diagnostic{
		Code: CodeOperationWrongType,
		Message: operandName(i, n) + " of " + o.name + " is " + operandText(args[i].n) +
			" and " + operandName(j, n) + " is " + operandText(args[j].n) +
			", and " + o.name + " takes operands of one type",
	}
}

// operandText describes an operand for a diagnostic: a pending operand by the
// constraint its type will satisfy, since that is what rules it out, and any
// other operand as describe names it.
func operandText(n *node) string {
	if n.state == statePending {
		return "pending with constraint " + n.data.(Constraint).String()
	}
	return n.describe()
}

// couldSatisfy reports whether some type satisfying c satisfies the operand
// constraint too, so that the operation could still apply once the type of a
// pending value is settled. Where c names one type, that type is the only one
// to ask about. Otherwise it decides only where the operand constraint names
// one type, alone or as a member of a OneOf, and answers true for any other
// operand constraint: it says that ListOf(Any()) could satisfy SetOf(Any()),
// although no list is a set, and that OneOf() could satisfy Any(), although
// OneOf() admits no type. Deciding those would mean comparing two constraints
// in general, and assuming the operation could apply leaves the answer to the
// value rather than inventing one here.
func couldSatisfy(c, operand Constraint) bool {
	if c.Kind() == ConstraintExactly {
		return Satisfies(operand, c.Type())
	}
	switch operand.Kind() {
	case ConstraintExactly:
		return Satisfies(c, operand.Type())
	case ConstraintOneOf:
		for _, m := range operand.Members() {
			if couldSatisfy(c, m) {
				return true
			}
		}
		return false
	}
	return true
}

// wrongType returns the diagnostic for a pending operand that can never have a
// type the operation accepts.
func (o *op) wrongType(i, n int, c Constraint) Diagnostic {
	return Diagnostic{
		Code: CodeOperationWrongType,
		Message: operandName(i, n) + " of " + o.name + " is pending with constraint " +
			c.String() + ", and no type it allows satisfies " + o.operands[i].constraint.String(),
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
	andOp = register(&op{
		name:     "And",
		operands: alike(2, boolOperand, false),
		result:   fixedResult(Type{boolType}),
		known: func(args []Value) Value {
			return Bool(args[0].n.data.(bool) && args[1].n.data.(bool))
		},
		decided: func(args []Value) (Value, bool) { return decidedBy(args, false) },
	})
	orOp = register(&op{
		name:     "Or",
		operands: alike(2, boolOperand, false),
		result:   fixedResult(Type{boolType}),
		known: func(args []Value) Value {
			return Bool(args[0].n.data.(bool) || args[1].n.data.(bool))
		},
		decided: func(args []Value) (Value, bool) { return decidedBy(args, true) },
	})
	notOp = register(&op{
		name:     "Not",
		operands: alike(1, boolOperand, false),
		result:   fixedResult(Type{boolType}),
		known:    func(args []Value) Value { return Bool(!args[0].n.data.(bool)) },
	})
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

var isNullOp = register(&op{
	name:     "IsNull",
	operands: alike(1, Any(), true),
	result:   fixedResult(Type{boolType}),
	known:    func(args []Value) Value { return Bool(args[0].n.state == stateNull) },
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
})
