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
			`redacted("secret") is not a number`,
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
