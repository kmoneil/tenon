//go:build go1.27

package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
)

// checkDisplayedCategories checks nothing on a toolchain of go1.27 or later,
// Go's general categories being those of a later version of Unicode there.
// internal/uni cross-checks the categories tenon holds against Go's wherever
// the toolchain still agrees.
func checkDisplayedCategories(t *testing.T, v tenon.Value) {}
