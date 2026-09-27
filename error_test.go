package tenon_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
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
			tenon.Map(num, map[string]tenon.Value{"a\xff": one}),
			tenon.CodeStringInvalidUTF8,
		},
		{
			"map keys that are the same key",
			tenon.Map(num, map[string]tenon.Value{"caf\u00e9": one, "cafe\u0301": two}),
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

// TestError holds tenon.Error to what a Go error is expected to do: render
// its diagnostics, be found through wrapping, lead errors.Is and errors.As to
// its causes, and never panic, even holding nothing.
func TestError(t *testing.T) {
	cause := errors.New("the disk is full")
	failure := tenon.ErrorVal(
		tenon.Diagnostic{Code: "app.failed", Message: "it failed", Path: tenon.Path{}.Attribute("a")},
		tenon.Diagnostic{Code: "app.other", Message: "and again"},
	)
	e := tenon.NewError(failure, cause, nil)
	if got, want := e.Error(), "app.failed: it failed at .a; app.other: and again"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !tenon.Identical(e.Value(), failure) || !slices.EqualFunc(e.Diagnostics(), failure.Diagnostics(), tenon.Diagnostic.Equal) {
		t.Errorf("the Error holds %v, want %v", e.Value(), failure)
	}
	causes := e.Unwrap()
	if len(causes) != 1 || causes[0] != cause {
		t.Errorf("Unwrap() = %v, want the one cause that is not nil", causes)
	}
	causes[0] = nil
	if e.Unwrap()[0] != cause {
		t.Error("changing the slice Unwrap returned changed the Error")
	}
	wrapped := fmt.Errorf("loading the plan: %w", e)
	var found *tenon.Error
	if !errors.As(wrapped, &found) || found != e {
		t.Errorf("errors.As did not find the Error in %v", wrapped)
	}
	if !errors.Is(wrapped, cause) {
		t.Errorf("errors.Is did not find the cause through %v", wrapped)
	}
	// An Error holding no error value answers as one with no diagnostics.
	for _, empty := range []*tenon.Error{nil, {}} {
		if empty.Error() == "" || empty.Diagnostics() != nil || !empty.Value().IsZero() || empty.Unwrap() != nil {
			t.Errorf("an Error holding nothing answered %q, %v, %v, %v", empty.Error(), empty.Diagnostics(), empty.Value(), empty.Unwrap())
		}
	}
	mustPanicUsage(t, "which is not an error value", func() { tenon.NewError(tenon.Bool(true)) })
	mustPanicUsage(t, "use of the zero Value", func() { tenon.NewError(tenon.Value{}) })
}

// TestFunctionsFailWithError holds each function that fails with diagnostics
// to failing with a *tenon.Error, not wrapped, holding them, and to returning
// an error that is nil, not a nil *tenon.Error, where it succeeds.
func TestFunctionsFailWithError(t *testing.T) {
	num, boolean := tenon.Exactly(tenon.NumberType()), tenon.Exactly(tenon.BoolType())
	encoded, err := tenon.Serialize(tenon.Bool(true))
	if err != nil {
		t.Fatalf("Serialize(true) failed: %v", err)
	}
	errorOf := func(_ any, err error) error { return err }
	for _, tt := range []struct {
		name          string
		succeed, fail error
		code          tenon.Code
	}{
		{
			"Serialize",
			errorOf(tenon.Serialize(tenon.Bool(true))),
			errorOf(tenon.Serialize(tenon.WithMarks(tenon.Bool(true), stamp{id: "plain"}))),
			tenon.CodeSerializeUnencodableMark,
		},
		{
			"Deserialize",
			errorOf(tenon.Deserialize(encoded, tenon.Decoders{})),
			errorOf(tenon.Deserialize(nil, tenon.Decoders{})),
			tenon.CodeSerializeMalformed,
		},
		{
			"ProjectJSON",
			errorOf(tenon.ProjectJSON(tenon.Bool(true))),
			errorOf(tenon.ProjectJSON(tenon.Unknown(tenon.BoolType()))),
			tenon.CodeSerializeNotKnown,
		},
		{
			"Unify",
			errorOf(tenon.Unify([]tenon.Constraint{num, num}, tenon.Safe)),
			errorOf(tenon.Unify([]tenon.Constraint{num, boolean}, tenon.Safe)),
			tenon.CodeUnifyNoCommonConstraint,
		},
	} {
		if tt.succeed != nil {
			t.Errorf("%s succeeded with the error %#v, want nil", tt.name, tt.succeed)
		}
		e, ok := tt.fail.(*tenon.Error)
		if !ok || len(e.Diagnostics()) == 0 || e.Diagnostics()[0].Code != tt.code {
			t.Errorf("%s failed with %#v, want a *tenon.Error of code %s", tt.name, tt.fail, tt.code)
		}
	}
}
