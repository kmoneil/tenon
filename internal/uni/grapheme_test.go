package uni

import (
	"testing"

	"golang.org/x/text/unicode/norm"

	"tenon/conformance"
)

// TestConformance_ST003_UnicodeVersion pins both sources of Unicode data to
// UnicodeVersion. Upgrading golang.org/x/text or github.com/rivo/uniseg, or
// building with a Go toolchain that selects newer normalization tables, can
// move one of them to another version, which changes which strings are equal
// or how long they are. That must be a deliberate change of UnicodeVersion,
// never a silent one.
func TestConformance_ST003_UnicodeVersion(t *testing.T) {
	conformance.Covers(t, "ST-003")
	if norm.Version != UnicodeVersion {
		t.Errorf("normalization uses Unicode %s, but UnicodeVersion is %s", norm.Version, UnicodeVersion)
	}
	// Segmentation does not report its version, so a rule that Unicode 15.1
	// added pins it: from 15.1 on (GB9c), a virama joins Devanagari consonants
	// into one cluster, while in 15.0 the conjunct KSSA is two clusters.
	if got := GraphemeCount("\u0915\u094d\u0937"); got != 2 {
		t.Errorf("KSSA segments into %d clusters, not the 2 of Unicode 15.0; the segmentation data has moved from UnicodeVersion %s", got, UnicodeVersion)
	}
}

func TestConformance_ST005_Length(t *testing.T) {
	conformance.Covers(t, "ST-005")
	for _, tt := range []struct {
		name                      string
		s                         string
		graphemes, scalars, bytes int
	}{
		{"empty", "", 0, 0, 0},
		{"ASCII", "hello", 5, 5, 5},
		{"CRLF is one cluster", "a\r\nb", 3, 4, 4},
		{"combining marks", "x\u0301\u0302", 1, 3, 5},
		{"precomposed letter", "caf\u00e9", 4, 4, 5},
		{"Hangul syllables", "\ud55c\uae00", 2, 2, 6},
		{"emoji with a skin tone", "\U0001f44d\U0001f3fd", 1, 2, 8},
		{"ZWJ sequence", "\U0001f468\u200d\U0001f469\u200d\U0001f467", 1, 5, 18},
		{"flag", "\U0001f1ef\U0001f1f5", 1, 2, 8},
		{"odd number of regional indicators", "\U0001f1ef\U0001f1f5\U0001f1fa", 2, 3, 12},
		{"two flags", "\U0001f1ef\U0001f1f5\U0001f1fa\U0001f1f8", 2, 4, 16},
		// One string on which the three measures all differ.
		{"combining mark and flag", "x\u0301\U0001f1ef\U0001f1f5", 2, 4, 11},
	} {
		s, err := Canonical(tt.s)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if g, n, b := GraphemeCount(s), ScalarCount(s), len(s); g != tt.graphemes || n != tt.scalars || b != tt.bytes {
			t.Errorf("%s: %d graphemes, %d scalar values, %d bytes; want %d, %d, %d", tt.name, g, n, b, tt.graphemes, tt.scalars, tt.bytes)
		}
	}
}
