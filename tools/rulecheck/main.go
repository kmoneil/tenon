// Command rulecheck keeps the conformance suite in step with the rule
// identifiers of the tenon specification.
//
// Usage:
//
//	rulecheck [check] [flags]    verify the rule manifest, the appendix of
//	                             diagnostic codes, and rule coverage
//	rulecheck manifest [flags]   regenerate the rule manifest
//	rulecheck codes [flags]      regenerate the appendix of diagnostic codes
//	rulecheck report [flags]     regenerate the conformance report
//
// check is the last step of make check. It verifies that the manifest and the
// specification's appendix of diagnostic codes are current, when it can read
// the specification, and then that every rule enforced by
// conformance/active-areas.txt was covered by a passing test in the preceding
// test run (see package conformance), and that the conformance report,
// CONFORMANCE.md, is what report would write from them. The -h flag lists the
// flags that override its inputs.
//
// The specification is read from the file named by -spec, which defaults to
// the TENON_SPEC environment variable. Without one, check skips the tests that
// need it and says so, and manifest and codes refuse to run.
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
// Rule identifiers are permanent. Both check and manifest fail if a rule in the
// existing manifest has disappeared from the specification or a withdrawn rule
// has been reinstated.
//
// The appendix of diagnostic codes, the section headed "Appendix D: diagnostic
// codes", lists every code that the registry file, codes.go by default,
// declares as a string constant, with the rules that name it. A code is written
// in backticks, as in `number.divide_by_zero`, and belongs to the rule whose
// definition most recently precedes it in its section. The registry and the
// codes that normative sections name must be the same codes, and any other
// backticked text in those sections must not be shaped like a code.
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

// paths locates the files that rulecheck reads and writes.
type paths struct {
	spec     string // the specification
	manifest string // the rule manifest
	active   string // the list of enforced coverage
	cover    string // the directory of coverage records
	codes    string // the registry of diagnostic codes
	record   string // the committed record of codes ever declared
	report   string // the conformance report
}

// run executes the command named by the first argument, or check if there is
// none.
func run(args []string, stdout io.Writer) error {
	cmd := "check"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	if cmd != "check" && cmd != "manifest" && cmd != "codes" && cmd != "report" {
		return fmt.Errorf("unknown command %q; want check, manifest, codes or report", cmd)
	}
	var p paths
	flags := flag.NewFlagSet("rulecheck "+cmd, flag.ContinueOnError)
	flags.StringVar(&p.spec, "spec", os.Getenv("TENON_SPEC"), "specification `file`; defaults to $TENON_SPEC")
	flags.StringVar(&p.manifest, "manifest", "", "rule manifest `file`; defaults to conformance/rules.json in the module root")
	flags.StringVar(&p.active, "active", "", "enforced coverage `file`; defaults to conformance/active-areas.txt in the module root")
	flags.StringVar(&p.cover, "cover", os.Getenv(coverEnv), "coverage record `directory`; defaults to $"+coverEnv+", else .rulecov in the module root")
	flags.StringVar(&p.codes, "codes", "", "diagnostic code registry `file`; defaults to codes.go in the module root")
	flags.StringVar(&p.record, "coderecord", "", "committed code record `file`; defaults to conformance/codes.json in the module root")
	flags.StringVar(&p.report, "report", "", "conformance report `file`; defaults to CONFORMANCE.md in the module root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if p.manifest == "" || p.active == "" || p.cover == "" || p.codes == "" || p.record == "" || p.report == "" {
		root, err := moduleRoot()
		if err != nil {
			return err
		}
		if p.manifest == "" {
			p.manifest = filepath.Join(root, "conformance", "rules.json")
		}
		if p.active == "" {
			p.active = filepath.Join(root, "conformance", "active-areas.txt")
		}
		if p.cover == "" {
			p.cover = filepath.Join(root, ".rulecov")
		}
		if p.codes == "" {
			p.codes = filepath.Join(root, "codes.go")
		}
		if p.record == "" {
			p.record = filepath.Join(root, "conformance", "codes.json")
		}
		if p.report == "" {
			p.report = filepath.Join(root, "CONFORMANCE.md")
		}
	}
	switch cmd {
	case "manifest":
		return runManifest(stdout, p)
	case "codes":
		return runCodes(stdout, p)
	case "report":
		return runReport(stdout, p)
	}
	return runCheck(stdout, p)
}

