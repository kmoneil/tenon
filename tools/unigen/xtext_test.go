//go:build !go1.27

package main

import (
	"bytes"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/uni"
)

// This file runs only below go1.27, where golang.org/x/text still normalizes
// by the Unicode version internal/uni's tables hold. There it is a second,
// independent implementation of the same algorithm over the same data, and
// holding one to the other is what keeps the generated tables honest. A go1.27
// build compiles none of it, x/text having moved on; Unicode's own tests, in
// oracle_test.go, run on every toolchain.

var (
	derivedOnce sync.Once
	derivedData normalization
	derivedErr  error
)

// derived returns the normalization data that unigen derives from x/text,
// which TestTablesAreGenerated holds internal/uni's tables to.
func derived(t testing.TB) normalization {
	t.Helper()
	derivedOnce.Do(func() { derivedData, derivedErr = deriveNormalization() })
	if derivedErr != nil {
		t.Fatal(derivedErr)
	}
	return derivedData
}

// TestTablesAreGenerated holds internal/uni's tables.go to what unigen
// generates from x/text and Go's unicode package, so that the committed tables
// are that data and no edit of it.
func TestTablesAreGenerated(t *testing.T) {
	if err := checkVersions(); err != nil {
		t.Fatal(err)
	}
	n := derived(t)
	if err := n.check(); err != nil {
		t.Fatal(err)
	}
	want, err := renderTables(n, deriveCategories())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../internal/uni/tables.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("internal/uni/tables.go is not what unigen generates; run unigen")
	}
}

// TestConformance_ST002_NormalizationAgreesWithXText holds internal/uni's
// Normalization Form C to x/text's, over every code point, over every code
// point beside a combining mark, and over generated sequences. The one
// difference is deliberate: x/text applies the Stream-Safe Text Process, and
// internal/uni does not.
func TestConformance_ST002_NormalizationAgreesWithXText(t *testing.T) {
	conformance.Covers(t, "ST-002", "ST-003")
	if norm.Version != uni.UnicodeVersion {
		t.Fatalf("x/text normalizes by Unicode %s, and internal/uni's tables are for %s", norm.Version, uni.UnicodeVersion)
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		s := string(r)
		if got, want := uni.NFC(s), norm.NFC.String(s); got != want {
			t.Fatalf("NFC(%U) = %q, and x/text gives %q", r, got, want)
		}
	}

	// Every code point that normalization has anything to say about, followed
	// by each of the marks that end a primary composite: that pair is where
	// composition and blocking are decided. A starter that decomposes to
	// itself, has no combining class and is in neither table is in none of
	// this, so a sample of those stands for the rest.
	n := derived(t)
	composing := slices.Sorted(maps.Keys(n.composing))
	for _, r := range actionable(n) {
		for _, m := range composing {
			s := string(r) + string(m)
			if got, want := uni.NFC(s), norm.NFC.String(s); got != want {
				t.Fatalf("NFC(%U %U) = %q, and x/text gives %q", r, m, got, want)
			}
		}
	}

	// Sequences drawn from the code points that normalization has something to
	// say about, which is where reordering meets composition. The runs are
	// kept under thirty non-starters, x/text inserting U+034F past that.
	interesting := []rune{'a', 'e', 's', 'd', 0x1E0D, 0x1E63, 0x1E69, 0x0300, 0x0301,
		0x0307, 0x0316, 0x0323, 0x0327, 0x0334, 0x034F, 0x05B0, 0x0F71, 0x0F72,
		0x1100, 0x1161, 0x11A8, 0xAC00, 0xAC01, 0xD7A3, 0x2126, 0x1F71, 0x03AC, 0x212B}
	interesting = append(interesting, n.rewritten[0], n.rewritten[len(n.rewritten)/2], n.rewritten[len(n.rewritten)-1])
	rng := rand.New(rand.NewPCG(1, 2))
	for range conformance.Iterations(t, 200000) {
		var b strings.Builder
		for n := rng.IntN(6) + 1; n > 0; n-- {
			b.WriteRune(interesting[rng.IntN(len(interesting))])
		}
		s := b.String()
		if got, want := uni.NFC(s), norm.NFC.String(s); got != want {
			t.Fatalf("NFC(%q) = %q, and x/text gives %q", s, got, want)
		}
	}
}

