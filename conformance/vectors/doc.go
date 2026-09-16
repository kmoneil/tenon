// Package vectors holds tenon's corpus of encodings, vectors.json, for testing
// an implementation of the encoding against this one. Each valid vector is a
// value and the exact bytes of its one encoding: a decoder must accept the
// bytes, and encoding the value it decodes must give them back. Each invalid
// vector is input that encodes no value, with the diagnostic code a decoder
// gives it.
//
// The test in this package builds every valid value several ways, with
// members in different orders, numbers and strings spelled differently and
// marks attached in different orders, requires every way to give the vector's
// bytes, and requires the file to be what it would write. Setting
// TENON_UPDATE_VECTORS=1 rewrites the file instead, for a deliberate change.
package vectors
