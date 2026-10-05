package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// at returns the path of argument i.
func at(i int) tenon.Path { return tenon.Path{}.Index(tenon.NumberFromInt(int64(i))) }

// operands are numbers an operator is called with: known ones, spelled more
// than one way, unknown ones narrowed by a range, and a marked one.
func operands() []tenon.Value {
	num := tenon.NumberType()
	return []tenon.Value{
		tenon.NumberFromInt(7),
		tenon.NumberFromText("-2.50"),
		tenon.NumberFromText("1e30"),
		tenon.NumberFromInt(0),
		tenon.NumberFromText("0.1"),
		tenon.Unknown(num),
		tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(tenon.NumberFromInt(1), true), tenon.NumberMax(tenon.NumberFromInt(5), false)),
		tenon.WithMarks(tenon.NumberFromInt(3), secret{}),
	}
}

// sameAs holds f, applied to every pair of operands, to the answer of op.
func sameAs(t *testing.T, name string, f tenon.Function, op func(a, b tenon.Value) tenon.Value) {
	t.Helper()
	for _, a := range operands() {
		for _, b := range operands() {
			got, want := call(f, a, b), op(a, b)
			if !tenon.Identical(got, want) {
				t.Errorf("%s(%v, %v) = %v, want %v", name, a, b, got, want)
			}
		}
	}
}

func TestConformance_LN001_Arithmetic(t *testing.T) {
	conformance.Covers(t, "LN-001")
	for name, op := range map[string]func(a, b tenon.Value) tenon.Value{
		"Add": tenon.Add, "Subtract": tenon.Sub, "Multiply": tenon.Mul, "Divide": tenon.Div, "Modulo": tenon.Mod,
	} {
		f := library[name]
		sameAs(t, name, f, op)
		// A null operand fails at the call, located at it.
		for i := range 2 {
			args := []tenon.Value{tenon.NumberFromInt(1), tenon.NumberFromInt(1)}
			args[i] = tenon.Null(tenon.NumberType())
			got := call(f, args...)
			if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeOperationNullOperand || !got.Diagnostics()[0].Path.Equal(at(i)) {
				t.Errorf("%s with a null operand %d gave %v, want %s at %s", name, i, got, tenon.CodeOperationNullOperand, at(i))
			}
		}
	}
	// What go-cty answers in binary floating point is exact here, and a
	// division by zero fails where go-cty answers an infinity.
	if got := call(stdlib.AddFunc, tenon.NumberFromText("0.1"), tenon.NumberFromText("0.2")); !got.Equal(tenon.NumberFromText("0.3")) {
		t.Errorf("Add(0.1, 0.2) = %v, want 0.3", got)
	}
	if got := call(stdlib.DivideFunc, tenon.NumberFromInt(1), tenon.NumberFromInt(0)); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeNumberDivideByZero {
		t.Errorf("Divide(1, 0) = %v, want %s", got, tenon.CodeNumberDivideByZero)
	}
	// A string of digits is a number under the unsafe policy.
	if got := tenon.Call(stdlib.AddFunc, []tenon.Value{tenon.String("2"), tenon.NumberFromInt(3)}, tenon.Unsafe); !got.Equal(tenon.NumberFromInt(5)) {
		t.Errorf("Add(\"2\", 3) under Unsafe = %v, want 5", got)
	}
}

func TestConformance_LN002_Negate(t *testing.T) {
	conformance.Covers(t, "LN-002")
	for _, a := range operands() {
		if got, want := call(stdlib.NegateFunc, a), tenon.Sub(tenon.NumberFromInt(0), a); !tenon.Identical(got, want) {
			t.Errorf("Negate(%v) = %v, want %v", a, got, want)
		}
	}
	if got := call(stdlib.NegateFunc, tenon.NumberFromText("-2.5")); !got.Equal(tenon.NumberFromText("2.5")) {
		t.Errorf("Negate(-2.5) = %v", got)
	}
}

