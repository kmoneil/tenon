package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// unicodeDataSHA256 is the digest of UnicodeData.txt of the version these
// tables are for. The file names no version of its own, so its digest is
// what tells it apart from another version's.
const unicodeDataSHA256 = "806e9aed65037197f1ec85e12be6e8cd870fc5608b4de0fffd990f689f376a73"

// sha256Hex returns the SHA-256 of data in lowercase hexadecimal.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// caseMapping is a code point's simple case mappings, each a code point.
type caseMapping struct{ upper, lower, title rune }

// specialCasing is a code point's full case mappings, from SpecialCasing.txt,
// each a sequence of code points.
type specialCasing struct{ lower, title, upper string }

// casing is the data that case conversion and trimming run on.
type casing struct {
	// simple holds the simple case mappings of UnicodeData.txt, of every code
	// point that has one other than itself.
	simple map[rune]caseMapping
	// special holds the unconditional entries of SpecialCasing.txt.
	special map[rune]specialCasing
	// cased, ignorable and white hold the code points with the Cased,
	// Case_Ignorable and White_Space properties.
	cased, ignorable, white map[rune]bool
}

// finalSigma is the only entry of SpecialCasing.txt whose condition is not
// a language's: Σ lowercases to ς at the end of a word. internal/uni applies
// it by name, so the file must say no more and no less.
const finalSigma = "03A3; 03C2; 03A3; 03A3; Final_Sigma;"

// deriveCasing reads the case data from Unicode's own files in dir:
// UnicodeData.txt, SpecialCasing.txt, DerivedCoreProperties.txt and
// PropList.txt.
func deriveCasing(dir string) (casing, error) {
	c := casing{simple: map[rune]caseMapping{}, special: map[rune]specialCasing{}, cased: map[rune]bool{}, ignorable: map[rune]bool{}, white: map[rune]bool{}}
	data, err := os.ReadFile(filepath.Join(dir, "UnicodeData.txt"))
	if err != nil {
		return casing{}, err
	}
	if err := checkUnicodeData(data); err != nil {
		return casing{}, err
	}
	if err := c.readUnicodeData(string(data)); err != nil {
		return casing{}, fmt.Errorf("UnicodeData.txt: %w", err)
	}
	special, err := readUCD(dir, "SpecialCasing.txt", "# SpecialCasing-"+version+".txt")
	if err != nil {
		return casing{}, err
	}
	if err := c.readSpecialCasing(special); err != nil {
		return casing{}, fmt.Errorf("SpecialCasing.txt: %w", err)
	}
	for _, p := range []struct {
		file, property string
		into           map[rune]bool
	}{
		{"DerivedCoreProperties.txt", "Cased", c.cased},
		{"DerivedCoreProperties.txt", "Case_Ignorable", c.ignorable},
		{"PropList.txt", "White_Space", c.white},
	} {
		data, err := readUCD(dir, p.file, "# "+strings.TrimSuffix(p.file, ".txt")+"-"+version+".txt")
		if err != nil {
			return casing{}, err
		}
		err = eachRange(data, func(first, last rune, value string) error {
			if value != p.property {
				return nil
			}
			for r := first; r <= last; r++ {
				p.into[r] = true
			}
			return nil
		})
		if err != nil {
			return casing{}, fmt.Errorf("%s: %w", p.file, err)
		}
		if len(p.into) == 0 {
			return casing{}, fmt.Errorf("%s gives no code point %s", p.file, p.property)
		}
	}
	return c, nil
}

// readUnicodeData reads the simple case mappings, fields 12 to 14 of each
// line. An empty titlecase mapping is the uppercase mapping, and an empty
// uppercase or lowercase mapping is the code point itself.
func (c casing) readUnicodeData(data string) error {
	for i, line := range strings.Split(strings.TrimSuffix(data, "\n"), "\n") {
		fields := strings.Split(line, ";")
		if len(fields) != 15 {
			return fmt.Errorf("line %d has %d fields, not 15", i+1, len(fields))
		}
		r, err := codePoint(fields[0])
		if err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		m := caseMapping{r, r, r}
		for j, into := range []*rune{&m.upper, &m.lower, &m.title} {
			if fields[12+j] == "" {
				continue
			}
			if *into, err = codePoint(fields[12+j]); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
		}
		if fields[14] == "" {
			m.title = m.upper
		}
		if m != (caseMapping{r, r, r}) {
			c.simple[r] = m
		}
	}
	return nil
}

