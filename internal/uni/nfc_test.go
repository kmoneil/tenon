package uni

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// TestConformance_ST002_NormalizationFixtures holds Normalization Form C to
// fixtures whose expected forms came from outside this module, and outside
// golang.org/x/text: Python's unicodedata, which is another implementation
// running on another copy of the Unicode data. The cross-check against x/text
// lives beside this and cannot run on a toolchain of go1.27 or later, x/text
// having moved to another Unicode version there; this runs everywhere, and is
// what says the version has not moved with the toolchain.
//
// The fixture holds the twenty sequences that compose in Unicode 17.0.0 and
// not in 15.0.0, which is exactly what a build that followed its toolchain
// would get wrong.
func TestConformance_ST002_NormalizationFixtures(t *testing.T) {
	conformance.Covers(t, "ST-002", "ST-003")
	data, err := os.ReadFile("testdata/nfc.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Oracle string `json:"oracle"`
		Cases  []struct {
			In   []rune `json:"in"`
			Out  []rune `json:"out"`
			Note string `json:"note"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) < 400 {
		t.Fatalf("only %d fixtures, which is too few to say much", len(doc.Cases))
	}
	notes := 0
	for _, c := range doc.Cases {
		if c.Note != "" {
			notes++
		}
		in, want := string(c.In), string(c.Out)
		if got := NFC(in); got != want {
			t.Errorf("NFC(%U) = %U, want %U (%s, oracle %s)", c.In, []rune(got), c.Out, c.Note, doc.Oracle)
		}
	}
	if notes != 20 {
		t.Errorf("%d fixtures name a divergence between Unicode versions, want the 20 sequences 17.0.0 composes", notes)
	}
}

// TestConformance_ST002_PlainNormalizationOfLongRuns checks that a run of more
// than thirty non-starters is normalized as UAX #15 sets out and not by the
// Stream-Safe Text Process, which would insert U+034F. A string that gained a
// code point on the way in is not the string that was given, and [ST-002]
// makes two strings equal exactly when their normalized forms are identical.
func TestConformance_ST002_PlainNormalizationOfLongRuns(t *testing.T) {
	conformance.Covers(t, "ST-002")
	long := "a" + strings.Repeat("́", 31)
	got := NFC(long)
	if strings.ContainsRune(got, 0x034F) {
		t.Errorf("a run of 31 non-starters normalizes to %q, which holds U+034F", got)
	}
	// The first acute composes with the letter, and the other thirty stay as
	// they are: nothing is inserted and nothing is dropped.
	if want := "á" + strings.Repeat("́", 30); got != want {
		t.Errorf("a run of 31 non-starters normalizes to %U, want %U", []rune(got), []rune(want))
	}
	// The stream-safe form is a different string, and stays one.
	safe := "a" + strings.Repeat("́", 30) + "͏́"
	if NFC(safe) == got {
		t.Error("a run of 31 non-starters and its stream-safe form normalize alike")
	}
}