func TestConformance_LN010_Orderings(t *testing.T) {
	conformance.Covers(t, "LN-010")
	sameAs(t, "LessThan", stdlib.LessThanFunc, tenon.LessThan)
	sameAs(t, "GreaterThan", stdlib.GreaterThanFunc, func(a, b tenon.Value) tenon.Value { return tenon.LessThan(b, a) })
	sameAs(t, "LessThanOrEqualTo", stdlib.LessThanOrEqualToFunc, func(a, b tenon.Value) tenon.Value { return tenon.Not(tenon.LessThan(b, a)) })
	sameAs(t, "GreaterThanOrEqualTo", stdlib.GreaterThanOrEqualToFunc, func(a, b tenon.Value) tenon.Value { return tenon.Not(tenon.LessThan(a, b)) })
	// Strings are compared as the numbers they convert to, as a language
	// converts its operands: "10" is greater than "9".
	if got := tenon.Call(stdlib.GreaterThanFunc, []tenon.Value{tenon.String("10"), tenon.String("9")}, tenon.Unsafe); !got.Equal(tenon.Bool(true)) {
		t.Errorf("GreaterThan(\"10\", \"9\") under Unsafe = %v, want true", got)
	}
	if got := call(stdlib.GreaterThanFunc, tenon.String("10"), tenon.String("9")); !got.IsError() {
		t.Errorf("GreaterThan(\"10\", \"9\") under Safe = %v, want the conversion's failure", got)
	}
}

func TestConformance_LB011_KnownFailuresFailNow(t *testing.T) {
	conformance.Covers(t, "LB-011")
	// A divisor of zero fails every outcome, whatever the dividend turns
	// out to be, so the call fails now.
	unknown := tenon.Unknown(tenon.NumberType())
	for name, code := range map[string]tenon.Code{"Divide": tenon.CodeNumberDivideByZero, "Modulo": tenon.CodeNumberModuloByZero} {
		got := call(library[name], unknown, tenon.NumberFromInt(0))
		if !got.IsError() || got.Diagnostics()[0].Code != code {
			t.Errorf("%s(unknown, 0) = %v, want %s now", name, got, code)
		}
	}
}

// num is the number text spells.
func num(text string) tenon.Value { return tenon.NumberFromText(text) }

// between returns the unknown number, not null, within the bounds given;
// an empty text leaves that side unbounded.
func between(lo string, loIn bool, hi string, hiIn bool) tenon.Value {
	ns := []tenon.Narrowing{tenon.NotNull()}
	if lo != "" {
		ns = append(ns, tenon.NumberMin(num(lo), loIn))
	}
	if hi != "" {
		ns = append(ns, tenon.NumberMax(num(hi), hiIn))
	}
	return tenon.Narrow(tenon.Unknown(tenon.NumberType()), ns...)
}

// answers holds f to answering each argument as the table says.
func answers(t *testing.T, name string, f tenon.Function, table [][2]tenon.Value) {
	t.Helper()
	for _, tt := range table {
		if got := call(f, tt[0]); !got.Equal(tt[1]) {
			t.Errorf("%s(%v) = %v, want %v", name, tt[0], got, tt[1])
		}
	}
}

func TestConformance_LN030_Absolute(t *testing.T) {
	conformance.Covers(t, "LN-030")
	answers(t, "Absolute", stdlib.AbsoluteFunc, [][2]tenon.Value{
		{num("-2.5"), num("2.5")}, {num("3"), num("3")}, {num("0"), num("0")},
		{num("-1e999999"), num("1e999999")}, {num("-1e-999999"), num("1e-999999")},
		{between("1", true, "5", false), between("1", true, "5", false)},
		{between("-5", true, "-1", false), between("1", false, "5", true)},
		{between("-5", true, "3", true), between("0", true, "5", true)},
		{between("-3", false, "3", true), between("0", true, "3", true)},
		{between("-2", true, "", false), between("0", true, "", false)},
		{tenon.Unknown(tenon.NumberType()), between("0", true, "", false)},
	})
}

func TestConformance_LN031_Signum(t *testing.T) {
	conformance.Covers(t, "LN-031")
	answers(t, "Signum", stdlib.SignumFunc, [][2]tenon.Value{
		{num("-0.5"), num("-1")}, {num("0"), num("0")}, {num("1e-999999"), num("1")}, {num("9223372036854775808"), num("1")},
		{between("1", true, "5", true), num("1")},
		{between("0", false, "", false), num("1")},
		{between("", false, "-3", true), num("-1")},
		{between("0", true, "5", true), between("0", true, "1", true)},
		{between("-5", true, "0", true), between("-1", true, "0", true)},
		{tenon.Unknown(tenon.NumberType()), between("-1", true, "1", true)},
	})
}

