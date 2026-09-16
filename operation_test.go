package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// equalDiagnostics reports whether two lists hold the same diagnostics in the
// same order.
func equalDiagnostics(got, want []tenon.Diagnostic) bool {
	return slices.EqualFunc(got, want, tenon.Diagnostic.Equal)
}

func TestConformance_ER005_DiagnosticsConcatenate(t *testing.T) {
	conformance.Covers(t, "ER-005")
	shared := tenon.Diagnostic{Code: "app.shared", Message: "shared", Path: tenon.Path{}.Attribute("a")}
	first := tenon.Diagnostic{Code: "app.first", Message: "first"}
	second := tenon.Diagnostic{Code: "app.second", Message: "second"}
	// The same code and message at another path is another diagnostic.
	elsewhere := tenon.Diagnostic{Code: shared.Code, Message: shared.Message, Path: tenon.Path{}.Attribute("b")}

	a := tenon.ErrorVal(first, shared)
	b := tenon.ErrorVal(shared, second, elsewhere)
	got := tenon.And(a, b)
	if !got.IsError() {
		t.Fatalf("an operation with two error operands gave %v", got)
	}
	if d := got.Diagnostics(); !equalDiagnostics(d, []tenon.Diagnostic{first, shared, second, elsewhere}) {
		t.Errorf("diagnostics %v; want both operands' diagnostics in operand order, the shared one once", d)
	}

	// Operand order decides the order of the diagnostics.
	if d := tenon.And(b, a).Diagnostics(); !equalDiagnostics(d, []tenon.Diagnostic{shared, second, elsewhere, first}) {
		t.Errorf("with the operands the other way around, diagnostics %v", d)
	}

	// Duplicates within one operand collapse as well.
	if d := tenon.Not(tenon.ErrorVal(first, first, second)).Diagnostics(); !equalDiagnostics(d, []tenon.Diagnostic{first, second}) {
		t.Errorf("diagnostics %v; want the repeated one once", d)
	}

	// An operation with one error operand carries that operand's diagnostics.
	if d := tenon.Or(tenon.Bool(false), a).Diagnostics(); !equalDiagnostics(d, []tenon.Diagnostic{first, shared}) {
		t.Errorf("diagnostics %v; want the error operand's own", d)
	}
}

func TestConformance_ER006_ErrorsDoNotShortCircuit(t *testing.T) {
	conformance.Covers(t, "ER-006")
	err := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"})
	tr, fa := tenon.Bool(true), tenon.Bool(false)
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"false AND error", tenon.And(fa, err)},
		{"error AND false", tenon.And(err, fa)},
		{"true AND error", tenon.And(tr, err)},
		{"error AND error", tenon.And(err, err)},
		{"true OR error", tenon.Or(tr, err)},
		{"error OR true", tenon.Or(err, tr)},
		{"false OR error", tenon.Or(fa, err)},
		{"NOT error", tenon.Not(err)},
	} {
		if !tt.got.IsError() {
			t.Errorf("%s = %v; want an error value", tt.name, tt.got)
		}
	}

	// Without an error operand, the operators are the ordinary ones.
	for _, tt := range []struct{ a, b, and, or bool }{
		{true, true, true, true},
		{true, false, false, true},
		{false, true, false, true},
		{false, false, false, false},
	} {
		if got := tenon.And(tenon.Bool(tt.a), tenon.Bool(tt.b)); got.AsBool() != tt.and {
			t.Errorf("%t AND %t = %v", tt.a, tt.b, got)
		}
		if got := tenon.Or(tenon.Bool(tt.a), tenon.Bool(tt.b)); got.AsBool() != tt.or {
			t.Errorf("%t OR %t = %v", tt.a, tt.b, got)
		}
	}
	if tenon.Not(tr).AsBool() || !tenon.Not(fa).AsBool() {
		t.Error("NOT does not negate")
	}

	// An operand of another type is a mistake in the calling program, and
	// stays one when the other operand is an error value that would otherwise
	// have decided the result.
	mustPanicUsage(t, "And: the first operand is a value of type number, which does not satisfy exactly(bool)", func() {
		tenon.And(tenon.NumberFromInt(1), tr)
	})
	mustPanicUsage(t, "Or: the second operand is a value of type string, which does not satisfy exactly(bool)", func() {
		tenon.Or(tr, tenon.String("x"))
	})
	mustPanicUsage(t, "Not: the operand is a value of type number, which does not satisfy exactly(bool)", func() {
		tenon.Not(tenon.NumberFromInt(1))
	})
	mustPanicUsage(t, "And: the second operand is a value of type number", func() {
		tenon.And(err, tenon.NumberFromInt(1))
	})
}

func TestIsNullOnAValueWithNoAnswer(t *testing.T) {
	// An error value carries forward, as through any other operation.
	bad := tenon.String("\xff")
	got := tenon.IsNull(bad)
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("IsNull of an error value is %v, want its diagnostics", got)
	}
	mustPanicUsage(t, "use of the zero Value", func() { tenon.IsNull(tenon.Value{}) })
}

