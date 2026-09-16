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
		{tenon.NullVal(tenon.StringType()), "resolved"},
		{tenon.Unknown(tenon.StringType()), "resolved"},
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

func TestConformance_VA003_KnownIsASingletonRange(t *testing.T) {
	conformance.Covers(t, "VA-003")
	str, num := tenon.StringType(), tenon.NumberType()
	five := tenon.NumberFromInt(5)
	for _, tt := range []struct {
		name  string
		v     tenon.Value
		known bool
	}{
		{"a string", tenon.String("text"), true},
		{"the null value", tenon.NullVal(str), true},
		{"a fresh unknown", tenon.Unknown(str), false},
		{"an unknown that cannot be null", tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), false},
		{
			"an unknown narrowed to one value",
			tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(five, true), tenon.NumberMax(five, true), tenon.NotNull()),
			true,
		},
		{"an empty list", tenon.ListVal(str), true},
		{"a list of known elements", tenon.ListVal(str, tenon.String("a")), true},
		{"a list holding a null", tenon.ListVal(num, tenon.NullVal(num)), true},
		{"a list holding an unknown", tenon.ListVal(num, tenon.Unknown(num)), false},
		{
			"a list holding a list that holds an unknown",
			tenon.ListVal(tenon.List(num), tenon.ListVal(num, tenon.Unknown(num))),
			false,
		},
		{"a set holding an unknown", tenon.SetVal(num, tenon.Unknown(num)), false},
		{"a tuple of known elements", tenon.TupleVal(tenon.Bool(true)), true},
		{"a tuple holding an unknown", tenon.TupleVal(tenon.Bool(true), tenon.Unknown(str)), false},
		{"an object with an unknown attribute", tenon.ObjectVal(map[string]tenon.Value{"a": tenon.Unknown(str)}), false},
		{"a map with an unknown element", tenon.MapVal(str, map[string]tenon.Value{"k": tenon.Unknown(str)}), false},
		{"a map of known elements", tenon.MapVal(str, map[string]tenon.Value{"k": tenon.String("v")}), true},
	} {
		if !tt.v.IsResolved() {
			t.Errorf("%s: %v is not a resolved value", tt.name, tt.v)
		}
		if got := tt.v.IsKnown(); got != tt.known {
			t.Errorf("%s: IsKnown of %v is %t, want %t", tt.name, tt.v, got, tt.known)
		}
	}
	// A value that is not known still holds the members it was built from:
	// knownness is a fact about the range, not about what is there to read.
	l := tenon.ListVal(num, tenon.Unknown(num))
	if l.Len() != 1 || l.Index(0).IsKnown() {
		t.Errorf("a list holding an unknown does not read back as one: %v", l)
	}
}