func TestConformance_LN032_Rounding(t *testing.T) {
	conformance.Covers(t, "LN-032")
	answers(t, "Int", stdlib.IntFunc, [][2]tenon.Value{
		{num("1.5"), num("1")}, {num("-1.5"), num("-1")}, {num("1e999999"), num("1e999999")},
		{num("-1e-999999"), num("0")}, {num("123456789.987654321"), num("123456789")},
		{between("-2", false, "3", false), between("-1", true, "2", true)},
		{between("2", false, "", false), between("2", true, "", false)},
	})
	answers(t, "Ceil", stdlib.CeilFunc, [][2]tenon.Value{
		{num("-1.5"), num("-1")}, {num("-0.5"), num("0")}, {num("1e-999999"), num("1")}, {num("2"), num("2")},
		{between("2", false, "3", false), between("3", true, "3", true)},
		{between("1.5", true, "", false), between("2", true, "", false)},
	})
	answers(t, "Floor", stdlib.FloorFunc, [][2]tenon.Value{
		{num("-1.5"), num("-2")}, {num("-1e-999999"), num("-1")}, {num("2.9"), num("2")},
		{between("2", false, "3", false), between("2", true, "2", true)},
		{between("", false, "-1.5", true), between("", false, "-2", true)},
	})
	// A quotient of 96 digits is specified (NU-012), so what rounds it is
	// the same everywhere: one third times three is 96 nines.
	nines := tenon.Mul(tenon.Div(num("1"), num("3")), num("3"))
	if got := call(stdlib.CeilFunc, nines); !got.Equal(num("1")) {
		t.Errorf("Ceil(%v) = %v, want 1", nines, got)
	}
	if got := call(stdlib.FloorFunc, nines); !got.Equal(num("0")) {
		t.Errorf("Floor(%v) = %v, want 0", nines, got)
	}
}

func TestConformance_LN033_MinAndMax(t *testing.T) {
	conformance.Covers(t, "LN-033")
	for _, tt := range []struct {
		f         tenon.Function
		args      []tenon.Value
		want      tenon.Value
		name, why string
	}{
		{stdlib.MinFunc, []tenon.Value{num("3"), num("1.5"), num("2")}, num("1.5"), "Min", "the least"},
		{stdlib.MaxFunc, []tenon.Value{num("3"), num("1.5"), num("2")}, num("3"), "Max", "the greatest"},
		{stdlib.MinFunc, []tenon.Value{num("2"), between("5", true, "9", true)}, num("2"), "Min", "a known number below every other's range"},
		{stdlib.MaxFunc, []tenon.Value{between("1", true, "4", true), num("4")}, num("4"), "Max", "a tie at the end"},
		{stdlib.MinFunc, []tenon.Value{num("7"), between("1", true, "4", false)}, between("1", true, "4", false), "Min", "an unknown wholly below"},
		{stdlib.MinFunc, []tenon.Value{between("1", true, "6", true), between("3", false, "5", true)}, between("1", true, "5", true), "Min", "overlapping ranges"},
		{stdlib.MaxFunc, []tenon.Value{tenon.Unknown(tenon.NumberType()), num("5")}, between("5", true, "", false), "Max", "an unbounded unknown"},
	} {
		if got := call(tt.f, tt.args...); !got.Equal(tt.want) {
			t.Errorf("%s(%v), %s: %v, want %v", tt.name, tt.args, tt.why, got, tt.want)
		}
	}
	if got := call(stdlib.MinFunc); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionArity {
		t.Errorf("Min() = %v, want %s", got, tenon.CodeFunctionArity)
	}
}