// runCheck verifies the manifest file, then the coverage it enforces.
func runCheck(w io.Writer, p paths) error {
	data, err := os.ReadFile(p.manifest)
	if err != nil {
		return err
	}
	m, err := decodeManifest(data)
	if err != nil {
		return fmt.Errorf("%s: %w", display(p.manifest), err)
	}
	if err := checkFresh(w, p, data, m); err != nil {
		return err
	}
	if err := checkCodes(w, p); err != nil {
		return err
	}
	registry, err := registryCodes(p.codes)
	if err != nil {
		return err
	}
	record, err := readCodeRecord(p.record)
	if err != nil {
		return err
	}
	if err := checkCodeRecord(w, registry, record, p.record); err != nil {
		return err
	}
	active, err := os.ReadFile(p.active)
	if err != nil {
		return err
	}
	list, err := parseActive(active)
	if err != nil {
		return fmt.Errorf("%s: %w", display(p.active), err)
	}
	enforced, deferred, err := resolveActive(m.Rules, list)
	if err != nil {
		return fmt.Errorf("%s: %w", display(p.active), err)
	}
	covered, err := readCoverage(p.cover)
	if err != nil {
		return err
	}
	if err := checkCoverage(w, m.Rules, enforced, deferred, covered, p.cover); err != nil {
		return err
	}
	return checkReport(w, p)
}

// checkFresh verifies that data, the manifest file holding committed, is
// exactly what runManifest would write, or says why it cannot tell.
func checkFresh(w io.Writer, p paths, data []byte, committed Manifest) error {
	if p.spec == "" {
		fmt.Fprintf(w, "rulecheck: TENON_SPEC not set; skipping the manifest freshness check (%s)\n", summarize(committed.Rules))
		return nil
	}
	m, err := loadSpec(p.spec)
	if err != nil {
		return err
	}
	if err := checkPermanence(committed.Rules, m.Rules); err != nil {
		return err
	}
	fresh, err := encodeManifest(m)
	if err != nil {
		return err
	}
	if !bytes.Equal(fresh, data) {
		changes := describeChanges(committed.Rules, m.Rules)
		if committed.Version != m.Version {
			changes = fmt.Sprintf("  version %s -> %s\n%s", committed.Version, m.Version, changes)
		}
		return fmt.Errorf("%s is stale; regenerate it with `make rules`:\n%s", display(p.manifest), changes)
	}
	fmt.Fprintf(w, "rulecheck: manifest up to date (version %s, %s)\n", m.Version, summarize(m.Rules))
	return nil
}

// runManifest regenerates the manifest file from the specification.
func runManifest(w io.Writer, p paths) error {
	if p.spec == "" {
		return errors.New("no specification: set TENON_SPEC or pass -spec")
	}
	m, err := loadSpec(p.spec)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(p.manifest)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		old, err := decodeManifest(existing)
		if err != nil {
			return fmt.Errorf("%s: %w", display(p.manifest), err)
		}
		if err := checkPermanence(old.Rules, m.Rules); err != nil {
			return err
		}
	}
	data, err := encodeManifest(m)
	if err != nil {
		return err
	}
	if bytes.Equal(data, existing) {
		fmt.Fprintf(w, "rulecheck: %s already up to date (%s)\n", display(p.manifest), summarize(m.Rules))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.manifest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p.manifest, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "rulecheck: wrote %s (%s)\n", display(p.manifest), summarize(m.Rules))
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
