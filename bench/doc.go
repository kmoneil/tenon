// Package bench measures what tenon costs beside what a program would
// otherwise use: encoding/json's map[string]any, the floor, and go-cty, the
// value system tenon's design answers. Each benchmark does one thing a program
// does with a configuration document, at three sizes, in each library that
// does it.
//
// It is a module of its own, so that go-cty never becomes a dependency of
// tenon's. make bench, from the repository's root, runs it and writes
// BENCHMARKS.md, and the summary in the README, from the results; that file
// says what each benchmark does and how to read what it finds.
package bench
