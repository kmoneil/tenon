//go:build !go1.27

package main

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/uni"
)

// This file runs only below go1.27, where golang.org/x/text and Go's
// unicode package still carry the Unicode version internal/uni's case tables
// hold, generated from Unicode's own files. There each is a second,
// independent source of the same mappings.

// TestConformance_LS002_CaseConversionAgreesWithXText holds Upper, Lower
// and Title to x/text's language-independent case conversion over every
// code point, and Lower's Final_Sigma to it over random Greek text.
func TestConformance_LS002_CaseConversionAgreesWithXText(t *testing.T) {
	conformance.Covers(t, "LS-002")
	if unicode.Version != version {
		t.Fatalf("Go's unicode package is Unicode %s, and these tables are for %s", unicode.Version, version)
	}
	upper, lower, title := cases.Upper(language.Und), cases.Lower(language.Und), cases.Title(language.Und)
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		s := string(r)
		if got, want := uni.Upper(s), upper.String(s); got != want {
			t.Errorf("Upper(%U) = %+q, and x/text gives %+q", r, got, want)
		}
		if got, want := uni.Lower(s), lower.String(s); got != want {
			t.Errorf("Lower(%U) = %+q, and x/text gives %+q", r, got, want)
		}
		if got, want := uni.Title(r), title.String(s); got != want {
			t.Errorf("Title(%U) = %+q, and x/text gives %+q", r, got, want)
		}
	}
	// Final_Sigma looks past case-ignorable code points on either side. A
	// code point both cased and case-ignorable, as U+0345 and U+02B0 are, is
	// left out: x/text takes it for the cased code point before a sigma,
	// where ICU, CPython and Rust pass over it as internal/uni does, which
	// TestConformance_LS002_CaseConversion states.
	alphabet := []rune{0x03A3, 0x03A3, 0x0391, 'a', ' ', '.', '\'', 0x0301, 0x00AD, '1'}
	rng := rand.New(rand.NewPCG(20261005, 1))
	for range 20000 {
		var b strings.Builder
		for range 1 + rng.IntN(6) {
			b.WriteRune(alphabet[rng.IntN(len(alphabet))])
		}
		s := b.String()
		if got, want := uni.Lower(s), lower.String(s); got != want {
			t.Errorf("Lower(%+q) = %+q, and x/text gives %+q", s, got, want)
		}
	}
}

// TestSimpleCaseAgreesWithGo holds the simple case mappings, which Upper,
// Lower and Title apply to every code point SpecialCasing.txt does not
// list, to Go's over every code point.
func TestSimpleCaseAgreesWithGo(t *testing.T) {
	if unicode.Version != version {
		t.Fatalf("Go's unicode package is Unicode %s, and these tables are for %s", unicode.Version, version)
	}
	c, err := deriveCasing(ucd)
	if err != nil {
		t.Fatal(err)
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if _, special := c.special[r]; special || r >= 0xD800 && r <= 0xDFFF || r == 0x03A3 {
			continue
		}
		s := string(r)
		if got, want := uni.Upper(s), string(unicode.ToUpper(r)); got != want {
			t.Errorf("Upper(%U) = %+q, and Go gives %+q", r, got, want)
		}
		if got, want := uni.Lower(s), string(unicode.ToLower(r)); got != want {
			t.Errorf("Lower(%U) = %+q, and Go gives %+q", r, got, want)
		}
		if got, want := uni.Title(r), string(unicode.ToTitle(r)); got != want {
			t.Errorf("Title(%U) = %+q, and Go gives %+q", r, got, want)
		}
	}
}

// TestConformance_LS003_WhiteSpaceAgreesWithGo holds IsWhiteSpace to Go's
// White_Space over every code point.
func TestConformance_LS003_WhiteSpaceAgreesWithGo(t *testing.T) {
	conformance.Covers(t, "LS-003")
	if unicode.Version != version {
		t.Fatalf("Go's unicode package is Unicode %s, and these tables are for %s", unicode.Version, version)
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if got, want := uni.IsWhiteSpace(r), unicode.Is(unicode.White_Space, r); got != want {
			t.Errorf("IsWhiteSpace(%U) = %v, and Go says %v", r, got, want)
		}
	}
}
