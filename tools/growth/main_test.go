package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// growth runs the command over the fixture's benchmarks, each run a thousand
// times, with the arguments given and a summary written to a file of its own.
// It returns what the command printed, the summary and the command's error.
//
// What a benchmark allocates once, however many times it runs, is shared out
// among its runs, so the figures can differ from the fixture's own sizes by a
// byte or so, and the tests read the verdicts rather than the digits.
func growth(t *testing.T, args ...string) (printed, summary string, err error) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "summary.md")
	var stdout, stderr bytes.Buffer
	args = append([]string{"-benchtime=1000x", "-summary=" + file}, args...)
	err = run(append(args, "./testdata/fixture"), &stdout, &stderr)
	written, readErr := os.ReadFile(file)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return stdout.String() + stderr.String(), string(written), err
}

// wantPrinted fails t unless each of the lines was printed, where a line
// ending in "..." stands for any line that begins with the text before it.
func wantPrinted(t *testing.T, printed string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		prefix, open := strings.CutSuffix(line, "...")
		if !slices.ContainsFunc(strings.Split(printed, "\n"), func(l string) bool {
			return l == line || open && strings.HasPrefix(l, prefix)
		}) {
			t.Errorf("growth printed\n%s\nwant a line %q", printed, line)
		}
	}
}

// row is the line that begins with the pair's name in the table printed.
func row(printed, name string) string {
	for l := range strings.SplitSeq(printed, "\n") {
		if strings.HasPrefix(l, name+" ") {
			return l
		}
	}
	return ""
}

func TestRunPassesWorkInProportion(t *testing.T) {
	printed, summary, err := growth(t, "-bench=Linear")
	if err != nil {
		t.Fatalf("growth over a linear pair failed: %v\n%s", err, printed)
	}
	// What go test printed, passed through, and then what was read of it.
	wantPrinted(t, printed,
		"BenchmarkLinear/1000...",
		"BenchmarkLinear/4000...",
		"fixture BenchmarkLinear  1000, 4000  ...",
		"growth: 1 pair read, none allocating more than five times as much at four times the size",
	)
	if r := row(printed, "fixture BenchmarkLinear"); strings.HasSuffix(r, "over five") {
		t.Errorf("the linear pair reads as over the limit: %q", r)
	}
	if !strings.Contains(summary, "| fixture `BenchmarkLinear` | 1000, 4000 | ") || strings.Contains(summary, "**") {
		t.Errorf("the summary is\n%s\nwant it to hold the linear pair, and nothing in bold", summary)
	}
}

func TestRunFailsWorkInProportionToTheSquare(t *testing.T) {
	printed, summary, err := growth(t)
	if err == nil || err.Error() != "1 of 2 pairs allocate more than five times as much at four times the size" {
		t.Fatalf("growth over a linear pair and a square gave %v\n%s", err, printed)
	}
	wantPrinted(t, printed, "growth: fixture BenchmarkSquare allocates ...")
	if r := row(printed, "fixture BenchmarkSquare"); !strings.HasSuffix(r, "over five") {
		t.Errorf("the square reads as %q, want it over the limit", r)
	}
	if r := row(printed, "fixture BenchmarkLinear"); r == "" || strings.HasSuffix(r, "over five") {
		t.Errorf("beside the square, the linear pair reads as %q", r)
	}
	if !strings.Contains(summary, "| fixture `BenchmarkSquare` | 100, 400 | **") {
		t.Errorf("the summary is\n%s\nwant it to hold the square's bytes in bold", summary)
	}
}

func TestRunFailsWhatItCannotRead(t *testing.T) {
	// One of a pair on its own.
	printed, _, err := growth(t, "-bench=Linear/1000$")
	if err == nil || err.Error() != "no benchmark was measured at a size and at four times it" {
		t.Errorf("growth over half a pair gave %v\n%s", err, printed)
	}
	wantPrinted(t, printed, "growth: BenchmarkLinear/1000 in github.com/kmoneil/tenon/tools/growth/testdata/fixture has no partner at four times or a quarter of its size")

	// A package go test cannot build.
	var stdout, stderr bytes.Buffer
	err = run([]string{"-summary=", "./testdata/missing"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("growth over a missing package succeeded\n%s", stdout.String())
	}
	wantPrinted(t, stdout.String(), "growth: go test failed: exit status 1")
}
