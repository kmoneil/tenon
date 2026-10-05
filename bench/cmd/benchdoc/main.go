// Command benchdoc writes BENCHMARKS.md, and the summary between the README's
// benchmark markers, from the output of the bench module's benchmarks run
// several times: the median of each measurement, so that one slow run does
// not move a figure. make bench runs it; see the Makefile.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("benchdoc: ")
	in := flag.String("in", "", "benchmark output `file`")
	doc := flag.String("doc", "BENCHMARKS.md", "report `file` to write")
	readme := flag.String("readme", "README.md", "README `file` whose summary block to rewrite")
	gomod := flag.String("gomod", "go.mod", "the bench module's go.mod `file`, for go-cty's version")
	flag.Parse()
	if err := run(*in, *doc, *readme, *gomod); err != nil {
		log.Fatal(err)
	}
}

func run(in, doc, readme, gomod string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	res, err := parse(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	mod, err := os.ReadFile(gomod)
	if err != nil {
		return err
	}
	res.cty = moduleVersion(string(mod), "github.com/zclconf/go-cty")
	res.goVersion = runtime.Version()
	res.commit = commit()
	res.date = time.Now().UTC().Format("2006-01-02")
	if err := os.WriteFile(doc, []byte(report(res)), 0o644); err != nil {
		return err
	}
	text, err := os.ReadFile(readme)
	if err != nil {
		return err
	}
	updated, err := replaceBlock(string(text), summary(res))
	if err != nil {
		return fmt.Errorf("%s: %w", readme, err)
	}
	return os.WriteFile(readme, []byte(updated), 0o644)
}

// key names one benchmark: a workload at a size in a library.
type key struct{ workload, size, lib string }

// sample is one run of one benchmark. Durations are in hundredths of a
// nanosecond, and lengths in hundredths of a byte, so that a figure printed
// with two decimals is held exactly.
type sample struct {
	ns, bytes, allocs, doc int64
}

// results is what the runs measured, and where.
type results struct {
	samples                      map[key][]sample
	cpu, goos, goarch            string
	cty, goVersion, commit, date string
}

var line = regexp.MustCompile(`^Benchmark(\w+)/size=(\w+)/lib=(\w+)-\d+\s+\d+\s+(.*)$`)

// parse reads benchmark output.
func parse(r io.Reader) (*results, error) {
	res := &results{samples: map[key][]sample{}}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		text := sc.Text()
		if v, ok := strings.CutPrefix(text, "cpu: "); ok {
			res.cpu = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(text, "goos: "); ok {
			res.goos = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(text, "goarch: "); ok {
			res.goarch = strings.TrimSpace(v)
		}
		m := line.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		fields := strings.Fields(m[4])
		var s sample
		for i := 0; i+1 < len(fields); i += 2 {
			v, err := hundredths(fields[i])
			if err != nil {
				return nil, fmt.Errorf("%q: %w", text, err)
			}
			switch fields[i+1] {
			case "ns/op":
				s.ns = v
			case "B/op":
				s.bytes = v
			case "allocs/op":
				s.allocs = v
			case "B/doc":
				s.doc = v
			}
		}
		k := key{m[1], m[2], m[3]}
		res.samples[k] = append(res.samples[k], s)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(res.samples) == 0 {
		return nil, errors.New("the input holds no benchmark results")
	}
	return res, nil
}

// hundredths parses a decimal figure as a count of hundredths.
func hundredths(s string) (int64, error) {
	whole, frac, _ := strings.Cut(s, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	frac = (frac + "00")[:2]
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return w*100 + f, nil
}

// median returns the median of the figure that get reads from each sample.
func median(ss []sample, get func(sample) int64) int64 {
	vs := make([]int64, len(ss))
	for i, s := range ss {
		vs[i] = get(s)
	}
	slices.Sort(vs)
	n := len(vs)
	if n%2 == 1 {
		return vs[n/2]
	}
	return (vs[n/2-1] + vs[n/2]) / 2
}

// figure is the median of each measurement of one benchmark.
type figure struct {
	ok                     bool
	ns, bytes, allocs, doc int64
}

func (r *results) figure(workload, size, lib string) figure {
	ss := r.samples[key{workload, size, lib}]
	if len(ss) == 0 {
		return figure{}
	}
	return figure{
		ok:     true,
		ns:     median(ss, func(s sample) int64 { return s.ns }),
		bytes:  median(ss, func(s sample) int64 { return s.bytes }),
		allocs: median(ss, func(s sample) int64 { return s.allocs }),
		doc:    median(ss, func(s sample) int64 { return s.doc }),
	}
}

// scaled writes v, in hundredths of a base unit, to three significant
// figures in the largest of units that leaves at least one whole unit.
func scaled(v int64, units []string, step int64) string {
	u, size := 0, int64(100)
	for u+1 < len(units) && v >= size*step {
		u, size = u+1, size*step
	}
	m := v * 1000 / size // thousandths of the unit
	switch {
	case m >= 100000:
		return strconv.FormatInt((m+500)/1000, 10) + " " + units[u]
	case m >= 10000:
		t := (m + 50) / 100
		return fmt.Sprintf("%d.%d %s", t/10, t%10, units[u])
	}
	t := (m + 5) / 10
	return fmt.Sprintf("%d.%02d %s", t/100, t%100, units[u])
}

func duration(v int64) string { return scaled(v, []string{"ns", "µs", "ms", "s"}, 1000) }
func length(v int64) string {
	if v < 100000 {
		return strconv.FormatInt(v/100, 10) + " B"
	}
	return scaled(v, []string{"B", "KB", "MB", "GB"}, 1000)
}

// count writes a count of allocations, in hundredths, as people read one.
func count(v int64) string {
	n := v / 100
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	return strings.ReplaceAll(scaled(v, []string{"", "k", "M"}, 1000), " ", "")
}

// The workloads, in the order the report gives them, and the libraries.
var (
	workloads = []struct{ name, title, what string }{
		{"Parse", "Parse JSON into a value", "Reads the document's bytes into a value that holds the whole of it."},
		{"Convert", "Convert to a schema", "Checks the parsed value against the document's schema and converts it there: each JSON array to a list, each object of environment variables to a map."},
		{"RoundTrip", "Encode and decode", "Encodes the converted value for another process and decodes it back."},
		{"Equal", "Compare two copies", "Compares two copies of the converted value, each built on its own, with the library's equality."},
		{"Lookup", "Read a nested value", "Reads one environment variable of the middle service."},
		{"Diff", "Diff one change", "Reports what changed between two versions of the converted value that differ in one replica count."},
		{"Cross", "Cross between go-cty and tenon", "Carries the converted value across ctytenon's bridge, from go-cty to tenon and from tenon to go-cty, as a program moving from one to the other a piece at a time does."},
		{"Call", "Call a function", "Calls a two-number function once per service, through each library's own convention: tenon converts the arguments to the parameters' constraints inside the call, where go-cty leaves converting to the caller."},
		{"Library", "Call a library function", "Merges each service's environment variables with two defaults through each library's own merge function."},
	}
	// The libraries the summary compares come first; the rest are the
	// directions of ctytenon's bridge, which the Cross workload measures.
	libs     = []struct{ name, title string }{{"json", "encoding/json"}, {"tenon", "tenon"}, {"cty", "go-cty"}, {"fromcty", "go-cty to tenon"}, {"tocty", "tenon to go-cty"}}
	compared = 3
	sizes    = []struct{ name, title string }{{"1KB", "1 KB"}, {"32KB", "32 KB"}, {"1MB", "1 MB"}}
)

// cell writes one benchmark's figures for a table.
func cell(f figure) string {
	if !f.ok {
		return "–"
	}
	allocs := " allocs"
	if f.allocs/100 == 1 {
		allocs = " alloc"
	}
	return duration(f.ns) + " · " + length(f.bytes) + " · " + count(f.allocs) + allocs
}

// wrap writes text as a paragraph wrapped at 78 columns, and a blank line.
func wrap(b *strings.Builder, text string) {
	col := 0
	for _, w := range strings.Fields(text) {
		n := len([]rune(w))
		switch {
		case col == 0:
		case col+1+n > 78:
			b.WriteString("\n")
			col = 0
		default:
			b.WriteString(" ")
			col++
		}
		b.WriteString(w)
		col += n
	}
	b.WriteString("\n\n")
}

func report(r *results) string {
	var b strings.Builder
	b.WriteString(`# What tenon costs

<!-- Generated by make bench from the bench module; do not edit. -->

These benchmarks measure what tenon costs beside what a program would
otherwise use: ` + "`encoding/json`" + ` decoding into ` + "`map[string]any`" + `, the floor,
and [go-cty](https://github.com/zclconf/go-cty), the value system that
Terraform and HCL use. Each does one thing a program does with a
configuration document, at three sizes, in each library that does it.

`)
	wrap(&b, fmt.Sprintf("Measured on %s, %s/%s, with %s, tenon at %s and go-cty %s, on %s. Each figure is the median of every run of its benchmark: the time one operation takes, the memory it allocates, and how many allocations that takes.",
		r.cpu, r.goos, r.goarch, r.goVersion, r.commit, r.cty, r.date))
	b.WriteString(`The document is a deployment configuration: a list of services, each with a
name, a replica count, a fractional CPU share, a list of ports, a map of
environment variables and a list of tags. The three sizes hold 6, 200 and
6,400 services, about a kilobyte, 32 kilobytes and a megabyte of JSON.

`)
	for _, w := range workloads {
		fmt.Fprintf(&b, "## %s\n\n", w.title)
		wrap(&b, w.what)
		var present []int
		for i, l := range libs {
			for _, s := range sizes {
				if r.figure(w.name, s.name, l.name).ok {
					present = append(present, i)
					break
				}
			}
		}
		b.WriteString("| Size |")
		for _, i := range present {
			fmt.Fprintf(&b, " %s |", libs[i].title)
		}
		b.WriteString("\n| --- |")
		for range present {
			b.WriteString(" --- |")
		}
		b.WriteString("\n")
		for _, s := range sizes {
			fmt.Fprintf(&b, "| %s |", s.title)
			for _, i := range present {
				fmt.Fprintf(&b, " %s |", cell(r.figure(w.name, s.name, libs[i].name)))
			}
			b.WriteString("\n")
		}
		if w.name == "RoundTrip" {
			b.WriteString("\nThe encoding's length:\n\n| Size |")
			for _, i := range present {
				fmt.Fprintf(&b, " %s |", libs[i].title)
			}
			b.WriteString("\n| --- |")
			for range present {
				b.WriteString(" --- |")
			}
			b.WriteString("\n")
			for _, s := range sizes {
				fmt.Fprintf(&b, "| %s |", s.title)
				for _, i := range present {
					f := r.figure(w.name, s.name, libs[i].name)
					if f.ok {
						fmt.Fprintf(&b, " %s |", length(f.doc))
					} else {
						b.WriteString(" – |")
					}
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(fairness)
	b.WriteString(`## Running them

` + "`make bench`" + ` runs every benchmark ten times, which takes some minutes, and
writes this file and the summary in the README from the medians.
` + "`BENCHCOUNT`" + ` sets the number of runs, and ` + "`BENCHTIME`" + ` how long each runs.
Figures from one machine are comparable with each other, and not with
another machine's; run them on a quiet one.
`)
	return b.String()
}

const fairness = `## Reading the figures

- **The same input, each library's usual way.** Every library reads the
  same bytes. encoding/json decodes into ` + "`any`" + ` with ` + "`UseNumber`" + `, so
  numbers keep their text. tenon reads the bytes with ` + "`ParseJSON`" + ` into
  what JSON implies, in one pass, refusing what JSON does not allow, as a
  name given twice. go-cty infers the document's type with ` + "`ImpliedType`" + `
  and unmarshals with it.
- **What tenon does per value.** It parses every number into an exact
  decimal, checks every string is UTF-8 and normalizes it to Normalization
  Form C, interns every type, and records what is known of each value so
  that later operations answer from it. That is most of the distance to
  encoding/json, which does none of it.
- **One schema.** The two value systems convert to the same schema. Its tags
  are a list: tenon converts an array to a set only under the Unsafe policy,
  since a set loses the array's order and duplicates, where go-cty converts
  it freely, so a set would time two different conversions.
- **Different encodings.** encoding/json writes JSON. tenon writes its
  canonical CBOR document, which carries the value's type, once, at its head,
  and each object as its attribute values alone, so a list of like objects
  spends nothing more on their names; it would carry unknown values, marks
  and diagnostics as well, and it reads back identical to the value written.
  go-cty writes msgpack, whose reader must be given the value's type.
- **Equality.** Each copy is built on its own, so no comparison can stop at a
  part the two share: ` + "`reflect.DeepEqual`" + ` for the Go values,
  ` + "`tenon.Equals`" + `, and go-cty's ` + "`Value.Equals`" + `. The copies are
  reused from one iteration to the next, which keeps them in the processor's
  caches for every library alike; neither value system keeps anything
  between comparisons.
- **Function calls.** The two-number function multiplies a service's
  replica count, once per service of the document. tenon's ` + "`Call`" + ` converts
  each argument to its parameter's constraint under the call's policy and
  answers every argument state itself; go-cty's ` + "`Call`" + ` checks conformance
  only, each host converting beforehand, so each measures the whole of its
  own convention. The library call is each library's own merge, the map of
  a service's environment variables after a map of two defaults, as a
  configuration adds settings to a map it was given.
- **Growth.** The sizes are 32 times apart, so work growing faster than the
  document shows as a step of more than 32 between rows. tenon keeps its work
  in proportion to its input (` + "`SECURITY.md`" + ` says where it bounds it), and
  a nightly job fails any of its benchmark pairs whose allocations grow
  faster than the input. Time can still step more at a megabyte, where two copies of the
  document outgrow the processor's caches.

`

// summary is the README's block: the middle size, one row a workload.
func summary(r *results) string {
	var b strings.Builder
	fmt.Fprintf(&b, "For a configuration of 32 KB, measured on %s with %s:\n\n", r.cpu, r.goVersion)
	b.WriteString("| | encoding/json | tenon | go-cty |\n| --- | --- | --- | --- |\n")
	for _, w := range workloads {
		var row strings.Builder
		some := false
		for _, l := range libs[:compared] {
			f := r.figure(w.name, "32KB", l.name)
			if f.ok {
				fmt.Fprintf(&row, " %s |", duration(f.ns))
				some = true
			} else {
				row.WriteString(" – |")
			}
		}
		// A workload none of the three does, as crossing the bridge, is
		// not theirs to compare.
		if some {
			fmt.Fprintf(&b, "| %s |%s\n", w.title, row.String())
		}
	}
	return b.String()
}

const (
	beginMarker = "<!-- benchmarks:begin -->"
	endMarker   = "<!-- benchmarks:end -->"
)

// replaceBlock puts body between the README's benchmark markers.
func replaceBlock(text, body string) (string, error) {
	begin := strings.Index(text, beginMarker)
	end := strings.Index(text, endMarker)
	if begin < 0 || end < begin {
		return "", fmt.Errorf("no %s ... %s block to write the summary into", beginMarker, endMarker)
	}
	return text[:begin+len(beginMarker)] + "\n" + body + text[end:], nil
}

// moduleVersion returns the version go.mod requires of path.
func moduleVersion(gomod, path string) string {
	for _, l := range strings.Split(gomod, "\n") {
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "require")))
		if len(fields) >= 2 && fields[0] == path {
			return fields[1]
		}
	}
	return "(unknown)"
}

// commit names the commit the benchmarks ran at, marked where tenon's own Go
// source held changes beside it; the bench module and the documents it
// writes are not tenon's source.
func commit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "an unknown commit"
	}
	c := strings.TrimSpace(string(out))
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no", "--",
		":(top,glob)**/*.go", ":(top)go.mod", ":(top)go.sum", ":(top,exclude)bench/**").Output()
	if err == nil && len(bytes.TrimSpace(status)) > 0 {
		c += " with changes"
	}
	return c
}
