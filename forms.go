package tenon

import (
	"errors"
	"log/slog"
)

// The forms other packages render values, types, paths and constraints in:
// log/slog's LogValuer and encoding's TextMarshaler give the display form,
// and encoding/json's Marshaler gives a value's JSON projection (DI-018).
// Each keeps what a redacting mark withholds withheld, as the display form
// and the projection do, and none panics.

// LogValue returns the display form of v, which log/slog logs in place of
// v's representation.
func (v Value) LogValue() slog.Value { return slog.StringValue(v.String()) }

// MarshalJSON returns v's JSON projection, as ProjectJSON gives it, which
// encoding/json writes in place of v's representation. It fails where the
// projection does, with a *Error: a value that is not known, one a redacting
// mark withholds, or a capsule value its type gives no display form. It fails
// as well for the zero Value, which is not a value; a struct field that may
// hold it is tagged omitzero, and left out. encoding/json escapes <, > and &
// in what a MarshalJSON returns, as it escapes them in strings by default, so
// the bytes json.Marshal writes can differ from ProjectJSON's; an Encoder with
// SetEscapeHTML(false) writes the projection as ProjectJSON gives it.
func (v Value) MarshalJSON() ([]byte, error) {
	if v.IsZero() {
		return nil, errors.New("tenon: the zero Value is not a value, and has no JSON form")
	}
	return ProjectJSON(v)
}

// LogValue returns the display form of t, which log/slog logs in place of t's
// representation.
func (t Type) LogValue() slog.Value { return slog.StringValue(t.String()) }

// MarshalText returns the display form of t, which encoding/json, among
// others, writes as a string. It fails for the zero Type, which is not a type.
func (t Type) MarshalText() ([]byte, error) {
	if t.IsZero() {
		return nil, errors.New("tenon: the zero Type is not a type, and has no text")
	}
	return []byte(t.String()), nil
}

// LogValue returns the display form of c, which log/slog logs in place of
// c's representation.
func (c Constraint) LogValue() slog.Value { return slog.StringValue(c.String()) }

// MarshalText returns the display form of c, which encoding/json, among
// others, writes as a string. It fails for the zero Constraint, which is not a
// constraint.
func (c Constraint) MarshalText() ([]byte, error) {
	if c.IsZero() {
		return nil, errors.New("tenon: the zero Constraint is not a constraint, and has no text")
	}
	return []byte(c.String()), nil
}

// LogValue returns the display form of p, which log/slog logs in place of
// p's representation.
func (p Path) LogValue() slog.Value { return slog.StringValue(p.String()) }

// MarshalText returns the display form of p, which encoding/json, among
// others, writes as a string: "." for the empty path.
func (p Path) MarshalText() ([]byte, error) { return []byte(p.String()), nil }
