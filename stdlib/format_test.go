package stdlib_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// format calls Format with the format string and the arguments.
func format(f string, args ...tenon.Value) tenon.Value {
	return call(stdlib.FormatFunc, append([]tenon.Value{tenon.String(f)}, args...)...)
}

// formats checks that Format of f and args answers want.
func formats(t *testing.T, f string, args []tenon.Value, want string) {
	t.Helper()
	if got := format(f, args...); got.IsError() || got.AsString() != tenon.String(want).AsString() {
		t.Errorf("Format(%+q, %v) = %v, want %+q", f, args, got, want)
	}
}

// escaped returns s with each @ a reverse solidus, so that a test can write
// JSON's escapes.
func escaped(s string) string { return strings.ReplaceAll(s, "@", `\`) }

// vals returns its arguments, for formats.
func vals(vs ...tenon.Value) []tenon.Value { return vs }

func TestConformance_LF001_Format(t *testing.T) {
	conformance.Covers(t, "LF-001")
	formats(t, "Hello, %s!", vals(tenon.String("world")), "Hello, world!")
	formats(t, "no verbs", nil, "no verbs")
	if !stdlib.FormatFunc.NotNull() {
		t.Error("Format does not declare its result never null")
	}
	// The boundary carries an argument's marks to the answer.
	if got := format("%s", tenon.WithMarks(tenon.String("x"), bare("arg"))); !tenon.HasMark(got, bare("arg")) {
		t.Errorf("Format of a marked argument = %v, want its mark", got)
	}
}

func TestConformance_LF002_Grammar(t *testing.T) {
	conformance.Covers(t, "LF-002")
	formats(t, "100%%", nil, "100%")
	formats(t, "%%s", nil, "%s")
	formats(t, "[%-+ #05.3[1]s]", vals(tenon.String("abcdef")), "[abc  ]")
	for _, f := range []string{"%", "abc%", "%z", "%c", "%U", "%[2]5d", "%[0]d", "%*d", "%[1d", "%5", "%\U000000E9"} {
		failsWith(t, "Format("+f+")", format(f, num("1"), num("2")), tenon.CodeFormatInvalidSyntax, at(0))
	}
	// The index comes just before the letter.
	formats(t, "%5[2]s", vals(tenon.String("a"), tenon.String("b")), "    b")
}

func TestConformance_LF003_Arguments(t *testing.T) {
	conformance.Covers(t, "LF-003")
	a, b, c := tenon.String("a"), tenon.String("b"), tenon.String("c")
	formats(t, "%s %s %s", vals(a, b, c), "a b c")
	formats(t, "%[2]s %s %[1]s %s", vals(a, b, c), "b c a b")
	// An argument below the highest read may go unread.
	formats(t, "%[2]s", vals(a, b), "b")
	failsWith(t, "Format(%s %s, a)", format("%s %s", a), tenon.CodeFunctionInvalidArgument, at(0))
	failsWith(t, "Format(%[2]s %s, a, b)", format("%[2]s %s", a, b), tenon.CodeFunctionInvalidArgument, at(0))
	failsWith(t, "Format(%s, a, b)", format("%s", a, b), tenon.CodeFunctionInvalidArgument, at(2))
	failsWith(t, "Format(x, a)", format("x", a), tenon.CodeFunctionInvalidArgument, at(1))
}

func TestConformance_LF004_Limits(t *testing.T) {
	conformance.Covers(t, "LF-004")
	formats(t, "%10000s", vals(tenon.String("")), strings.Repeat(" ", 10000))
	for _, f := range []string{"%10001s", "%.10001s", "%18446744073709551621s", "%99999999999999999999.1s"} {
		failsWith(t, "Format("+f+")", format(f, tenon.String("x")), tenon.CodeFunctionTooLarge, at(0))
	}
}

func TestConformance_LF005_Padding(t *testing.T) {
	conformance.Covers(t, "LF-005")
	ab := tenon.String("ab")
	formats(t, "[%5s]", vals(ab), "[   ab]")
	formats(t, "[%-5s]", vals(ab), "[ab   ]")
	formats(t, "[%05s]", vals(ab), "[000ab]")
	formats(t, "[%-05s]", vals(ab), "[ab   ]")
	formats(t, "[%1s]", vals(ab), "[ab]")
	// Widths count grapheme clusters: an e with an acute is one, and so is
	// x with two thousand combining marks.
	formats(t, "[%3s]", vals(tenon.String("e\U00000301")), "[  e\U00000301]")
	long := "x" + strings.Repeat("\U00000301", 2000)
	formats(t, "[%3s]", vals(tenon.String(long)), "[  "+long+"]")
	formats(t, "[%5s]", vals(tenon.String("\U00000915\U0000094D\U00000937")), "[   \U00000915\U0000094D\U00000937]")
}

func TestConformance_LF006_Null(t *testing.T) {
	conformance.Covers(t, "LF-006")
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	for _, n := range []tenon.Value{tenon.Null(tenon.StringType()), tenon.Null(tenon.NumberType()), untyped} {
		formats(t, "%v", vals(n), "null")
		formats(t, "%#v", vals(n), "null")
		formats(t, "[%6v]", vals(n), "[  null]")
		failsWith(t, "Format(%s, null)", format("%s", n), tenon.CodeOperationNullOperand, at(1))
		failsWith(t, "Format(%t, null)", format("%t", n), tenon.CodeOperationNullOperand, at(1))
	}
}

func TestConformance_LF007_Value(t *testing.T) {
	conformance.Covers(t, "LF-007")
	formats(t, "%v", vals(tenon.String("hello")), "hello")
	formats(t, "%.2v", vals(tenon.String("hello")), "he")
	formats(t, "%v %v", vals(tenon.Bool(true), tenon.Bool(false)), "true false")
	// A number is its canonical text, as a string converts to.
	formats(t, "%v", vals(num("1000000")), "1000000")
	formats(t, "%v", vals(num("1073741824")), "1073741824")
	formats(t, "%v", vals(num("1e30")), "1e30")
	formats(t, "%v", vals(num("0.00001")), "0.00001")
	formats(t, "%v", vals(num("-1.5")), "-1.5")
	formats(t, "[%05v] [%-5v] [%+v] [% v]", vals(num("-1"), num("1"), num("1"), num("1")), "[-0001] [1    ] [+1] [ 1]")
	// Anything else is its JSON text.
	formats(t, "%v", vals(tenon.List(tenon.StringType(), tenon.String("a"), tenon.String("b"))), `["a","b"]`)
	formats(t, "%v", vals(tenon.Object(map[string]tenon.Value{"b": num("1"), "a": tenon.Bool(true)})), `{"a":true,"b":1}`)
}

func TestConformance_LF008_JSON(t *testing.T) {
	conformance.Covers(t, "LF-008")
	formats(t, "%#v", vals(tenon.String("hi")), `"hi"`)
	formats(t, "%#v", vals(tenon.String("<a&b>\U00002028\n\"\\\t\x01")), escaped(`"@u003ca@u0026b@u003e@u2028@n@"@@@t@u0001"`))
	// Numbers are written positionally, whatever their exponent.
	formats(t, "%#v", vals(num("1e21")), "1000000000000000000000")
	formats(t, "%#v", vals(num("1.5e-5")), "0.000015")
	formats(t, "%#v", vals(num("-12.5")), "-12.5")
	formats(t, "%#v", vals(tenon.Tuple(num("1"), tenon.String("x"), tenon.Null(tenon.BoolType()))), `[1,"x",null]`)
	m := tenon.Map(tenon.NumberType(), map[string]tenon.Value{"z": num("1"), "a": num("2")})
	formats(t, "%#v", vals(m), `{"a":2,"z":1}`)
	formats(t, "%#v", vals(tenon.Set(tenon.NumberType(), num("3"), num("1"))), "[1,3]")
}

func TestConformance_LF009_Strings(t *testing.T) {
	conformance.Covers(t, "LF-009")
	formats(t, "%s", vals(tenon.String("x")), "x")
	formats(t, "%.3s", vals(tenon.String("abcdef")), "abc")
	formats(t, "[%.0s] [%.s]", vals(tenon.String("abc"), tenon.String("abc")), "[] []")
	formats(t, "%.1s", vals(tenon.String("\U00000915\U0000094D\U00000937")), "\U00000915\U0000094D")
	formats(t, "%q", vals(tenon.String("<a&b>")), escaped(`"@u003ca@u0026b@u003e"`))
	formats(t, "[%8.2q]", vals(tenon.String("abc")), `[    "ab"]`)
	// Converted to a string under the call's policy: a number under
	// Unsafe, refused under Safe, located at the argument.
	if got := tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%s"), num("1e30")}, tenon.Unsafe); got.AsString() != "1e30" {
		t.Errorf("Format(%%s, 1e30) under Unsafe = %v, want 1e30", got)
	}
	failsWith(t, "Format(%s, 1) under Safe", format("%s", num("1")), tenon.CodeConvertUnsafe, at(1))
	failsWith(t, "Format(%s, [a])", tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%s"), strs("a")}, tenon.Unsafe), tenon.CodeConvertNoConversion, at(1))
}

func TestConformance_LF010_Bool(t *testing.T) {
	conformance.Covers(t, "LF-010")
	formats(t, "%t", vals(tenon.Bool(true)), "true")
	formats(t, "[%7t] [%-7t]", vals(tenon.Bool(false), tenon.Bool(true)), "[  false] [true   ]")
	if got := tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%t"), tenon.String("true")}, tenon.Unsafe); got.AsString() != "true" {
		t.Errorf("Format(%%t, \"true\") under Unsafe = %v, want true", got)
	}
	failsWith(t, "Format(%t, 1)", tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%t"), num("1")}, tenon.Unsafe), tenon.CodeConvertNoConversion, at(1))
}

func TestConformance_LF011_FormatNotKnown(t *testing.T) {
	conformance.Covers(t, "LF-011")
	str := tenon.Unknown(tenon.StringType())
	// What is known fails now: the grammar, the arguments, a null, a
	// conversion no value could make.
	failsWith(t, "Format(%z, unknown)", format("%z", str), tenon.CodeFormatInvalidSyntax, at(0))
	failsWith(t, "Format(x%s, a, unknown)", format("x%s", tenon.String("a"), str), tenon.CodeFunctionInvalidArgument, at(2))
	failsWith(t, "Format(%s %t, unknown, null)", format("%s %t", str, tenon.Null(tenon.BoolType())), tenon.CodeOperationNullOperand, at(2))
	failsWith(t, "Format(%s %s, unknown, [a]) under Unsafe", tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%s %s"), str, strs("a")}, tenon.Unsafe), tenon.CodeConvertNoConversion, at(2))
	// Otherwise the answer begins with the text up to the first verb whose
	// argument is not known.
	if got := format("id-%s-%s", tenon.String("web"), str); !promises(got, "id-web-") {
		t.Errorf("Format(id-%%s-%%s, web, unknown) = %v, want an unknown beginning id-web-", got)
	}
	if got := call(stdlib.FormatFunc, prefixed("abc-%s"), tenon.String("x")); !promises(got, "abc-") {
		t.Errorf("Format(unknown beginning abc-%%s, x) = %v, want an unknown beginning abc-", got)
	}
	// For random formats and arguments, the answer for the arguments known
	// begins with the promise made where the last was not.
	rng := rand.New(rand.NewPCG(20261005, 6))
	words := []string{"a", "%s", "%5s", "%-3v", "%%", "\U00000301", " ", "%q", "%.1s"}
	for range 3000 {
		var f strings.Builder
		for range 1 + rng.IntN(5) {
			f.WriteString(words[rng.IntN(len(words))])
		}
		verbs := strings.Count(strings.ReplaceAll(f.String(), "%%", ""), "%")
		args := make([]tenon.Value, verbs)
		for i := range args {
			args[i] = tenon.String(randomText(rng, 2))
		}
		if verbs == 0 {
			continue
		}
		whole := format(f.String(), args...)
		args[verbs-1] = tenon.Unknown(tenon.StringType())
		promised := format(f.String(), args...)
		if whole.IsError() || promised.IsError() {
			continue
		}
		if !strings.HasPrefix(whole.AsString(), promised.Range().StringPrefix()) {
			t.Fatalf("Format(%+q) = %+q, not beginning with %+q, promised with its last argument not known", f.String(), whole.AsString(), promised.Range().StringPrefix())
		}
	}
}

func TestConformance_LF012_Integers(t *testing.T) {
	conformance.Covers(t, "LF-012")
	for _, tt := range []struct {
		f, n, want string
	}{
		{"%d", "42", "42"}, {"%d", "-42", "-42"}, {"%d", "0", "0"},
		{"%b", "5", "101"}, {"%o", "8", "10"}, {"%x", "255", "ff"}, {"%X", "255", "FF"},
		{"%x", "-255", "-ff"},
		// Of any magnitude, exactly.
		{"%x", "1e30", "c9f2c9cd04674edea40000000"},
		{"%d", "123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
			"123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890"},
		{"%d", "2.5e3", "2500"},
	} {
		formats(t, tt.f, vals(num(tt.n)), tt.want)
	}
	// Converted to a number under the call's policy.
	if got := tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%d"), tenon.String("12")}, tenon.Unsafe); got.AsString() != "12" {
		t.Errorf("Format(%%d, \"12\") under Unsafe = %v, want 12", got)
	}
	failsWith(t, "Format(%d, \"12\") under Safe", format("%d", tenon.String("12")), tenon.CodeConvertUnsafe, at(1))
	failsWith(t, "Format(%d, 1.5)", format("%d", num("1.5")), tenon.CodeFunctionInvalidArgument, at(1))
	failsWith(t, "Format(%x, true)", tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%x"), tenon.Bool(true)}, tenon.Unsafe), tenon.CodeConvertNoConversion, at(1))
}

func TestConformance_LF013_IntegerFlags(t *testing.T) {
	conformance.Covers(t, "LF-013")
	for _, tt := range []struct {
		f, n, want string
	}{
		{"%+d", "5", "+5"}, {"% d", "5", " 5"}, {"%+d", "-5", "-5"},
		{"%#x", "255", "0xff"}, {"%#X", "255", "0XFF"}, {"%#b", "5", "0b101"}, {"%#o", "8", "010"},
		// The octal prefix only where the digits do not begin with 0.
		{"%#o", "0", "0"}, {"%#.3o", "8", "010"},
		{"%.3d", "5", "005"}, {"%.0d", "0", ""}, {"%.0d", "7", "7"},
		{"[%5d]", "-42", "[  -42]"}, {"[%-5d]", "42", "[42   ]"}, {"[%05d]", "-42", "[-0042]"},
		{"[%#08x]", "255", "[0x0000ff]"},
		// A precision drops the 0 flag; - overrides it.
		{"[%08.3d]", "5", "[     005]"}, {"[%-05d]", "5", "[5    ]"},
	} {
		formats(t, tt.f, vals(num(tt.n)), tt.want)
	}
}

func TestConformance_LF014_Exponent(t *testing.T) {
	conformance.Covers(t, "LF-014")
	for _, tt := range []struct{ f, n, want string }{
		{"%e", "1234.5678", "1.234568e+03"}, {"%E", "1234.5678", "1.234568E+03"},
		{"%.2e", "9.995", "1.00e+01"}, {"%.0e", "15", "2e+01"}, {"%.0e", "25", "2e+01"},
		{"%e", "0", "0.000000e+00"}, {"%e", "1e-400", "1.000000e-400"},
		{"%.3e", "-0.0001234", "-1.234e-04"}, {"%e", "1e100", "1.000000e+100"},
	} {
		formats(t, tt.f, vals(num(tt.n)), tt.want)
	}
}

func TestConformance_LF015_Fixed(t *testing.T) {
	conformance.Covers(t, "LF-015")
	for _, tt := range []struct{ f, n, want string }{
		{"%f", "3.14159265", "3.141593"}, {"%.2f", "1", "1.00"}, {"%.0f", "1234.5", "1234"},
		{"%f", "0", "0.000000"}, {"%.3f", "1e-10", "0.000"}, {"%f", "1e21", "1000000000000000000000.000000"},
		{"%.1f", "-12.34", "-12.3"},
		{"%.170f", "0.1", "0.1" + strings.Repeat("0", 169)},
	} {
		formats(t, tt.f, vals(num(tt.n)), tt.want)
	}
}

func TestConformance_LF016_General(t *testing.T) {
	conformance.Covers(t, "LF-016")
	for _, tt := range []struct{ f, n, want string }{
		{"%g", "100000", "100000"}, {"%g", "1000000", "1e+06"}, {"%G", "1000000", "1E+06"},
		{"%g", "0.0001", "0.0001"}, {"%g", "0.00001", "1e-05"},
		{"%g", "1234.5678", "1234.5678"}, {"%.3g", "1234.5678", "1.23e+03"},
		{"%.3g", "1", "1"}, {"%.10g", "123", "123"}, {"%.0g", "1.5", "2"},
		{"%g", "0", "0"}, {"%g", "-2.5", "-2.5"},
		// Every significant digit of the exact number, where no precision is
		// given.
		{"%g", "0.333333333333333333333333333333", "0.333333333333333333333333333333"},
	} {
		formats(t, tt.f, vals(num(tt.n)), tt.want)
	}
}

func TestConformance_LF017_Rounding(t *testing.T) {
	conformance.Covers(t, "LF-017")
	// Ties round half to even on the exact value, where binary floats
	// round by the error of their representation.
	for _, tt := range []struct{ f, n, want string }{
		{"%.2f", "2.675", "2.68"}, {"%.2f", "1.015", "1.02"}, {"%.1f", "0.35", "0.4"},
		{"%.1f", "0.25", "0.2"}, {"%.0f", "0.5", "0"}, {"%.0f", "1.5", "2"}, {"%.0f", "2.5", "2"},
		{"%.2e", "1.125", "1.12e+00"}, {"%.3g", "2.675", "2.68"},
		// The sign is the value's before rounding.
		{"%.2f", "-0.0001", "-0.00"},
	} {
		formats(t, tt.f, vals(num(tt.n)), tt.want)
	}
	// The flags, as for every number.
	formats(t, "[%+.1f] [% .1f] [%08.2f] [%-8.2f] [%+e]", vals(num("1.25"), num("1.25"), num("-3.14159"), num("3.14159"), num("5")),
		"[+1.2] [ 1.2] [-0003.14] [3.14    ] [+5.000000e+00]")
}

// formatList calls FormatList with the format string and the arguments.
func formatList(f string, args ...tenon.Value) tenon.Value {
	return call(stdlib.FormatListFunc, append([]tenon.Value{tenon.String(f)}, args...)...)
}

func TestConformance_LF018_FormatList(t *testing.T) {
	conformance.Covers(t, "LF-018")
	ab := strs("a", "b")
	for _, tt := range []struct {
		f    string
		args []tenon.Value
		want tenon.Value
	}{
		{"%s-%s", vals(ab, tenon.String("x")), strs("a-x", "b-x")},
		{"%s=%s", vals(ab, tenon.Tuple(tenon.String("1"), tenon.String("2"))), strs("a=1", "b=2")},
		{"%v", vals(tenon.Set(tenon.NumberType(), num("3"), num("1"))), strs("1", "3")},
		{"hello", nil, strs("hello")},
		{"%s", vals(tenon.String("x")), strs("x")},
		{"%s", vals(strs()), strs()},
		// A null list is repeated, not iterated.
		{"%v", vals(tenon.Null(tenon.ListType(tenon.StringType()))), strs("null")},
	} {
		if got := formatList(tt.f, tt.args...); !got.Equal(tt.want) {
			t.Errorf("FormatList(%+q, %v) = %v, want %v", tt.f, tt.args, got, tt.want)
		}
	}
	failsWith(t, "FormatList with lengths 2 and 1", formatList("%s%s", ab, strs("x")), tenon.CodeFunctionInvalidArgument, at(2))
	// The format and its arguments are checked however many times it is
	// written.
	failsWith(t, "FormatList(%z, [])", formatList("%z", strs()), tenon.CodeFormatInvalidSyntax, at(0))
	failsWith(t, "FormatList(%s, [], x, y)", formatList("%s", strs(), tenon.String("x"), tenon.String("y")), tenon.CodeFunctionInvalidArgument, at(2))
	if !stdlib.FormatListFunc.NotNull() {
		t.Error("FormatList does not declare its result never null")
	}
}

func TestConformance_LF019_FormatListFailures(t *testing.T) {
	conformance.Covers(t, "LF-019")
	withNull := tenon.List(tenon.StringType(), tenon.String("a"), tenon.Null(tenon.StringType()))
	failsWith(t, "FormatList(%s, [a, null])", formatList("%s", withNull), tenon.CodeOperationNullOperand, at(1).Index(tenon.NumberFromInt(1)))
	nums := tenon.List(tenon.NumberType(), num("1"), num("1.5"))
	failsWith(t, "FormatList(%s %d, x, [1, 1.5])", formatList("%s %d", tenon.String("x"), nums), tenon.CodeFunctionInvalidArgument, at(2).Index(tenon.NumberFromInt(1)))
}

func TestConformance_LF020_FormatListNotKnown(t *testing.T) {
	conformance.Covers(t, "LF-020")
	unknownList := tenon.Unknown(tenon.ListType(tenon.StringType()))
	// An unknown list beside a known one of two: two, or the call fails.
	got := call(stdlib.FormatListFunc, tenon.String("%s%s"), strs("a", "b"), unknownList)
	if hi, ok := got.Range().LengthMax(); got.IsKnown() || got.Range().LengthMin() != 2 || !ok || hi != 2 || !notNull(got) {
		t.Errorf("FormatList(%%s%%s, [a, b], unknown) = %v, want an unknown list of 2", got)
	}
	// A known failure fails now: lengths an unknown list's range rules out.
	short := tenon.Narrow(unknownList, tenon.LengthMax(1))
	failsWith(t, "FormatList with [a, b] and an unknown of at most 1", formatList("%s%s", strs("a", "b"), short), tenon.CodeFunctionInvalidArgument, at(2))
	// An element whose argument is not known yet is not known, the others
	// are.
	partly := tenon.List(tenon.StringType(), tenon.String("a"), tenon.Unknown(tenon.StringType()))
	got = formatList("x-%s", partly)
	if !got.HasMembers() || got.Len() != 2 || !got.Index(0).Equal(tenon.String("x-a")) || !promises(got.Index(1), "x-") {
		t.Errorf("FormatList(x-%%s, [a, unknown]) = %v, want [x-a, unknown beginning x-]", got)
	}
	// An unknown tuple's length is its type's.
	tuple := tenon.Unknown(tenon.TupleType(tenon.StringType(), tenon.StringType(), tenon.StringType()))
	if got := formatList("%s", tuple); !got.HasMembers() || got.Len() != 3 {
		t.Errorf("FormatList(%%s, unknown tuple of 3) = %v, want a list of 3", got)
	}
}

func TestConformance_LF021_FormatListBound(t *testing.T) {
	conformance.Covers(t, "LF-021")
	many := make([]string, 100)
	for i := range many {
		many[i] = "x"
	}
	failsWith(t, "FormatList(%10000s, a hundred x)", formatList("%10000s", strs(many...)), tenon.CodeFunctionTooLarge, at(0))
	if got := formatList("%100s", strs(many...)); got.IsError() || got.Len() != 100 {
		t.Errorf("FormatList(%%100s, a hundred x) = %v, want a hundred strings", got)
	}
	// A number's size is its canonical text: two of 1e60000 are 21 bytes
	// of argument, though each element is 60,001 digits, within Format's
	// own bound; 120,002 bytes pass 64 times 23 and 65,536.
	n := tenon.NumberFromText("1e60000")
	if got := format("%d", n); got.IsError() || len(got.AsString()) != 60001 {
		t.Errorf("Format(%%d, 1e60000) = %v, want 60,001 digits", got)
	}
	failsWith(t, "FormatList(%d, [1e60000, 1e60000])", formatList("%d", tenon.List(tenon.NumberType(), n, n)), tenon.CodeFunctionTooLarge, at(0))
}

func TestConformance_LF022_FormatBound(t *testing.T) {
	conformance.Covers(t, "LF-022")
	big, small := tenon.NumberFromText("1e999999"), tenon.NumberFromText("-1e-999999")
	// A number written out to more digits than 64 times what the arguments
	// say, and 64 KiB, fails, before its digits are made.
	for _, tt := range []struct {
		f string
		n tenon.Value
	}{
		{"%d", big}, {"%x", big}, {"%X", big}, {"%o", big}, {"%b", big},
		{"%f", big}, {"%.10000f", big}, {"%#v", big}, {"%#v", small},
		{"%v", tenon.List(tenon.NumberType(), big)},
		{"%b", tenon.NumberFromText("1e20000")},
	} {
		failsWith(t, "Format("+tt.f+", "+tt.n.String()+")", format(tt.f, tt.n), tenon.CodeFunctionTooLarge, at(0))
	}
	// Written in its canonical text, or to a precision, it fits.
	for _, f := range []string{"%v", "%e", "%g", "%.3e", "%s"} {
		if got := tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String(f), big}, tenon.Unsafe); got.IsError() {
			t.Errorf("Format(%s, 1e999999) = %v", f, got)
		}
	}
	// At the bound: 1e70000 is 70,001 digits, past 64 times the 10 bytes of
	// "%d" and "1e+70000", and 65,536; 1e60000's 60,001 are not.
	if got := format("%d", tenon.NumberFromText("1e60000")); got.IsError() {
		t.Errorf("Format(%%d, 1e60000) = %v", got)
	}
	failsWith(t, "Format(%d, 1e70000)", format("%d", tenon.NumberFromText("1e70000")), tenon.CodeFunctionTooLarge, at(0))
	// A string read as a number is its bytes, and is written as its number.
	failsWith(t, "Format(%d, \"1e999999\")", tenon.Call(stdlib.FormatFunc, []tenon.Value{tenon.String("%d"), tenon.String("1e999999")}, tenon.Unsafe), tenon.CodeFunctionTooLarge, at(0))
	// What the known arguments write passes the bound before an argument
	// not known yet: it fails now.
	failsWith(t, "Format(%d%s, 1e999999, unknown)", format("%d%s", big, tenon.Unknown(tenon.StringType())), tenon.CodeFunctionTooLarge, at(0))
	// The widest verb fits: 10,000 spaces.
	formats(t, "%10000d", vals(tenon.NumberFromInt(1)), strings.Repeat(" ", 9999)+"1")
}
