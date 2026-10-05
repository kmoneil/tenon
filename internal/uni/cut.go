package uni

import "iter"

// Clusters returns the extended grapheme clusters of s, in order, as
// GraphemeCount counts them. The caller must ensure that s is well-formed
// UTF-8.
func Clusters(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		var g segmenter
		start := 0
		for i, r := range s {
			if g.begins(graphemeBreakOf(r)) && i > 0 {
				if !yield(s[start:i]) {
					return
				}
				start = i
			}
		}
		if start < len(s) {
			yield(s[start:])
		}
	}
}

// Cuts holds the cut positions of a string: the byte offsets where a
// function searching or trimming it may cut it. They are the boundaries of
// its extended grapheme clusters, its start and its end among them, and the
// position between the CR and the LF of a CR LF, which one cluster holds,
// so that every position of ASCII text is a cut position.
type Cuts struct {
	bits []uint64
}

// CutsOf returns the cut positions of s. The caller must ensure that s is
// well-formed UTF-8.
func CutsOf(s string) Cuts {
	c := Cuts{bits: make([]uint64, len(s)/64+1)}
	var g segmenter
	cr := false
	for i, r := range s {
		if g.begins(graphemeBreakOf(r)) || cr && r == '\n' {
			c.bits[i/64] |= 1 << (i % 64)
		}
		cr = r == '\r'
	}
	c.bits[len(s)/64] |= 1 << (len(s) % 64)
	return c
}

// At reports whether the byte offset i is a cut position. An offset outside
// the string is none.
func (c Cuts) At(i int) bool {
	return i >= 0 && i/64 < len(c.bits) && c.bits[i/64]&(1<<(i%64)) != 0
}
