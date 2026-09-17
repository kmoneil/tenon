// Package tenon is a dynamic type and value system for applications that must
// represent user-supplied data whose types are not known at compile time, such
// as the value layer of a configuration language.
//
// This package implements tenon specification version 0.1.0-draft.
//
// # Unicode
//
// tenon holds strings in Normalization Form C and measures their length in
// grapheme clusters, both under Unicode 15.0.0, and its display form escapes
// text by the same version. Which Unicode version is in use decides which
// strings are equal and how long they are, so changing it is a breaking
// change.
//
// Known issue: the version is not yet held inside this module. Normalization
// and display escaping read tables that follow the toolchain a consumer builds
// with, so a build with Go 1.27 or later applies Unicode 17.0.0 to both, while
// grapheme cluster lengths stay at 15.0.0. Some sequences normalize
// differently between the two versions, so a value built under one can
// serialize to bytes that a build under the other refuses as not canonical.
// Build with Go 1.26 for the version this module states.
package tenon
