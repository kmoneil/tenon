//go:build !go1.27

package uni

import (
	"math/rand/v2"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/kmoneil/tenon/internal/conformance"
)

// This file runs only below go1.27, where golang.org/x/text still normalizes
// by the Unicode version these tables hold. There it is a second, independent
// implementation of the same algorithm over the same data, and holding one to
// the other is what keeps the generated tables honest. A go1.27 build compiles
// none of it, x/text having moved on.

// TestConformance_ST002_NormalizationAgreesWithXText holds this package's
// Normalization Form C to x/text's, over every code point, over every code
// point beside a combining mark, and over generated sequences. The one
// difference is deliberate: x/text applies the Stream-Safe Text Process, and
// this does not.
func TestConformance_ST002_NormalizationAgreesWithXText(t *testing.T) {
	conformance.Covers(t, "ST-002", "ST-003")
	if norm.Version != UnicodeVersion {
		t.Fatalf("x/text normalizes by Unicode %s, and these tables are for %s", norm.Version, UnicodeVersion)
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		s := string(r)
		if got, want := nfc(s), norm.NFC.String(s); got != want {
			t.Fatalf("NFC(%U) = %q, and x/text gives %q", r, got, want)
		}
	}

	// Every code point that normalization has anything to say about, followed
	// by each of the marks that end a primary composite: that pair is where
	// composition and blocking are decided. A starter that decomposes to
	// itself, has no combining class and is in neither table is in none of
	// this, so a sample of those stands for the rest.
	for _, r := range actionable() {
		for _, m := range composing {
			s := string(r) + string(m)
			if got, want := nfc(s), norm.NFC.String(s); got != want {
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
	interesting = append(interesting, rewritten[0], rewritten[len(rewritten)/2], rewritten[len(rewritten)-1])
	rng := rand.New(rand.NewPCG(1, 2))
	for range conformance.Iterations(t, 200000) {
		var b strings.Builder
		for n := rng.IntN(6) + 1; n > 0; n-- {
			b.WriteRune(interesting[rng.IntN(len(interesting))])
		}
		s := b.String()
		if got, want := nfc(s), norm.NFC.String(s); got != want {
			t.Fatalf("NFC(%q) = %q, and x/text gives %q", s, got, want)
		}
	}
}

// actionable returns the code points that normalization can act on, with a
// sample of the ones it cannot.
func actionable() []rune {
	seen := map[rune]bool{}
	var out []rune
	add := func(r rune) {
		if !seen[r] {
			seen[r], out = true, append(out, r)
		}
	}
	for _, e := range combiningClasses {
		add(rune(e >> 8))
	}
	for _, r := range decomposable {
		add(r)
	}
	for _, r := range rewritten {
		add(r)
	}
	for _, r := range composing {
		add(r)
	}
	for r := rune(hangulSBase); r < hangulSBase+hangulSCount; r += 7 {
		add(r)
	}
	for r := rune(0); r <= 0x10FFFF; r += 521 {
		if r < 0xD800 || r > 0xDFFF {
			add(r)
		}
	}
	return out
}

// TestStreamSafeIsNotApplied states the one place this package and x/text
// part: x/text inserts U+034F after thirty non-starters, following the
// Stream-Safe Text Process, and plain UAX #15 normalization does not.
func TestStreamSafeIsNotApplied(t *testing.T) {
	s := "a" + strings.Repeat("́", 31)
	if got := nfc(s); strings.ContainsRune(got, 0x034F) {
		t.Errorf("NFC of a run of 31 non-starters holds U+034F: %q", got)
	}
	if !strings.ContainsRune(norm.NFC.String(s), 0x034F) {
		t.Skip("x/text no longer inserts U+034F, so there is nothing to differ about")
	}
	if nfc(s) == norm.NFC.String(s) {
		t.Error("x/text inserts U+034F and this package agrees with it")
	}
}
