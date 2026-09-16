// Package gotenon maps Go values to tenon values and back.
//
// Encode makes a tenon value from a Go value, and Decode makes a Go value from
// a tenon value by converting it, under a policy the caller chooses, to the
// constraint the Go type maps to. Go's booleans, strings and numbers map to
// Bool, String and Number; slices and arrays to lists; maps with string keys to
// maps; structs to objects; and tenon.Value to any value at all. Struct fields
// are named by their tenon tag, `tenon:"name,optional"`, or by the field's
// name, and `tenon:"-"` leaves one out.
//
// A type can encode and decode itself by implementing ValueMarshaler and, on
// its pointer, ValueUnmarshaler. Failures are reported as a *DiagnosticError,
// with a diagnostic for each part of the value that fails, located by its
// path.
package gotenon
