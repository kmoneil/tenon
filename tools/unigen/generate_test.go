package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGraphemeTablesAreGenerated holds internal/uni's grapheme_tables.go to
// what unigen generates from the files in ucd, on every toolchain, so that the
// committed table is Unicode's data and no edit of it.
func TestGraphemeTablesAreGenerated(t *testing.T) {
	s, err := deriveSegmentation(ucd)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.check(); err != nil {
		t.Fatal(err)
	}
	want, err := renderGraphemes(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../internal/uni/grapheme_tables.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("internal/uni/grapheme_tables.go is not what unigen generates from ucd; run unigen")
	}
}

// TestSegmentationCheckRefuses holds the check of the segmentation data to
// each part of the data that the segmenter reads without the table.
func TestSegmentationCheckRefuses(t *testing.T) {
	s, err := deriveSegmentation(ucd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		change func(segmentation)
		want   string
	}{
		{"an ASCII letter given a value", func(s segmentation) { s['a'] = "gbExtend" }, "ASCII 0061"},
		{"an ASCII control made Other", func(s segmentation) { delete(s, 0x07) }, "ASCII 0007"},
		{"CR made a control", func(s segmentation) { s['\r'] = "gbControl" }, "ASCII 000D"},
		{"a syllable without a trailing consonant made LVT", func(s segmentation) { s[0xAC00] = "gbLVT" }, "AC00"},
		{"a syllable with one made LV", func(s segmentation) { s[0xAC01] = "gbLV" }, "AC01"},
		{"a syllable made Other", func(s segmentation) { delete(s, 0xD7A3) }, "D7A3"},
		{"LV beyond the syllables", func(s segmentation) { s[0x1100] = "gbLV" }, "no Hangul syllable"},
	} {
		changed := maps.Clone(s)
		tt.change(changed)
		if err := changed.check(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: check gave %v, want an error naming %q", tt.name, err, tt.want)
		}
	}
	if err := s.check(); err != nil {
		t.Errorf("the data as Unicode gives it fails the check: %v", err)
	}
}

// TestDeriveSegmentationRefuses holds the reading of Unicode's files to the
// version they say they are for and to the values internal/uni has constants
// for, and refuses a code point given two values.
func TestDeriveSegmentationRefuses(t *testing.T) {
	breaks, err := os.ReadFile(filepath.Join(ucd, "GraphemeBreakProperty.txt"))
	if err != nil {
		t.Fatal(err)
	}
	emoji, err := os.ReadFile(filepath.Join(ucd, "emoji-data.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name          string
		breaks, emoji string
		want          string
	}{
		{"another version's properties", strings.Replace(string(breaks), "-"+version+".txt", "-15.1.0.txt", 1), string(emoji), "not for Unicode"},
		{"another version's emoji", string(breaks), strings.Replace(string(emoji), "Emoji Version 15.0 ", "Emoji Version 15.1 ", 1), "not for Unicode"},
		{"a value with no constant", string(breaks) + "E0000 ; Glue\n", string(emoji), "no constant for"},
		{"a code point given two values", string(breaks) + "0600 ; Extend\n", string(emoji), "a second value"},
		{"a pictograph that is not Other", string(breaks), string(emoji) + "0300 ; Extended_Pictographic\n", "rather than Other"},
		{"a range backwards", string(breaks) + "0602..0600 ; Prepend\n", string(emoji), "no range of code points"},
		{"a line with no value", string(breaks) + "0600\n", string(emoji), "no value"},
		{"a code point past the last", string(breaks) + "110000 ; Control\n", string(emoji), "no range of code points"},
		{"a number past any rune", string(breaks) + "100000000 ; Control\n", string(emoji), "no range of code points"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "GraphemeBreakProperty.txt"), []byte(tt.breaks), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "emoji-data.txt"), []byte(tt.emoji), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := deriveSegmentation(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: deriveSegmentation gave %v, want an error naming %q", tt.name, err, tt.want)
		}
	}
}

// TestCaseTablesAreGenerated holds internal/uni's case_tables.go to what
// unigen generates from the files in ucd, on every toolchain.
func TestCaseTablesAreGenerated(t *testing.T) {
	c, err := deriveCasing(ucd)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.check(); err != nil {
		t.Fatal(err)
	}
	want, err := renderCasing(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../internal/uni/case_tables.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("internal/uni/case_tables.go is not what unigen generates from ucd; run unigen")
	}
}

