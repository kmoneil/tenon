package tenon_test

import (
	"math"
	"math/big"
	"slices"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

func TestConformance_VA001_ThreeStates(t *testing.T) {
	conformance.Covers(t, "VA-001")
	type thing struct{}
	holder := tenon.Capsule("holder", tenon.CapsuleOps[thing]{})
	for _, tt := range []struct {
		v     tenon.Value
		state string
	}{
		{tenon.Bool(true), "resolved"},
		{tenon.NumberFromInt(7), "resolved"},
		{tenon.NumberFromText("-1.5e3"), "resolved"},
		{tenon.String("text"), "resolved"},
		{tenon.CapsuleVal(holder, &thing{}), "resolved"},
		{tenon.String("\xff"), "error"},
		{tenon.NumberFromText("1,000"), "error"},
		{tenon.Pending(tenon.Any()), "pending"},
		{tenon.Pending(tenon.ListOf(tenon.Exactly(tenon.StringType()))), "pending"},
	} {
		got := []bool{tt.v.IsError(), tt.v.IsResolved(), tt.v.IsPending()}
		want := []bool{tt.state == "error", tt.state == "resolved", tt.state == "pending"}
		if !slices.Equal(got, want) {
			t.Errorf("%v: IsError, IsResolved, IsPending = %v; want a %s value", tt.v, got, tt.state)
		}
	}
}

func TestConformance_ER001_UsageErrorsPanic(t *testing.T) {
	conformance.Covers(t, "ER-001")
	bad, pending := tenon.String("\xff"), tenon.Pending(tenon.Any())
	mustPanicUsage(t, "Type called on an error value, which has no type", func() { bad.Type() })
	mustPanicUsage(t, "Type called on a pending value, which has no type", func() { pending.Type() })
	mustPanicUsage(t, "not a value of kind Bool", func() { tenon.NumberFromInt(1).AsBool() })
	mustPanicUsage(t, "not a value of kind String", func() { bad.AsString() })
	mustPanicUsage(t, "which is not an error value", func() { tenon.Bool(true).Diagnostics() })
	mustPanicUsage(t, "not a collection kind", func() { tenon.NumberType().ElementType() })
	mustPanicUsage(t, "use of the zero Value", func() { tenon.Value{}.IsResolved() })
	mustPanicUsage(t, "zero Constraint", func() { tenon.Pending(tenon.Constraint{}) })
}

func TestConformance_ST004_InvalidUTF8(t *testing.T) {
	conformance.Covers(t, "ST-004")
	for _, tt := range []struct {
		s  string
		at string // where the message locates the problem
	}{
		{"\xff", "byte 0"},
		{"a\xe2\x82b", "byte 1"},
		{"\xed\xa0\x80", "byte 0"},
		{"\xc0\x80", "byte 0"},
		{"caf\xc3\xa9\xff", "byte 5"},
	} {
		v := tenon.String(tt.s)
		if !v.IsError() {
			t.Errorf("String(%+q) = %v; want an error value", tt.s, v)
			continue
		}
		if d := v.Diagnostics(); len(d) != 1 || d[0].Code != tenon.CodeStringInvalidUTF8 || !strings.HasSuffix(d[0].Message, tt.at) {
			t.Errorf("String(%+q) has diagnostics %v", tt.s, d)
		}
	}

	// Well-formed text is accepted and normalized, and gains no replacement
	// characters.
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"text", "text"},
		{"cafe\u0301", "caf\u00e9"},
		{"\ufffd", "\ufffd"},
	} {
		if v := tenon.String(tt.in); !v.IsResolved() || v.AsString() != tt.want {
			t.Errorf("String(%+q) = %v; want %+q", tt.in, v, tt.want)
		}
	}
}

func TestConformance_BO001_BoolDomain(t *testing.T) {
	conformance.Covers(t, "BO-001")
	tr, fa := tenon.Bool(true), tenon.Bool(false)
	if tr.Type() != tenon.BoolType() || fa.Type() != tenon.BoolType() {
		t.Error("Bool values do not have type Bool")
	}
	if !tr.AsBool() || fa.AsBool() || tr == fa {
		t.Error("true and false are not two distinct Bool values")
	}
	// Every Bool value is one of the two.
	for _, b := range []bool{true, false} {
		if v := tenon.Bool(b); v != tr && v != fa {
			t.Errorf("Bool(%t) is neither true nor false", b)
		}
	}
	if tr.String() != "true" || fa.String() != "false" {
		t.Errorf("Bool values display as %s and %s", tr, fa)
	}
}

