//go:build go1.27

package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
)

// checkAgainstXText checks nothing on a toolchain of go1.27 or later.
// golang.org/x/text normalizes by a later version of Unicode there, so it
// would disagree with tenon about the sequences that version composes, which
// is exactly what tenon holding its own data is for. The tight check against
// x/text, and the check against an oracle outside both, are in internal/uni.
func checkAgainstXText(t *testing.T, s string, v tenon.Value, content string) {}

// checkDisplayedCategories checks nothing on a toolchain of go1.27 or later,
// Go's general categories being those of a later version of Unicode there.
// internal/uni cross-checks the categories tenon holds against Go's wherever
// the toolchain still agrees.
func checkDisplayedCategories(t *testing.T, v tenon.Value) {}