// readSpecialCasing reads the unconditional entries of SpecialCasing.txt,
// and refuses a condition that is not a language's but Final_Sigma's.
func (c casing) readSpecialCasing(data string) error {
	sigma := false
	for i, line := range strings.Split(data, "\n") {
		if j := strings.IndexByte(line, '#'); j >= 0 {
			line = line[:j]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, ";")
		for j := range fields {
			fields[j] = strings.TrimSpace(fields[j])
		}
		// A line ends in a semicolon, so its last field is empty.
		if n := len(fields); n != 5 && n != 6 || fields[n-1] != "" {
			return fmt.Errorf("line %d has %d fields", i+1, len(fields))
		}
		if len(fields) == 6 {
			if isLanguage(strings.Fields(fields[4])[0]) {
				continue
			}
			if strings.Join(fields[:5], "; ")+";" != finalSigma {
				return fmt.Errorf("line %d has the condition %s, which is no language's and not Final_Sigma's for 03A3", i+1, fields[4])
			}
			sigma = true
			continue
		}
		r, err := codePoint(fields[0])
		if err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		var s specialCasing
		for j, into := range []*string{&s.lower, &s.title, &s.upper} {
			if *into, err = codeSequence(fields[1+j]); err != nil {
				return fmt.Errorf("line %d: %w", i+1, err)
			}
		}
		if _, dup := c.special[r]; dup {
			return fmt.Errorf("line %d gives %04X a second entry", i+1, r)
		}
		c.special[r] = s
	}
	if !sigma {
		return fmt.Errorf("it holds no entry %q", finalSigma)
	}
	return nil
}

// isLanguage reports whether a condition of SpecialCasing.txt names a
// language, as lt, tr and az do: a tailoring internal/uni does not apply.
func isLanguage(condition string) bool {
	return len(condition) == 2 && strings.ToLower(condition) == condition
}

// codePoint reads a code point written in hex.
func codePoint(s string) (rune, error) {
	n, err := strconv.ParseUint(s, 16, 21)
	if err != nil || n > unicode.MaxRune {
		return 0, fmt.Errorf("%q is no code point", s)
	}
	return rune(n), nil
}

// codeSequence reads code points written in hex, separated by spaces.
func codeSequence(s string) (string, error) {
	var b strings.Builder
	for _, f := range strings.Fields(s) {
		r, err := codePoint(f)
		if err != nil {
			return "", err
		}
		b.WriteRune(r)
	}
	return b.String(), nil
}

// check holds the case data to what internal/uni relies on: ASCII maps by
// the simple mappings alone, A to Z and a to z to each other and nothing
// else; and a mapping of SpecialCasing.txt that is one code point is the
// simple mapping, so the table need not hold it.
func (c casing) check() error {
	for r := rune(0); r < 0x80; r++ {
		want, ok := caseMapping{r, r, r}, false
		switch {
		case r >= 'A' && r <= 'Z':
			want, ok = caseMapping{r, r + 32, r}, true
		case r >= 'a' && r <= 'z':
			want, ok = caseMapping{r - 32, r, r - 32}, true
		}
		if got, has := c.simple[r]; has != ok || got != want && ok {
			return fmt.Errorf("ASCII %04X maps to %v, and ASCII maps between A to Z and a to z alone", r, got)
		}
		if _, has := c.special[r]; has {
			return fmt.Errorf("ASCII %04X has an entry in SpecialCasing.txt", r)
		}
	}
	for r, s := range c.special {
		m, ok := c.simple[r]
		if !ok {
			m = caseMapping{r, r, r}
		}
		for _, p := range []struct {
			full   string
			simple rune
		}{{s.upper, m.upper}, {s.lower, m.lower}, {s.title, m.title}} {
			if rs := []rune(p.full); len(rs) == 1 && rs[0] != p.simple {
				return fmt.Errorf("%04X maps to %04X in SpecialCasing.txt and to %04X simply", r, rs[0], p.simple)
			}
		}
	}
	return nil
}

// caseRange is code points from first to last whose simple mappings share
// their offsets, or, where alternating, alternate between an uppercase code
// point and the lowercase one after it.
type caseRange struct {
	first, last         rune
	upper, lower, title rune
	alternating         bool
}

