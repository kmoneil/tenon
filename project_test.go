package tenon_test

import (
	"encoding/json"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

// wantProjection fails t unless v projects to want.
func wantProjection(t *testing.T, what string, v tenon.Value, want string) {
	t.Helper()
	got, failure, ok := tenon.ProjectJSON(v)
	switch {
	case !ok:
		t.Errorf("%s: ProjectJSON(%v) failed: %v", what, v, failure)
	case string(got) != want:
		t.Errorf("%s: ProjectJSON(%v) = %s, want %s", what, v, got, want)
	case !json.Valid(got):
		t.Errorf("%s: ProjectJSON(%v) = %s, which is not JSON", what, v, got)
	}
}

// wantProjectionFailure fails t unless v fails to project with exactly these
// diagnostics.
func wantProjectionFailure(t *testing.T, what string, v tenon.Value, want ...wantDiag) {
	t.Helper()
	got, failure, ok := tenon.ProjectJSON(v)
	if ok {
		t.Errorf("%s: ProjectJSON(%v) = %s, want a failure", what, v, got)
		return
	}
	wantErrors(t, what, failure, want...)
}

func TestConformance_SE062_Projection(t *testing.T) {
	conformance.Covers(t, "SE-062", "SE-060")
	degreesShown := tenon.Capsule("shown", tenon.CapsuleOps[celsius]{
		Display: func(v *celsius) string { return tenon.NumberFromInt(v.degrees).String() + " degrees" },
	})
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"null", tenon.NullVal(tenon.List(num)), `null`},
		{"true", tenon.Bool(true), `true`},
		{"an integer", n(-42), `-42`},
		{"an exact fraction", tenon.NumberFromText("0.1000"), `0.1`},
		{"a large integer, every digit kept", tenon.NumberFromText("123456789012345678901234567890"), `1.2345678901234567890123456789e29`},
		{"an integer below the scientific form", tenon.NumberFromText("123456789012345678901"), `123456789012345678901`},
		{"a number in scientific form", tenon.NumberFromText("1.5e30"), `1.5e30`},
		{"a small number", tenon.NumberFromText("-2.5e-25"), `-2.5e-25`},
		{"a string", s("tenon"), `"tenon"`},
		{"a list", tenon.ListVal(num, n(1), n(2)), `[1,2]`},
		{"a set, in iteration order", tenon.SetVal(num, n(3), n(1), n(2)), `[1,2,3]`},
		{"a tuple", tenon.TupleVal(tenon.Bool(false), s("x"), tenon.NullVal(num)), `[false,"x",null]`},
		{"a map, in key order", tenon.MapVal(num, map[string]tenon.Value{"b": n(2), "": n(0), "a": n(1)}), `{"":0,"a":1,"b":2}`},
		{"an object, in name order", obj(map[string]tenon.Value{"z": tenon.ListVal(str), "a": obj(nil)}), `{"a":{},"z":[]}`},
		{"a capsule value", tenon.CapsuleVal(degreesShown, &celsius{21}), `"21 degrees"`},
		// Marks that do not redact are left out.
		{"a marked value", tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(n(1), stamp{id: "m"})), stamp{id: "n"}), `[1]`},
	} {
		wantProjection(t, tt.name, tt.v, tt.want)
	}
	// One value, however it was built, projects to the same text.
	a, _, _ := tenon.ProjectJSON(tenon.SetVal(str, s("b"), s("a")))
	b, _, _ := tenon.ProjectJSON(tenon.SetVal(str, s("a"), s("b"), s("a")))
	if string(a) != string(b) {
		t.Errorf("one set projects as %s and as %s", a, b)
	}
}

func TestConformance_SE063_ProjectedStrings(t *testing.T) {
	conformance.Covers(t, "SE-063")
	wantProjection(t, "escapes", s("\"\\\b\t\n\f\r\x00\x1f/\x7f"), `"\"\\\b\t\n\f\r`+"\\"+`u0000`+"\\"+`u001f/`+"\x7f"+`"`)
	// Other characters are their UTF-8 bytes, unescaped.
	wantProjection(t, "unicode", s("caf\U000000e9 \U0001F600 \U00002028"), "\"caf\U000000e9 \U0001F600 \U00002028\"")
	// A composed character stays composed, since the string holds it so.
	wantProjection(t, "normalized", s("e\U00000301"), "\"\U000000e9\"")
	var decoded string
	got, _, _ := tenon.ProjectJSON(s("\x01\"x\\"))
	if err := json.Unmarshal(got, &decoded); err != nil || decoded != "\x01\"x\\" {
		t.Errorf("%s reads back as %q, %v", got, decoded, err)
	}
}

func TestConformance_SE061_WhatDoesNotProject(t *testing.T) {
	conformance.Covers(t, "SE-061", "SE-060", "SE-051")
	secret := stamp{id: "secret", redact: true}
	wantProjectionFailure(t, "an unknown", tenon.Unknown(num), wantDiag{tenon.CodeSerializeNotKnown, ""})
	wantProjectionFailure(t, "a pending value", tenon.Pending(tenon.Any()), wantDiag{tenon.CodeSerializeNotKnown, ""})
	wantProjectionFailure(t, "unknown members, each located", obj(map[string]tenon.Value{
		"a": tenon.ListVal(num, n(1), tenon.Unknown(num)),
		"b": tenon.Unknown(str),
	}), wantDiag{tenon.CodeSerializeNotKnown, ".a[1]"}, wantDiag{tenon.CodeSerializeNotKnown, ".b"})
	// A redacted value is refused whole: what it holds is not looked at.
	wantProjectionFailure(t, "a redacted list holding an unknown", tenon.WithMarks(tenon.ListVal(num, tenon.Unknown(num)), secret),
		wantDiag{tenon.CodeSerializeRedacted, ""})
	wantProjectionFailure(t, "a redacted member", tenon.MapVal(str, map[string]tenon.Value{"password": tenon.WithMarks(s("hunter2"), secret)}),
		wantDiag{tenon.CodeSerializeRedacted, `["password"]`})
	_, failure, _ := tenon.ProjectJSON(tenon.WithMarks(s("hunter2"), secret))
	if strings.Contains(failure.String(), "hunter2") {
		t.Errorf("the failure shows what the mark withholds: %v", failure)
	}
	// Unmarking deliberately projects what the mark withheld.
	unmarked, _ := tenon.UnmarkDeep(tenon.WithMarks(s("hunter2"), secret))
	wantProjection(t, "unmarked", unmarked, `"hunter2"`)

	opaque := tenon.Capsule("opaque", tenon.CapsuleOps[celsius]{})
	wantProjectionFailure(t, "a capsule with no display form", tenon.ListVal(opaque, tenon.CapsuleVal(opaque, &celsius{})),
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "[0]"})
	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"})
	if _, got, ok := tenon.ProjectJSON(failed); ok || !tenon.Identical(got, failed) {
		t.Errorf("projecting an error value gave %v", got)
	}
	mustPanicUsage(t, "use of the zero Value", func() { tenon.ProjectJSON(tenon.Value{}) })
}
