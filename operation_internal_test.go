package tenon

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
)

// mustPanicInternal runs f and checks that it reports a defect in this package
// rather than in a caller.
func mustPanicInternal(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		msg, ok := r.(string)
		if !ok || !strings.HasPrefix(msg, "tenon: internal: ") || !strings.Contains(msg, want) {
			t.Errorf("recovered %#v, want an internal defect panic containing %q", r, want)
		}
	}()
	f()
}

func TestConformance_UN008_KnownOperandsAnswerOrFail(t *testing.T) {
	conformance.Covers(t, "UN-008")
	// Every operation that ships keeps the rule.
	for _, got := range []Value{
		And(Bool(true), Bool(false)),
		Or(Bool(true), Bool(false)),
		Not(Bool(true)),
		IsNull(String("x")),
		IsNull(NullVal(StringType())),
	} {
		if !got.IsKnown() {
			t.Errorf("an operation with known operands gave %v, which is not known", got)
		}
	}
	// One that breaks it is caught where it breaks it, rather than downstream
	// where an unknown value would turn up with nothing to explain it.
	// These operations exist only here, so they are marked registered rather
	// than registered: the operand matrix runs over the package's own.
	broken := &op{
		name:       "broken",
		operands:   alike(1, boolOperand, false),
		result:     fixedResult(Type{boolType}),
		known:      func([]Value) Value { return Unknown(Type{boolType}) },
		registered: true,
	}
	mustPanicInternal(t, "broken: every operand was known, but the result is an unknown value of type bool", func() {
		broken.apply(Bool(true))
	})
	// Failing is the other answer the rule allows, and is not caught.
	failing := &op{
		name:     "failing",
		operands: alike(1, boolOperand, false),
		result:   fixedResult(Type{boolType}),
		known: func([]Value) Value {
			return errorValue(Diagnostic{Code: "app.failed", Message: "it failed"})
		},
		registered: true,
	}
	if got := failing.apply(Bool(true)); !got.IsError() {
		t.Errorf("an operation that failed gave %v, want an error value", got)
	}
}

func TestConformance_UN023_PendingWithNoSettledResultType(t *testing.T) {
	conformance.Covers(t, "UN-023")
	// An operation whose result type follows its operand's. Every operation
	// that ships has a fixed result type, so this one stands in for those that
	// conversion and access will bring.
	same := &op{
		name:     "same",
		operands: alike(1, Any(), true),
		result: func(types []Type) Constraint {
			if types[0].t == nil {
				return Any()
			}
			return Exactly(types[0])
		},
		known:      func(args []Value) Value { return args[0] },
		registered: true,
	}
	// A pending operand whose constraint names one type settles the result
	// type, so the answer is an unknown of that type and not another pending.
	if got := same.apply(Pending(Exactly(StringType()))).String(); got != "unknown(string, not null)" {
		t.Errorf("same of a pending string gave %s", got)
	}
	// A constraint that names no single type leaves the result pending, with
	// the constraint that the operation derived.
	got := same.apply(Pending(Any()))
	if !got.IsPending() {
		t.Fatalf("same of a pending value gave %v, want a pending value", got)
	}
	if want := "pending(any, not null)"; got.String() != want {
		t.Errorf("same of a pending value gave %s, want %s", got, want)
	}
	// An operand that is unknown has a type even without content, so the
	// result type follows from it and nothing stays pending.
	if got := same.apply(Unknown(NumberType())).String(); got != "unknown(number, not null)" {
		t.Errorf("same of an unknown number gave %s", got)
	}
}

// TestParameterizedOperationsAreBound holds an operation that takes parameters
// to its template: applying the template itself is a defect in the package,
// and so is binding parameters to an operation that takes none.
func TestParameterizedOperationsAreBound(t *testing.T) {
	mustPanicInternal(t, "Convert takes parameters, and was applied without them", func() {
		convertOp.apply(NumberFromInt(1))
	})
	mustPanicInternal(t, "Not takes no parameters", func() {
		notOp.with(conversion{Any(), Safe})
	})
	// Binding leaves the template as it was.
	b := convertOp.with(conversion{SetOf(Any()), Unsafe})
	if !b.operands[0].marksWithin || b.operands[0].within || convertOp.operands[0].marksWithin || convertOp.known != nil {
		t.Error("binding a conversion changed the registered template")
	}
	if len(convertOp.samples) == 0 {
		t.Error("Convert names no parameters for the operand matrix to check it with")
	}
}

// TestMakesSetLooksThroughOneOf holds makesSet, which tells the operand matrix
// where a conversion gives the marks within its operand to the result, to its
// doc: a OneOf makes a set when every member that admits a type does, and a
// constraint that admits no type makes nothing.
func TestMakesSetLooksThroughOneOf(t *testing.T) {
	num := NumberType()
	for _, tt := range []struct {
		c    Constraint
		want bool
	}{
		{SetOf(Any()), true},
		{Exactly(Set(num)), true},
		{ListOf(Any()), false},
		{Any(), false},
		{OneOf(SetOf(Any()), SetOf(Exactly(num))), true},
		{OneOf(SetOf(Any()), OneOf()), true},
		{OneOf(SetOf(Any()), ListOf(Any())), false},
		{OneOf(OneOf(SetOf(Any())), Exactly(Set(num))), true},
		{OneOf(), false},
		{SetOf(OneOf()), false},
	} {
		if got := makesSet(tt.c); got != tt.want {
			t.Errorf("makesSet(%s) = %t, want %t", tt.c, got, tt.want)
		}
	}
}
