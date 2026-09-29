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
