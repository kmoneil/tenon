// Package resyntax is a copy of the regexp/syntax package of Go 1.26, whose
// license (LICENSE, beside this file) it is distributed under, changed so
// that what a pattern means does not follow the Go toolchain a consumer
// builds with:
//
//   - the Unicode properties a pattern names, \p{Greek} and \pL among them,
//     their aliases, and the case folding (?i) applies, are read from
//     tenon's own tables of the Unicode version tenon states
//     (internal/uni), not from Go's unicode package;
//   - Regexp.String writes every code point that is not printable ASCII as
//     \x{...}, so the text it writes does not depend on which code points
//     Go's unicode package calls printable.
//
// Nothing else is changed. tenon's patterns are parsed here, the case
// folding of their literals made explicit, and the text String writes,
// which then names only explicit ranges, is what Go's regexp package
// compiles and matches.
package resyntax
