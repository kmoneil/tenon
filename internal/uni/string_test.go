package uni

import (
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/kmoneil/tenon/conformance"
)

func TestConformance_ST001_ScalarValues(t *testing.T) {
	conformance.Covers(t, "ST-001")
	// Every finite sequence of Unicode scalar values is a string, and these,
	// already in NFC, are their own canonical forms.
	for _, s := range []string{
		"",
		"\x00",
		"plain ASCII",
		"\uffff\ufdd0\U0010fffe",         // noncharacters are scalar values
		"\ue000\U000f0000",               // private use
		"\U0001f600\U00020000\U000e0001", // characters beyond the BMP
		"\ufffd",                         // the replacement character itself
		"\ud7ff\ue000\U0010ffff",         // either side of the surrogates, and the last scalar value
	} {
		if got, err := Canonical(s); err != nil || got != s {
			t.Errorf("Canonical(%+q) = %+q, %v; want it unchanged", s, got, err)
		}
	}

	// Surrogates and numbers beyond U+10FFFF are not scalar values, whatever
	// bytes spell them.
	for _, s := range []string{"\xed\xa0\x80", "\xed\xbf\xbf", "\xed\xa0\xbd\xed\xb8\x80", "\xf4\x90\x80\x80", "\xf7\xbf\xbf\xbf"} {
		if got, err := Canonical(s); err != ErrInvalidUTF8 || got != "" {
			t.Errorf("Canonical(%+q) = %+q, %v; want ErrInvalidUTF8", s, got, err)
		}
	}
}

func TestConformance_ST002_Normalization(t *testing.T) {
	conformance.Covers(t, "ST-002")
	// The spellings in each group construct one string, whose canonical form
	// is in NFC.
	for _, group := range [][]string{
		{"caf\u00e9", "cafe\u0301"},
		{"\u1ead", "a\u0323\u0302", "a\u0302\u0323", "\u1ea1\u0302"},
		{"\ud55c", "\u1112\u1161\u11ab"},
		{"\u00c5", "\u212b", "A\u030a"},
	} {
		want, err := Canonical(group[0])
		if err != nil || !norm.NFC.IsNormalString(want) {
			t.Fatalf("Canonical(%+q) = %+q, %v; want an NFC string", group[0], want, err)
		}
		for _, s := range group[1:] {
			if got, err := Canonical(s); err != nil || got != want {
				t.Errorf("Canonical(%+q) = %+q, %v; want %+q", s, got, err, want)
			}
		}
		if again, _ := Canonical(want); again != want {
			t.Errorf("Canonical is not idempotent on %+q", want)
		}
	}

	// Strings whose normalized forms differ are different strings, however
	// alike they look: NFC applies no compatibility mappings.
	for _, pair := range [][2]string{
		{"e", "\u00e9"},
		{"\ufb01", "fi"},
		{"\uff21", "A"},
		{"x\u0301", "x"},
	} {
		a, errA := Canonical(pair[0])
		b, errB := Canonical(pair[1])
		if errA != nil || errB != nil || a == b {
			t.Errorf("%+q and %+q have the same canonical form %+q", pair[0], pair[1], a)
		}
	}
}

func TestCanonicalRejectsInvalidUTF8(t *testing.T) {
	for _, tt := range []struct{ name, s string }{
		{"overlong NUL", "\xc0\x80"},
		{"overlong two-byte sequence", "\xc1\xbf"},
		{"overlong three-byte sequence", "\xe0\x80\x80"},
		{"overlong U+07FF", "\xe0\x9f\xbf"},
		{"overlong four-byte sequence", "\xf0\x80\x80\x80"},
		{"overlong U+FFFF", "\xf0\x8f\xbf\xbf"},
		{"lone high surrogate", "\xed\xa0\x80"},
		{"lone low surrogate", "\xed\xbf\xbf"},
		{"surrogate pair in CESU-8", "\xed\xa0\xbd\xed\xb8\x80"},
		{"beyond U+10FFFF", "\xf4\x90\x80\x80"},
		{"lead byte F5", "\xf5\x80\x80\x80"},
		{"byte FF", "\xff"},
		{"byte FE", "\xfe"},
		{"lone continuation byte", "\x80"},
		{"truncated two-byte sequence", "\xc3"},
		{"truncated three-byte sequence", "\xe2\x82"},
		{"truncated four-byte sequence", "\xf0\x9f\x98"},
		{"truncated in the middle", "a\xe2\x82b"},
		{"valid text, then an invalid byte", "caf\xc3\xa9\xff"},
	} {
		if got, err := Canonical(tt.s); err != ErrInvalidUTF8 || got != "" {
			t.Errorf("%s: Canonical(%+q) = %+q, %v; want ErrInvalidUTF8 and no text", tt.name, tt.s, got, err)
		}
	}
}

// TestNormalizationIsLossy documents a consequence of normalizing on
// construction: text not already in NFC, such as the NFD file names that some
// file systems produce, does not survive byte for byte.
func TestNormalizationIsLossy(t *testing.T) {
	nfd := "cafe\u0301.txt"
	got, err := Canonical(nfd)
	if err != nil {
		t.Fatal(err)
	}
	if got == nfd || got != "caf\u00e9.txt" || len(got) != len(nfd)-1 {
		t.Errorf("Canonical(%+q) = %+q; want the composed name, one byte shorter", nfd, got)
	}
}
