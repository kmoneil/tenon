package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// patterns is the data that tenon's copy of regexp/syntax reads: the
// general categories, their major classes, Go's LC and Cn among them; the
// scripts; the code points each category and script holds only by case
// folding; the aliases of the categories; and the simple case-folding
// orbits.
type patterns struct {
	categories, scripts         map[string]map[rune]bool
	foldCategories, foldScripts map[string]map[rune]bool
	aliases                     map[string]string
	// next gives, for each code point that simple case folding makes
	// equivalent to another, the next larger in its orbit, the smallest
	// after the largest.
	next map[rune]rune
}

// derivePatterns reads the pattern data from Unicode's own files in dir:
// UnicodeData.txt, Scripts.txt, CaseFolding.txt and
// PropertyValueAliases.txt.
func derivePatterns(dir string) (patterns, error) {
	p := patterns{categories: map[string]map[rune]bool{}, scripts: map[string]map[rune]bool{}, aliases: map[string]string{}, next: map[rune]rune{}}
	data, err := os.ReadFile(filepath.Join(dir, "UnicodeData.txt"))
	if err != nil {
		return patterns{}, err
	}
	if err := checkUnicodeData(data); err != nil {
		return patterns{}, err
	}
	assigned := map[rune]bool{}
	var first rune
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		fields := strings.Split(line, ";")
		r, err := codePoint(fields[0])
		if err != nil {
			return patterns{}, fmt.Errorf("UnicodeData.txt: line %d: %w", i+1, err)
		}
		gc := fields[2]
		lo := r
		switch {
		case strings.HasSuffix(fields[1], ", First>"):
			first = r
			continue
		case strings.HasSuffix(fields[1], ", Last>"):
			lo = first
		}
		if p.categories[gc] == nil {
			p.categories[gc] = map[rune]bool{}
		}
		for c := lo; c <= r; c++ {
			p.categories[gc][c] = true
			assigned[c] = true
		}
	}
	// Cn is what nothing assigns; the major classes are the unions of the
	// categories of their letter, C with Cn among them, as
	// PropertyValueAliases.txt has it; and LC is the cased letters.
	cn := map[rune]bool{}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !assigned[r] {
			cn[r] = true
		}
	}
	p.categories["Cn"] = cn
	for _, major := range []string{"C", "L", "M", "N", "P", "S", "Z"} {
		union := map[rune]bool{}
		for name, set := range p.categories {
			if len(name) == 2 && name[:1] == major {
				maps.Copy(union, set)
			}
		}
		p.categories[major] = union
	}
	lc := map[rune]bool{}
	for _, name := range []string{"Lu", "Ll", "Lt"} {
		maps.Copy(lc, p.categories[name])
	}
	p.categories["LC"] = lc

	scripts, err := readUCD(dir, "Scripts.txt", "# Scripts-"+version+".txt")
	if err != nil {
		return patterns{}, err
	}
	err = eachRange(scripts, func(first, last rune, name string) error {
		if p.scripts[name] == nil {
			p.scripts[name] = map[rune]bool{}
		}
		for r := first; r <= last; r++ {
			p.scripts[name][r] = true
		}
		return nil
	})
	if err != nil {
		return patterns{}, fmt.Errorf("reading Scripts.txt: %w", err)
	}

	folding, err := readUCD(dir, "CaseFolding.txt", "# CaseFolding-"+version+".txt")
	if err != nil {
		return patterns{}, err
	}
	orbits := map[rune][]rune{}
	err = eachRange(folding, func(first, _ rune, value string) error {
		fields := strings.Split(value, ";")
		status := strings.TrimSpace(fields[0])
		if status != "C" && status != "S" {
			return nil
		}
		f, err := codePoint(strings.TrimSpace(fields[1]))
		if err != nil {
			return err
		}
		if len(orbits[f]) == 0 {
			orbits[f] = []rune{f}
		}
		orbits[f] = append(orbits[f], first)
		return nil
	})
	if err != nil {
		return patterns{}, fmt.Errorf("reading CaseFolding.txt: %w", err)
	}
	for _, orbit := range orbits {
		slices.Sort(orbit)
		for i, r := range orbit {
			p.next[r] = orbit[(i+1)%len(orbit)]
		}
	}

	p.foldCategories = foldsOf(p.categories, p.next, "L", "Ll", "Lt", "Lu", "M", "Mn")
	p.foldScripts = foldsOf(p.scripts, p.next, "Common", "Greek", "Inherited")

	aliases, err := readUCD(dir, "PropertyValueAliases.txt", "# PropertyValueAliases-"+version+".txt")
	if err != nil {
		return patterns{}, err
	}
	for line := range strings.Lines(aliases) {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Split(line, ";")
		if len(fields) < 3 || strings.TrimSpace(fields[0]) != "gc" {
			continue
		}
		short := strings.TrimSpace(fields[1])
		for _, alias := range fields[2:] {
			if alias = strings.TrimSpace(alias); alias != "" && alias != short {
				p.aliases[alias] = short
			}
		}
	}
	return p, nil
}

// foldsOf returns, for each table named, the code points it does not hold
// whose case-folding orbit holds one it does, where there are any: the
// code points a case-insensitive class of it adds. Go's tables name these
// for the categories and scripts given, and for no other.
func foldsOf(tables map[string]map[rune]bool, next map[rune]rune, names ...string) map[string]map[rune]bool {
	out := map[string]map[rune]bool{}
	for _, name := range names {
		set := tables[name]
		fold := map[rune]bool{}
		for r := range next {
			if set[r] {
				continue
			}
			for c := next[r]; c != r; c = next[c] {
				if set[c] {
					fold[r] = true
					break
				}
			}
		}
		out[name] = fold
	}
	return out
}

