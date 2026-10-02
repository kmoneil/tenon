package tenon

import (
	"slices"
	"testing"
)

// RegisteredOperation describes a registered operation to the operand matrix,
// which lives outside the package.
type RegisteredOperation struct {
	Name string
	// Params describes the parameters an operation that takes them was bound
	// to, and is empty for one that takes none.
	Params      string
	Constraints []Constraint
	Nulls       []bool
	Within      []bool
	Agree       bool
	Fixed       bool
	Call        func(args ...Value) Value
}

// RegisteredOperations returns every registered operation, in the order they
// were declared. An operation that takes parameters appears once for each
// choice of them that it names for the matrix.
func RegisteredOperations() []RegisteredOperation {
	var bound []*op
	for _, o := range operations {
		if o.bind == nil {
			bound = append(bound, o)
			continue
		}
		for _, p := range o.samples {
			bound = append(bound, o.with(p))
		}
	}
	out := make([]RegisteredOperation, len(bound))
	for i, o := range bound {
		r := RegisteredOperation{Name: o.name, Agree: o.agree, Call: o.apply}
		if o.param != nil {
			r.Params = o.param.String()
		}
		for _, operand := range o.operands {
			r.Constraints = append(r.Constraints, operand.constraint)
			r.Nulls = append(r.Nulls, operand.nulls)
			r.Within = append(r.Within, operand.within || operand.marksWithin)
		}
		// A result function is asked about operands whose types nothing has
		// settled; one that names a type then names it whatever they are.
		r.Fixed = o.result(make([]Type, len(o.operands))).Kind() == ConstraintExactly
		out[i] = r
	}
	return out
}

// SharedType, SoleType and AdmitsNone expose three decisions about
// constraints to the property tests outside the package, which generate the
// constraints they are checked on.
func SharedType(cs ...Constraint) (Type, bool) { return sharedType(cs...) }

func SoleType(c Constraint) (Type, bool) { return soleType(c) }

func AdmitsNone(c Constraint) bool { return admitsNone(c) }

// FlagsChecked verifies, for tests outside the package, that v and every
// value within it carry the partial and markedWithin flags a full walk
// finds, and keep only what asking again gives (checkKept), and returns how
// many values it checked.
func FlagsChecked(t *testing.T, v Value) int { return checkFlags(t, v) }

// SameNode reports whether a and b hold one node: whether an operation handed
// back the value it was given rather than a copy of it.
func SameNode(a, b Value) bool { return a.n == b.n }

// ConvertBothWays converts v, which is not an error value, to c under p as
// Convert does, and as the reference that converts a container's members and
// fits them to its element type at every level does, for the property test
// that holds the two to one result. Neither goes through the operation
// framework, which they would share.
//
// Where the reference's result is of a type still open, nothing settled what
// it left open, and the conversion fails (CV-021). The reference does not say
// where, so the conversion's failure stands in for it, where it is one.
func ConvertBothWays(v Value, c Constraint, p Policy) (converted, fitted Value) {
	converted = converter{policy: p, carried: &carrying{}, memo: &convertMemo{}}.value(v, c)
	fitted = converter{policy: p, carried: &carrying{}, memo: &convertMemo{}}.fittingValue(v, c)
	if t := fitted.n.typ.t; t != nil && t.open && converted.IsError() {
		fitted = converted
	}
	return converted, fitted
}

// CarryBothWays gives each value of into, what the value at its index in from
// converted to without its own marks, from's Propagate marks two ways: as a
// conversion carries them, through one converter for them all, so that what
// members share is kept once for all of them; and as WithMarks gives them,
// one by one, which is what carrying's shortcut stands in for. shortcut says
// where it applies: the value into holds carries no marks, and from holds a
// layer, or no deep mark in its own list.
func CarryBothWays(into, from []Value) (carried, given []Value, shortcut []bool) {
	x := converter{policy: Unsafe, carried: &carrying{}, memo: &convertMemo{}}
	for i, r := range into {
		n := from[i].n
		s := n.marks
		shortcut = append(shortcut, s != nil && r.n.marks == nil && (s.layer || !slices.ContainsFunc(s.list, isDeep)))
		carried = append(carried, x.carry(r, n))
		given = append(given, WithMarks(r, propagateMarks(n)...))
	}
	return carried, given, shortcut
}
