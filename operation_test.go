package tenon_test

import (
	"slices"
	"testing"

	"tenon"
	"tenon/conformance"
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

	// Operands are Bool values or error values, and nothing else.
	mustPanicUsage(t, "And: a value of type number is not a known Bool value", func() { tenon.And(tenon.NumberFromInt(1), tr) })
	mustPanicUsage(t, "is not a known Bool value", func() { tenon.Or(tr, tenon.String("x")) })
	mustPanicUsage(t, "a pending value is not a known Bool value", func() { tenon.Not(tenon.Pending(tenon.Any())) })
}

func TestIsNullOnValuesWithoutARange(t *testing.T) {
	// An error value carries forward, as through any other operation.
	bad := tenon.String("\xff")
	got := tenon.IsNull(bad)
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("IsNull of an error value is %v, want its diagnostics", got)
	}
	// A pending value has no range to answer from, until pending values carry
	// a nullness of their own.
	mustPanicUsage(t, "IsNull called on a pending value, which has no range", func() {
		tenon.IsNull(tenon.Pending(tenon.Any()))
	})
	mustPanicUsage(t, "use of the zero Value", func() { tenon.IsNull(tenon.Value{}) })
}
