// Command relnotes prints the notes of one release: its section of
// CHANGELOG.md, without the heading, for the GitHub Release made from its tag.
//
// Usage:
//
//	relnotes [-changelog file] version
//
// The version is as a tag or a heading writes it, v0.6.0 or 0.6.0. The section
// is what follows the heading "## 0.6.0 (date)" up to the next heading of that
// level, with the blank lines at either end left out. A version the changelog
// has no section for is an error, so a tag cannot be released with no notes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	changelog := flag.String("changelog", "CHANGELOG.md", "the changelog `file` to read")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: relnotes [-changelog file] version")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	data, err := os.ReadFile(*changelog)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relnotes:", err)
		os.Exit(1)
	}
	notes, err := section(string(data), flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "relnotes: %s: %v\n", *changelog, err)
		os.Exit(1)
	}
	fmt.Print(notes)
}

// section returns the notes of version in the changelog text, ending in a
// newline, or an error where the changelog has no section for it or the
// section is empty.
func section(changelog, version string) (string, error) {
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return "", errors.New("no version given")
	}
	heading := "## " + version + " ("
	var lines []string
	found, in := false, false
	for line := range strings.SplitSeq(changelog, "\n") {
		switch {
		case strings.HasPrefix(line, heading):
			if found {
				return "", fmt.Errorf("two sections for %s", version)
			}
			found, in = true, true
			continue
		case in && strings.HasPrefix(line, "## "):
			in = false
		}
		if in {
			lines = append(lines, strings.TrimRight(line, "\r"))
		}
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	switch {
	case !found:
		return "", fmt.Errorf("no section for %s", version)
	case len(lines) == 0:
		return "", fmt.Errorf("the section for %s is empty", version)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
