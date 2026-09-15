package uni

import (
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// GraphemeCount returns the number of extended grapheme clusters in s, as
// Unicode Standard Annex #29 defines them for UnicodeVersion. That count is the
// length of a string value; the number of scalar values, ScalarCount, and the
// number of bytes, len(s), are different measures.
func GraphemeCount(s string) int {
	return uniseg.GraphemeClusterCount(s)
}

// ScalarCount returns the number of Unicode scalar values in s, which must be
// well-formed UTF-8.
func ScalarCount(s string) int {
	return utf8.RuneCountInString(s)
}
