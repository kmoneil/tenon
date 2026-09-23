package main

import (
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// printed is what go test -bench -benchmem prints over two packages, with the
// lines around the results that are not results: a benchmark's name printed
// before a line it logged, a failure, and the lines that end a package.
const printed = `goos: linux
goarch: arm64
pkg: example.com/m
cpu: a machine
BenchmarkDecode/2000-8   	    1902	    633716 ns/op	  675290 B/op	   18077 allocs/op
BenchmarkDecode/8000-8   	     458	   3670663 ns/op	 2795371 B/op	   72163 allocs/op
BenchmarkNamed/250-8     	   14390	     82504 ns/op	  30.56 MB/s	   99128 B/op	    2433 allocs/op
BenchmarkWide/add-8      	      45	  26109527 ns/op	 2141820 B/op	      32 allocs/op
BenchmarkPlain-8         	 1773242	       626.0 ns/op	     848 B/op	      20 allocs/op
BenchmarkLogs/100
    x_test.go:12: a line a benchmark logged
BenchmarkLogs/100-8      	     100	     143.8 ns/op	    1024 B/op	       1 allocs/op
--- FAIL: BenchmarkBroken/2000
    x_test.go:9: the document did not decode
FAIL
exit status 1
FAIL	example.com/m	55.235s
pkg: example.com/m/sub
BenchmarkSolo/10         	     100	       1.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkTimeOnly/10-8   	     100	       1.5 ns/op
PASS
ok  	example.com/m/sub	0.007s
`

func TestParse(t *testing.T) {
	results, err := parse(strings.NewReader(printed))
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		pkg, name         string
		size              uint64
		ns, bytes, allocs string
	}
	want := []row{
		{"example.com/m", "BenchmarkDecode/2000", 2000, "633716", "675290", "18077"},
		{"example.com/m", "BenchmarkDecode/8000", 8000, "3670663", "2795371", "72163"},
		{"example.com/m", "BenchmarkNamed/250", 250, "82504", "99128", "2433"},
		{"example.com/m", "BenchmarkWide/add-8", 0, "26109527", "2141820", "32"},
		{"example.com/m", "BenchmarkPlain-8", 0, "626", "848", "20"},
		{"example.com/m", "BenchmarkLogs/100", 100, "719/5", "1024", "1"},
		// Run with GOMAXPROCS at one, go test adds no -N.
		{"example.com/m/sub", "BenchmarkSolo/10", 10, "3/2", "0", "0"},
	}
	var got []row
	for _, r := range results {
		got = append(got, row{r.pkg, r.name, r.size, r.ns.RatString(), r.bytes.RatString(), r.allocs.RatString()})
	}
	if !slices.Equal(got, want) {
		t.Errorf("parse read\n%v\nwant\n%v", got, want)
	}
}

func TestNameAndSize(t *testing.T) {
	for _, tt := range []struct {
		reported, name string
		size           uint64
	}{
		{"BenchmarkX/2000-8", "BenchmarkX/2000", 2000},
		{"BenchmarkX/2000", "BenchmarkX/2000", 2000},
		{"BenchmarkX/decode/250-12", "BenchmarkX/decode/250", 250},
		// A size is a name element of its own, written in decimal digits.
		{"BenchmarkX-8", "BenchmarkX-8", 0},
		{"BenchmarkX/walk1-8", "BenchmarkX/walk1-8", 0},
		{"BenchmarkX/decode-2000", "BenchmarkX/decode-2000", 0},
		{"BenchmarkX/1e3-8", "BenchmarkX/1e3-8", 0},
		{"BenchmarkX/+5-8", "BenchmarkX/+5-8", 0},
		{"BenchmarkX/-8", "BenchmarkX/-8", 0},
		{"BenchmarkX/2000-a", "BenchmarkX/2000-a", 0},
		{"BenchmarkX/0-8", "BenchmarkX/0-8", 0},
	} {
		if name, size := nameAndSize(tt.reported); name != tt.name || size != tt.size {
			t.Errorf("nameAndSize(%q) = %q, %d; want %q, %d", tt.reported, name, size, tt.name, tt.size)
		}
	}
}

// measured is a result of a benchmark in a package at a size, allocating the
// bytes and making the allocations given.
func measured(pkg, name string, size uint64, bytes, allocs int64) result {
	return result{pkg: pkg, name: name, size: size, ns: big.NewRat(1, 1), bytes: big.NewRat(bytes, 1), allocs: big.NewRat(allocs, 1)}
}

