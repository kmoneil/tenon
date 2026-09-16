package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// constraintOf returns what an operand says about its type: the one type a
// resolved value has, or the constraint a pending value carries.
func constraintOf(v tenon.Value) tenon.Constraint {
	if v.IsPending() {
		return v.Constraint()
	}
	return tenon.Exactly(v.Type())
}

// compareAsAFrontendDoes compares two operands the way a frontend is asked to:
// unify what the two say about their types, convert each to the result, and
// compare what they convert to.
func compareAsAFrontendDoes(p tenon.Policy, a, b tenon.Value) tenon.Value {
	u, failure, ok := tenon.Unify(p, constraintOf(a), constraintOf(b))
	if !ok {
		return failure
	}
	return tenon.Equals(tenon.Convert(a, u, p), tenon.Convert(b, u, p))
}

// TestConformance_UN024_NullLiteralsCompareAsUsersExpect is the acceptance
// test of the null design: a null literal, which has no type, compared with a
// typed operand after unification, for operands of two types.
func TestConformance_UN024_NullLiteralsCompareAsUsersExpect(t *testing.T) {
	conformance.Covers(t, "UN-024", "EQ-005", "CV-040", "CV-032")
	null := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())
	unknownBool := tenon.Narrow(tenon.Unknown(boo), tenon.NotNull())
	for _, tt := range []struct {
		typ     tenon.Type
		present tenon.Value
	}{
		{str, s("a")},
		{num, n(1)},
	} {
		for _, c := range []struct {
			name    string
			operand tenon.Value
			want    tenon.Value
		}{
			{"a null of the type", tenon.NullVal(tt.typ), tenon.Bool(true)},
			{"a value of the type", tt.present, tenon.Bool(false)},
			{"an unknown that may be null", tenon.Unknown(tt.typ), unknownBool},
			{"an unknown that is not null", tenon.Narrow(tenon.Unknown(tt.typ), tenon.NotNull()), tenon.Bool(false)},
		} {
			for _, p := range []tenon.Policy{safe, uns} {
				what := tt.typ.String() + ", " + c.name + ", " + p.String()
				wantValue(t, what+" == null", compareAsAFrontendDoes(p, c.operand, null), c.want)
				wantValue(t, "null == "+what, compareAsAFrontendDoes(p, null, c.operand), c.want)
			}
		}
		// Two typed nulls of one type are equal with or without unifying.
		wantValue(t, "typed nulls of "+tt.typ.String(), compareAsAFrontendDoes(safe, tenon.NullVal(tt.typ), tenon.NullVal(tt.typ)), tenon.Bool(true))

		// The unification is what settles it: without it, a null literal
		// could still take another type than the operand's, and a null of
		// that type is not this one.
		wantValue(t, "a null of "+tt.typ.String()+" == null, not unified", tenon.Equals(tenon.NullVal(tt.typ), null), unknownBool)
	}

	// Two null literals give unification no type, so both stay pending, and
	// the answer stays open: they could still take different types. A frontend
	// that wants true here decides it itself.
	wantValue(t, "null == null", compareAsAFrontendDoes(uns, null, null), unknownBool)
}
