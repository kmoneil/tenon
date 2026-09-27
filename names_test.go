package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// TestConformance_TY018_NamesFromDataFailAsData holds Object to reporting
// names that cannot be attribute names as a map's keys are reported, rather
// than panicking: each name's fault and each error attribute's diagnostics in
// the order of the names, then a diagnostic for each name shared. An error
// attribute is located by its name where the name can be one, and keeps its
// paths where it cannot.
func TestConformance_TY018_NamesFromDataFailAsData(t *testing.T) {
	conformance.Covers(t, "TY-018", "ER-002")
	one := tenon.NumberFromInt(1)
	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed", Path: tenon.Path{}.Index(one)})
	type diag struct {
		code tenon.Code
		path string
	}
	for _, tt := range []struct {
		name  string
		attrs map[string]tenon.Value
		want  []diag
	}{
		{"an empty name", map[string]tenon.Value{"": one}, []diag{{tenon.CodeObjectEmptyName, "."}}},
		{"a name that is not UTF-8", map[string]tenon.Value{"a\xff": one}, []diag{{tenon.CodeStringInvalidUTF8, "."}}},
		{
			"two names that normalize alike",
			map[string]tenon.Value{"caf\U000000e9": one, "cafe\U00000301": one},
			[]diag{{tenon.CodeObjectDuplicateName, "."}},
		},
		{
			// The empty name sorts first, "b" among the rest, a name that is
			// not UTF-8 by its bytes, and the name shared after them all.
			"every fault at once, beside error attributes",
			map[string]tenon.Value{
				"":               one,
				"b":              failed,
				"caf\U000000e9":  failed,
				"cafe\U00000301": one,
				"\xff":           failed,
			},
			[]diag{
				{tenon.CodeObjectEmptyName, "."},
				{"app.failed", ".b[1]"},
				{"app.failed", `."caf` + "\U000000e9" + `"[1]`},
				{tenon.CodeStringInvalidUTF8, "."},
				{"app.failed", ".[1]"},
				{tenon.CodeObjectDuplicateName, "."},
			},
		},
	} {
		got := tenon.Object(tt.attrs)
		if !got.IsError() {
			t.Errorf("%s: Object gave %v, want an error value", tt.name, got)
			continue
		}
		diags := got.Diagnostics()
		if len(diags) != len(tt.want) {
			t.Errorf("%s: Object gave %v, want %d diagnostics", tt.name, got, len(tt.want))
			continue
		}
		for i, w := range tt.want {
			if diags[i].Code != w.code || diags[i].Path.String() != w.path {
				t.Errorf("%s: diagnostic %d is %s at %s, want %s at %s", tt.name, i, diags[i].Code, diags[i].Path, w.code, w.path)
			}
		}
	}
	// A program's own mistake still panics, before any fault in the names is
	// reported.
	mustPanicUsage(t, "is a pending value", func() {
		tenon.Object(map[string]tenon.Value{"": tenon.Pending(tenon.Any())})
	})
}

// TestCheckAttributeNames holds the exported check to what Object reports
// of names alone, and to answering nil for names that can be attribute names.
func TestCheckAttributeNames(t *testing.T) {
	if err := tenon.CheckAttributeNames("name", "a b", "caf\U000000e9", "x"); err != nil {
		t.Errorf("CheckAttributeNames of good names = %v, want nil", err)
	}
	if err := tenon.CheckAttributeNames(); err != nil {
		t.Errorf("CheckAttributeNames of no names = %v, want nil", err)
	}
	names := []string{"\xff", "cafe\U00000301", "b", "", "caf\U000000e9"}
	err := tenon.CheckAttributeNames(names...)
	e, ok := err.(*tenon.Error)
	if !ok {
		t.Fatalf("CheckAttributeNames(%q) = %#v, want a *tenon.Error", names, err)
	}
	attrs := map[string]tenon.Value{}
	for _, name := range names {
		attrs[name] = tenon.NumberFromInt(1)
	}
	want := tenon.Object(attrs).Diagnostics()
	if got := e.Diagnostics(); len(got) != len(want) || len(got) != 3 {
		t.Fatalf("CheckAttributeNames gave %v, want the %v Object gives", got, want)
	}
	for i, d := range e.Diagnostics() {
		if !d.Equal(want[i]) {
			t.Errorf("diagnostic %d is %v, want %v as Object gives it", i, d, want[i])
		}
	}
	// A name given twice is one name given twice.
	if e, ok := tenon.CheckAttributeNames("a", "a").(*tenon.Error); !ok || e.Diagnostics()[0].Code != tenon.CodeObjectDuplicateName {
		t.Errorf("CheckAttributeNames(a, a) = %v, want %s", e, tenon.CodeObjectDuplicateName)
	}
	// The constructors a program writes names for panic on such a name.
	mustPanicUsage(t, "must not be empty", func() { tenon.ObjectType(map[string]tenon.Type{"": tenon.BoolType()}) })
	mustPanicUsage(t, "must not be empty", func() { tenon.Path{}.Attribute("") })
	mustPanicUsage(t, "must not be empty", func() {
		tenon.ObjectWith(map[string]tenon.Field{"": tenon.Required(tenon.Any())}, false)
	})
}
