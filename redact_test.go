package tenon_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

func TestConformance_MK011_DiagnosticsWithholdRedactedContents(t *testing.T) {
	conformance.Covers(t, "MK-011")
	num, str := tenon.NumberType(), tenon.StringType()
	secret := stamp{id: "secret", redact: true}
	pii := stamp{id: "pii", policy: tenon.Isolate, redact: true}
	plain := stamp{id: "plain"}
	hunter, fortyTwo, fifty := tenon.String("hunter2"), tenon.NumberFromInt(42), tenon.NumberFromInt(50)
	// Crossed bounds leave a range that holds null null alone (UN-004), so
	// the ranges below exclude it to reach a contradiction.
	notNull := tenon.Narrow(tenon.Unknown(num), tenon.NotNull())

	// A division by zero over a secret numerator says nothing of the
	// numerator, and the error value carries the secret's mark.
	div := tenon.Div(tenon.WithMarks(fortyTwo, secret), tenon.NumberFromInt(0))
	if !div.IsError() || strings.Contains(div.String(), "42") || !tenon.HasMark(div, secret) {
		t.Errorf("dividing a secret by zero gave %v", div)
	}

	// Where a message would render a value carrying a redacting mark, it shows
	// a placeholder naming the mark: for the value, for a value within it, for
	// what its range says, and for a bound taken from it, whether the bound
	// is in the narrowing given, in the range already, or in a narrowing given
	// earlier in the same call. An Isolate mark redacts as a Propagate mark
	// does, because what is withheld is the value that carries it.
	secretBound := tenon.NumberMax(tenon.WithMarks(fortyTwo, secret), true)
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{
			"a value",
			tenon.Narrow(tenon.WithMarks(hunter, secret), tenon.StringPrefix("ab-")),
			`the value redacted("secret") does not satisfy prefix "ab-"`,
		},
		{
			"a value within a value",
			tenon.Narrow(tenon.ListVal(num, tenon.NumberFromInt(7), tenon.WithMarks(fortyTwo, pii)), tenon.LengthMax(1)),
			`the value list(number)[7, redacted("pii")] does not satisfy length <= 1`,
		},
		{
			"what a range says",
			tenon.Narrow(tenon.WithMarks(tenon.Narrow(notNull, tenon.NumberMax(fortyTwo, true)), secret),
				tenon.NumberMin(fifty, true)),
			`no value of type number satisfies both redacted("secret") and >= 50`,
		},
		{
			"a bound given",
			tenon.Narrow(fifty, secretBound),
			`the value 50 does not satisfy <= redacted("secret")`,
		},
		{
			"a bound in the range",
			tenon.Narrow(tenon.Narrow(notNull, secretBound), tenon.NumberMin(fifty, true)),
			`no value of type number satisfies both redacted("secret") and >= 50`,
		},
		{
			"a bound given earlier in the same call",
			tenon.Narrow(notNull, secretBound, tenon.NumberMin(fifty, true)),
			`no value of type number satisfies both redacted("secret") and >= 50`,
		},
		{
			// The bounds leave null alone, which NotNull then excludes; the
			// message names the bounds, as it does with NotNull given first.
			"bounds that leave only null, then NotNull",
			tenon.Narrow(tenon.Unknown(num), secretBound, tenon.NumberMin(fifty, true), tenon.NotNull()),
			`no value of type number satisfies both redacted("secret") and >= 50`,
		},
		{
			"a narrowing given earlier in the same call, which a set holding an unknown cannot meet beside this one",
			tenon.Narrow(tenon.WithMarks(tenon.SetVal(num, tenon.NumberFromInt(7), tenon.Unknown(num)), secret),
				tenon.LengthMin(2), tenon.LengthMax(1)),
			`the value redacted("secret") does not satisfy both redacted("secret") and length <= 1`,
		},
		{
			"a bound under an Isolate mark",
			tenon.Narrow(fifty, tenon.NumberMax(tenon.WithMarks(fortyTwo, pii), true)),
			`the value 50 does not satisfy <= redacted("pii")`,
		},
		{
			"a value under two redacting marks",
			tenon.Narrow(tenon.WithMarks(hunter, secret, pii), tenon.StringPrefix("ab-")),
			`the value redacted("pii", "secret") does not satisfy prefix "ab-"`,
		},
		// A mark that does not redact withholds nothing, and it does not show
		// either, since it cannot change a message (MK-005).
		{
			"a mark that does not redact",
			tenon.Narrow(tenon.WithMarks(hunter, plain), tenon.StringPrefix("ab-")),
			`the value "hunter2" does not satisfy prefix "ab-"`,
		},
		{
			"a mark that does not redact beside one that does",
			tenon.Narrow(tenon.WithMarks(hunter, secret, plain), tenon.StringPrefix("ab-")),
			`the value redacted("secret") does not satisfy prefix "ab-"`,
		},
		{
			"values within a value, one redacted and one not",
			tenon.Narrow(tenon.ListVal(num, tenon.WithMarks(tenon.NumberFromInt(7), plain), tenon.WithMarks(fortyTwo, pii)), tenon.LengthMax(1)),
			`the value list(number)[7, redacted("pii")] does not satisfy length <= 1`,
		},
		{
			"a string that is not a number",
			tenon.Convert(tenon.WithMarks(hunter, plain), tenon.Exactly(num), tenon.Unsafe),
			`"hunter2" is not a number`,
		},
		{
			"a string that is not a number, redacted",
			tenon.Convert(tenon.WithMarks(hunter, secret, plain), tenon.Exactly(num), tenon.Unsafe),
			`redacted("secret") does not convert to exactly(number)`,
		},
	} {
		if !tt.got.IsError() {
			t.Errorf("%s: %v is not an error value", tt.name, tt.got)
			continue
		}
		if ds := tt.got.Diagnostics(); len(ds) != 1 || ds[0].Message != tt.want {
			t.Errorf("%s: the diagnostics are %v, want the message %s", tt.name, ds, tt.want)
		}
	}

	// A value is described the same way outside a diagnostic, so a message a
	// caller builds from String withholds it too. An error value shows its
	// diagnostics, which withheld what they had to when they were made, and
	// unmarking a value is how a caller shows it on purpose.
	sealed := tenon.WithMarks(tenon.SetVal(str, hunter), stamp{id: "secret", redact: true, deep: true})
	for _, tt := range []struct {
		name, got, want string
	}{
		{"a known value", tenon.WithMarks(hunter, secret).String(), `redacted("secret")`},
		{"a null value", tenon.WithMarks(tenon.NullVal(str), secret).String(), `redacted("secret")`},
		{
			"an unknown value",
			tenon.WithMarks(tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(fortyTwo, true)), secret).String(),
			`redacted("secret")`,
		},
		{
			"the range of an unknown value",
			tenon.WithMarks(tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(fortyTwo, true)), secret).Range().String(),
			`redacted("secret")`,
		},
		{
			"a pending value",
			tenon.WithMarks(tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null()), secret).String(),
			`redacted("secret")`,
		},
		{
			"a value within a value",
			tenon.ListVal(str, tenon.String("shown"), tenon.WithMarks(hunter, secret)).String(),
			`list(string)["shown", redacted("secret")]`,
		},
		{
			"a value under two marks that share an identifier",
			tenon.WithMarks(hunter, secret, stamp{id: "secret", redact: true, deep: true}).String(),
			`redacted("secret")`,
		},
		{"a set under a deep redacting mark", sealed.String(), `redacted("secret")`},
		{"a member read out of that set", sealed.Elements()[0].String(), `redacted("secret")`},
		{"a narrowing with a secret bound", secretBound.String(), `<= redacted("secret")`},
		{
			"an error value",
			tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), secret).String(),
			`marked(error(app.failed: "it failed"), "secret")`,
		},
		{"a value unmarked on purpose", unmarkedDeep(sealed).String(), `set(string)["hunter2"]`},
	} {
		if tt.got != tt.want {
			t.Errorf("%s reads %s, want %s", tt.name, tt.got, tt.want)
		}
	}
}

