//go:build !go1.27

package uni

import (
	"math/rand/v2"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// This file runs only below go1.27, where golang.org/x/text still normalizes
// by the Unicode version these tables hold, as nfc_xtext_test.go does.

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
	if norm.Version != UnicodeVersion {
		t.Fatalf("x/text normalizes by Unicode %s, and these tables are for %s", norm.Version, UnicodeVersion)
	}
	continuations := []string{
		"a", "e", "-", "\U00000301", "\U00000323", "\U00000327", "\U0000031B", "\U00000345",
		"\U00000F71", "\U00000B3E", "\U000009BE", "\U00001161", "\U000011A8", "\U00003099",
		"\U00000301\U00000323", "\U00000323\U00000301",
	}
	same, longer := 0, 0
	check := func(s string) {
		t.Helper()
		s = nfc(s)
		got := StablePrefix(s)
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
			if n := nfc(s + next); !strings.HasPrefix(n, got) {
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
			if combiningClass([]rune(p)[0]) != 0 {
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
