package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The Hangul syllables compose and decompose by arithmetic, so they are left
// out of the tables. The names are those of UAX #15.
const (
	sBase, lBase, vBase, tBase      = 0xAC00, 0x1100, 0x1161, 0x11A7
	lCount, vCount, tCount          = 19, 21, 28
	nCount, sCount                  = vCount * tCount, lCount * nCount
	hangulFirst, hangulLastSyllable = sBase, sBase + sCount - 1
)

// pair is two code points, the first of which composes with the second.
type pair struct{ a, b rune }

// normalization is the data that Normalization Form C runs on.
type normalization struct {
	// ccc holds every code point of nonzero canonical combining class.
	ccc map[rune]uint8
	// decomposable holds, in order, every code point with a canonical
	// decomposition but the Hangul syllables, and decomp holds each one's
	// full decomposition.
	decomposable []rune
	decomp       map[rune]string
	// composed holds each primary composite, by the pair that composes to it.
	composed map[pair]rune
	// rewritten holds, in order, the code points that Normalization Form C
	// does not leave as they are on their own.
	rewritten []rune
	// composing holds the code points that end a primary composite, the
	// Hangul vowels and trailing consonants among them.
	composing map[rune]bool
}

// deriveNormalization reads the normalization data from x/text.
func deriveNormalization() (normalization, error) {
	n := normalization{ccc: map[rune]uint8{}, decomp: map[rune]string{}, composed: map[pair]rune{}, composing: map[rune]bool{}}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		s := string(r)
		if c := norm.NFC.PropertiesString(s).CCC(); c != 0 {
			n.ccc[r] = c
		}
		// The quick check of UAX #15. A code point that Normalization Form C
		// rewrites on its own cannot stand in a normalized string: the
		// singletons and the composition exclusions are these.
		if norm.NFC.String(s) != s {
			n.rewritten = append(n.rewritten, r)
		}
		if r >= hangulFirst && r <= hangulLastSyllable {
			continue
		}
		if d := norm.NFD.String(s); d != s {
			n.decomp[r] = d
			n.decomposable = append(n.decomposable, r)
		}
	}

	// A primary composite is a code point whose canonical decomposition has
	// more than one rune and which composition produces; a composition
	// exclusion does not, and neither does a singleton. Its one-step
	// decomposition is what all but the last rune compose to, with the last
	// rune, so the pair is read off the full decomposition by looking the head
	// up among the composites. Only what composition produces is indexed: a
	// singleton such as U+2126 OHM SIGN shares its decomposition with the code
	// point it decomposes to, which is the one a head of one rune means, and
	// an excluded code point such as U+1F71 shares a decomposition with the
	// one that composition does produce, U+03AC.
	byDecomposition := map[string]rune{}
	for _, r := range n.decomposable {
		d := n.decomp[r]
		if len([]rune(d)) < 2 || norm.NFC.String(d) != string(r) {
			continue
		}
		if other, dup := byDecomposition[d]; dup {
			return normalization{}, fmt.Errorf("%04X and %04X share a canonical decomposition", other, r)
		}
		byDecomposition[d] = r
	}
	for _, c := range n.decomposable {
		rs := []rune(n.decomp[c])
		if len(rs) < 2 || norm.NFC.String(n.decomp[c]) != string(c) {
			continue
		}
		head := string(rs[:len(rs)-1])
		a, ok := byDecomposition[head]
		if !ok {
			if hr := []rune(head); len(hr) == 1 {
				a, ok = hr[0], true
			}
		}
		if !ok {
			return normalization{}, fmt.Errorf("the decomposition of %04X begins with a sequence that is no code point", c)
		}
		n.composed[pair{a, rs[len(rs)-1]}] = c
		// A code point that ends a primary composite may compose onto what
		// precedes it, so a string holding one has to be normalized to find
		// out.
		n.composing[rs[len(rs)-1]] = true
	}
	// The Hangul vowels and trailing consonants compose onto what precedes
	// them by arithmetic, so they are not in the table of pairs.
	for r := rune(vBase); r < vBase+vCount; r++ {
		n.composing[r] = true
	}
	for r := rune(tBase + 1); r < tBase+tCount; r++ {
		n.composing[r] = true
	}
	return n, nil
}

// deriveCategories returns the ranges of code points whose general category
// is a letter, mark, number, punctuation or symbol, which is what the display
// form leaves as it is, from Go's unicode package. Each range is a first and
// a last code point, and the ranges are in order and do not touch.
func deriveCategories() [][2]rune {
	var ranges [][2]rune
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !inDisplayCategories(r) {
			continue
		}
		last := r
		for last+1 <= unicode.MaxRune && inDisplayCategories(last+1) {
			last++
		}
		ranges = append(ranges, [2]rune{r, last})
		r = last
	}
	return ranges
}

