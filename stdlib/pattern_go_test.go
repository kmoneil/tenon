//go:build !go1.27

package stdlib

import (
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// This file runs only below go1.27, where Go's regexp reads the Unicode
// version tenon's tables hold: there it is the oracle the explicit
// patterns are held to.

func TestPatternsAgreeWithGo(t *testing.T) {
	if unicode.Version != "15.0.0" {
		t.Fatalf("Go's unicode package is Unicode %s", unicode.Version)
	}
	pieces := []string{"a", "b", "k", "s", `\x{212A}`, `\x{17F}`, `\x{3A3}`, `\x{1E9E}`, "(?i)", "(?-i)", ".", `\pL`, `\p{Greek}`, `[a-z]`, `[^k]`, "(", ")", "|", "*", "+", "?", "{2}", `\w`, `(?P<n>x)`, "^", "$"}
	texts := []string{"", "a", "K", "\U0000212A", "s\U0000017F", "\U000003A3\U000003C2", "\U000000DF\U00001E9E", "abc", "kKk", "\U000003B1x", "nx"}
	rng := rand.New(rand.NewPCG(20261005, 7))
	compared := 0
	for range 20000 {
		var b strings.Builder
		for range 1 + rng.IntN(6) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		p := b.String()
		want, werr := regexp.Compile(p)
		got, failure := compilePattern("Regex", 0, p)
		if (werr == nil) != failure.IsZero() {
			t.Fatalf("pattern %q: Go compiles %v, tenon %v", p, werr == nil, failure.IsZero())
		}
		if werr != nil {
			continue
		}
		compared++
		for _, s := range texts {
			if g, w := got.FindAllStringSubmatchIndex(s, -1), want.FindAllStringSubmatchIndex(s, -1); !slices.EqualFunc(g, w, slices.Equal) {
				t.Fatalf("pattern %q on %q: tenon %v, Go %v", p, s, g, w)
			}
			if !slices.Equal(got.SubexpNames(), want.SubexpNames()) {
				t.Fatalf("pattern %q: groups %v, Go %v", p, got.SubexpNames(), want.SubexpNames())
			}
		}
	}
	if compared < 5000 {
		t.Fatalf("only %d patterns compiled", compared)
	}
}
