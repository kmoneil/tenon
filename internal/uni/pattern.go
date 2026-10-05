package uni

import "slices"

// SimpleFold returns the code point after r in its orbit under simple case
// folding, as Go's unicode.SimpleFold does, by the data of UnicodeVersion:
// the smallest equivalent code point greater than r, or else the smallest,
// and r itself where nothing folds with it.
func SimpleFold(r rune) rune {
	i, found := slices.BinarySearchFunc(foldOrbit[:], r, func(e [2]rune, r rune) int { return int(e[0] - r) })
	if !found {
		return r
	}
	return foldOrbit[i][1]
}
