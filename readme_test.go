package tenon_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadmeCodeIsRun holds every Go block in the README to a program the test
// suite runs. A block must appear, line for line, inside an example, so a
// reader can paste it and a change that breaks it fails the gate rather than
// leaving a README that no longer compiles.
func TestReadmeCodeIsRun(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	examples := exampleSource(t)
	blocks := goBlocks(string(readme))
	if len(blocks) < 4 {
		t.Errorf("the README holds %d Go blocks, too few to show what the library does", len(blocks))
	}
	for i, block := range blocks {
		if !strings.Contains(examples, stripped(block)) {
			t.Errorf("Go block %d of the README is in no example:\n%s", i+1, block)
		}
	}

	// A block that is not Go is output, and every line of it is a line some
	// example prints.
	printed := map[string]bool{}
	for _, line := range strings.Split(examples, "\n") {
		if text, ok := strings.CutPrefix(line, "//"); ok {
			printed[strings.TrimSpace(text)] = true
		}
	}
	for i, block := range fencedBlocks(string(readme), "") {
		for _, line := range strings.Split(block, "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			if !printed[line] {
				t.Errorf("output block %d of the README holds a line no example prints:\n%s", i+1, line)
			}
		}
	}
}

// goBlocks returns the contents of the ```go fences of a markdown document.
func goBlocks(text string) []string { return fencedBlocks(text, "go") }

// fencedBlocks returns the contents of the fenced blocks whose opening fence
// names the given language, which is empty for a fence naming none.
func fencedBlocks(text, language string) []string {
	var blocks []string
	var block []string
	inside, wanted := false, false
	for _, line := range strings.Split(text, "\n") {
		if named, fence := strings.CutPrefix(line, "```"); fence {
			switch {
			case inside && wanted:
				blocks = append(blocks, strings.Join(block, "\n"))
			case !inside:
				wanted = named == language
			}
			inside, block = !inside, nil
			continue
		}
		if inside {
			block = append(block, line)
		}
	}
	return blocks
}

// exampleSource returns the source of every example file, stripped, so that a
// block indented as the README has it matches one indented as Go has it.
func exampleSource(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, dir := range []string{".", "gotenon"} {
		names, err := filepath.Glob(filepath.Join(dir, "example*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			source, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			b.WriteString(stripped(string(source)))
			b.WriteString("\n\x00\n") // nothing spans two files
		}
	}
	if b.Len() == 0 {
		t.Fatal("no example files were found")
	}
	return b.String()
}

// stripped returns text with the indentation of every line removed, since the
// README holds at the margin what Go holds inside a function.
func stripped(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(strings.TrimLeft(line, " \t"), " \t")
	}
	return strings.Join(lines, "\n")
}