func TestConformance_LN050_ParseInt(t *testing.T) {
	conformance.Covers(t, "LN-050")
	str := tenon.String
	for _, tt := range []struct {
		text string
		base int64
		want string
	}{
		{"ff", 16, "255"}, {"+ff", 16, "255"}, {"-0", 10, "0"}, {"012", 8, "10"},
		{"zz", 36, "1295"}, {"ZZ", 36, "1295"}, {"Zz", 62, "3817"}, {"aA", 62, "656"}, {"10", 62, "62"},
		{"123456789012345678901234567890", 10, "123456789012345678901234567890"},
	} {
		if got := call(stdlib.ParseIntFunc, str(tt.text), tenon.NumberFromInt(tt.base)); !got.Equal(num(tt.want)) {
			t.Errorf("ParseInt(%q, %d) = %v, want %s", tt.text, tt.base, got, tt.want)
		}
	}
	// Nothing else is read.
	for _, text := range []string{"0xff", "1_000", " 1", "1 ", "", "-", "--1", "1.5", "1e3", "\U00000663", "g"} {
		got := call(stdlib.ParseIntFunc, str(text), tenon.NumberFromInt(10))
		if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeNumberInvalidSyntax || !got.Diagnostics()[0].Path.Equal(at(0)) {
			t.Errorf("ParseInt(%q, 10) = %v, want %s at [0]", text, got, tenon.CodeNumberInvalidSyntax)
		}
	}
	got := call(stdlib.ParseIntFunc, str(strings.Repeat("1", 10001)), tenon.NumberFromInt(10))
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeNumberTooLong {
		t.Errorf("ParseInt of 10,001 digits = %v, want %s", got, tenon.CodeNumberTooLong)
	}
	// A number is no text to read; the refusal is the derivation's.
	got = call(stdlib.ParseIntFunc, tenon.NumberFromInt(10), tenon.NumberFromInt(16))
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeOperationWrongType || !got.Diagnostics()[0].Path.Equal(at(0)) {
		t.Errorf("ParseInt(10, 16) = %v, want %s at [0]", got, tenon.CodeOperationWrongType)
	}
	// A redacted text is named by its placeholder.
	got = call(stdlib.ParseIntFunc, tenon.WithMarks(str("hunter2"), secret{}), tenon.NumberFromInt(10))
	if !got.IsError() || strings.Contains(got.Diagnostics()[0].Message, "hunter2") || !tenon.HasMark(got, secret{}) {
		t.Errorf("ParseInt(a redacted text) = %v, want a failure withholding it", got)
	}
	// Under a base not known yet, text no base reads fails now, and other
	// text answers an unknown number.
	unknownBase := tenon.Unknown(tenon.NumberType())
	if got := call(stdlib.ParseIntFunc, str("!"), unknownBase); !got.IsError() {
		t.Errorf("ParseInt(\"!\", unknown) = %v, want a failure now", got)
	}
	if got := call(stdlib.ParseIntFunc, str("ff"), unknownBase); got.IsKnown() || got.IsError() || !notNull(got) {
		t.Errorf("ParseInt(\"ff\", unknown) = %v, want an unknown number, not null", got)
	}
}

func TestConformance_LB030_ArgumentDomains(t *testing.T) {
	conformance.Covers(t, "LB-030")
	// A base outside 2 to 62 is outside what ParseInt has a meaning for,
	// and the failure is located at it.
	for _, base := range []string{"1", "63", "1.5", "1e30", "-16"} {
		got := call(stdlib.ParseIntFunc, tenon.String("10"), num(base))
		if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionInvalidArgument || !got.Diagnostics()[0].Path.Equal(at(1)) {
			t.Errorf("ParseInt(\"10\", %s) = %v, want %s at [1]", base, got, tenon.CodeFunctionInvalidArgument)
		}
	}
	if got := call(stdlib.ParseIntFunc, tenon.String("10"), num("16.0")); !got.Equal(num("16")) {
		t.Errorf("ParseInt(\"10\", 16.0) = %v, want 16", got)
	}
}