// TestConformance_MK011_RedactionWithholdsStructure holds a redacting mark to
// withholding a value's structure as well as its contents: the keys of a map
// and the names of an object's attributes, which a diagnostic's path, a
// message naming the value's type, or a result whose type is built from them
// would otherwise show.
func TestConformance_MK011_RedactionWithholdsStructure(t *testing.T) {
	conformance.Covers(t, "MK-011", "MK-005", "CV-050", "CV-033", "SE-050")
	num, str := tenon.NumberType(), tenon.StringType()
	secret := stamp{id: "secret", redact: true}
	leaks := func(what string, v tenon.Value) {
		t.Helper()
		for _, d := range errorsOf(v) {
			if strings.Contains(d.Message, "hunter2") || strings.Contains(d.Path.String(), "hunter2") {
				t.Errorf("%s: %s at %s shows what the mark withholds", what, d.Message, d.Path)
			}
		}
		if strings.Contains(v.String(), "hunter2") {
			t.Errorf("%s: %v shows what the mark withholds", what, v)
		}
	}
	vault := tenon.WithMarks(tenon.MapVal(str, map[string]tenon.Value{"hunter2": tenon.String("x"), "other": tenon.String("y")}), secret)

	// A failure within a redacted map is located at the map, once for each
	// code, named by the placeholder and the constraint converted to, which
	// a field that admits one type gives as Exactly of it (CV-026); the code
	// is kept (MK-005).
	for _, tt := range []struct {
		name  string
		v     tenon.Value
		c     tenon.Constraint
		p     tenon.Policy
		code  tenon.Code
		path  string
		inner string
	}{
		{"the map itself", vault, tenon.MapOf(tenon.Exactly(num)), tenon.Unsafe, tenon.CodeNumberInvalidSyntax, ".", "map_of(exactly(number))"},
		{"a map within an object", tenon.ObjectVal(map[string]tenon.Value{"vault": vault}), tenon.ObjectWith(map[string]tenon.Field{"vault": tenon.Required(tenon.MapOf(tenon.Exactly(num)))}, true),
			tenon.Unsafe, tenon.CodeNumberInvalidSyntax, ".vault", "exactly(map(number))"},
		{"under the safe policy", vault, tenon.MapOf(tenon.Exactly(num)), tenon.Safe, tenon.CodeConvertUnsafe, ".", "map_of(exactly(number))"},
	} {
		got := tenon.Convert(tt.v, tt.c, tt.p)
		ds := errorsOf(got)
		want := `redacted("secret") does not convert to ` + tt.inner
		if len(ds) != 1 || ds[0].Code != tt.code || ds[0].Path.String() != tt.path || ds[0].Message != want {
			t.Errorf("%s: %v, want %s: %q at %s", tt.name, got, tt.code, want, tt.path)
		}
		leaks(tt.name, got)
	}

	// A redacted object that does not convert names no attribute.
	record := tenon.WithMarks(tenon.ObjectVal(map[string]tenon.Value{"hunter2": tenon.NumberFromInt(1)}), secret)
	leaks("a redacted object converted to a number", tenon.Convert(record, tenon.Exactly(num), tenon.Unsafe))

	// A list whose element type takes attribute names from a redacted map,
	// through the object it converts to, carries the mark, since its type and
	// the members given those attributes would show them. One whose element
	// type takes nothing from its redacted members is left as it is.
	mixed := tenon.ListVal(tenon.Map(num),
		tenon.WithMarks(tenon.MapVal(num, map[string]tenon.Value{"hunter2": tenon.NumberFromInt(1)}), secret),
		tenon.MapVal(num, map[string]tenon.Value{"b": tenon.NumberFromInt(2)}))
	objects := tenon.Convert(mixed, tenon.ListOf(tenon.ObjectWith(nil, false)), tenon.Unsafe)
	if objects.IsError() || !tenon.HasMark(objects, secret) {
		t.Errorf("a list taking attribute names from a redacted map converted to %v, not carrying its mark", objects)
	}
	leaks("a list taking attribute names from a redacted map", objects)
	numbers := tenon.Convert(tenon.ListVal(num, tenon.WithMarks(tenon.NumberFromInt(1), secret), tenon.NumberFromInt(2)), tenon.ListOf(tenon.Any()), tenon.Safe)
	if _, marks := tenon.Unmark(numbers); len(marks) != 0 || numbers.String() != `list(number)[redacted("secret"), 2]` {
		t.Errorf("a list of numbers, one redacted, converted to %v", numbers)
	}

	// An operand is named by the placeholder: whether it is null, and the
	// constraint a pending one will satisfy, are what the mark withholds.
	null := tenon.Add(tenon.WithMarks(tenon.NullVal(num), secret), tenon.NumberFromInt(1))
	if ds := errorsOf(null); len(ds) != 1 || ds[0].Code != tenon.CodeOperationNullOperand || strings.Contains(ds[0].Message, "null") {
		t.Errorf("adding to a redacted null gave %v", null)
	}
	pending := tenon.Add(tenon.WithMarks(tenon.Pending(tenon.Exactly(str)), secret), tenon.NumberFromInt(1))
	if ds := errorsOf(pending); len(ds) != 1 || ds[0].Code != tenon.CodeOperationWrongType || strings.Contains(ds[0].Message, "string") {
		t.Errorf("adding to a redacted pending string gave %v", pending)
	}

	// Serialize locates what fails within a redacted value at the value.
	holder := tenon.ObjectVal(map[string]tenon.Value{"vault": tenon.WithMarks(
		tenon.MapVal(num, map[string]tenon.Value{"hunter2": tenon.WithMarks(tenon.NumberFromInt(1), stamp{id: "plain"})}), secret)})
	_, failure, ok := tenon.Serialize(holder)
	if ds := errorsOf(failure); ok || len(ds) != 1 || ds[0].Code != tenon.CodeSerializeUnencodableMark || ds[0].Path.String() != ".vault" {
		t.Errorf("serializing a redacted map holding an unencodable mark gave %v", failure)
	}
	leaks("serializing a redacted map", failure)
}

