// Command unigen generates the Unicode data that internal/uni holds.
//
// tenon states the Unicode version its string semantics use, and that version
// must not follow whichever Go toolchain a consumer builds with. The tables
// this writes are therefore generated once, committed, and checked against
// golang.org/x/text by a test that runs only where x/text still ships the
// same version.
//
// It reads the data from x/text rather than from the UCD files so that no
// build step needs the network, and it refuses to run unless x/text reports
// the version the tables are for. Run it from the module root:
//
//	go run ./tools/unigen > internal/uni/tables.go
//
// A toolchain of go1.27 or later selects x/text's 17.0.0 tables, and the
// version check below then stops the run.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// version is the Unicode version these tables are for. It must match
// uni.UnicodeVersion.
const version = "15.0.0"

// The Hangul syllables compose and decompose by arithmetic, so they are left
// out of the tables. The names are those of UAX #15.
const (
	sBase, lBase, vBase, tBase      = 0xAC00, 0x1100, 0x1161, 0x11A7
	lCount, vCount, tCount          = 19, 21, 28
	nCount, sCount                  = vCount * tCount, lCount * nCount
	hangulFirst, hangulLastSyllable = sBase, sBase + sCount - 1
)

func main() {
	if norm.Version != version {
		fmt.Fprintf(os.Stderr, "unigen: x/text normalizes by Unicode %s, and these tables are for %s.\n"+
			"Build with a Go toolchain below go1.27, which selects x/text's %s tables.\n",
			norm.Version, version, version)
		os.Exit(1)
	}

	ccc := map[rune]uint8{}
	decomp := map[rune]string{}
	var decomposable []rune
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		s := string(r)
		if c := norm.NFC.PropertiesString(s).CCC(); c != 0 {
			ccc[r] = c
		}
		if r >= hangulFirst && r <= hangulLastSyllable {
			continue
		}
		if d := norm.NFD.String(s); d != s {
			decomp[r] = d
			decomposable = append(decomposable, r)
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
	for _, r := range decomposable {
		d := decomp[r]
		if len([]rune(d)) < 2 || norm.NFC.String(d) != string(r) {
			continue
		}
		if other, dup := byDecomposition[d]; dup {
			fmt.Fprintf(os.Stderr, "unigen: %04X and %04X share a canonical decomposition\n", other, r)
			os.Exit(1)
		}
		byDecomposition[d] = r
	}
	type pair struct{ a, b rune }
	composed := map[pair]rune{}
	for _, c := range decomposable {
		rs := []rune(decomp[c])
		if len(rs) < 2 || norm.NFC.String(decomp[c]) != string(c) {
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
			fmt.Fprintf(os.Stderr, "unigen: the decomposition of %04X begins with a sequence that is no code point\n", c)
			os.Exit(1)
		}
		composed[pair{a, rs[len(rs)-1]}] = c
	}
	// An all-ASCII string is already in Normalization Form C, which the
	// normalizer relies on. Nothing below is allowed to make that untrue.
	for p := range composed {
		if p.a < 0x80 && p.b < 0x80 {
			fmt.Fprintf(os.Stderr, "unigen: %04X and %04X are both ASCII and compose\n", p.a, p.b)
			os.Exit(1)
		}
	}
	for r := range decomp {
		if r < 0x80 {
			fmt.Fprintf(os.Stderr, "unigen: ASCII %04X has a canonical decomposition\n", r)
			os.Exit(1)
		}
	}

	// The quick check of UAX #15. A code point that Normalization Form C
	// rewrites on its own cannot stand in a normalized string: the singletons
	// and the composition exclusions are these. A code point that ends a
	// primary composite may compose onto what precedes it, so a string holding
	// one has to be normalized to find out. Everything else leaves a string
	// that is in canonical order as it is.
	var rewritten []rune
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if norm.NFC.String(string(r)) != string(r) {
			rewritten = append(rewritten, r)
		}
	}
	composing := map[rune]bool{}
	for p := range composed {
		composing[p.b] = true
	}
	// The Hangul vowels and trailing consonants compose onto what precedes
	// them by arithmetic, so they are not in the table of pairs.
	for r := rune(vBase); r < vBase+vCount; r++ {
		composing[r] = true
	}
	for r := rune(tBase + 1); r < tBase+tCount; r++ {
		composing[r] = true
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by tools/unigen. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package uni\n\n")
	fmt.Fprintf(&b, "// The data of Unicode %s: canonical combining classes, canonical\n"+
		"// decompositions and primary composites. The Hangul syllables are left out,\n"+
		"// composing and decomposing by arithmetic instead.\n\n", version)
	fmt.Fprintf(&b, "// tablesVersion is the Unicode version this data was generated from. A test\n"+
		"// holds it to UnicodeVersion, so the two cannot drift apart unremarked.\n"+
		"const tablesVersion = %q\n\n", version)

	// Combining classes, packed as the code point in the high bits and the
	// class in the low eight, sorted so that a lookup is a binary search.
	fmt.Fprintf(&b, "// combiningClasses holds every code point of nonzero canonical combining\n"+
		"// class, packed as the code point shifted up by eight with the class beneath,\n"+
		"// in code point order.\nvar combiningClasses = [...]uint32{\n")
	for _, r := range slices.Sorted(maps(ccc)) {
		fmt.Fprintf(&b, "0x%X<<8 | 0x%02X,\n", r, ccc[r])
	}
	fmt.Fprintf(&b, "}\n\n")

	// Decompositions: the code points in order, the offset of each one's
	// decomposition in a single string, and that string.
	slices.Sort(decomposable)
	fmt.Fprintf(&b, "// decomposable holds every code point with a canonical decomposition, in\n"+
		"// order. The decomposition of decomposable[i] is decompositions sliced from\n"+
		"// decompositionOffsets[i] to decompositionOffsets[i+1]. Each is the full\n"+
		"// canonical decomposition, so it never has to be applied again.\n"+
		"var decomposable = [...]rune{\n")
	for _, r := range decomposable {
		fmt.Fprintf(&b, "0x%X,\n", r)
	}
	fmt.Fprintf(&b, "}\n\n")
	var text strings.Builder
	offsets := make([]int, 0, len(decomposable)+1)
	for _, r := range decomposable {
		offsets = append(offsets, text.Len())
		text.WriteString(decomp[r])
	}
	offsets = append(offsets, text.Len())
	fmt.Fprintf(&b, "var decompositionOffsets = [...]uint32{\n")
	for _, o := range offsets {
		fmt.Fprintf(&b, "%d,\n", o)
	}
	fmt.Fprintf(&b, "}\n\n")
	fmt.Fprintf(&b, "const decompositions = %s\n\n", quoteEscaped(text.String()))

	// Primary composites, packed as the two code points in one key, sorted so
	// that a lookup is a binary search.
	keys := make([]pair, 0, len(composed))
	for p := range composed {
		keys = append(keys, p)
	}
	slices.SortFunc(keys, func(x, y pair) int {
		if x.a != y.a {
			return int(x.a) - int(y.a)
		}
		return int(x.b) - int(y.b)
	})
	fmt.Fprintf(&b, "// compositionKeys holds the pairs that form a primary composite, packed as\n"+
		"// the first code point shifted up by 21 with the second beneath, in order.\n"+
		"// compositionValues[i] is what compositionKeys[i] composes to.\n"+
		"var compositionKeys = [...]uint64{\n")
	for _, p := range keys {
		fmt.Fprintf(&b, "0x%X<<21 | 0x%X,\n", p.a, p.b)
	}
	fmt.Fprintf(&b, "}\n\nvar compositionValues = [...]rune{\n")
	for _, p := range keys {
		fmt.Fprintf(&b, "0x%X,\n", composed[p])
	}
	fmt.Fprintf(&b, "}\n\n")

	fmt.Fprintf(&b, "// rewritten holds the code points that Normalization Form C does not leave\n"+
		"// as they are on their own: the singleton decompositions and the composition\n"+
		"// exclusions. In order.\nvar rewritten = [...]rune{\n")
	for _, r := range rewritten {
		fmt.Fprintf(&b, "0x%X,\n", r)
	}
	fmt.Fprintf(&b, "}\n\n")
	fmt.Fprintf(&b, "// composing holds the code points that end a primary composite, the Hangul\n"+
		"// vowels and trailing consonants among them, so that a string holding one has\n"+
		"// to be normalized to find out what it does. In order.\nvar composing = [...]rune{\n")
	for _, r := range slices.Sorted(maps(composing)) {
		fmt.Fprintf(&b, "0x%X,\n", r)
	}
	fmt.Fprintf(&b, "}\n")

	out, err := format.Source(b.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "unigen: the generated source does not parse: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "unigen: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "unigen: Unicode %s: %d combining classes, %d decompositions, %d primary composites, "+
		"%d rewritten, %d composing\n",
		version, len(ccc), len(decomposable), len(composed), len(rewritten), len(composing))
}

// maps returns the keys of m, which slices.Sorted then orders.
func maps[K ~int32, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// quoteEscaped renders s as a Go string literal in which every rune is written
// by its code point, so that the generated file holds no text of its own and
// is legible in any editor.
func quoteEscaped(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		fmt.Fprintf(&b, "\\U%08X", r)
	}
	b.WriteByte('"')
	return b.String()
}
