package uni

import "slices"

// IsSeparatorOrOther reports whether the general category of r is a separator
// (Z) or an other (C), which takes in every code point that Unicode has not
// assigned. Equivalently, r is not a letter, mark, number, punctuation or
// symbol.
//
// The categories are those of UnicodeVersion, which this package holds, so
// they do not follow whichever Go toolchain a consumer builds with.
func IsSeparatorOrOther(r rune) bool {
	_, inRange := slices.BinarySearchFunc(assigned[:], r, func(e [2]rune, r rune) int {
		switch {
		case r < e[0]:
			return 1
		case r > e[1]:
			return -1
		}
		return 0
	})
	return !inRange
}
