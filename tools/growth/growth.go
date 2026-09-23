package main

import (
	"bufio"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

// A result is what go test reports for one benchmark: the nanoseconds, the
// bytes allocated and the allocations made, each per operation.
type result struct {
	pkg               string // the import path of its package
	name              string // its name, without the -N go test adds for GOMAXPROCS
	size              uint64 // the size its name ends in, or 0 where it ends in none
	ns, bytes, allocs *big.Rat
}

// base is the result's name without its size, the name the two of a pair
// share.
func (r result) base() string {
	if r.size == 0 {
		return r.name
	}
	return r.name[:strings.LastIndexByte(r.name, '/')]
}

// parse reads what go test -bench -benchmem prints, returning the results in
// the order they were reported. A line that is not a result is passed over,
// and a result belongs to the package the last "pkg:" line before it names.
func parse(r io.Reader) ([]result, error) {
	var results []result
	pkg := ""
	lines := bufio.NewReader(r)
	for {
		line, err := lines.ReadString('\n')
		if p, ok := strings.CutPrefix(line, "pkg: "); ok {
			pkg = strings.TrimSpace(p)
		} else if res, ok := parseResult(line); ok {
			res.pkg = pkg
			results = append(results, res)
		}
		if err == io.EOF {
			return results, nil
		}
		if err != nil {
			return results, err
		}
	}
}

// parseResult reads a line reporting a benchmark, as in
//
//	BenchmarkSetListings/decode/2000-8  1902  633716 ns/op  675290 B/op  18077 allocs/op
//
// where 1902 is the number of operations the figures average over. Other
// value and unit pairs, such as the MB/s of a benchmark that sets its bytes,
// may stand among them.
func parseResult(line string) (result, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || len(fields)%2 != 0 || !strings.HasPrefix(fields[0], "Benchmark") {
		return result{}, false
	}
	if _, err := strconv.ParseUint(fields[1], 10, 64); err != nil {
		return result{}, false
	}
	var r result
	r.name, r.size = nameAndSize(fields[0])
	for i := 2; i < len(fields); i += 2 {
		v, ok := new(big.Rat).SetString(fields[i])
		if !ok {
			return result{}, false
		}
		switch fields[i+1] {
		case "ns/op":
			r.ns = v
		case "B/op":
			r.bytes = v
		case "allocs/op":
			r.allocs = v
		}
	}
	return r, r.ns != nil && r.bytes != nil && r.allocs != nil
}

// nameAndSize reads the size a reported benchmark name ends in, taking off
// the -N that go test adds where GOMAXPROCS is not one. A name that does not
// end in a size is given back as it was reported, with a size of 0.
func nameAndSize(reported string) (string, uint64) {
	i := strings.LastIndexByte(reported, '/')
	if i < 0 {
		return reported, 0
	}
	digits, procs, hasProcs := strings.Cut(reported[i+1:], "-")
	size, err := strconv.ParseUint(digits, 10, 62)
	if err != nil || size == 0 {
		return reported, 0
	}
	if _, err := strconv.ParseUint(procs, 10, 64); hasProcs && err != nil {
		return reported, 0
	}
	return reported[:i+1] + digits, size
}

// A pair is one benchmark measured at a size and at four times that size.
type pair struct{ small, large result }

// pairUp finds the pairs among the results, in the order the smaller of each
// was reported. It counts the results whose names end in no size, which are
// not read, and gives a problem for each result that ends in a size and has
// no partner.
func pairUp(results []result) (pairs []pair, unread int, problems []string) {
	type key struct {
		pkg, base string
		size      uint64
	}
	at := map[key]int{}
	var sized []int
	for i, r := range results {
		if r.size == 0 {
			unread++
			continue
		}
		k := key{r.pkg, r.base(), r.size}
		if _, ok := at[k]; ok {
			problems = append(problems, fmt.Sprintf("%s in %s is reported twice", r.name, r.pkg))
			continue
		}
		at[k] = i
		sized = append(sized, i)
	}
	paired := make([]bool, len(results))
	for _, i := range sized {
		r := results[i]
		if j, ok := at[key{r.pkg, r.base(), 4 * r.size}]; ok {
			pairs = append(pairs, pair{r, results[j]})
			paired[i], paired[j] = true, true
		}
	}
	for _, i := range sized {
		if !paired[i] {
			problems = append(problems, fmt.Sprintf("%s in %s has no partner at four times or a quarter of its size", results[i].name, results[i].pkg))
		}
	}
	return pairs, unread, problems
}

// limit is how many times what a benchmark allocates at a size it may
// allocate at four times that size, in bytes and in allocations. Work in
// proportion to its input reads about four, and work in proportion to the
// square of it sixteen.
var limit = big.NewRat(5, 1)

// A reading is how a pair grows from its size to four times it: what the
// larger reads in each measure, as a multiple of what the smaller reads.
type reading struct {
	pkg, bench   string // the package's short name, and the name the pair shares
	small, large result
	// Each is nil where the smaller reads nothing and the larger something.
	bytes, allocs, time *big.Rat
}

// read reads the pairs.
func read(pairs []pair) []reading {
	names := shortNames(pairs)
	readings := make([]reading, len(pairs))
	for i, p := range pairs {
		readings[i] = reading{
			pkg:    names[p.small.pkg],
			bench:  p.small.base(),
			small:  p.small,
			large:  p.large,
			bytes:  grows(p.small.bytes, p.large.bytes),
			allocs: grows(p.small.allocs, p.large.allocs),
			time:   grows(p.small.ns, p.large.ns),
		}
	}
	return readings
}

// shortNames names each package by its path from the directory holding the
// packages' common root, so that a module's packages read as tenon,
// tenon/gotenon and tenon/internal/uni.
func shortNames(pairs []pair) map[string]string {
	var root []string
	for i, p := range pairs {
		elems := strings.Split(p.small.pkg, "/")
		if i == 0 {
			root = elems
			continue
		}
		n := 0
		for n < len(root) && n < len(elems) && root[n] == elems[n] {
			n++
		}
		root = root[:n]
	}
	names := map[string]string{}
	for _, p := range pairs {
		names[p.small.pkg] = strings.Join(strings.Split(p.small.pkg, "/")[max(len(root)-1, 0):], "/")
	}
	return names
}

// grows is large as a multiple of small: one where both are nothing, and nil
// where only small is.
func grows(small, large *big.Rat) *big.Rat {
	switch {
	case small.Sign() != 0:
		return new(big.Rat).Quo(large, small)
	case large.Sign() == 0:
		return big.NewRat(1, 1)
	}
	return nil
}

// beyond reports whether a growth is past the limit, as growth from nothing
// is.
func beyond(g *big.Rat) bool {
	return g == nil || g.Cmp(limit) > 0
}

// name is how a reading names its pair.
func (r reading) name() string {
	if r.pkg == "" {
		return r.bench
	}
	return r.pkg + " " + r.bench
}

// overLimit describes each measure in which the reading grows past the limit.
func (r reading) overLimit() []string {
	var msgs []string
	for _, m := range []struct {
		verb, what, unit     string
		growth, small, large *big.Rat
	}{
		{"allocates", "bytes", "B/op", r.bytes, r.small.bytes, r.large.bytes},
		{"makes", "allocations", "allocs/op", r.allocs, r.small.allocs, r.large.allocs},
	} {
		switch {
		case !beyond(m.growth):
		case m.growth == nil:
			msgs = append(msgs, fmt.Sprintf("%s %s %s %s at %d and none at %d",
				r.name(), m.verb, m.large.RatString(), m.unit, r.large.size, r.small.size))
		default:
			msgs = append(msgs, fmt.Sprintf("%s %s %s times the %s at %d as at %d, %s %s against %s",
				r.name(), m.verb, ratio(m.growth), m.what, r.large.size, r.small.size, m.large.RatString(), m.unit, m.small.RatString()))
		}
	}
	return msgs
}

// ratio shows a growth to two places, rounded up, so that a growth past the
// limit never reads as the limit itself.
func ratio(g *big.Rat) string {
	if g == nil {
		return "from none"
	}
	hundredths := new(big.Int).Mul(g.Num(), big.NewInt(100))
	hundredths.Add(hundredths, new(big.Int).Sub(g.Denom(), big.NewInt(1)))
	hundredths.Quo(hundredths, g.Denom())
	return new(big.Rat).SetFrac(hundredths, big.NewInt(100)).FloatString(2)
}

// report prints the readings, a pair to a line, after a note of how many
// results were not read.
func report(w io.Writer, readings []reading, unread int) error {
	if unread > 0 {
		fmt.Fprintf(w, "growth: %s not read, having no size in the name\n", count(unread, "benchmark"))
	}
	if len(readings) == 0 {
		return nil
	}
	fmt.Fprintln(w, "growth: what each pair reads at four times its size, as a multiple of what it reads at its size; time is reported and decides nothing")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "benchmark\tsizes\tbytes\tallocations\ttime")
	for _, r := range readings {
		fmt.Fprintf(tw, "%s\t%d, %d\t%s\t%s\t%s", r.name(), r.small.size, r.large.size, ratio(r.bytes), ratio(r.allocs), ratio(r.time))
		if beyond(r.bytes) || beyond(r.allocs) {
			fmt.Fprint(tw, "\tover five")
		}
		fmt.Fprintln(tw)
	}
	return tw.Flush()
}

