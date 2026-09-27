package uni

import "unicode/utf8"

// StablePrefix returns the part of s that text following s cannot change: for
// every t, the canonical form of s + t begins with StablePrefix(s). Only the
// last normalization segment of s is at risk, so only it is dropped, and it is
// kept when nothing can combine with it. "cafe" keeps "caf", because a
// following combining acute composes with the "e"; "caf-" is kept whole,
// because nothing composes with a hyphen.
//
// The result is not itself immune to text appended directly to it: "caf" and a
// combining dot above compose to "caḟ". It is the text that s can grow into
// that the result holds for.
//
// It is the boundary UAX #15 defines, computed from this package's Unicode
// data, so the prefix a narrowing records does not follow the toolchain. s is
// kept whole where its last code point is inert: a starter with no
// decomposition that composes with nothing on either side. Otherwise the last
// segment goes: s is cut before its last code point whose decomposition
// begins with a starter, however many non-starters follow it, since plain
// NFC, which tenon applies (ST-002), caps no run of them.
//
// s must be well-formed UTF-8 in Normalization Form C, as the content of a
// string value is.
func StablePrefix(s string) string {
	if s == "" {
		return ""
	}
	if last, _ := utf8.DecodeLastRuneInString(s); inert(last) {
		return s
	}
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		if startsWithStarter(r) {
			return s[:i]
		}
	}
	return ""
}

// inert reports whether nothing appended after r can change r or what comes
// before it: r is a starter, has no decomposition, and composes with nothing
// either way.
func inert(r rune) bool {
	return combiningClass(r) == 0 && !decomposes(r) && !composesForward(r) && !composesBackward(r)
}
