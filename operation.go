package tenon

import "slices"

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

// And returns the conjunction of two Bool values. If either operand is an error
// value the result is an error value, even when the other operand is false.
//
// And panics if an operand is neither a Bool value nor an error value.
func And(a, b Value) Value {
	if v, ok := propagate(a, b); ok {
		return v
	}
	x, y := boolOperand("And", a), boolOperand("And", b)
	return Bool(x && y)
}

// Or returns the disjunction of two Bool values. If either operand is an error
// value the result is an error value, even when the other operand is true.
//
// Or panics if an operand is neither a Bool value nor an error value.
func Or(a, b Value) Value {
	if v, ok := propagate(a, b); ok {
		return v
	}
	x, y := boolOperand("Or", a), boolOperand("Or", b)
	return Bool(x || y)
}

// Not returns the negation of a Bool value, or an error value if a is one. It
// panics if a is neither a Bool value nor an error value.
func Not(a Value) Value {
	if v, ok := propagate(a); ok {
		return v
	}
	return Bool(!boolOperand("Not", a))
}

// boolOperand returns the content of a Bool operand, panicking if v is not
// one. fn names the operation for the message.
func boolOperand(fn string, v Value) bool {
	n := v.data()
	if n.state != stateResolved || n.typ.t.kind != KindBool {
		usagePanic("%s: %s is not a Bool value", fn, n.describe())
	}
	return n.data.(bool)
}
