// Command growth runs the module's benchmarks once and reads how what they
// allocate grows with their size.
//
// Usage:
//
//	growth [flags] [packages]
//
// It runs go test -bench over the packages, ./... by default, with -benchmem
// and -count=1, and prints what go test prints as it runs. It then reads the
// pairs among the results: a benchmark measured at a size and at four times
// that size, the size being the last element of its name, as in
// BenchmarkSetListings/decode/2000 and BenchmarkSetListings/decode/8000. Work
// in proportion to its input allocates about four times as much at four times
// the size, and work in proportion to the square of it sixteen times. Where
// the larger allocates more than five times the bytes of the smaller, or makes
// more than five times its allocations, the run fails.
//
// Allocation is what decides because the same code allocates the same however
// loaded the machine is, where its time can move tenfold from one run to the
// next. Time is reported beside it and decides nothing.
//
// A benchmark whose name ends in a size must have a partner at four times or a
// quarter of that size, so that renaming one of a pair cannot leave it unread.
// The size is a decimal number standing as a name element of its own, as
// go test -bench prints it: BenchmarkSetListings/decode/2000-8 is size 2000
// run with GOMAXPROCS at 8. A benchmark whose name does not end in a size is
// run and not read.
//
// The -summary flag names a file to append a Markdown table of the pairs to,
// by default $GITHUB_STEP_SUMMARY, which is where GitHub Actions shows it on
// the run's page.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("growth: ")
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

// run runs the benchmarks the arguments select and reads the pairs among
// them. What go test prints goes to stdout as it arrives, and the reading
// follows it; what go test writes to its standard error goes to stderr.
func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("growth", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bench := flags.String("bench", ".", "run the benchmarks matching `regexp`, as go test -bench does")
	benchtime := flags.String("benchtime", "", "run each benchmark for `d`, or Nx times, as go test -benchtime does; go test's own default if empty")
	summary := flags.String("summary", os.Getenv("GITHUB_STEP_SUMMARY"), "append a Markdown table of the pairs to `file`; defaults to $GITHUB_STEP_SUMMARY")
	if err := flags.Parse(args); err != nil {
		return err
	}
	packages := flags.Args()
	if len(packages) == 0 {
		packages = []string{"./..."}
	}
	goArgs := []string{"test", "-run=^$", "-bench=" + *bench, "-benchmem", "-count=1"}
	if *benchtime != "" {
		goArgs = append(goArgs, "-benchtime="+*benchtime)
	}
	cmd := exec.Command("go", append(goArgs, packages...)...)
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	results, readErr := parse(io.TeeReader(out, stdout))
	// Whatever a failed read left behind is still copied out, so that go test
	// is never left blocked writing it.
	if _, err := io.Copy(stdout, out); readErr == nil {
		readErr = err
	}
	testErr := cmd.Wait()
	if readErr != nil {
		return fmt.Errorf("reading what go test printed: %w", readErr)
	}

	pairs, unread, problems := pairUp(results)
	readings := read(pairs)
	fmt.Fprintln(stdout)
	if err := report(stdout, readings, unread); err != nil {
		return err
	}
	if *summary != "" {
		if err := appendSummary(*summary, readings, unread); err != nil {
			return err
		}
	}
	if testErr != nil {
		problems = append(problems, fmt.Sprintf("go test failed: %v", testErr))
	}
	failed := 0
	for _, r := range readings {
		if msgs := r.overLimit(); len(msgs) > 0 {
			failed++
			problems = append(problems, msgs...)
		}
	}
	switch {
	case len(readings) == 0:
		problems = append(problems, "no benchmark was measured at a size and at four times it")
	case failed > 0:
		problems = append(problems, fmt.Sprintf("%d of %d pairs allocate more than five times as much at four times the size", failed, len(readings)))
	}
	if len(problems) == 0 {
		fmt.Fprintf(stdout, "growth: %s read, none allocating more than five times as much at four times the size\n", count(len(readings), "pair"))
		return nil
	}
	for _, p := range problems[:len(problems)-1] {
		fmt.Fprintf(stdout, "growth: %s\n", p)
	}
	return errors.New(problems[len(problems)-1])
}

// count gives n of a thing, as in 1 pair and 2 pairs.
func count(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