// checkUnicodeData refuses UnicodeData.txt of another version.
func checkUnicodeData(data []byte) error {
	if sum := sha256Hex(data); sum != unicodeDataSHA256 {
		return fmt.Errorf("UnicodeData.txt is not for Unicode %s: its SHA-256 is %s, not %s", version, sum, unicodeDataSHA256)
	}
	return nil
}

// strideRange is code points from lo to hi, stride apart.
type strideRange struct{ lo, hi, stride rune }

// strides returns the code points of set as the fewest ranges of one
// stride each, greedily, as Go's tables hold them.
func strides(set map[rune]bool) []strideRange {
	points := slices.Sorted(maps.Keys(set))
	var out []strideRange
	for i := 0; i < len(points); {
		r := strideRange{points[i], points[i], 1}
		j := i + 1
		if j < len(points) {
			r.stride = points[j] - points[i]
			for j < len(points) && points[j]-r.hi == r.stride {
				r.hi = points[j]
				j++
			}
		}
		if r.lo == r.hi {
			r.stride = 1
		}
		// A range of two code points far apart holds a stride that a
		// third code point would not keep: take one alone instead, so the
		// next can start a run.
		if r.hi-r.lo == r.stride && r.stride > 1 && j < len(points) {
			r, j = strideRange{points[i], points[i], 1}, i+1
		}
		out = append(out, r)
		i = j
	}
	return out
}

// renderTable writes the Go source of a *unicode.RangeTable holding set.
func renderTable(b *bytes.Buffer, set map[rune]bool) {
	var r16, r32 []strideRange
	for _, r := range strides(set) {
		if r.hi <= 0xFFFF {
			r16 = append(r16, r)
		} else if r.lo > 0xFFFF {
			r32 = append(r32, r)
		} else {
			// A range across the boundary splits at it.
			last16 := r.lo + (0xFFFF-r.lo)/r.stride*r.stride
			r16 = append(r16, strideRange{r.lo, last16, r.stride})
			r32 = append(r32, strideRange{last16 + r.stride, r.hi, r.stride})
		}
	}
	latin := 0
	for _, r := range r16 {
		if r.hi <= unicode.MaxLatin1 {
			latin++
		}
	}
	fmt.Fprintf(b, "&unicode.RangeTable{\n")
	if len(r16) > 0 {
		fmt.Fprintf(b, "R16: []unicode.Range16{\n")
		for _, r := range r16 {
			fmt.Fprintf(b, "{0x%04x, 0x%04x, %d},\n", r.lo, r.hi, r.stride)
		}
		fmt.Fprintf(b, "},\n")
	}
	if len(r32) > 0 {
		fmt.Fprintf(b, "R32: []unicode.Range32{\n")
		for _, r := range r32 {
			fmt.Fprintf(b, "{0x%x, 0x%x, %d},\n", r.lo, r.hi, r.stride)
		}
		fmt.Fprintf(b, "},\n")
	}
	if latin > 0 {
		fmt.Fprintf(b, "LatinOffset: %d,\n", latin)
	}
	fmt.Fprintf(b, "}")
}

// renderPatterns returns the source of internal/uni's pattern_tables.go.
func renderPatterns(p patterns) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by tools/unigen. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package uni\n\nimport \"unicode\"\n\n")
	fmt.Fprintf(&b, "// The data of Unicode %s that patterns read: the general categories of\n"+
		"// its UnicodeData.txt, the scripts of its Scripts.txt, the aliases of the\n"+
		"// categories in its PropertyValueAliases.txt, and the simple case folding\n"+
		"// of its CaseFolding.txt, as the tables of Go's unicode package hold them.\n\n", version)
	fmt.Fprintf(&b, "// patternTablesVersion is the Unicode version this data was generated\n"+
		"// from. A test holds it to UnicodeVersion, so the two cannot drift apart\n"+
		"// unremarked.\nconst patternTablesVersion = %q\n\n", version)
	for _, t := range []struct {
		name, doc string
		tables    map[string]map[rune]bool
	}{
		{"Categories", "the code points of each general category, its major classes, LC and Cn", p.categories},
		{"FoldCategory", "the code points a category holds only by case folding, for those Go's\n// tables name", p.foldCategories},
		{"Scripts", "the code points of each script", p.scripts},
		{"FoldScript", "the code points a script holds only by case folding, for those Go's\n// tables name", p.foldScripts},
	} {
		fmt.Fprintf(&b, "// %s holds %s.\nvar %s = map[string]*unicode.RangeTable{\n", t.name, t.doc, t.name)
		for _, name := range slices.Sorted(maps.Keys(t.tables)) {
			fmt.Fprintf(&b, "%q: ", name)
			renderTable(&b, t.tables[name])
			fmt.Fprintf(&b, ",\n")
		}
		fmt.Fprintf(&b, "}\n\n")
	}
	fmt.Fprintf(&b, "// CategoryAliases maps each alias of a general category to its name.\nvar CategoryAliases = map[string]string{\n")
	for _, alias := range slices.Sorted(maps.Keys(p.aliases)) {
		fmt.Fprintf(&b, "%q: %q,\n", alias, p.aliases[alias])
	}
	fmt.Fprintf(&b, "}\n\n")
	fmt.Fprintf(&b, "// foldOrbit holds, for each code point simple case folding makes\n"+
		"// equivalent to another, the next larger in its orbit, or the smallest\n"+
		"// after the largest, in code point order.\nvar foldOrbit = [...][2]rune{\n")
	for _, r := range slices.Sorted(maps.Keys(p.next)) {
		fmt.Fprintf(&b, "{0x%04X, 0x%04X},\n", r, p.next[r])
	}
	fmt.Fprintf(&b, "}\n")
	return formatted(b.Bytes())
}
