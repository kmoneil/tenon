package uni

import (
	"github.com/rivo/uniseg"
)

// GraphemeCount returns the number of extended grapheme clusters in s, as
// Unicode Standard Annex #29 defines them for UnicodeVersion. That count is the
// length of a string value; the number of scalar values and the number of
// bytes, len(s), are different measures.
func GraphemeCount(s string) int {
	return uniseg.GraphemeClusterCount(s)
}
