package uni

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

// TestUnicodeVersionPinned fails when the normalization data moves to another
// Unicode version, as it can when golang.org/x/text is upgraded or a newer Go
// toolchain selects newer tables. Such a move changes which strings are equal,
// so it must be made deliberately, by updating UnicodeVersion.
func TestUnicodeVersionPinned(t *testing.T) {
	if norm.Version != UnicodeVersion {
		t.Fatalf("normalization uses Unicode %s, but UnicodeVersion is %s", norm.Version, UnicodeVersion)
	}
}

func TestNFC(t *testing.T) {
	tests := []struct {
		name  string
		forms []string // spellings that must all normalize to nfc
		nfc   string
	}{
		{"empty", []string{""}, ""},
		{"ASCII", []string{"attribute_name"}, "attribute_name"},
		{"combining acute", []string{"é", "é"}, "é"},
		{"two combining marks in either order", []string{"ậ", "ậ", "ậ", "ậ"}, "ậ"},
		{"Hangul syllable from jamo", []string{"한", "한"}, "한"},
		{"singleton decomposition", []string{"Å", "Å", "Å"}, "Å"},
		{"composition exclusion stays decomposed", []string{"क़", "क़"}, "क़"},
		{"mixed text", []string{"Café 한", "Café 한"}, "Café 한"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, in := range tt.forms {
				got := NFC(in)
				if got != tt.nfc {
					t.Errorf("NFC(%+q) = %+q, want %+q", in, got, tt.nfc)
				}
				if again := NFC(got); again != got {
					t.Errorf("NFC is not idempotent: NFC(%+q) = %+q", got, again)
				}
			}
		})
	}
}