func TestConformance_VA004_NullIsAMemberOfTheRange(t *testing.T) {
	conformance.Covers(t, "VA-004")
	str := tenon.StringType()
	null := tenon.NullVal(str)
	// A known null is an ordinary resolved value of its type, not a state of
	// its own, and it is known, because its range holds one value.
	if !null.IsResolved() || null.Type() != str || !null.IsKnown() {
		t.Errorf("the null value is not a known resolved value of its type: %v", null)
	}
	if null.IsError() || null.IsPending() {
		t.Errorf("the null value is in another state as well: %v", null)
	}
	// It is the value whose range is exactly null: narrowing to null reaches
	// it, and narrowing null away from it leaves nothing.
	if got := tenon.Narrow(tenon.Unknown(str), tenon.Null()); !got.IsKnown() || got.String() != null.String() {
		t.Errorf("narrowing to null gave %v, want %v", got, null)
	}
	if got := tenon.Narrow(null, tenon.NotNull()); !got.IsError() {
		t.Errorf("narrowing the null value to not null gave %v, want an error value", got)
	}
	// Null starts out in the range of every unknown, and leaves it only by
	// narrowing, which IsNull reports from the range.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"the null value", null, "true"},
		{"a known string", tenon.String("text"), "false"},
		{"a list, which is a value and not null", tenon.ListVal(str), "false"},
		{"a fresh unknown", tenon.Unknown(str), "unknown(bool, not null)"},
		{"an unknown that cannot be null", tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), "false"},
		{"an unknown that is still open about it", tenon.Narrow(tenon.Unknown(str), tenon.LengthMax(3)), "unknown(bool, not null)"},
	} {
		if got := tenon.IsNull(tt.v).String(); got != tt.want {
			t.Errorf("%s: IsNull is %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestConformance_UN020_PendingCarriesAConstraint(t *testing.T) {
	conformance.Covers(t, "UN-020")
	str := tenon.StringType()
	lst := tenon.List(str)
	p := tenon.Pending(tenon.ListOf(tenon.Exactly(str)))
	if !p.IsPending() {
		t.Fatalf("%v is not a pending value", p)
	}
	// The constraint says what the type may turn out to be: a type that
	// satisfies it resolves, and one that does not is the caller's mistake.
	got := tenon.Resolve(p, lst)
	if !got.IsResolved() || got.Type() != lst {
		t.Errorf("resolving %v to %v gave %v", p, lst, got)
	}
	if want := "unknown(list(string))"; got.String() != want {
		t.Errorf("resolving gave %s, want %s", got, want)
	}
	mustPanicUsage(t, "does not satisfy the constraint", func() { tenon.Resolve(p, str) })
	mustPanicUsage(t, "Resolve called on a value of type string, which is not a pending value", func() {
		tenon.Resolve(tenon.String("x"), str)
	})
}

func TestConformance_UN021_PendingAnyIsTheLeastInformative(t *testing.T) {
	conformance.Covers(t, "UN-021")
	type thing struct{}
	p := tenon.Pending(tenon.Any())
	// It resolves to every type there is, which no narrower constraint does.
	for _, ty := range []tenon.Type{
		tenon.BoolType(),
		tenon.NumberType(),
		tenon.StringType(),
		tenon.List(tenon.StringType()),
		tenon.Set(tenon.NumberType()),
		tenon.Map(tenon.BoolType()),
		tenon.Tuple(),
		tenon.Object(nil),
		tenon.Capsule("thing", tenon.CapsuleOps[thing]{}),
	} {
		if got := tenon.Resolve(p, ty); got.Type() != ty {
			t.Errorf("resolving the least-informative value to %v gave %v", ty, got)
		}
	}
	// It says nothing about null either, so nothing about it is settled.
	if want := "unknown(bool, not null)"; tenon.IsNull(p).String() != want {
		t.Errorf("IsNull of the least-informative value is %s, want %s", tenon.IsNull(p), want)
	}
}

func TestConformance_UN022_PendingHasNoType(t *testing.T) {
	conformance.Covers(t, "UN-022")
	str := tenon.StringType()
	p := tenon.Pending(tenon.Any())
	// The discriminator answers before a type is asked for, and asking for one
	// without testing first is a usage error rather than an answer.
	if !p.IsPending() || p.IsResolved() || p.IsError() {
		t.Errorf("the discriminators do not agree that %v is pending", p)
	}
	mustPanicUsage(t, "Type called on a pending value, which has no type", func() { p.Type() })
	mustPanicUsage(t, "Range called on a pending value, which has no range", func() { p.Range() })
	// Every other value answers the same discriminator.
	for _, v := range []tenon.Value{
		tenon.String("text"),
		tenon.NullVal(str),
		tenon.Unknown(str),
		tenon.String("\xff"),
	} {
		if v.IsPending() {
			t.Errorf("%v reports itself pending", v)
		}
	}
}

func TestConformance_UN024_PendingNullness(t *testing.T) {
	conformance.Covers(t, "UN-024")
	str := tenon.StringType()
	p := tenon.Pending(tenon.Any())
	null, notNull := tenon.Narrow(p, tenon.Null()), tenon.Narrow(p, tenon.NotNull())
	// The fact is there to read before the type is.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"undetermined", p, "unknown(bool, not null)"},
		{"known null", null, "true"},
		{"known not null", notNull, "false"},
	} {
		if got := tenon.IsNull(tt.v).String(); got != tt.want {
			t.Errorf("%s: IsNull is %s, want %s", tt.name, got, tt.want)
		}
	}
	if want := "pending(" + tenon.Any().String() + ", null)"; null.String() != want {
		t.Errorf("a pending value known to be null reads as %s, want %s", null, want)
	}
	// A null met before its type is known keeps the one thing the document
	// said: resolving it gives the known null of the type.
	resolved := tenon.Resolve(null, str)
	if !resolved.IsKnown() || resolved.Type() != str || tenon.IsNull(resolved).String() != "true" {
		t.Errorf("resolving a pending null to string gave %v, want the null string", resolved)
	}
	// The other two facts carry into the range of the result as well.
	if got, want := tenon.Resolve(notNull, str).String(), "unknown(string, not null)"; got != want {
		t.Errorf("resolving a pending not-null gave %s, want %s", got, want)
	}
	if got, want := tenon.Resolve(p, str).String(), "unknown(string)"; got != want {
		t.Errorf("resolving an undetermined pending gave %s, want %s", got, want)
	}
	// Facts that conflict leave nothing possible, and narrowings that speak of
	// a structure have no type here to speak of.
	got := tenon.Narrow(null, tenon.NotNull())
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeRangeContradiction {
		t.Errorf("narrowing a pending null to not null gave %v, want a contradiction", got)
	}
	mustPanicUsage(t, "does not apply to a pending value", func() { tenon.Narrow(p, tenon.LengthMax(1)) })
	// A narrowing that says nothing new leaves the value as it was.
	if again := tenon.Narrow(null, tenon.Null()); again != null {
		t.Errorf("narrowing a pending null to null again produced a new value")
	}
}