// appendSummary appends the readings to the named file as a Markdown table,
// with each growth past the limit in bold.
func appendSummary(file string, readings []reading, unread int) error {
	var b strings.Builder
	b.WriteString("### Allocation growth\n\n")
	b.WriteString("Each benchmark is measured at a size and at four times that size, and each figure is what it reads at the larger as a multiple of what it reads at the smaller. ")
	b.WriteString("More than five times the bytes or the allocations fails the run; time is reported and decides nothing.\n\n")
	if len(readings) > 0 {
		b.WriteString("| Benchmark | Sizes | Bytes | Allocations | Time |\n")
		b.WriteString("| :-- | --: | --: | --: | --: |\n")
		for _, r := range readings {
			bench := "`" + strings.ReplaceAll(r.bench, "|", `\|`) + "`"
			if r.pkg != "" {
				bench = r.pkg + " " + bench
			}
			fmt.Fprintf(&b, "| %s | %d, %d | %s | %s | %s |\n", bench, r.small.size, r.large.size,
				judged(r.bytes), judged(r.allocs), ratio(r.time))
		}
		b.WriteString("\n")
	}
	if unread > 0 {
		fmt.Fprintf(&b, "%s not read, having no size in the name.\n\n", count(unread, "benchmark"))
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(b.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// judged shows a growth to two places, in bold where it is past the limit.
func judged(g *big.Rat) string {
	if beyond(g) {
		return "**" + ratio(g) + "**"
	}
	return ratio(g)
}