// TestCasingCheckRefuses holds the check of the case data to what
// internal/uni relies on.
func TestCasingCheckRefuses(t *testing.T) {
	c, err := deriveCasing(ucd)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		change func(casing)
		want   string
	}{
		{"an ASCII letter mapped elsewhere", func(c casing) { c.simple['a'] = caseMapping{'B', 'a', 'B'} }, "ASCII 0061"},
		{"an ASCII digit given a mapping", func(c casing) { c.simple['1'] = caseMapping{'2', '1', '2'} }, "ASCII 0031"},
		{"an ASCII entry in SpecialCasing", func(c casing) { c.special['a'] = specialCasing{"a", "A", "A"} }, "ASCII 0061"},
		{"a full mapping of one code point that is not the simple one", func(c casing) { c.special[0x00DF] = specialCasing{"\U000000DF", "S", "SS"} }, "00DF"},
	} {
		changed := casing{simple: maps.Clone(c.simple), special: maps.Clone(c.special)}
		tt.change(changed)
		if err := changed.check(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: check gave %v, want an error naming %q", tt.name, err, tt.want)
		}
	}
	if err := c.check(); err != nil {
		t.Errorf("the data as Unicode gives it fails the check: %v", err)
	}
}

// TestDeriveCasingRefuses holds the reading of the case files to the
// version they are for and to the one condition internal/uni applies.
func TestDeriveCasingRefuses(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"UnicodeData.txt", "SpecialCasing.txt", "DerivedCoreProperties.txt", "PropList.txt"} {
		data, err := os.ReadFile(filepath.Join(ucd, name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	for _, tt := range []struct {
		name, file string
		change     func(string) string
		want       string
	}{
		{"another UnicodeData.txt", "UnicodeData.txt", func(s string) string { return s + "E0000;X;Lu;0;L;;;;;N;;;;E0001;\n" }, "SHA-256"},
		{"another version's SpecialCasing.txt", "SpecialCasing.txt", func(s string) string {
			return strings.Replace(s, "SpecialCasing-"+version, "SpecialCasing-15.1.0", 1)
		}, "not for Unicode"},
		{"a condition no language's", "SpecialCasing.txt", func(s string) string { return s + "0130; 0069; 0130; 0130; After_I;\n" }, "After_I"},
		{"Final_Sigma's entry changed", "SpecialCasing.txt", func(s string) string {
			return strings.Replace(s, "03A3; 03C2; 03A3; 03A3; Final_Sigma;", "03A3; 03C3; 03A3; 03A3; Final_Sigma;", 1)
		}, "Final_Sigma"},
		{"Final_Sigma's entry gone", "SpecialCasing.txt", func(s string) string {
			return strings.Replace(s, "03A3; 03C2; 03A3; 03A3; Final_Sigma;", "", 1)
		}, "no entry"},
		{"a code point given two entries", "SpecialCasing.txt", func(s string) string { return s + "00DF; 00DF; 0053 0073; 0053 0053;\n" }, "second entry"},
		{"an entry of three fields", "SpecialCasing.txt", func(s string) string { return s + "00DF; 00DF;\n" }, "fields"},
		{"an entry naming no code point", "SpecialCasing.txt", func(s string) string { return s + "XYZ; 00DF; 0053 0073; 0053 0053;\n" }, "no code point"},
		{"another version's properties", "DerivedCoreProperties.txt", func(s string) string {
			return strings.Replace(s, "DerivedCoreProperties-"+version, "DerivedCoreProperties-15.1.0", 1)
		}, "not for Unicode"},
		{"no White_Space", "PropList.txt", func(s string) string { return strings.ReplaceAll(s, "; White_Space", "; Not_Space") }, "no code point White_Space"},
		{"a range backwards", "PropList.txt", func(s string) string { return s + "0020..0010 ; White_Space\n" }, "no range of code points"},
	} {
		dir := t.TempDir()
		for name, data := range files {
			if name == tt.file {
				data = tt.change(data)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := deriveCasing(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: deriveCasing gave %v, want an error naming %q", tt.name, err, tt.want)
		}
	}
}
