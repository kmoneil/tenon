// Command unigen generates the Unicode data that internal/uni holds.
//
// tenon states the Unicode version its string semantics use, and that version
// must follow neither the Go toolchain a consumer builds with nor any module
// their build requires. The tables this writes are therefore generated once and
// committed. This module's tests hold internal/uni to Unicode's own
// conformance tests, on every toolchain, and to golang.org/x/text wherever it
// still ships the same version. The module is apart from tenon's, so that
// tenon requires no other module.
//
// The normalization data and the general categories come from x/text and Go's
// unicode package, and the segmentation data from Unicode's own files, in ucd,
// since neither of those carries it. unigen refuses to run unless each source
// is of the version the tables are for. Run it from this directory, on a
// toolchain below go1.27:
//
//	go run . ../../internal/uni
//
// A toolchain of go1.27 or later selects x/text's 17.0.0 tables, and the
// version check then stops the run.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// version is the Unicode version these tables are for. It must match
// uni.UnicodeVersion.
const version = "15.0.0"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: unigen <directory of internal/uni>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "unigen: %v\n", err)
		os.Exit(1)
	}
}

// run generates internal/uni's tables.go, grapheme_tables.go and
// case_tables.go into dir, and reports to log what they hold.
func run(dir string, log io.Writer) error {
	if err := checkVersions(); err != nil {
		return err
	}
	n, err := deriveNormalization()
	if err != nil {
		return err
	}
	if err := n.check(); err != nil {
		return err
	}
	s, err := deriveSegmentation(ucd)
	if err != nil {
		return err
	}
	if err := s.check(); err != nil {
		return err
	}
	c, err := deriveCasing(ucd)
	if err != nil {
		return err
	}
	if err := c.check(); err != nil {
		return err
	}
	categories := deriveCategories()
	tables, err := renderTables(n, categories)
	if err != nil {
		return err
	}
	graphemes, err := renderGraphemes(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tables.go"), tables, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "grapheme_tables.go"), graphemes, 0o644); err != nil {
		return err
	}
	cases, err := renderCasing(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "case_tables.go"), cases, 0o644); err != nil {
		return err
	}
	_, err = fmt.Fprintf(log, "unigen: Unicode %s: %d combining classes, %d decompositions, %d primary composites, "+
		"%d rewritten, %d composing, %d category ranges, %d grapheme break ranges, %d case ranges, "+
		"%d special casings\n",
		version, len(n.ccc), len(n.decomposable), len(n.composed), len(n.rewritten), len(n.composing),
		len(categories), len(s.ranges()), len(c.ranges()), len(c.special))
	return err
}
