// Package tenon is a dynamic type and value system for applications that must
// represent user-supplied data whose types are not known at compile time, such
// as the value layer of a configuration language.
//
// This package implements tenon specification version 0.1.0.
//
// # Unicode
//
// tenon holds strings in Normalization Form C and measures their length in
// grapheme clusters, both under Unicode 15.0.0, and its display form escapes
// text by the same version. Which Unicode version is in use decides which
// strings are equal and how long they are, so changing it is a breaking
// change, and the version is held inside this module: it does not follow the
// Go toolchain a consumer builds with, and two builds of one version of tenon
// agree on every string whatever toolchain made them.
//
// Normalization is plain UAX #15. In particular tenon does not apply the
// Stream-Safe Text Process, which inserts U+034F into a run of more than
// thirty non-starters: a string that gained a code point on the way in would
// not be the string that was given.
package tenon
