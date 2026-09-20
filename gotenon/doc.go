// Package gotenon maps Go values to tenon values and back.
//
// Encode makes a tenon value from a Go value, and Decode makes a Go value from
// a tenon value by converting it, under a policy the caller chooses, to the
// constraint the Go type maps to. Go's booleans, strings and numbers map to
// Bool, String and Number; slices and arrays to lists; maps with string keys to
// maps; structs to objects; an interface to what its value holds; and
// tenon.Value to any value at all. Struct fields
// are named by their tenon tag, `tenon:"name,optional"`, or by the field's
// name, and `tenon:"-"` leaves one out.
//
// # Data whose types are not known at compile time
//
// Such data reaches a Go program as any: encoding/json gives map[string]any,
// and so does every other reader of foreign data. Encode takes it, by what
// each value holds, so a map of anything is an object and a slice of anything
// a tuple, at any depth. Read the document with json.Decoder.UseNumber, and
// its numbers arrive as json.Number and encode as the numbers they spell,
// keeping the distinction the document drew between 8080 and "8080"; read it
// without, and each number is the float64 nearest to it, which is a different
// number and says so.
//
// Two things such data cannot carry by itself. A JSON null says null without
// saying null of what, and a null has a type, so encoding a nil interface
// fails with tenon.CodeEncodeUntypedNil, located by its path, and the schema
// the caller converts to is what says which null it meant. And decoding back
// into an interface is a usage error: nothing in a value says which Go type it
// would take, so decode into tenon.Value, which holds any value, or into the
// Go types the program actually has.
//
// A type can encode and decode itself by implementing ValueMarshaler and, on
// its pointer, ValueUnmarshaler. Failures are reported as a *DiagnosticError,
// with a diagnostic for each part of the value that fails, located by its
// path.
//
// # What Go has no type for
//
// A field of type tenon.Value carries whatever Go cannot hold: a value that is
// not known yet and what it is bounded by, a value carrying marks, or one
// whose type is not settled. Encode keeps it as it is, and Decode fills it
// without converting, so a program can hand a Go struct to a plan engine or a
// plugin protocol and get it back with the parts that were open still open.
//
// Everything else must be known and unmarked to decode: a value that is not
// known gives tenon.CodeDecodeNotKnown, and a marked one
// tenon.CodeDecodeMarked, both located by their path, since a Go string has
// nowhere to keep either fact.
//
// # Numbers
//
// Numbers cross exactly. A float64 encodes as the terminating decimal it
// holds, which is not the decimal its literal was written as: 0.1 encodes as
// 0.1000000000000000055511151231257827021181583404541015625, the number the
// float64 is. Text, a big.Rat or a big.Int carries the number that was meant.
// Decoding into a float64 takes the nearest float64, ties to even, so a round
// trip through one is not the identity and says so in what it gives back.
//
// # Mistakes against failures
//
// A Go type that does not map to tenon is a mistake in the program, and Encode
// and Decode panic on it: an interface, a channel, a function, a complex
// number, a map without string keys, a type that holds itself, or a struct
// whose tags are malformed. Data that does not fit a type that maps is a
// failure in the data, and comes back as a *DiagnosticError.
package gotenon
