//go:build !go1.27

package main

import (
	"maps"
	"slices"
	"testing"
	"unicode"

	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/uni"
)

// This file runs only below go1.27, where Go's unicode package carries the
// Unicode version internal/uni's pattern tables hold, generated from
// Unicode's own files: there it is a second source of the same data.

// points returns the code points t holds.
func points(t *unicode.RangeTable) []rune {
	var out []rune
	if t == nil {
		return out
	}
	for _, r := range t.R16 {
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			out = append(out, c)
		}
	}
	for _, r := range t.R32 {
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			out = append(out, c)
		}
	}
	return out
}

// sameTables holds the tables of one kind to Go's, by name and by every
// code point each holds.
func sameTables(t *testing.T, kind string, got, want map[string]*unicode.RangeTable) {
	t.Helper()
	if g, w := slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)); !slices.Equal(g, w) {
		t.Errorf("%s names %v, and Go's %v", kind, g, w)
	}
	for name, w := range want {
		if g := points(got[name]); !slices.Equal(g, points(w)) {
			t.Errorf("%s[%q] holds %d code points, and Go's %d", kind, name, len(g), len(points(w)))
		}
	}
}

// TestConformance_LR003_TablesAgreeWithGo holds the pattern tables to Go's
// unicode package, table by table, alias by alias, and the simple case
// folding orbit by orbit, over every code point.
func TestConformance_LR003_TablesAgreeWithGo(t *testing.T) {
	conformance.Covers(t, "LR-003")
	if unicode.Version != version {
		t.Fatalf("Go's unicode package is Unicode %s, and these tables are for %s", unicode.Version, version)
	}
	sameTables(t, "Categories", uni.Categories, unicode.Categories)
	sameTables(t, "FoldCategory", uni.FoldCategory, unicode.FoldCategory)
	sameTables(t, "Scripts", uni.Scripts, unicode.Scripts)
	sameTables(t, "FoldScript", uni.FoldScript, unicode.FoldScript)
	if !maps.Equal(uni.CategoryAliases, unicode.CategoryAliases) {
		t.Errorf("CategoryAliases is %v, and Go's %v", uni.CategoryAliases, unicode.CategoryAliases)
	}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if got, want := uni.SimpleFold(r), unicode.SimpleFold(r); got != want {
			t.Errorf("SimpleFold(%U) = %U, and Go's %U", r, got, want)
		}
	}
	// Lookups through the tables agree with Go's on the boundaries too.
	for name, tab := range uni.Categories {
		for _, r := range []rune{0, 0x7F, 0xFF, 0x100, 0xFFFF, 0x10000, 0x10FFFF} {
			if unicode.Is(tab, r) != unicode.Is(unicode.Categories[name], r) {
				t.Errorf("unicode.Is(Categories[%q], %U) differs from Go's", name, r)
			}
		}
	}
}
