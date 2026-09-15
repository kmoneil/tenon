package uni

import "testing"

func TestNFC(t *testing.T) {
	tests := []struct {
		name  string
		forms []string // spellings that must all normalize to nfc
		nfc   string
	}{
		{"empty", []string{""}, ""},
		{"ASCII", []string{"attribute_name"}, "attribute_name"},
		{"combining acute", []string{"\u00e9", "e\u0301"}, "\u00e9"},
		{"two combining marks in either order", []string{"\u1ead", "a\u0323\u0302", "a\u0302\u0323", "\u1ea1\u0302"}, "\u1ead"},
		{"Hangul syllable from jamo", []string{"\ud55c", "\u1112\u1161\u11ab"}, "\ud55c"},
		{"singleton decomposition", []string{"\u212b", "\u00c5", "A\u030a"}, "\u00c5"},
		{"composition exclusion stays decomposed", []string{"\u0958", "\u0915\u093c"}, "\u0915\u093c"},
		{"mixed text", []string{"Cafe\u0301 \u1112\u1161\u11ab", "Caf\u00e9 \ud55c"}, "Caf\u00e9 \ud55c"},
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
