//go:build !go1.27

package tenon_test

import (
	"testing"
	"unicode"

	"github.com/kmoneil/tenon"
)

// checkDisplayedCategories holds the display form to Go's general categories,
// which below go1.27 are the version tenon states. Above it they are not, and
// the other half of this pair checks nothing.
func checkDisplayedCategories(t *testing.T, v tenon.Value) {
	t.Helper()
	for _, r := range v.String() {
		if r != ' ' && !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S) {
			t.Fatalf("%v displays U+%04X as itself", v, r)
		}
	}
}
