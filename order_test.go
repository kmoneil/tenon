package tenon_test

import (
	"testing"

	"tenon"
	"tenon/conformance"
)

func TestConformance_EQ020_Ordering(t *testing.T) {
	conformance.Covers(t, "EQ-020")
	num, str := tenon.NumberType(), tenon.StringType()
	n := func(text string) tenon.Value { return tenon.NumberFromText(text) }
	s := func(text string) tenon.Value { return tenon.String(text) }
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		// Numbers compare numerically, however each was written.
		{"two integers", n("1"), n("2"), "true"},
		{"the other way about", n("2"), n("1"), "false"},
		{"a number with itself", n("1"), n("1.000"), "false"},
		{"across zero", n("-1"), n("0"), "true"},
		{"a fraction", n("1.5"), n("1.51"), "true"},
		{"magnitudes", n("9e99"), n("1e100"), "true"},
		{"a long coefficient against a short one", n("1.0000000000000000000000001"), n("1.1"), "true"},
		// Strings compare by scalar value over the form the value holds.
		{"two letters", s("a"), s("b"), "true"},
		{"a prefix of the other", s("ab"), s("abc"), "true"},
		{"the empty string", s(""), s("a"), "true"},
		{"a string with itself", s("a"), s("a"), "false"},
		// The normalized form is what is compared, not the text that was
		// handed over: "e" and a combining acute is one character, U+00E9,
		// which comes after "f" rather than before it.
		{"a decomposed character against a letter", s("e\U00000301"), s("f"), "false"},
		{"the composed character, which is the same value", s("\U000000e9"), s("f"), "false"},
		{"the letter against it", s("f"), s("e\U00000301"), "true"},
		// An operand that is not known leaves the answer open.
		{"an unknown number", tenon.Unknown(num), n("1"), "unknown(bool, not null)"},
		{"an unknown string", s("a"), tenon.Unknown(str), "unknown(bool, not null)"},
		{"a pending operand", n("1"), tenon.Pending(tenon.Exactly(num)), "unknown(bool, not null)"},
	} {
		if got := tenon.LessThan(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: %v before %v is %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
	}
	// Ordering is defined for these two types and for nothing else, and asking
	// for it elsewhere is a mistake in the calling program.
	for _, tt := range []struct {
		name string
		v    tenon.Value
	}{
		{"a bool", tenon.Bool(true)},
		{"a list", tenon.ListVal(str)},
		{"a set", tenon.SetVal(str)},
		{"a map", tenon.MapVal(str, nil)},
		{"a tuple", tenon.TupleVal()},
		{"an object", tenon.ObjectVal(nil)},
		{"an unknown of a type with no order", tenon.Unknown(tenon.List(str))},
	} {
		mustPanicUsage(t, "does not satisfy one_of([exactly(number), exactly(string)])", func() {
			tenon.LessThan(tt.v, tt.v)
		})
	}
	// Two types that each have an order still have none between them.
	mustPanicUsage(t, "LessThan: the first operand is a value of type number and the second operand is a value of type string, but LessThan takes operands of one type", func() {
		tenon.LessThan(n("1"), s("1"))
	})
	mustPanicUsage(t, "takes operands of one type", func() {
		tenon.LessThan(tenon.Unknown(num), s("1"))
	})
	// Null has no place in an order, and an error operand carries forward.
	nullOrder := tenon.LessThan(tenon.NullVal(num), n("1"))
	if !nullOrder.IsError() || nullOrder.Diagnostics()[0].Code != tenon.CodeOperationNullOperand {
		t.Errorf("a null operand gave %v, want a null-operand error value", nullOrder)
	}
	bad := tenon.LessThan(tenon.String("\xff"), s("a"))
	if !bad.IsError() || bad.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("an error operand gave %v, want its diagnostics", bad)
	}
	// A pending operand that can never be ordered is bad data rather than a
	// panic: nothing about the call was wrong when it was made.
	wrong := tenon.LessThan(tenon.Pending(tenon.Exactly(tenon.List(str))), n("1"))
	if !wrong.IsError() || wrong.Diagnostics()[0].Code != tenon.CodeOperationWrongType {
		t.Errorf("a pending list operand gave %v, want a wrong-type error value", wrong)
	}
	// So is one that could be ordered on its own but not against this operand.
	mismatch := tenon.LessThan(tenon.Pending(tenon.Exactly(str)), n("1"))
	if !mismatch.IsError() || mismatch.Diagnostics()[0].Code != tenon.CodeOperationWrongType {
		t.Errorf("a pending string against a number gave %v, want a wrong-type error value", mismatch)
	}
	// The message says what each pending operand's type will be, rather than
	// only that it is pending.
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{
			"a pending string against a number",
			mismatch,
			"the first operand of LessThan is pending with constraint exactly(string) and the second operand is a value of type number, and LessThan takes operands of one type",
		},
		{
			"two pending operands of different types",
			tenon.LessThan(tenon.Pending(tenon.Exactly(num)), tenon.Pending(tenon.Exactly(str))),
			"the first operand of LessThan is pending with constraint exactly(number) and the second operand is pending with constraint exactly(string), and LessThan takes operands of one type",
		},
	} {
		if ds := tt.got.Diagnostics(); len(ds) != 1 || ds[0].Message != tt.want {
			t.Errorf("%s: the diagnostics are %v, want the message %q", tt.name, ds, tt.want)
		}
	}
	// A constraint that rules nothing out leaves the answer open instead.
	if got := tenon.LessThan(tenon.Pending(tenon.Any()), n("1")).String(); got != "unknown(bool, not null)" {
		t.Errorf("a pending operand of no settled type gave %s, want an unknown Bool", got)
	}
}

func TestOrderIsATotalOrderOverKnownValues(t *testing.T) {
	// LessThan is irreflexive and transitive, and of any two values that are
	// not equal exactly one comes first.
	nums := []tenon.Value{
		tenon.NumberFromText("-1e10"), tenon.NumberFromText("-1"), tenon.NumberFromInt(0),
		tenon.NumberFromText("0.5"), tenon.NumberFromInt(1), tenon.NumberFromText("1.000"),
		tenon.NumberFromInt(2), tenon.NumberFromText("1e100"),
	}
	strs := []tenon.Value{
		tenon.String(""), tenon.String("a"), tenon.String("ab"), tenon.String("b"),
		tenon.String("e\U00000301"), tenon.String("\U000000e9"), tenon.String("f"),
	}
	for _, group := range [][]tenon.Value{nums, strs} {
		for _, a := range group {
			if tenon.LessThan(a, a).AsBool() {
				t.Errorf("%v comes before itself", a)
			}
			for _, b := range group {
				ab, ba := tenon.LessThan(a, b).AsBool(), tenon.LessThan(b, a).AsBool()
				eq := tenon.Equals(a, b).AsBool()
				if eq && (ab || ba) {
					t.Errorf("%v and %v are equal but one comes before the other", a, b)
				}
				if !eq && ab == ba {
					t.Errorf("%v and %v differ, and neither or both come first", a, b)
				}
				if !ab {
					continue
				}
				for _, c := range group {
					if tenon.LessThan(b, c).AsBool() && !tenon.LessThan(a, c).AsBool() {
						t.Errorf("%v before %v before %v, but not %v before %v", a, b, c, a, c)
					}
				}
			}
		}
	}
}
