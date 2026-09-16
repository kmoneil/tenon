package gotenon_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
)

// Shorthands for the tests.
var (
	num  = tenon.NumberType()
	str  = tenon.StringType()
	boo  = tenon.BoolType()
	n    = tenon.NumberFromInt
	s    = tenon.String
	safe = tenon.Safe
	uns  = tenon.Unsafe
)

// is returns Exactly(t).
func is(t tenon.Type) tenon.Constraint { return tenon.Exactly(t) }

// obj returns an object value.
func obj(attrs map[string]tenon.Value) tenon.Value { return tenon.ObjectVal(attrs) }

// stamp is a Mark with a configurable identity and propagation.
type stamp struct {
	id     string
	policy tenon.Propagation
}

func (m stamp) MarkID() string                 { return m.id }
func (m stamp) Propagation() tenon.Propagation { return m.policy }
func (m stamp) Redacting() bool                { return false }

// wantValue fails t unless got is identical to want.
func wantValue(t *testing.T, what string, got, want tenon.Value) {
	t.Helper()
	if !tenon.Identical(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// wantDiag is a diagnostic a test expects: its code, and its path rendered.
type wantDiag struct {
	code tenon.Code
	path string
}

// wantErrors fails t unless got is an error value with exactly these
// diagnostics, in order.
func wantErrors(t *testing.T, what string, got tenon.Value, want ...wantDiag) {
	t.Helper()
	if !got.IsError() {
		t.Errorf("%s = %v, want an error value", what, got)
		return
	}
	var have []wantDiag
	for _, d := range got.Diagnostics() {
		have = append(have, wantDiag{d.Code, d.Path.String()})
	}
	if !slices.Equal(have, want) {
		t.Errorf("%s = %v, want diagnostics %v", what, got, want)
	}
}

// mustPanicUsage runs f and fails t unless f panics with a usage error whose
// message contains want.
func mustPanicUsage(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		msg, ok := r.(string)
		if !ok || !strings.HasPrefix(msg, "tenon: usage: ") || !strings.Contains(msg, want) {
			t.Errorf("recovered %#v, want a usage error panic containing %q", r, want)
		}
	}()
	f()
}
