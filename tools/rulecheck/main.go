// Command rulecheck keeps the conformance suite in step with the rule
// identifiers of the tenon specification.
//
// Usage:
//
//	rulecheck [check] [flags]    verify the rule manifest; run by make check
//	rulecheck manifest [flags]   regenerate the rule manifest
//
// The specification is read from the file named by -spec, which defaults to
// the TENON_SPEC environment variable. Without one, check skips the manifest
// freshness test and says so, and manifest refuses to run.
//
// The manifest, conformance/rules.json, lists every rule identifier in the
// specification with its area and whether the rule is withdrawn or belongs to
// an outline section. It is derived from the specification's Markdown by these
// conventions:
//
//   - A rule identifier is written `[XX-nnn]`, backticks included, where XX is
//     an area prefix listed in the table under the "Rule identifiers" heading.
//     Any other text shaped like an identifier is an error.
//   - A rule is defined by the paragraph that begins with its identifier, and
//     every other occurrence is a reference. A rule is defined at most once,
//     and a rule referenced from a normative section must be defined.
//   - Numbered sections ("## 2. Title") are normative unless the heading
//     carries *(outline)*. A rule defined outside the normative sections, or
//     never defined and referenced only outside them, is recorded as outline,
//     and its coverage is not enforced.
//   - A defining paragraph whose identifier is followed by *(withdrawn)* marks
//     the rule withdrawn.
//
// Rule identifiers are permanent. Both commands fail if a rule in the existing
// manifest has disappeared from the specification or a withdrawn rule has been
// reinstated.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("rulecheck: ")
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run executes the command named by the first argument, or check if there is
// none.
func run(args []string, stdout io.Writer) error {
	cmd := "check"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	if cmd != "check" && cmd != "manifest" {
		return fmt.Errorf("unknown command %q; want check or manifest", cmd)
	}
	flags := flag.NewFlagSet("rulecheck "+cmd, flag.ContinueOnError)
	specPath := flags.String("spec", os.Getenv("TENON_SPEC"), "specification `file`; defaults to $TENON_SPEC")
	manifestPath := flags.String("manifest", "", "rule manifest `file`; defaults to conformance/rules.json in the module root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *manifestPath == "" {
		root, err := moduleRoot()
		if err != nil {
			return err
		}
		*manifestPath = filepath.Join(root, "conformance", "rules.json")
	}
	if cmd == "manifest" {
		return runManifest(stdout, *specPath, *manifestPath)
	}
	return runCheck(stdout, *specPath, *manifestPath)
}

// runCheck verifies the manifest file and, given a specification, that the
// file is exactly what runManifest would write.
func runCheck(w io.Writer, specPath, manifestPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	committed, err := decodeManifest(data)
	if err != nil {
		return fmt.Errorf("%s: %w", display(manifestPath), err)
	}
	if specPath == "" {
		fmt.Fprintf(w, "rulecheck: TENON_SPEC not set; skipping the manifest freshness check (%s)\n", summarize(committed))
		return nil
	}
	rules, err := loadSpec(specPath)
	if err != nil {
		return err
	}
	if err := checkPermanence(committed, rules); err != nil {
		return err
	}
	fresh, err := encodeManifest(rules)
	if err != nil {
		return err
	}
	if !bytes.Equal(fresh, data) {
		return fmt.Errorf("%s is stale; regenerate it with `make rules`:\n%s", display(manifestPath), describeChanges(committed, rules))
	}
	fmt.Fprintf(w, "rulecheck: manifest up to date (%s)\n", summarize(rules))
	return nil
}

// runManifest regenerates the manifest file from the specification.
func runManifest(w io.Writer, specPath, manifestPath string) error {
	if specPath == "" {
		return errors.New("no specification: set TENON_SPEC or pass -spec")
	}
	rules, err := loadSpec(specPath)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(manifestPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		old, err := decodeManifest(existing)
		if err != nil {
			return fmt.Errorf("%s: %w", display(manifestPath), err)
		}
		if err := checkPermanence(old, rules); err != nil {
			return err
		}
	}
	data, err := encodeManifest(rules)
	if err != nil {
		return err
	}
	if bytes.Equal(data, existing) {
		fmt.Fprintf(w, "rulecheck: %s already up to date (%s)\n", display(manifestPath), summarize(rules))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "rulecheck: wrote %s (%s)\n", display(manifestPath), summarize(rules))
	return nil
}

// moduleRoot returns the nearest directory at or above the working directory
// that contains a go.mod file.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod found at or above the working directory")
		}
		dir = parent
	}
}

// display shortens path to be relative to the working directory when it lies
// beneath it.
func display(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
