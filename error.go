package tenon

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Error is the error that [Serialize], [Deserialize], [ProjectJSON] and
// [Unify] return where they fail, and that package gotenon's Encode and
// Decode return: an error value, whose diagnostics say what failed, each
// located by its path, and the Go errors that caused it, if any. errors.As
// finds an Error through whatever wraps it, and errors.Is and errors.As find
// its causes through it.
//
// An Error is built by [NewError]. The zero Error and a nil *Error hold no
// error value; their methods answer as for an error with no diagnostics
// rather than panicking.
type Error struct {
	v      Value
	causes []error
}

// Equal reports whether e and f are the same error: both nil, or holding
// identical error values, as Value.Equal says, and as many causes, each the
// same error as the cause beside it in the other, as errors.Is says either way
// round. It is the method go-cmp's cmp.Equal calls.
func (e *Error) Equal(f *Error) bool {
	if e == nil || f == nil {
		return e == f
	}
	if !e.v.Equal(f.v) || len(e.causes) != len(f.causes) {
		return false
	}
	for i, c := range e.causes {
		if !errors.Is(c, f.causes[i]) || !errors.Is(f.causes[i], c) {
			return false
		}
	}
	return true
}

// NewError returns an Error holding v, an error value, and causes, the Go
// errors behind it, which Unwrap returns; a nil cause is left out. NewError
// panics if v is not an error value.
func NewError(v Value, causes ...error) *Error {
	if n := v.data(); n.state != stateError {
		usagePanic("NewError called with %s, which is not an error value", n.describe())
	}
	var kept []error
	for _, c := range causes {
		if c != nil {
			kept = append(kept, c)
		}
	}
	return &Error{v: v, causes: kept}
}

// Error renders the diagnostics, each as its code, its message and, where it
// is located within the value, its path, as in
// "decode.null: the value is null at .name", separated by "; ". It never
// panics.
func (e *Error) Error() string {
	if e == nil || e.v.IsZero() {
		return "tenon: an Error holding no error value"
	}
	var b strings.Builder
	for i, d := range e.v.Diagnostics() {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(string(d.Code))
		b.WriteString(": ")
		b.WriteString(d.Message)
		if d.Path.Len() > 0 {
			b.WriteString(" at ")
			b.WriteString(d.Path.String())
		}
	}
	return b.String()
}

// GoString returns Go syntax that builds e, which the %#v verb prints, as in
// tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{Code:"decode.null",
// Message:"the value is null"}), cause): the error value as Value.GoString
// writes it, and each cause as %#v writes it.
func (e *Error) GoString() string {
	switch {
	case e == nil:
		return "(*tenon.Error)(nil)"
	case e.v.IsZero():
		return "&tenon.Error{}"
	}
	return goSyntax("*tenon.Error", func(w *goWriter) {
		w.WriteString("tenon.NewError(")
		w.writeValue(e.v)
		for _, c := range e.causes {
			if w.full() {
				return
			}
			w.WriteString(", ")
			w.WriteString(fmt.Sprintf("%#v", c))
		}
		w.WriteByte(')')
	})
}

// Diagnostics returns the diagnostics of the error value, in order, in a new
// slice, and nil for an Error holding no error value.
func (e *Error) Diagnostics() []Diagnostic {
	if e == nil || e.v.IsZero() {
		return nil
	}
	return e.v.Diagnostics()
}

// Value returns the error value, with whatever marks it carries, and the zero
// Value for an Error holding none.
func (e *Error) Value() Value {
	if e == nil {
		return Value{}
	}
	return e.v
}

// Unwrap returns the Go errors that caused the error, in a new slice, for
// errors.Is and errors.As to look through.
func (e *Error) Unwrap() []error {
	if e == nil {
		return nil
	}
	return slices.Clone(e.causes)
}

// asError returns the error a function that failed with failure, an error
// value, returns.
func asError(failure Value) error { return &Error{v: failure} }
