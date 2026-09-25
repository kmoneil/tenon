package uni

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
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

// TestNormalizationLeavesIllFormedTextAlone checks the one thing nfc does with
// text that is not well-formed UTF-8, which Canonical refuses and every other
// caller has already refused: it returns it as it is. Decomposing would write
// the replacement character over the bytes, and a normalizer that quietly
// rewrote its input would be worse than one that did nothing.
func TestNormalizationLeavesIllFormedTextAlone(t *testing.T) {
	for _, s := range []string{
		"\xff",
		"a\xffb",
		"café\xff",    // a sequence that would otherwise compose
		"\xffé",       // and one with the bad byte first
		"\xed\xa0\x80", // a surrogate half
		"á\xff́́b",    // marks on both sides of it
	} {
		if got := NFC(s); got != s {
			t.Errorf("NFC(%q) = %q, want it unchanged", s, got)
		}
	}
}

// TestReorderIsTheStableSortByClass holds reorder, whichever way it sorts a
// run, to what canonical ordering is: the stable sort of each run of
// non-starters by combining class, which the insertion sort reorder used for
// every run is the plainest statement of. Runs are drawn from marks of several
// classes, the same class repeated among them, both sides of the length at
// which reorder stops sorting by insertion, and starters between runs.
func TestReorderIsTheStableSortByClass(t *testing.T) {
	r := rand.New(rand.NewPCG(1340, 0))
	// Marks of classes 202, 216, 220, 230 and 232, two of 230 so that ties
	// show whether the order within a class is kept, and two starters.
	alphabet := []rune{0x0327, 0x031B, 0x0316, 0x0301, 0x0308, 0x0315, 'a', 0x0915}
	for range conformance.Iterations(t, 2000) {
		n := r.IntN(3 * shortRun)
		rs := make([]rune, n)
		for i := range rs {
			if r.IntN(12) == 0 {
				rs[i] = alphabet[6+r.IntN(2)] // a starter now and then
			} else {
				rs[i] = alphabet[r.IntN(6)]
			}
		}
		want := slices.Clone(rs)
		insertionOrder(want)
		got := slices.Clone(rs)
		reorder(got)
		if !slices.Equal(got, want) {
			t.Fatalf("reorder(%U) = %U, want %U", rs, got, want)
		}
	}
}

// insertionOrder is canonical ordering as an insertion sort over the whole
// text, which is what reorder did before runs were sorted one at a time.
func insertionOrder(rs []rune) {
	for i := 1; i < len(rs); i++ {
		cc := combiningClass(rs[i])
		if cc == 0 {
			continue
		}
		for j := i; j > 0; j-- {
			if prev := combiningClass(rs[j-1]); prev == 0 || prev <= cc {
				break
			}
			rs[j-1], rs[j] = rs[j], rs[j-1]
		}
	}
}

// BenchmarkLongRun measures normalizing a run of marks whose classes
// alternate, so that canonical ordering moves every other one, at a length and
// four times it: the growth from one to the other is the reading.
func BenchmarkLongRun(b *testing.B) {
	for _, n := range []int{10000, 40000} {
		rs := []rune{'a'}
		for i := range n {
			rs = append(rs, []rune{0x0316, 0x0301}[i%2])
		}
		text := string(rs)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				NFC(text)
			}
		})
	}
}
