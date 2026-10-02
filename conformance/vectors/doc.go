// Package vectors holds tenon's corpus of encodings, vectors.json, for testing
// an implementation of the encoding against this one. Each valid vector is a
// value, its display form, and the exact bytes of its one encoding: a decoder
// must accept the bytes, and encoding the value it decodes must give them back.
// Each invalid vector is input that encodes no value, with the diagnostic code
// a decoder gives it.
//
// Beside it, diffs.json holds the diff of each valid vector's value against
// copies changed in a few ways: marked, made unknown, and with a member added,
// removed or changed. Each entry gives the copy's encoding and the diff's
// display form.
//
// json.json holds JSON texts read with a constraint under a policy, for
// testing an implementation of reading JSON (ParseJSON): each text's bytes, the
// constraint as the encoding of a pending value of it, and either what the
// text reads as, its display form and its encoding, or the failures it gives,
// each a code and a path.
//
// The test in this package builds every valid value several ways, with
// members in different orders, numbers and strings spelled differently and
// marks attached in different orders, requires every way to give the vector's
// bytes, and requires the files to be what it would write. Setting
// TENON_UPDATE_VECTORS=1 rewrites the files instead, for a deliberate change.
package vectors