func TestNumberValues(t *testing.T) {
	for _, tt := range []struct {
		v     tenon.Value
		text  string
		i     int64
		isInt bool
	}{
		{tenon.NumberFromInt(0), "0", 0, true},
		{tenon.NumberFromInt(math.MinInt64), "-9223372036854775808", math.MinInt64, true},
		{tenon.NumberFromText("1.2e3"), "1200", 1200, true},
		{tenon.NumberFromText("-0.25"), "-0.25", 0, false},
		{tenon.NumberFromText("1e30"), "1e30", 0, false},
	} {
		if tt.v.Type() != tenon.NumberType() || tt.v.String() != tt.text {
			t.Errorf("%v has type %v; want a Number with text %s", tt.v, tt.v.Type(), tt.text)
		}
		if i, ok := tt.v.AsInt64(); i != tt.i || ok != tt.isInt {
			t.Errorf("%v: AsInt64() = %d, %t", tt.v, i, ok)
		}
		if want, _ := new(big.Rat).SetString(tt.text); tt.v.AsBigRat().Cmp(want) != 0 {
			t.Errorf("%v: AsBigRat() = %s", tt.v, tt.v.AsBigRat().RatString())
		}
	}

	for _, tt := range []struct {
		text string
		code tenon.Code
	}{
		{"", tenon.CodeNumberInvalidSyntax},
		{"+5", tenon.CodeNumberInvalidSyntax},
		{"NaN", tenon.CodeNumberInvalidSyntax},
		{"1e1000000", tenon.CodeNumberOutOfRange},
	} {
		if v := tenon.NumberFromText(tt.text); !v.IsError() || v.Diagnostics()[0].Code != tt.code {
			t.Errorf("NumberFromText(%q) = %v; want an error value with code %s", tt.text, v, tt.code)
		}
	}
}

func TestCapsuleValues(t *testing.T) {
	type point struct{ x, y int }
	type other struct{}
	pointType := tenon.Capsule("point", tenon.CapsuleOps[point]{})
	p := &point{1, 2}
	v := tenon.CapsuleVal(pointType, p)
	if v.Type() != pointType || tenon.CapsuleValue[point](v) != p || v.String() != `capsule("point")` {
		t.Errorf("CapsuleVal(%v, %p) = %v", pointType, p, v)
	}
	mustPanicUsage(t, "does not encapsulate", func() { tenon.CapsuleVal(pointType, &other{}) })
	mustPanicUsage(t, "nil pointer", func() { tenon.CapsuleVal(pointType, (*point)(nil)) })
	mustPanicUsage(t, "does not encapsulate", func() { tenon.CapsuleValue[other](v) })
	mustPanicUsage(t, "not a value of kind Capsule", func() { tenon.CapsuleValue[point](tenon.Bool(true)) })
	mustPanicUsage(t, "whose kind is String, not Capsule", func() { tenon.CapsuleVal(tenon.StringType(), p) })
}

func TestPendingValues(t *testing.T) {
	c := tenon.ObjectWith(map[string]tenon.Field{"name": tenon.Required(tenon.Exactly(tenon.StringType()))}, false)
	if v := tenon.Pending(c); !v.IsPending() || v.String() != "pending("+c.String()+")" {
		t.Errorf("Pending(%v) = %v", c, v)
	}
}

func TestValueString(t *testing.T) {
	for _, tt := range []struct {
		v    tenon.Value
		want string
	}{
		{tenon.Bool(false), "false"},
		{tenon.NumberFromText("-2.50"), "-2.5"},
		{tenon.String(`say "hi"`), `"say \"hi\""`},
		{tenon.Pending(tenon.SetOf(tenon.Any())), "pending(set_of(any))"},
		{tenon.NumberFromText("x"), `error(number.invalid_syntax: "x" is not a number)`},
		{
			tenon.NumberFromText(strings.Repeat("7", 40) + "!"),
			`error(number.invalid_syntax: "` + strings.Repeat("7", 32) + `"... is not a number)`,
		},
		{tenon.Value{}, "<zero Value>"},
	} {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("String() = %s, want %s", got, tt.want)
		}
	}
}