// inDisplayCategories reports whether the general category of r is a letter,
// mark, number, punctuation or symbol, which is what the display form leaves
// as it is.
func inDisplayCategories(r rune) bool {
	return unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S)
}

// ucd is the directory of Unicode's own files, which the segmentation data
// is read from and this module's tests hold internal/uni to.
const ucd = "ucd"

// graphemeValues names each Grapheme_Cluster_Break value that
// GraphemeBreakProperty.txt gives by the constant internal/uni has for it.
// Other, the value of every code point the file does not list, has none in
// the table.
var graphemeValues = map[string]string{
	"CR": "gbCR", "LF": "gbLF", "Control": "gbControl", "Extend": "gbExtend", "ZWJ": "gbZWJ",
	"Regional_Indicator": "gbRegionalIndicator", "Prepend": "gbPrepend", "SpacingMark": "gbSpacingMark",
	"L": "gbL", "V": "gbV", "T": "gbT", "LV": "gbLV", "LVT": "gbLVT",
}

// pictographic is the constant internal/uni has for a code point that is
// Extended_Pictographic, whose Grapheme_Cluster_Break value is Other.
const pictographic = "gbExtendedPictographic"

// segmentation is the data that extended grapheme clusters are found by:
// the value of every code point whose Grapheme_Cluster_Break is not Other, or
// which is Extended_Pictographic, by the constant internal/uni has for it.
type segmentation map[rune]string

// deriveSegmentation reads the segmentation data from Unicode's own files in
// dir: GraphemeBreakProperty.txt and emoji-data.txt. Neither x/text nor Go's
// unicode package carries it.
func deriveSegmentation(dir string) (segmentation, error) {
	s := segmentation{}
	breaks, err := readUCD(dir, "GraphemeBreakProperty.txt", "# GraphemeBreakProperty-"+version+".txt")
	if err != nil {
		return nil, err
	}
	err = eachRange(breaks, func(first, last rune, value string) error {
		c, ok := graphemeValues[value]
		if !ok {
			return fmt.Errorf("it gives %04X the value %s, which internal/uni has no constant for", first, value)
		}
		for r := first; r <= last; r++ {
			if s[r] != "" {
				return fmt.Errorf("it gives %04X a second value, %s", r, value)
			}
			s[r] = c
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GraphemeBreakProperty.txt: %w", err)
	}
	// Emoji versions follow the Unicode version they are for, as 15.0 does
	// 15.0.0, since Emoji 11.0.
	emoji, err := readUCD(dir, "emoji-data.txt", "# Used with Emoji Version "+strings.TrimSuffix(version, ".0")+" and subsequent minor revisions (if any)")
	if err != nil {
		return nil, err
	}
	err = eachRange(emoji, func(first, last rune, property string) error {
		if property != "Extended_Pictographic" {
			return nil
		}
		for r := first; r <= last; r++ {
			if s[r] != "" {
				return fmt.Errorf("%04X is Extended_Pictographic, and its Grapheme_Cluster_Break is %s rather than Other", r, s[r])
			}
			s[r] = pictographic
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("emoji-data.txt: %w", err)
	}
	return s, nil
}

// readUCD returns the file name in dir, refusing it unless one of its lines
// is header, which says the version it is for.
func readUCD(dir, name, header string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if strings.TrimRight(line, "\n") == header {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("%s is not for Unicode %s: no line of it reads %q", name, version, header)
}

// eachRange calls f with each line of the data of a file in the Unicode
// Character Database's format: a code point or a range of them, and the value
// the line gives them. Comments and blank lines are skipped.
func eachRange(data string, f func(first, last rune, value string) error) error {
	for i, line := range strings.Split(data, "\n") {
		if j := strings.IndexByte(line, '#'); j >= 0 {
			line = line[:j]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		points, value, ok := strings.Cut(line, ";")
		if !ok {
			return fmt.Errorf("line %d has no value", i+1)
		}
		lo, hi, isRange := strings.Cut(strings.TrimSpace(points), "..")
		if !isRange {
			hi = lo
		}
		// Twenty-one bits hold every code point, and so does a rune.
		first, errFirst := strconv.ParseUint(lo, 16, 21)
		last, errLast := strconv.ParseUint(hi, 16, 21)
		if errFirst != nil || errLast != nil || first > last || last > unicode.MaxRune {
			return fmt.Errorf("line %d gives no range of code points: %q", i+1, points)
		}
		if err := f(rune(first), rune(last), strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
	}
	return nil
}
