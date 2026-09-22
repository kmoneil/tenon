package tenon_test

import (
	"slices"
	"strconv"
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

	// Operands carrying many diagnostics collapse the same way, on either
	// side of the count where propagation stops comparing each diagnostic
	// with every one taken and starts looking it up: the first operand's in
	// order, then those of the second that are new.
	for _, count := range []int{4, 40} {
		var left, right []tenon.Diagnostic
		var want []tenon.Diagnostic
		for i := range count {
			d := tenon.Diagnostic{Code: "app.left", Message: "d" + strconv.Itoa(i)}
			left = append(left, d)
			want = append(want, d)
			if i%2 == 0 {
				right = append(right, d) // shared, so recorded once
				continue
			}
			only := tenon.Diagnostic{Code: "app.right", Message: "r" + strconv.Itoa(i)}
			right = append(right, only)
		}
		for _, d := range right {
			if d.Code == "app.right" {
				want = append(want, d)
			}
		}
		got := tenon.And(tenon.ErrorVal(left...), tenon.ErrorVal(right...)).Diagnostics()
		if !equalDiagnostics(got, want) {
			t.Errorf("with %d diagnostics on each operand, got %d, want %d", count, len(got), len(want))
		}
	}

	// Two operands carrying thousands of diagnostics between them propagate
	// in time proportional to them, which took seconds when each was compared
	// with every one taken.
	const many = 10_000
	var left, right []tenon.Diagnostic
	for i := range many {
		left = append(left, tenon.Diagnostic{Code: "app.left", Message: "l" + strconv.Itoa(i)})
		right = append(right, tenon.Diagnostic{Code: "app.right", Message: "r" + strconv.Itoa(i)})
	}
	if d := tenon.And(tenon.ErrorVal(left...), tenon.ErrorVal(right...)).Diagnostics(); len(d) != 2*many {
		t.Errorf("two operands of %d diagnostics gave %d", many, len(d))
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
	conformance.Covers(t, "UN-023", "ER-001")
	bl, num, str := tenon.BoolType(), tenon.NumberType(), tenon.StringType()
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
	// A pending operand that could still be of a type the operation accepts is
	// answered as a value of that type would be, whatever else its constraint
	// admits.
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{"NOT of a pending bool", tenon.Not(tenon.Pending(tenon.Exactly(bl))), "unknown(bool, not null)"},
		{"Length of a pending list", tenon.Length(tenon.Pending(tenon.Exactly(tenon.List(str)))), "unknown(number, not null, >= 0)"},
		{"Contains of a pending set", tenon.Contains(tenon.Pending(tenon.Exactly(tenon.Set(str))), tenon.String("a")), "unknown(bool, not null)"},
		{"Length of a pending list of anything", tenon.Length(tenon.Pending(tenon.ListOf(tenon.Any()))), "unknown(number, not null, >= 0)"},
		{
			"Contains of a pending set of strings",
			tenon.Contains(tenon.Pending(tenon.SetOf(tenon.Exactly(str))), tenon.String("a")),
			"unknown(bool, not null)",
		},
		{
			"Length of a pending string or tuple",
			tenon.Length(tenon.Pending(tenon.OneOf(tenon.Exactly(str), tenon.TupleOf(tenon.Any())))),
			"unknown(number, not null, >= 0)",
		},
	} {
		if got := tt.got.String(); got != tt.want {
			t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
		}
	}
	// One that can only be of a type the operation rejects is an error value:
	// the operation can never apply, whatever the value turns out to be. That
	// holds where the operation accepts one of several kinds, as Length does,
	// or a kind with any element type, as Contains does, and not only where it
	// accepts a single type. It holds however the operand's constraint is
	// written, one type or a kind of type or several, and for a constraint no
	// type satisfies at all. Operands that must have one type between them
	// fail alike where no type the operation accepts satisfies both, although
	// each alone could be one it accepts.
	lengthOperand := "one_of([exactly(string), list_of(any), set_of(any), map_of(any)])"
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{
			"NOT of a pending number",
			tenon.Not(tenon.Pending(tenon.Exactly(num))),
			"the operand of Not is pending with constraint exactly(number), and no type it allows satisfies exactly(bool)",
		},
		{
			"Length of a pending number",
			tenon.Length(tenon.Pending(tenon.Exactly(num))),
			"the operand of Length is pending with constraint exactly(number), and no type it allows satisfies " + lengthOperand,
		},
		{
			"Contains of a pending list",
			tenon.Contains(tenon.Pending(tenon.Exactly(tenon.List(str))), tenon.String("a")),
			"the first operand of Contains is pending with constraint exactly(list(string)), and no type it allows satisfies set_of(any)",
		},
		{
			"Contains of a pending list of anything",
			tenon.Contains(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.String("a")),
			"the first operand of Contains is pending with constraint list_of(any), and no type it allows satisfies set_of(any)",
		},
		{
			"Length of a pending empty tuple",
			tenon.Length(tenon.Pending(tenon.TupleOf())),
			"the operand of Length is pending with constraint tuple_of([]), and no type it allows satisfies " + lengthOperand,
		},
		{
			"Length of a pending tuple of one",
			tenon.Length(tenon.Pending(tenon.TupleOf(tenon.Any()))),
			"the operand of Length is pending with constraint tuple_of([any]), and no type it allows satisfies " + lengthOperand,
		},
		{
			"Length of a pending number written as one of one",
			tenon.Length(tenon.Pending(tenon.OneOf(tenon.Exactly(num)))),
			"the operand of Length is pending with constraint one_of([exactly(number)]), and no type it allows satisfies " + lengthOperand,
		},
		{
			"IsNull of a pending value that no type satisfies",
			tenon.IsNull(tenon.Pending(tenon.OneOf())),
			"the operand of IsNull is pending with constraint one_of([]), and no type it allows satisfies any",
		},
		{
			"LessThan of a pending number written as one of one, and a string",
			tenon.LessThan(tenon.Pending(tenon.OneOf(tenon.Exactly(num))), tenon.String("a")),
			"the first operand of LessThan is pending with constraint one_of([exactly(number)]) and the second operand is " +
				"a value of type string, and LessThan takes operands of one type",
		},
		{
			"LessThan of two pendings that share only a type it rejects",
			tenon.LessThan(
				tenon.Pending(tenon.OneOf(tenon.Exactly(num), tenon.Exactly(bl))),
				tenon.Pending(tenon.OneOf(tenon.Exactly(str), tenon.Exactly(bl))),
			),
			"the first operand of LessThan is pending with constraint one_of([exactly(number), exactly(bool)]) and the second " +
				"operand is pending with constraint one_of([exactly(string), exactly(bool)]), and LessThan takes operands of one type",
		},
	} {
		want := []tenon.Diagnostic{{Code: tenon.CodeOperationWrongType, Message: tt.want}}
		if !tt.got.IsError() || !equalDiagnostics(tt.got.Diagnostics(), want) {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, want)
		}
	}
	// Once resolved, the operand is a value the calling program chose to pass,
	// and passing one of a type the operation rejects is that program's mistake
	// (ER-001). While the type is still the data's, so is the mistake, which is
	// why the answers above are error values.
	mustPanicUsage(t, "does not satisfy one_of", func() {
		tenon.Length(tenon.Resolve(tenon.Pending(tenon.Exactly(num)), num))
	})
}