func TestConformance_LN060_Log(t *testing.T) {
	conformance.Covers(t, "LN-060")
	log := func(x, b string) tenon.Value { return call(stdlib.LogFunc, num(x), num(b)) }
	for _, tt := range [][3]string{
		{"1000", "10", "3"}, {"8", "2", "3"}, {"9", "3", "2"}, {"1e-400", "10", "-400"}, {"1", "7", "0"},
		{"2", "10", "0.301029995663981195213738894724493026768189881462108541310427461127108189274424509486927252118186"},
	} {
		if got := log(tt[0], tt[1]); !got.Equal(num(tt[2])) {
			t.Errorf("Log(%s, %s) = %v, want %s", tt[0], tt[1], got, tt[2])
		}
	}
	// log(2, 8) is a third, which does not terminate: 96 threes, as
	// Div(1, 3) has it.
	if got, want := log("2", "8"), tenon.Div(num("1"), num("3")); !got.Equal(want) {
		t.Errorf("Log(2, 8) = %v, want %v", got, want)
	}
	// The domain, at the argument, whatever the other turns out to be.
	unknown := tenon.Unknown(tenon.NumberType())
	for _, tt := range []struct {
		x, b tenon.Value
		at   []int
	}{
		{num("0"), num("10"), []int{0}}, {num("-1"), num("10"), []int{0}},
		{num("10"), num("1"), []int{1}}, {num("10"), num("0"), []int{1}}, {num("10"), num("-10"), []int{1}},
		{num("-1"), num("1"), []int{0, 1}},
		{num("-1"), unknown, []int{0}}, {unknown, num("1"), []int{1}},
	} {
		got := call(stdlib.LogFunc, tt.x, tt.b)
		if !got.IsError() || len(got.Diagnostics()) != len(tt.at) {
			t.Errorf("Log(%v, %v) = %v, want %d failures", tt.x, tt.b, got, len(tt.at))
			continue
		}
		for i, d := range got.Diagnostics() {
			if d.Code != tenon.CodeNumberDomain || !d.Path.Equal(at(tt.at[i])) {
				t.Errorf("Log(%v, %v) failed with %+v, want %s at %v", tt.x, tt.b, d, tenon.CodeNumberDomain, at(tt.at[i]))
			}
		}
	}
	if got := call(stdlib.LogFunc, unknown, num("10")); got.IsKnown() || !notNull(got) {
		t.Errorf("Log(unknown, 10) = %v, want an unknown number, not null", got)
	}
}

func TestConformance_LN061_Pow(t *testing.T) {
	conformance.Covers(t, "LN-061")
	pow := func(x, y tenon.Value) tenon.Value { return call(stdlib.PowFunc, x, y) }
	for _, tt := range [][3]string{
		{"10", "23", "100000000000000000000000"}, {"3", "40", "12157665459056928801"}, {"0.1", "2", "0.01"},
		{"10", "-400", "1e-400"}, {"4", "0.5", "2"}, {"2.25", "0.5", "1.5"},
		{"2", "0.5", "1.41421356237309504880168872420969807856967187537694807317667973799073247846210703885038753432764"},
		{"0", "0", "1"}, {"-3", "0", "1"}, {"0", "5", "0"}, {"1", "1e400", "1"},
		{"-2", "3", "-8"}, {"-2", "2", "4"}, {"-2", "-1", "-0.5"},
	} {
		if got := pow(num(tt[0]), num(tt[1])); !got.Equal(num(tt[2])) {
			t.Errorf("Pow(%s, %s) = %v, want %s", tt[0], tt[1], got, tt[2])
		}
	}
	// 5^138 is a rounding midpoint, 97 digits ending in 5: half to even
	// keeps the 96th digit, a 2.
	five138 := num("2.86985925493722536125179818657774823686197645696310430055847584540629213734064251184463500976562e96")
	if got := pow(num("5"), num("138")); !got.Equal(five138) {
		t.Errorf("Pow(5, 138) = %v, want %v", got, five138)
	}
	// What fails, and how.
	for _, tt := range []struct {
		x, y string
		code tenon.Code
	}{
		{"0", "-1", tenon.CodeNumberDivideByZero}, {"-1", "0.5", tenon.CodeNumberDomain},
		{"-8", "0.3333333333", tenon.CodeNumberDomain}, {"2", "1e400", tenon.CodeNumberOutOfRange},
		{"10", "-1000000", tenon.CodeNumberOutOfRange},
	} {
		got := pow(num(tt.x), num(tt.y))
		if !got.IsError() || got.Diagnostics()[0].Code != tt.code {
			t.Errorf("Pow(%s, %s) = %v, want %s", tt.x, tt.y, got, tt.code)
		}
	}
	if got := pow(tenon.Unknown(tenon.NumberType()), num("0.5")); got.IsKnown() || !notNull(got) {
		t.Errorf("Pow(unknown, 0.5) = %v, want an unknown number, not null", got)
	}
}