func TestConformance_UN007_OperationsNarrowWhatTheyCan(t *testing.T) {
	conformance.Covers(t, "UN-007")
	tr, fa := tenon.Bool(true), tenon.Bool(false)
	unknown := tenon.Unknown(tenon.BoolType())
	pending := tenon.Pending(tenon.Any())
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		// An operand that settles the answer settles it however little is
		// known about the other one: one value is as narrow as a result gets.
		{"false AND unknown", tenon.And(fa, unknown), "false"},
		{"unknown AND false", tenon.And(unknown, fa), "false"},
		{"false AND pending", tenon.And(fa, pending), "false"},
		{"true OR unknown", tenon.Or(tr, unknown), "true"},
		{"unknown OR true", tenon.Or(unknown, tr), "true"},
		{"pending OR true", tenon.Or(pending, tr), "true"},
		// Where it does not, the result holds both answers, and not null,
		// which these operations never produce.
		{"true AND unknown", tenon.And(tr, unknown), "unknown(bool, not null)"},
		{"false OR unknown", tenon.Or(fa, unknown), "unknown(bool, not null)"},
		{"NOT unknown", tenon.Not(unknown), "unknown(bool, not null)"},
	} {
		if got := tt.got.String(); got != tt.want {
			t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
		}
	}
	if tenon.Not(unknown).Range().AllowsNull() {
		t.Error("the result of Not holds null, which Not never produces")
	}
}

func TestConformance_UN009_NullOperands(t *testing.T) {
	conformance.Covers(t, "UN-009")
	bl := tenon.BoolType()
	null, tr, fa := tenon.NullVal(bl), tenon.Bool(true), tenon.Bool(false)
	for _, tt := range []struct {
		name string
		got  tenon.Value
		msg  string
	}{
		{"null AND true", tenon.And(null, tr), "the first operand of And is null, which And cannot use"},
		{"true AND null", tenon.And(tr, null), "the second operand of And is null, which And cannot use"},
		{"NOT null", tenon.Not(null), "the operand of Not is null, which Not cannot use"},
	} {
		if !tt.got.IsError() {
			t.Errorf("%s = %v, want an error value", tt.name, tt.got)
			continue
		}
		diags := tt.got.Diagnostics()
		if len(diags) != 1 || diags[0].Code != tenon.CodeOperationNullOperand || diags[0].Message != tt.msg {
			t.Errorf("%s gave %v, want one %s saying %q", tt.name, diags, tenon.CodeOperationNullOperand, tt.msg)
		}
	}
	// A null operand is not short-circuited away, for the reason an error
	// operand is not: whoever wrote it wants to hear about it.
	if got := tenon.And(fa, null); !got.IsError() {
		t.Errorf("false AND null = %v, want an error value", got)
	}
	// Every null operand is reported, not only the first.
	if got := tenon.And(null, null); len(got.Diagnostics()) != 2 {
		t.Errorf("null AND null gave %v, want a diagnostic for each operand", got)
	}
	// An operation that has an answer for null gives it.
	if got := tenon.IsNull(null).String(); got != "true" {
		t.Errorf("IsNull of null is %s, want true", got)
	}
	// An operand that is unknown and may still be null is not caught here: the
	// operation answers from the range, and the error waits for the value.
	if got := tenon.Not(tenon.Unknown(bl)); got.IsError() {
		t.Errorf("NOT of an unknown that may be null is %v, want an unknown Bool", got)
	}
	// A pending operand known to be null is caught, since that much is settled.
	if got := tenon.Not(tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())); !got.IsError() {
		t.Errorf("NOT of a pending null is %v, want an error value", got)
	}
}

func TestConformance_UN023_PendingOperands(t *testing.T) {
	conformance.Covers(t, "UN-023")
	bl, num := tenon.BoolType(), tenon.NumberType()
	pending := tenon.Pending(tenon.Any())
	// These operations have a fixed result type, so a pending operand gives an
	// unknown of that type rather than another pending value.
	for _, tt := range []struct {
		name string
		got  tenon.Value
	}{
		{"NOT pending", tenon.Not(pending)},
		{"true AND pending", tenon.And(tenon.Bool(true), pending)},
		{"pending OR false", tenon.Or(pending, tenon.Bool(false))},
		{"IsNull of pending", tenon.IsNull(pending)},
	} {
		if got, want := tt.got.String(), "unknown(bool, not null)"; got != want {
			t.Errorf("%s = %s, want %s", tt.name, got, want)
		}
	}
	// A pending operand that can only be a Bool is as good as a Bool.
	if got := tenon.Not(tenon.Pending(tenon.Exactly(bl))).String(); got != "unknown(bool, not null)" {
		t.Errorf("NOT of a pending bool = %s", got)
	}
	// One that cannot be a Bool at all is an error value: the operation can
	// never apply, whatever the value turns out to be.
	bad := tenon.Not(tenon.Pending(tenon.Exactly(num)))
	if !bad.IsError() {
		t.Fatalf("NOT of a pending number = %v, want an error value", bad)
	}
	d := bad.Diagnostics()[0]
	want := "the operand of Not is pending with constraint exactly(number), and no type it allows satisfies exactly(bool)"
	if d.Code != tenon.CodeOperationWrongType || d.Message != want {
		t.Errorf("NOT of a pending number gave %v, want %s saying %q", d, tenon.CodeOperationWrongType, want)
	}
}
