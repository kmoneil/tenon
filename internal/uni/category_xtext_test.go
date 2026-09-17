//go:build !go1.27

package uni

import (
	"testing"
	"unicode"

	"github.com/kmoneil/tenon/conformance"
)

// TestConformance_ST003_CategoriesAgreeWithGo holds the general categories
// this package carries to Go's, over every code point. It runs only below
// go1.27, where Go's tables are still the version these are for. Above it Go
// has moved on, and the point of carrying the categories is that tenon does
// not move with it.
func TestConformance_ST003_CategoriesAgreeWithGo(t *testing.T) {
	conformance.Covers(t, "ST-003")
	if unicode.Version != UnicodeVersion {
		t.Fatalf("Go's general categories are Unicode %s, and these tables are for %s", unicode.Version, UnicodeVersion)
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		want := !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S)
		if got := IsSeparatorOrOther(r); got != want {
			t.Fatalf("IsSeparatorOrOther(%U) = %v, and Go's categories say %v", r, got, want)
		}
	}
}
