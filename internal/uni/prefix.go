package uni

import "golang.org/x/text/unicode/norm"

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
// s must be well-formed UTF-8 in Normalization Form C, as the content of a
// string value is.
func StablePrefix(s string) string {
	i := norm.NFC.LastBoundary([]byte(s))
	if i < 0 {
		// No position in s is safe, which is also the case for empty text.
		return ""
	}
	return s[:i]
}
