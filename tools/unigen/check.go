package main

import (
	"fmt"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// checkVersions refuses the run unless x/text and Go's unicode package carry
// the Unicode version the tables are for.
func checkVersions() error {
	if norm.Version != version {
		return fmt.Errorf("x/text normalizes by Unicode %s, and these tables are for %s: build with a Go toolchain below go1.27, which selects x/text's %s tables",
			norm.Version, version, version)
	}
	if unicode.Version != version {
		return fmt.Errorf("the general categories of Go's unicode package are Unicode %s, and these tables are for %s", unicode.Version, version)
	}
	return nil
}

// check holds the normalization data to what the normalizer relies on: an
// all-ASCII string is already in Normalization Form C, so nothing may make
// that untrue.
func (n normalization) check() error {
	for p := range n.composed {
		if p.a < 0x80 && p.b < 0x80 {
			return fmt.Errorf("%04X and %04X are both ASCII and compose", p.a, p.b)
		}
	}
	for r := range n.decomp {
		if r < 0x80 {
			return fmt.Errorf("ASCII %04X has a canonical decomposition", r)
		}
	}
	return nil
}

// check holds the segmentation data to what the segmenter relies on, which
// reads two parts of it without the table: ASCII, which is Other but for
// the controls, CR and LF among them, and the Hangul syllables, which are LV
// where they have no trailing consonant and LVT where they have one. Nothing
// else is LV or LVT.
func (s segmentation) check() error {
	for r := rune(0); r < 0x80; r++ {
		want := ""
		switch {
		case r == '\r':
			want = "gbCR"
		case r == '\n':
			want = "gbLF"
		case r < 0x20 || r == 0x7F:
			want = "gbControl"
		}
		if s[r] != want {
			return fmt.Errorf("ASCII %04X is %q, and the segmenter reads it as %q", r, s[r], want)
		}
	}
	for r, v := range s {
		syllable := r >= hangulFirst && r <= hangulLastSyllable
		if !syllable && (v == "gbLV" || v == "gbLVT") {
			return fmt.Errorf("%04X is %s and no Hangul syllable", r, v)
		}
	}
	for r := rune(hangulFirst); r <= hangulLastSyllable; r++ {
		want := "gbLVT"
		if (r-sBase)%tCount == 0 {
			want = "gbLV"
		}
		if s[r] != want {
			return fmt.Errorf("the Hangul syllable %04X is %q, and its decomposition makes it %s", r, s[r], want)
		}
	}
	return nil
}
