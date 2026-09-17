//go:build !go1.27

package tenon_test

import (
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/kmoneil/tenon"
)

// checkAgainstXText holds a String's content to golang.org/x/text, which below
// go1.27 normalizes by the Unicode version tenon states. tenon holds that
// version's data itself, so x/text is an oracle here and not the normalizer
// under test answering its own question.
//
// A go1.27 build compiles the other half of this pair, which checks nothing:
// there x/text normalizes by a later version of Unicode and would disagree
// about the sequences that version composes, which is the whole point of
// tenon holding the data.
func checkAgainstXText(t *testing.T, s string, v tenon.Value, content string) {
	t.Helper()
	// tenon normalizes by plain UAX #15. x/text also applies the Stream-Safe
	// Text Process, which inserts U+034F into a run of more than thirty
	// non-starters, so that is the one string it will not call normalized.
	// Removing what it inserts must give back what tenon produced.
	if !norm.NFC.IsNormalString(content) && withoutJoiners(norm.NFC.String(content)) != withoutJoiners(content) {
		t.Fatalf("String(%q) holds %q, which is not in Normalization Form C", s, content)
	}
	for _, form := range []norm.Form{norm.NFD, norm.NFKC} {
		written := form.String(s)
		if inserts(form, s) {
			// x/text broke a long run up and tenon did not, so the two are
			// being asked about different strings.
			continue
		}
		other := tenon.String(written)
		eq := tenon.Equals(v, other)
		if want := form == norm.NFD || norm.NFC.String(written) == content; !eq.IsKnown() || eq.AsBool() != want {
			t.Fatalf("Equals(%v, %v) = %v, want %t", v, other, eq, want)
		}
	}
}

// inserts reports whether x/text's form adds a combining grapheme joiner to s
// that s did not hold, which is the Stream-Safe Text Process at work.
func inserts(form norm.Form, s string) bool {
	return strings.Count(form.String(s), "͏") != strings.Count(s, "͏")
}

// withoutJoiners returns s without any combining grapheme joiner.
func withoutJoiners(s string) string { return strings.ReplaceAll(s, "͏", "") }
