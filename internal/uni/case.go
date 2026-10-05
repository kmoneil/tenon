package uni

import (
	"slices"
	"unicode/utf8"
)

// Case conversion and White_Space, by the data of UnicodeVersion that
// tools/unigen generates into case_tables.go from Unicode's own files, so
// that neither follows the Go toolchain a consumer builds with.

// caseRange is code points from first to last whose simple case mappings
// are the code point offset by upper, lower and title, or, where
// alternating, alternate between an uppercase code point, at an even offset
// from first, and its lowercase after it.
type caseRange struct {
	first, last         rune
	upper, lower, title rune
	alternating         bool
}

// specialCasing is a code point's full case mappings, each a sequence of
// code points: an unconditional entry of SpecialCasing.txt.
type specialCasing struct {
	r                   rune
	lower, title, upper string
}

// simpleCase returns the simple case mappings of r: uppercase, lowercase
// and titlecase, each r itself where r has none.
func simpleCase(r rune) (upper, lower, title rune) {
	switch {
	case r >= 'a' && r <= 'z':
		return r - 32, r, r - 32
	case r < 0x80:
		if r >= 'A' && r <= 'Z' {
			return r, r + 32, r
		}
		return r, r, r
	}
	i, found := slices.BinarySearchFunc(caseRanges[:], r, func(c caseRange, r rune) int {
		switch {
		case r < c.first:
			return 1
		case r > c.last:
			return -1
		}
		return 0
	})
	if !found {
		return r, r, r
	}
	c := &caseRanges[i]
	if c.alternating {
		k := r - c.first
		upper = c.first + (k &^ 1)
		return upper, c.first + (k | 1), upper
	}
	return r + c.upper, r + c.lower, r + c.title
}

// specialCaseOf returns r's unconditional entry of SpecialCasing.txt, and
// false where it has none. ASCII has none.
func specialCaseOf(r rune) (*specialCasing, bool) {
	if r < 0x80 {
		return nil, false
	}
	i, found := slices.BinarySearchFunc(specialCasings[:], r, func(s specialCasing, r rune) int { return int(s.r - r) })
	if !found {
		return nil, false
	}
	return &specialCasings[i], true
}

// inRanges reports whether r lies in one of ranges, which are in order.
func inRanges(ranges [][2]rune, r rune) bool {
	_, found := slices.BinarySearchFunc(ranges, r, func(e [2]rune, r rune) int {
		switch {
		case r < e[0]:
			return 1
		case r > e[1]:
			return -1
		}
		return 0
	})
	return found
}

// IsWhiteSpace reports whether r has the White_Space property.
func IsWhiteSpace(r rune) bool { return inRanges(whiteSpace[:], r) }

// Upper returns s converted to uppercase by Unicode's default case
// conversion (the Unicode Standard, section 3.13, R1): each code point
// replaced by its full uppercase mapping, the unconditional entry of
// SpecialCasing.txt where it has one and its simple mapping otherwise, with
// no language's tailoring, so "ß" is "SS". The result may not be in
// Normalization Form C. The caller must ensure that s is well-formed UTF-8.
func Upper(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if c, ok := specialCaseOf(r); ok {
			b = append(b, c.upper...)
			continue
		}
		u, _, _ := simpleCase(r)
		b = utf8.AppendRune(b, u)
	}
	return string(b)
}

// Lower returns s converted to lowercase by Unicode's default case
// conversion (R2), as Upper does, and with the one condition the default
// conversion has: a capital sigma that ends a word, Final_Sigma, is the
// final sigma, so "ΟΔΟΣ" is "οδος".
func Lower(s string) string {
	b := make([]byte, 0, len(s))
	for i, r := range s {
		switch c, ok := specialCaseOf(r); {
		case r == capitalSigma && finalSigma(s, i):
			b = utf8.AppendRune(b, smallFinalSigma)
		case ok:
			b = append(b, c.lower...)
		default:
			_, l, _ := simpleCase(r)
			b = utf8.AppendRune(b, l)
		}
	}
	return string(b)
}

// Title returns the full titlecase mapping of r, as Upper maps a code point
// to uppercase: "ß" is "Ss" and "ǆ" is "ǅ".
func Title(r rune) string {
	if c, ok := specialCaseOf(r); ok {
		return c.title
	}
	_, _, t := simpleCase(r)
	return string(t)
}

const (
	capitalSigma    = 0x03A3
	smallFinalSigma = 0x03C2
)

// finalSigma reports whether the code point at i in s meets the condition
// Final_Sigma (Table 3-17): passing over the case-ignorable code points on
// either side of it, a cased code point comes before it and none after it.
// A code point both cased and case-ignorable, as U+0345 is, is passed over
// on both sides, as ICU, CPython and Rust pass over it; golang.org/x/text
// passes over it after the sigma, and takes it for the cased code point
// before it.
func finalSigma(s string, i int) bool {
	before := false
	for j := i; j > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		if !inRanges(caseIgnorable[:], r) {
			before = inRanges(cased[:], r)
			break
		}
		j -= size
	}
	if !before {
		return false
	}
	_, size := utf8.DecodeRuneInString(s[i:])
	for _, r := range s[i+size:] {
		if !inRanges(caseIgnorable[:], r) {
			return !inRanges(cased[:], r)
		}
	}
	return true
}
