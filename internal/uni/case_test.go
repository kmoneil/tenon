package uni

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
)

func TestCaseTablesVersion(t *testing.T) {
	if caseTablesVersion != UnicodeVersion {
		t.Fatalf("case_tables.go holds Unicode %s, and the package states %s", caseTablesVersion, UnicodeVersion)
	}
}

func TestConformance_LS002_CaseConversion(t *testing.T) {
	conformance.Covers(t, "LS-002")
	for _, tt := range []struct{ in, upper, lower string }{
		{"Hello", "HELLO", "hello"},
		// The full mappings of SpecialCasing.txt: sharp s, the fi ligature,
		// n preceded by an apostrophe, iota with dialytika and tonos.
		{"\U000000DF", "SS", "\U000000DF"},
		{"\U0000FB01", "FI", "\U0000FB01"},
		{"\U00000149", "\U000002BCN", "\U00000149"},
		{"\U00000390", "\U00000399\U00000308\U00000301", "\U00000390"},
		// Capital I with dot above lowercases to i and the dot.
		{"\U00000130", "\U00000130", "i\U00000307"},
		// No language's tailoring: I is i, and i is I.
		{"I", "I", "i"},
		{"i", "I", "i"},
		// Ypogegrammeni, a mark, uppercases to a letter.
		{"x\U00000345", "X\U00000399", "x\U00000345"},
		// j with caron has no precomposed capital.
		{"\U000001F0", "J\U0000030C", "\U000001F0"},
	} {
		if got := Upper(tt.in); got != tt.upper {
			t.Errorf("Upper(%+q) = %+q, want %+q", tt.in, got, tt.upper)
		}
		if got := Lower(tt.in); got != tt.lower {
			t.Errorf("Lower(%+q) = %+q, want %+q", tt.in, got, tt.lower)
		}
	}
	// Final_Sigma: a capital sigma ending a word is the final sigma.
	sigma, alpha := "\U000003A3", "\U00000391"
	for _, tt := range []struct{ in, lower string }{
		{"\U0000039F\U00000394\U0000039F" + sigma, "\U000003BF\U000003B4\U000003BF\U000003C2"},
		{sigma, "\U000003C3"},
		{alpha + sigma + ".", "\U000003B1\U000003C2."},
		{alpha + sigma + alpha, "\U000003B1\U000003C3\U000003B1"},
		{alpha + "." + sigma, "\U000003B1.\U000003C2"},
		{alpha + sigma + "'" + alpha, "\U000003B1\U000003C3'\U000003B1"},
		// Ypogegrammeni is cased and case-ignorable, and is passed over.
		{alpha + sigma + "\U00000345", "\U000003B1\U000003C2\U00000345"},
		{"\U00000345" + sigma, "\U00000345\U000003C3"},
	} {
		if got := Lower(tt.in); got != tt.lower {
			t.Errorf("Lower(%+q) = %+q, want %+q", tt.in, got, tt.lower)
		}
	}
	for _, tt := range []struct {
		r     rune
		title string
	}{
		{'a', "A"}, {'A', "A"}, {'1', "1"},
		{0xDF, "Ss"}, {0xFB01, "Fi"},
		// The dz digraphs titlecase to their own form.
		{0x01C6, "\U000001C5"}, {0x01C4, "\U000001C5"},
	} {
		if got := Title(tt.r); got != tt.title {
			t.Errorf("Title(%U) = %+q, want %+q", tt.r, got, tt.title)
		}
	}
}

func TestConformance_LS003_WhiteSpace(t *testing.T) {
	conformance.Covers(t, "LS-003")
	want := []rune{0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x20, 0x85, 0xA0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200A,
		0x2028, 0x2029, 0x202F, 0x205F, 0x3000}
	var got []rune
	for r := rune(0); r <= 0x10FFFF; r++ {
		if IsWhiteSpace(r) {
			got = append(got, r)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("the White_Space code points are %U, want the 25 of PropList.txt, %U", got, want)
	}
	// Not White_Space: the byte order mark, the zero width space, the
	// Mongolian vowel separator.
	for _, r := range []rune{0xFEFF, 0x200B, 0x180E} {
		if IsWhiteSpace(r) {
			t.Errorf("IsWhiteSpace(%U) = true", r)
		}
	}
}

func TestConformance_LS001_CutPositions(t *testing.T) {
	conformance.Covers(t, "LS-001")
	cutsOf := func(s string) []int {
		c := CutsOf(s)
		var out []int
		for i := -1; i <= len(s)+1; i++ {
			if c.At(i) {
				out = append(out, i)
			}
		}
		return out
	}
	for _, tt := range []struct {
		in       string
		cuts     []int
		clusters []string
	}{
		{"", []int{0}, nil},
		// Every position of ASCII text, between the CR and the LF of a CR LF
		// too, which one cluster holds.
		{"ab", []int{0, 1, 2}, []string{"a", "b"}},
		{"a\r\nb", []int{0, 1, 2, 3, 4}, []string{"a", "\r\n", "b"}},
		// No cut inside a cluster: e and a combining acute; two flags, each
		// two regional indicators; a thumb and its skin tone.
		{"e\U00000301x", []int{0, 3, 4}, []string{"e\U00000301", "x"}},
		{"\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7", []int{0, 8, 16}, []string{"\U0001F1FA\U0001F1F8", "\U0001F1EC\U0001F1E7"}},
		{"\U0001F44D\U0001F3FDx", []int{0, 8, 9}, []string{"\U0001F44D\U0001F3FD", "x"}},
	} {
		if got := cutsOf(tt.in); !slices.Equal(got, tt.cuts) {
			t.Errorf("the cut positions of %+q are %v, want %v", tt.in, got, tt.cuts)
		}
		if got := slices.Collect(Clusters(tt.in)); !slices.Equal(got, tt.clusters) {
			t.Errorf("the clusters of %+q are %+q, want %+q", tt.in, got, tt.clusters)
		}
	}
	// A loop stopping early stops the iteration.
	for c := range Clusters("abc") {
		if c != "a" {
			t.Errorf("the first cluster of abc is %q", c)
		}
		break
	}
}