func TestPairUp(t *testing.T) {
	results := []result{
		measured("a", "BenchmarkX/1000", 1000, 1, 1),
		measured("a", "BenchmarkX/4000", 4000, 1, 1),
		measured("a", "BenchmarkX/16000", 16000, 1, 1),
		measured("a", "BenchmarkY/decode/10", 10, 1, 1),
		measured("a", "BenchmarkY/decode/10", 10, 1, 1),
		measured("a", "BenchmarkY/decode/40", 40, 1, 1),
		measured("a", "BenchmarkZ/7", 7, 1, 1),
		measured("a", "BenchmarkV/100", 100, 1, 1),
		measured("a", "BenchmarkV/200", 200, 1, 1),
		measured("a", "BenchmarkW/add-8", 0, 1, 1),
		// The same benchmark in another package is another benchmark.
		measured("b", "BenchmarkX/1000", 1000, 1, 1),
		measured("b", "BenchmarkX/4000", 4000, 1, 1),
		measured("b", "BenchmarkU/40", 40, 1, 1),
	}
	pairs, unread, problems := pairUp(results)
	var got []string
	for _, p := range pairs {
		got = append(got, p.small.pkg+" "+p.small.name+" "+p.large.name)
	}
	want := []string{
		"a BenchmarkX/1000 BenchmarkX/4000",
		"a BenchmarkX/4000 BenchmarkX/16000",
		"a BenchmarkY/decode/10 BenchmarkY/decode/40",
		"b BenchmarkX/1000 BenchmarkX/4000",
	}
	if !slices.Equal(got, want) {
		t.Errorf("pairUp paired %q, want %q", got, want)
	}
	if unread != 1 {
		t.Errorf("pairUp left %d unread, want 1", unread)
	}
	wantProblems := []string{
		"BenchmarkY/decode/10 in a is reported twice",
		"BenchmarkZ/7 in a has no partner at four times or a quarter of its size",
		"BenchmarkV/100 in a has no partner at four times or a quarter of its size",
		"BenchmarkV/200 in a has no partner at four times or a quarter of its size",
		"BenchmarkU/40 in b has no partner at four times or a quarter of its size",
	}
	if !slices.Equal(problems, wantProblems) {
		t.Errorf("pairUp found\n%q\nwant\n%q", problems, wantProblems)
	}
}

func TestShortNames(t *testing.T) {
	for _, tt := range []struct {
		pkgs, want []string
	}{
		{[]string{"example.com/m", "example.com/m/sub", "example.com/m/internal/x"}, []string{"m", "m/sub", "m/internal/x"}},
		{[]string{"example.com/m/internal/x"}, []string{"x"}},
		{[]string{"a/b", "c/d"}, []string{"a/b", "c/d"}},
		{[]string{""}, []string{""}},
	} {
		var pairs []pair
		for _, pkg := range tt.pkgs {
			pairs = append(pairs, pair{small: result{pkg: pkg}})
		}
		names := shortNames(pairs)
		for i, pkg := range tt.pkgs {
			if names[pkg] != tt.want[i] {
				t.Errorf("among %q, %q is named %q, want %q", tt.pkgs, pkg, names[pkg], tt.want[i])
			}
		}
	}
}

// grown reads one pair in package m, at 1000 and 4000, whose readings at the
// smaller and the larger size are the ones given.
func grown(smallBytes, largeBytes, smallAllocs, largeAllocs int64) reading {
	return read([]pair{{
		measured("example.com/m", "BenchmarkX/1000", 1000, smallBytes, smallAllocs),
		measured("example.com/m", "BenchmarkX/4000", 4000, largeBytes, largeAllocs),
	}})[0]
}

func TestOverLimit(t *testing.T) {
	for _, tt := range []struct {
		what string
		r    reading
		want []string
	}{
		{"four times", grown(1000, 4000, 10, 40), nil},
		{"five times exactly", grown(1000, 5000, 10, 50), nil},
		{"nothing at either size", grown(0, 0, 0, 0), nil},
		{"less at the larger size", grown(1000, 10, 10, 1), nil},
		{"just over five times the bytes", grown(1000, 5001, 10, 50), []string{
			"m BenchmarkX allocates 5.01 times the bytes at 4000 as at 1000, 5001 B/op against 1000",
		}},
		{"sixteen times the allocations", grown(1000, 4000, 10, 160), []string{
			"m BenchmarkX makes 16.00 times the allocations at 4000 as at 1000, 160 allocs/op against 10",
		}},
		{"something from nothing", grown(0, 32, 0, 2), []string{
			"m BenchmarkX allocates 32 B/op at 4000 and none at 1000",
			"m BenchmarkX makes 2 allocs/op at 4000 and none at 1000",
		}},
	} {
		if got := tt.r.overLimit(); !slices.Equal(got, tt.want) {
			t.Errorf("%s: overLimit gave %q, want %q", tt.what, got, tt.want)
		}
	}
}

func TestReport(t *testing.T) {
	readings := []reading{grown(1000, 4000, 10, 40), grown(1000, 16000, 10, 40)}
	var b strings.Builder
	if err := report(&b, readings, 2); err != nil {
		t.Fatal(err)
	}
	want := `growth: 2 benchmarks not read, having no size in the name
growth: what each pair reads at four times its size, as a multiple of what it reads at its size; time is reported and decides nothing
benchmark     sizes       bytes  allocations  time
m BenchmarkX  1000, 4000  4.00   4.00         1.00
m BenchmarkX  1000, 4000  16.00  4.00         1.00  over five
`
	if b.String() != want {
		t.Errorf("report printed\n%s\nwant\n%s", b.String(), want)
	}
}

func TestAppendSummary(t *testing.T) {
	file := filepath.Join(t.TempDir(), "summary.md")
	piped := grown(1000, 16000, 10, 40)
	piped.bench = "BenchmarkX/a|b"
	for range 2 {
		if err := appendSummary(file, []reading{grown(1000, 4000, 0, 0), piped}, 1); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	table := "| Benchmark | Sizes | Bytes | Allocations | Time |\n" +
		"| :-- | --: | --: | --: | --: |\n" +
		"| m `BenchmarkX` | 1000, 4000 | 4.00 | 1.00 | 1.00 |\n" +
		"| m `BenchmarkX/a\\|b` | 1000, 4000 | **16.00** | 4.00 | 1.00 |\n"
	if strings.Count(string(got), table) != 2 || strings.Count(string(got), "1 benchmark not read") != 2 {
		t.Errorf("two summaries appended gave\n%s\nwant each to hold\n%s", got, table)
	}
}