// actionable returns the code points that normalization can act on, with a
// sample of the ones it cannot.
func actionable(n normalization) []rune {
	seen := map[rune]bool{}
	var out []rune
	add := func(r rune) {
		if !seen[r] {
			seen[r], out = true, append(out, r)
		}
	}
	for _, r := range slices.Sorted(maps.Keys(n.ccc)) {
		add(r)
	}
	for _, r := range n.decomposable {
		add(r)
	}
	for _, r := range n.rewritten {
		add(r)
	}
	for _, r := range slices.Sorted(maps.Keys(n.composing)) {
		add(r)
	}
	for r := rune(sBase); r < sBase+sCount; r += 7 {
		add(r)
	}
	for r := rune(0); r <= 0x10FFFF; r += 521 {
		if r < 0xD800 || r > 0xDFFF {
			add(r)
		}
	}
	return out
}

// TestStreamSafeIsNotApplied states the one place internal/uni and x/text
// part: x/text inserts U+034F after thirty non-starters, following the
// Stream-Safe Text Process, and plain UAX #15 normalization does not.
func TestStreamSafeIsNotApplied(t *testing.T) {
	s := "a" + strings.Repeat("\U00000301", 31)
	if got := uni.NFC(s); strings.ContainsRune(got, 0x034F) {
		t.Errorf("NFC of a run of 31 non-starters holds U+034F: %q", got)
	}
	if !strings.ContainsRune(norm.NFC.String(s), 0x034F) {
		t.Skip("x/text no longer inserts U+034F, so there is nothing to differ about")
	}
	if uni.NFC(s) == norm.NFC.String(s) {
		t.Error("x/text inserts U+034F and internal/uni agrees with it")
	}
}