// ranges returns the simple mappings as the fewest such ranges, in order.
// A run of alternating pairs becomes one range; every other code point
// joins the range before it where its offsets are the same.
func (c casing) ranges() []caseRange {
	var out []caseRange
	points := slices.Sorted(maps.Keys(c.simple))
	pairOf := func(i int) bool {
		if i+1 >= len(points) {
			return false
		}
		r, next := points[i], points[i+1]
		return next == r+1 && c.simple[r] == caseMapping{r, r + 1, r} && c.simple[next] == caseMapping{r, r + 1, r}
	}
	for i := 0; i < len(points); i++ {
		r := points[i]
		if pairOf(i) {
			if n := len(out); n > 0 && out[n-1].alternating && out[n-1].last == r-1 {
				out[n-1].last = r + 1
			} else {
				out = append(out, caseRange{first: r, last: r + 1, alternating: true})
			}
			i++
			continue
		}
		m := c.simple[r]
		d := caseRange{first: r, last: r, upper: m.upper - r, lower: m.lower - r, title: m.title - r}
		if n := len(out); n > 0 && !out[n-1].alternating && out[n-1].last == r-1 &&
			out[n-1].upper == d.upper && out[n-1].lower == d.lower && out[n-1].title == d.title {
			out[n-1].last = r
			continue
		}
		out = append(out, d)
	}
	return out
}

// propertyRanges returns the code points p holds as the fewest ranges, in
// order.
func propertyRanges(p map[rune]bool) [][2]rune {
	var out [][2]rune
	for _, r := range slices.Sorted(maps.Keys(p)) {
		if n := len(out); n > 0 && out[n-1][1] == r-1 {
			out[n-1][1] = r
			continue
		}
		out = append(out, [2]rune{r, r})
	}
	return out
}

// renderCasing returns the source of internal/uni's case_tables.go.
func renderCasing(c casing) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by tools/unigen. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package uni\n\n")
	fmt.Fprintf(&b, "// The data of Unicode %s that case conversion and trimming run on: the\n"+
		"// simple case mappings of its UnicodeData.txt, the unconditional entries of\n"+
		"// its SpecialCasing.txt, the Cased and Case_Ignorable properties of its\n"+
		"// DerivedCoreProperties.txt, and the White_Space property of its\n"+
		"// PropList.txt.\n\n", version)
	fmt.Fprintf(&b, "// caseTablesVersion is the Unicode version this data was generated from. A\n"+
		"// test holds it to UnicodeVersion, so the two cannot drift apart unremarked.\n"+
		"const caseTablesVersion = %q\n\n", version)
	fmt.Fprintf(&b, "// caseRanges holds every code point with a simple case mapping other than\n"+
		"// itself, as ranges whose code points share the offsets of their mappings,\n"+
		"// upper, lower and title, or alternate, an uppercase code point and its\n"+
		"// lowercase after it. The ranges are in order and do not overlap.\n"+
		"var caseRanges = [...]caseRange{\n")
	for _, r := range c.ranges() {
		if r.alternating {
			fmt.Fprintf(&b, "{0x%04X, 0x%04X, 0, 0, 0, true},\n", r.first, r.last)
			continue
		}
		fmt.Fprintf(&b, "{0x%04X, 0x%04X, %d, %d, %d, false},\n", r.first, r.last, r.upper, r.lower, r.title)
	}
	fmt.Fprintf(&b, "}\n\n")
	fmt.Fprintf(&b, "// specialCasings holds the unconditional entries of SpecialCasing.txt, by\n"+
		"// code point, in order: the full lowercase, titlecase and uppercase\n"+
		"// mappings of each.\nvar specialCasings = [...]specialCasing{\n")
	for _, r := range slices.Sorted(maps.Keys(c.special)) {
		s := c.special[r]
		fmt.Fprintf(&b, "{0x%04X, %s, %s, %s},\n", r, quoteEscaped(s.lower), quoteEscaped(s.title), quoteEscaped(s.upper))
	}
	fmt.Fprintf(&b, "}\n\n")
	for _, p := range []struct {
		name, doc string
		set       map[rune]bool
	}{
		{"cased", "the code points with the Cased property", c.cased},
		{"caseIgnorable", "the code points with the Case_Ignorable property", c.ignorable},
		{"whiteSpace", "the code points with the White_Space property", c.white},
	} {
		fmt.Fprintf(&b, "// %s holds %s, as ranges of a first\n"+
			"// and a last code point, in order.\nvar %s = [...][2]rune{\n", p.name, p.doc, p.name)
		for _, r := range propertyRanges(p.set) {
			fmt.Fprintf(&b, "{0x%04X, 0x%04X},\n", r[0], r[1])
		}
		fmt.Fprintf(&b, "}\n\n")
	}
	return formatted(b.Bytes())
}
