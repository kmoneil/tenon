package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// TestConformance_UN007_OrderReadsBoundsAndPrefixes holds LessThan to what the
// ranges of its operands settle: where every value one may have comes before
// every value the other may have, or none does, the answer is known.
func TestConformance_UN007_OrderReadsBoundsAndPrefixes(t *testing.T) {
	conformance.Covers(t, "UN-007", "EQ-020", "UN-006")
	num, str := tenon.NumberType(), tenon.StringType()
	n := func(text string) tenon.Value { return tenon.NumberFromText(text) }
	s := func(text string) tenon.Value { return tenon.String(text) }
	between := func(lo, hi string) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(lo), true), tenon.NumberMax(n(hi), true))
	}
	atLeast := func(lo string, inclusive bool) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(lo), inclusive))
	}
	atMost := func(hi string, inclusive bool) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMax(n(hi), inclusive))
	}
	prefixed := func(p string) tenon.Value {
		return tenon.Narrow(tenon.Unknown(str), tenon.NotNull(), tenon.StringPrefix(p))
	}
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"a port of 1024 or more against 80", atLeast("1024", true), n("80"), "false"},
		{"at most 10 against 100", atMost("10", true), n("100"), "true"},
		{"below 10 against 10 or more", atMost("10", false), atLeast("10", true), "true"},
		{"at most 10 against 10 or more, both of which may be 10", atMost("10", true), atLeast("10", true), "unknown(bool, not null)"},
		{"10 or more against at most 10", atLeast("10", true), atMost("10", true), "false"},
		{"two ranges apart", between("1", "2"), between("3", "4"), "true"},
		{"the other way about", between("3", "4"), between("1", "2"), "false"},
		{"two ranges that overlap", between("1", "3"), between("2", "4"), "unknown(bool, not null)"},
		// The values a range holds other than null decide it, as they bound
		// a sum: a null operand is an error, which appears once it is known.
		{"a range that may be null", tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n("1024"), true)), n("80"), "false"},
		// A prefix orders every value that begins with it. These end in a
		// character nothing composes with, which a prefix keeps (UN-006).
		{"a prefix against a string it comes before", prefixed("ab-"), s("ab."), "true"},
		{"a prefix against a string it comes after", prefixed("ab."), s("ab-"), "false"},
		{"a prefix against a string that properly begins it", prefixed("ab-"), s("ab"), "false"},
		{"a string that properly begins a prefix", s("ab"), prefixed("ab-"), "true"},
		{"a prefix against the same string", prefixed("ab-"), s("ab-"), "unknown(bool, not null)"},
		{"a prefix against a longer prefix", prefixed("a-"), prefixed("a-b-"), "unknown(bool, not null)"},
		{"two prefixes apart", prefixed("ab-"), prefixed("ab."), "true"},
		// A prefix whose last letter a mark may follow keeps only what the
		// mark cannot change: "abc" is recorded as "ab", since a cedilla
		// after the c makes U+00E7, which comes after "abd".
		{"a prefix that loses its last letter", prefixed("abc"), s("abd"), "unknown(bool, not null)"},
		// A prefix is what normalization leaves standing (UN-006): "cafe"
		// is recorded as "caf", since what follows may compose with the e,
		// and "caf" with U+00E9 comes after "caff" where "cafe" comes before it.
		{"a prefix whose last character may compose", prefixed("cafe"), s("caff"), "unknown(bool, not null)"},
		{"what it may compose to", s("caf\U000000e9"), s("caff"), "false"},
	} {
		if got := tenon.LessThan(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: %v before %v is %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
	}
}

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