// TestStablePrefixAgreesWithXText holds StablePrefix to x/text's
// norm.NFC.LastBoundary, the boundary it replaces, over every code point
// alone, after a starter and before marks, and over generated text whose runs
// of non-starters are at most thirty long, since past thirty x/text assumes
// Stream-Safe text, which tenon does not produce, and stops counting.
//
// StablePrefix keeps at least what x/text keeps, and exactly that nearly
// everywhere. Where it keeps more, x/text was cautious beyond NFC: it counts a
// code point's non-starters by its compatibility decomposition, which NFKC
// shares, so U+00AF MACRON, a space and a combining macron under NFKD, is to
// it text that may change; and it takes a starter that composes only
// backward, U+09BE after "a", as extending the segment before it. NFC changes
// neither. Each such difference is held to soundness instead: the text
// followed by a range of continuations still begins with what was kept.
func TestStablePrefixAgreesWithXText(t *testing.T) {
	if norm.Version != uni.UnicodeVersion {
		t.Fatalf("x/text normalizes by Unicode %s, and internal/uni's tables are for %s", norm.Version, uni.UnicodeVersion)
	}
	continuations := []string{
		"a", "e", "-", "\U00000301", "\U00000323", "\U00000327", "\U0000031B", "\U00000345",
		"\U00000F71", "\U00000B3E", "\U000009BE", "\U00001161", "\U000011A8", "\U00003099",
		"\U00000301\U00000323", "\U00000323\U00000301",
	}
	same, longer := 0, 0
	check := func(s string) {
		t.Helper()
		s = uni.NFC(s)
		got := uni.StablePrefix(s)
		want := ""
		if i := norm.NFC.LastBoundary([]byte(s)); i >= 0 {
			want = s[:i]
		}
		if got == want {
			same++
			return
		}
		if !strings.HasPrefix(got, want) {
			t.Fatalf("StablePrefix(%+q) = %+q, shorter than x/text's %+q or apart from it", s, got, want)
		}
		longer++
		for _, next := range continuations {
			if n := uni.NFC(s + next); !strings.HasPrefix(n, got) {
				t.Fatalf("StablePrefix(%+q) = %+q, but followed by %+q it normalizes to %+q", s, got, next, n)
			}
		}
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		check(string(r))
		check("a" + string(r))
		check(string(r) + "\U00000301")
		check("x" + string(r) + "\U00000323\U00000301")
	}
	// Starters that compose, decompose, or neither; marks of several classes;
	// Hangul jamo and syllables; code points x/text judges by compatibility.
	pieces := []string{
		"a", "e", "q", "-", "1", " ", "\U000000E9", "\U00001E0D", "\U00000B47", "\U00000B3E", "\U000009C7", "\U000009BE",
		"\U00001100", "\U00001161", "\U000011A8", "\U0000AC00", "\U0000AC01", "\U000000AF", "\U0000FF9E",
		"\U00000301", "\U00000323", "\U00000327", "\U0000031B", "\U00000345", "\U00000F71", "\U00000F73",
	}
	r := rand.New(rand.NewPCG(20260927, 1))
	for range 200000 {
		var b strings.Builder
		marks := 0
		for range 1 + r.IntN(8) {
			p := pieces[r.IntN(len(pieces))]
			if norm.NFC.PropertiesString(p).CCC() != 0 {
				if marks++; marks > 30 {
					continue
				}
			} else {
				marks = 0
			}
			b.WriteString(p)
		}
		check(b.String())
	}
	t.Logf("%d texts kept as x/text keeps them, %d kept longer and held to soundness", same, longer)
	if longer == 0 || same < 10*longer {
		t.Errorf("%d texts agreed and %d did not: the differences should be few, and present", same, longer)
	}
}

// FuzzStringAgainstXText holds a String's content to x/text: the content is
// in Normalization Form C, and the string equals itself written in NFD, and
// written in NFKC exactly where that normalizes to the same content. tenon
// holds the data itself, so x/text is an oracle here and not the normalizer
// under test answering its own question. Its seeds are the inputs that fuzzing
// tenon's FuzzString has kept, so each is held to x/text as well.
func FuzzStringAgainstXText(f *testing.F) {
	for _, s := range keptInputs(f, "../../testdata/fuzz/FuzzString") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return // FuzzString holds such input to its error value
		}
		v := tenon.String(s)
		content := v.AsString()
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
	})
}

// inserts reports whether x/text's form adds a combining grapheme joiner to s
// that s did not hold, which is the Stream-Safe Text Process at work.
func inserts(form norm.Form, s string) bool {
	return strings.Count(form.String(s), "\U0000034F") != strings.Count(s, "\U0000034F")
}

// withoutJoiners returns s without any combining grapheme joiner.
func withoutJoiners(s string) string { return strings.ReplaceAll(s, "\U0000034F", "") }

// keptInputs returns the inputs that a fuzz target keeps in dir, each a file
// in the format go test writes: its version line, then one string argument.
func keptInputs(f *testing.F, dir string) []string {
	f.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		f.Fatal(err)
	}
	var inputs []string
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			f.Fatal(err)
		}
		version, arg, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
		quoted, opened := strings.CutPrefix(arg, "string(")
		quoted, closed := strings.CutSuffix(quoted, ")")
		s, err := strconv.Unquote(quoted)
		if version != "go test fuzz v1" || !opened || !closed || err != nil {
			f.Fatalf("%s holds no string input: %q", e.Name(), data)
		}
		inputs = append(inputs, s)
	}
	if len(inputs) < 500 {
		f.Fatalf("%s keeps %d inputs, and FuzzString has kept more than 500", dir, len(inputs))
	}
	return inputs
}
