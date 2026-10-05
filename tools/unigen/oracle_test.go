package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/uni"
)

// Unicode's own conformance tests for normalization and segmentation, which
// hold internal/uni to the version tenon states on every toolchain: they are
// Unicode's answers, from neither Go's data nor x/text's, and they are files
// committed here, which no toolchain moves.

// readOracle returns the file name in ucd, failing t unless its first line
// says it is for uni.UnicodeVersion.
func readOracle(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ucd, name))
	if err != nil {
		t.Fatal(err)
	}
	want := "# " + strings.TrimSuffix(name, ".txt") + "-" + uni.UnicodeVersion + ".txt"
	if first, _, _ := strings.Cut(string(data), "\n"); first != want {
		t.Fatalf("%s begins %q, not %q: it is not the test for Unicode %s", name, first, want, uni.UnicodeVersion)
	}
	return string(data)
}

// codePoints returns the text that fields spell as code points in hex, as
// Unicode's test files write them.
func codePoints(t *testing.T, line int, fields []string) string {
	t.Helper()
	var b strings.Builder
	for _, f := range fields {
		r, err := strconv.ParseUint(f, 16, 32)
		if err != nil || r > unicode.MaxRune || r >= 0xD800 && r <= 0xDFFF {
			t.Fatalf("line %d: %q is no scalar value", line, f)
		}
		b.WriteRune(rune(r))
	}
	return b.String()
}

// TestConformance_ST005_GraphemeBreakTest holds GraphemeCount to Unicode's
// GraphemeBreakTest.txt, in which each case is a sequence of code points
// with a mark before each of them saying whether a cluster begins there, "÷",
// or not, "×". GraphemeCount says only how many clusters a text holds, but no
// rule of UAX #29 looks past the code point it decides a boundary before, so
// a cluster begins at a code point exactly where the count of the text up to
// it is one more than the count of the text before it: counting every prefix
// of a case holds it to each of its marks, not only to their number.
func TestConformance_ST005_GraphemeBreakTest(t *testing.T) {
	conformance.Covers(t, "ST-005", "ST-003")
	cases := 0
	for i, line := range strings.Split(readOracle(t, "GraphemeBreakTest.txt"), "\n") {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields)%2 == 0 || fields[0] != "÷" || fields[len(fields)-1] != "÷" {
			t.Fatalf("line %d: %q is not code points between marks, beginning and ending with a break", i+1, line)
		}
		cases++
		var text strings.Builder
		clusters := 0
		for k := 1; k < len(fields); k += 2 {
			switch fields[k-1] {
			case "÷":
				clusters++
			case "×":
			default:
				t.Fatalf("line %d: %q is no mark", i+1, fields[k-1])
			}
			text.WriteString(codePoints(t, i+1, fields[k:k+1]))
			if got := uni.GraphemeCount(text.String()); got != clusters {
				t.Errorf("line %d: %s: the first %d code points hold %d clusters, and GraphemeCount gives %d",
					i+1, strings.TrimSpace(line), k/2+1, clusters, got)
				break
			}
		}
	}
	if cases < 600 {
		t.Fatalf("GraphemeBreakTest.txt held %d cases, fewer than the 602 of Unicode 15.0.0", cases)
	}
}

// TestConformance_LS001_ClustersAndCuts holds Clusters and CutsOf to
// Unicode's GraphemeBreakTest.txt: the clusters are the text between the
// breaks, "÷", and the cut positions are the breaks and, beside them, the
// position between the CR and the LF of a CR LF.
func TestConformance_LS001_ClustersAndCuts(t *testing.T) {
	conformance.Covers(t, "LS-001")
	cases := 0
	for i, line := range strings.Split(readOracle(t, "GraphemeBreakTest.txt"), "\n") {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cases++
		var text strings.Builder
		var clusters []string
		cuts := map[int]bool{0: true}
		start := 0
		for k := 1; k < len(fields); k += 2 {
			if fields[k-1] == "÷" && k > 1 {
				clusters = append(clusters, text.String()[start:])
				start = text.Len()
				cuts[start] = true
			}
			r := codePoints(t, i+1, fields[k:k+1])
			if r == "\n" && strings.HasSuffix(text.String(), "\r") {
				cuts[text.Len()] = true
			}
			text.WriteString(r)
		}
		s := text.String()
		clusters = append(clusters, s[start:])
		cuts[len(s)] = true
		if got := slices.Collect(uni.Clusters(s)); !slices.Equal(got, clusters) {
			t.Errorf("line %d: %s: Clusters gives %+q, want %+q", i+1, strings.TrimSpace(line), got, clusters)
		}
		c := uni.CutsOf(s)
		for j := 0; j <= len(s); j++ {
			if c.At(j) != cuts[j] {
				t.Errorf("line %d: %s: CutsOf(...).At(%d) = %v, want %v", i+1, strings.TrimSpace(line), j, c.At(j), cuts[j])
			}
		}
	}
	if cases < 600 {
		t.Fatalf("GraphemeBreakTest.txt held %d cases, fewer than the 602 of Unicode 15.0.0", cases)
	}
}

// TestConformance_ST002_NormalizationTest holds NFC to Unicode's
// NormalizationTest.txt. For each case c1;c2;c3;c4;c5, NFC gives c2 for c1, c2
// and c3, and c4 for c4 and c5; and each code point that part 1 does not list
// is its own NFC.
func TestConformance_ST002_NormalizationTest(t *testing.T) {
	conformance.Covers(t, "ST-002", "ST-003")
	listed := map[rune]bool{}
	part, cases := "", 0
	for i, line := range strings.Split(readOracle(t, "NormalizationTest.txt"), "\n") {
		line, _, _ = strings.Cut(line, "#")
		if strings.HasPrefix(line, "@") {
			part = strings.TrimSpace(line)
			continue
		}
		columns := strings.Split(line, ";")
		if len(columns) == 1 && strings.TrimSpace(line) == "" {
			continue
		}
		if len(columns) != 6 || strings.TrimSpace(columns[5]) != "" {
			t.Fatalf("line %d: %q is not five columns of code points", i+1, line)
		}
		var c [5]string
		for j := range c {
			c[j] = codePoints(t, i+1, strings.Fields(columns[j]))
		}
		cases++
		if rs := []rune(c[0]); part == "@Part1" && len(rs) == 1 {
			listed[rs[0]] = true
		}
		for j, want := range [5]string{c[1], c[1], c[1], c[3], c[3]} {
			if got := uni.NFC(c[j]); got != want {
				t.Errorf("line %d: NFC(c%d %U) = %U, want %U", i+1, j+1, []rune(c[j]), []rune(got), []rune(want))
			}
		}
	}
	if cases < 19000 || len(listed) < 2000 {
		t.Fatalf("NormalizationTest.txt held %d cases, %d in part 1: too few for the file of Unicode 15.0.0", cases, len(listed))
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF || listed[r] {
			continue
		}
		if got := uni.NFC(string(r)); got != string(r) {
			t.Errorf("NFC(%U) = %U, and part 1 does not list it, so it is its own", r, []rune(got))
		}
	}
}