// errorsOf returns the diagnostics of v, or none where v is no error value.
func errorsOf(v tenon.Value) []tenon.Diagnostic {
	if v == (tenon.Value{}) || !v.IsError() {
		return nil
	}
	return v.Diagnostics()
}

// TestConformance_MK002_RedactingMarksAlwaysPropagate holds a redacting mark
// whose policy is Isolate to propagating as any redacting mark does, so that
// nothing derived from the value it withholds shows it: a narrowing taken
// from it, an operation over it, and a conversion of a set it is deep on. An
// Isolate mark that does not redact still stays where it was put.
func TestConformance_MK002_RedactingMarksAlwaysPropagate(t *testing.T) {
	conformance.Covers(t, "MK-002", "MK-003", "MK-011")
	num, str := tenon.NumberType(), tenon.StringType()
	pii := stamp{id: "pii", policy: tenon.Isolate, redact: true}
	deepPii := stamp{id: "pii", policy: tenon.Isolate, redact: true, deep: true}
	fortyTwo := tenon.WithMarks(tenon.NumberFromInt(42), pii)
	for _, tt := range []struct {
		name string
		got  tenon.Value
		mark tenon.Mark
	}{
		{"a narrowing taken from it", tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(fortyTwo, true)), pii},
		{"an operation over it", tenon.Add(fortyTwo, tenon.NumberFromInt(0)), pii},
		{"a set it is deep on, converted to a list", tenon.Convert(
			tenon.WithMarks(tenon.SetVal(str, tenon.String("hunter2")), deepPii), tenon.ListOf(tenon.Exactly(str)), tenon.Safe), deepPii},
	} {
		if !tenon.HasMark(tt.got, tt.mark) {
			t.Errorf("%s gave %v, not carrying the redacting mark", tt.name, tt.got)
		}
		if text := tt.got.String(); strings.Contains(text, "42") || strings.Contains(text, "hunter2") {
			t.Errorf("%s displays as %s, showing what the mark withholds", tt.name, text)
		}
	}
	quiet := stamp{id: "quiet", policy: tenon.Isolate}
	if sum := tenon.Add(tenon.WithMarks(tenon.NumberFromInt(42), quiet), tenon.NumberFromInt(0)); tenon.HasMark(sum, quiet) || sum.String() != "42" {
		t.Errorf("an Isolate mark that does not redact reached the sum: %v", sum)
	}
}
