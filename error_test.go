package tenon_test

import (
	"testing"

	"tenon"
	"tenon/conformance"
)

func TestConformance_ER002_DataErrorsAreValues(t *testing.T) {
	conformance.Covers(t, "ER-002")
	num := tenon.NumberType()
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	// Every way that data can be wrong produces an error value. None of these
	// panics: a panic would fail this test rather than return.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		code tenon.Code
	}{
		{"text that is not well-formed UTF-8", tenon.String("\xff"), tenon.CodeStringInvalidUTF8},
		{"text that is not a number", tenon.NumberFromText("1,000"), tenon.CodeNumberInvalidSyntax},
		{"a number out of range", tenon.NumberFromText("1e1000000"), tenon.CodeNumberOutOfRange},
		{
			"a map key that is not well-formed UTF-8",
			tenon.MapVal(num, map[string]tenon.Value{"a\xff": one}),
			tenon.CodeStringInvalidUTF8,
		},
		{
			"map keys that are the same key",
			tenon.MapVal(num, map[string]tenon.Value{"caf\u00e9": one, "cafe\u0301": two}),
			tenon.CodeMapDuplicateKey,
		},
	} {
		if !tt.v.IsError() {
			t.Errorf("%s gave %v; want an error value", tt.name, tt.v)
			continue
		}
		if d := tt.v.Diagnostics(); len(d) != 1 || d[0].Code != tt.code {
			t.Errorf("%s gave diagnostics %v; want one with code %s", tt.name, d, tt.code)
		}
	}
}

func TestConformance_ER003_ErrorValuesCarryDiagnostics(t *testing.T) {
	conformance.Covers(t, "ER-003")
	path := tenon.Path{}.Attribute("servers").Index(tenon.NumberFromInt(0))
	diags := []tenon.Diagnostic{
		{Code: tenon.CodeStringInvalidUTF8, Message: "the text is not well-formed UTF-8", Path: path},
		{Code: "app.unknown_setting", Message: "there is no such setting"},
	}
	v := tenon.ErrorVal(diags...)

	got := v.Diagnostics()
	if len(got) != 2 || got[0].Code != diags[0].Code || got[0].Message != diags[0].Message ||
		!got[0].Path.Equal(path) || got[1].Path.Len() != 0 {
		t.Errorf("Diagnostics() = %v; want the diagnostics it was built with", got)
	}

	// The diagnostics are the caller's own, going in and coming out.
	diags[0].Message, got[1].Message = "changed", "changed"
	if again := v.Diagnostics(); again[0].Message == "changed" || again[1].Message == "changed" {
		t.Errorf("changing a diagnostic changed the error value: %v", again)
	}

	// Each diagnostic has a code and a message, and an error value has at
	// least one diagnostic.
	mustPanicUsage(t, "needs at least one diagnostic", func() { tenon.ErrorVal() })
	mustPanicUsage(t, "needs a message", func() {
		tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeStringInvalidUTF8})
	})

	// The display of an error value shows the code, message and path of each
	// diagnostic.
	want := `error(string.invalid_utf8: "the text is not well-formed UTF-8" at .servers[0]; ` +
		`app.unknown_setting: "there is no such setting")`
	if got := v.String(); got != want {
		t.Errorf("String() = %s, want %s", got, want)
	}
}

func TestConformance_ER004_ErrorValuesHaveNoType(t *testing.T) {
	conformance.Covers(t, "ER-004")
	for _, v := range []tenon.Value{
		tenon.String("\xff"),
		tenon.NumberFromText("x"),
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}),
	} {
		if !v.IsError() || v.IsResolved() || v.IsPending() {
			t.Errorf("%v is not in the error state", v)
		}
		mustPanicUsage(t, "which has no type", func() { v.Type() })
		if len(v.Diagnostics()) == 0 {
			t.Errorf("%v carries no diagnostics", v)
		}
	}
}
