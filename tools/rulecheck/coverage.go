package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// coverEnv names the environment variable through which package conformance
// learns where to write coverage records.
const coverEnv = "TENON_RULECOV_DIR"

// areaID matches a bare area prefix.
var areaID = regexp.MustCompile(`^[A-Z]{2}$`)

// activeList is the parsed active-areas file.
type activeList struct {
	areas    []string          // areas enforced whole, in file order
	rules    []string          // single rules enforced, in file order
	deferred map[string]string // exempted rule -> reason
	line     map[string]int    // entry -> the line it appears on
}

// parseActive parses the active-areas file. Blank lines and lines starting
// with # are ignored. Every other line is an area prefix, a rule identifier,
// or "defer XX-nnn reason", and no entry may appear twice.
func parseActive(data []byte) (activeList, error) {
	list := activeList{deferred: map[string]string{}, line: map[string]int{}}
	var problems problemList
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		var key, kind string
		switch {
		case fields[0] == "defer" && len(fields) >= 3 && ruleID.MatchString(fields[1]):
			key, kind = fields[1], "defer"
		case fields[0] == "defer":
			problems.add(n, "want %q, got %q", "defer XX-nnn reason", line)
			continue
		case len(fields) == 1 && areaID.MatchString(line):
			key, kind = line, "area"
		case len(fields) == 1 && ruleID.MatchString(line):
			key, kind = line, "rule"
		default:
			problems.add(n, "want an area, a rule identifier or %q, got %q", "defer XX-nnn reason", line)
			continue
		}
		if first, dup := list.line[key]; dup {
			problems.add(n, "%s already appears at line %d", key, first)
			continue
		}
		list.line[key] = n
		switch kind {
		case "area":
			list.areas = append(list.areas, key)
		case "rule":
			list.rules = append(list.rules, key)
		default:
			list.deferred[key] = strings.Join(fields[2:], " ")
		}
	}
	if err := scanner.Err(); err != nil {
		return activeList{}, err
	}
	if err := problems.err(); err != nil {
		return activeList{}, err
	}
	return list, nil
}

// resolveActive validates the active list against the manifest. It returns
// the rules whose coverage is enforced and the rules deferred, both in
// manifest order.
func resolveActive(rules []Rule, list activeList) (enforced, deferred []string, err error) {
	var problems problemList
	byID := make(map[string]Rule, len(rules))
	enforceable := map[string]int{} // area -> rules neither outline nor withdrawn
	for _, r := range rules {
		byID[r.ID] = r
		if enforceableRule(r) {
			enforceable[r.Area]++
		}
	}
	activeArea := map[string]bool{}
	for _, a := range list.areas {
		activeArea[a] = true
		if enforceable[a] == 0 {
			problems.add(list.line[a], "area %s has no enforceable rules in the manifest", a)
		}
	}
	single := map[string]bool{}
	for _, id := range list.rules {
		single[id] = true
		switch r, ok := byID[id]; {
		case !ok:
			problems.add(list.line[id], "rule %s is not in the manifest", id)
		case !enforceableRule(r):
			problems.add(list.line[id], "rule %s is %s and cannot be enforced", id, status(r))
		case activeArea[r.Area]:
			problems.add(list.line[id], "rule %s is already enforced by area %s", id, r.Area)
		}
	}
	for id := range list.deferred {
		switch r, ok := byID[id]; {
		case !ok:
			problems.add(list.line[id], "deferred rule %s is not in the manifest", id)
		case !enforceableRule(r):
			problems.add(list.line[id], "deferred rule %s is %s and is not enforced anyway", id, status(r))
		case !activeArea[r.Area]:
			problems.add(list.line[id], "deferred rule %s is not in an active area", id)
		}
	}
	// An enforceable rule that no line reaches would fall out of enforcement
	// without a word: with every area commented out, v0.1.0's rulecheck said
	// "no rules enforced yet" and exited zero. Every enforceable rule must be
	// reached by an area, its own identifier, or a deferral, which says why.
	unenforced := map[string]int{}
	for _, r := range rules {
		if _, isDeferred := list.deferred[r.ID]; enforceableRule(r) && !activeArea[r.Area] && !single[r.ID] && !isDeferred {
			unenforced[r.Area]++
		}
	}
	for _, a := range slices.Sorted(maps.Keys(unenforced)) {
		problems.add(0, "area %s has %d enforceable rule(s) that no line enforces; activate the area, list the rules, or defer them with a reason", a, unenforced[a])
	}
	if err := problems.err(); err != nil {
		return nil, nil, err
	}
	for _, r := range rules {
		_, isDeferred := list.deferred[r.ID]
		switch {
		case !enforceableRule(r):
		case isDeferred:
			deferred = append(deferred, r.ID)
		case activeArea[r.Area] || single[r.ID]:
			enforced = append(enforced, r.ID)
		}
	}
	return enforced, deferred, nil
}

// enforceableRule reports whether coverage of r can be enforced.
func enforceableRule(r Rule) bool {
	return !r.Outline && !r.Withdrawn
}

// status names why a rule is not enforceable.
func status(r Rule) string {
	if r.Withdrawn {
		return "withdrawn"
	}
	return "outline"
}

// coverageRecord is one line of a coverage file, as package conformance
// writes it.
type coverageRecord struct {
	Rule string `json:"rule"`
	Test string `json:"test"`
}

// readCoverage reads the coverage files in dir and maps each covered rule to
// the first test recorded as covering it. A missing directory holds no
// records.
func readCoverage(dir string) (map[string]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "cover-*.jsonl"))
	if err != nil {
		return nil, err
	}
	covered := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		n := 0
		for line := range strings.Lines(string(data)) {
			n++
			dec := json.NewDecoder(strings.NewReader(line))
			dec.DisallowUnknownFields()
			var rec coverageRecord
			if err := dec.Decode(&rec); err != nil || !ruleID.MatchString(rec.Rule) || rec.Test == "" {
				return nil, fmt.Errorf("%s:%d: malformed coverage record %q", path, n, strings.TrimSpace(line))
			}
			// The record kept is a named one where any exists, so a rule
			// covered by a named test and others shows the named one, and
			// only a rule no named test covers shows another.
			if current, seen := covered[rec.Rule]; !seen ||
				!namedFor(rec.Rule, current) && namedFor(rec.Rule, rec.Test) {
				covered[rec.Rule] = rec.Test
			}
		}
	}
	return covered, nil
}

// namedFor reports whether the test is named for the rule by the operating
// loop's convention: TestConformance_<XXnnn>_ begins its name, so running
// the rule's identifier runs a test, and deleting that test is visible
// however many other tests still cover the rule.
func namedFor(rule, test string) bool {
	return strings.HasPrefix(test, "TestConformance_"+strings.ReplaceAll(rule, "-", "")+"_")
}

// checkCoverage checks the coverage records: every enforced rule must be
// covered, at least once by a test named for it, no record may name a rule
// that the manifest lacks or has withdrawn, and no covered rule may stay
// deferred.
func checkCoverage(w io.Writer, rules []Rule, enforced, deferred []string, covered map[string]string, coverDir string) error {
	byID := make(map[string]Rule, len(rules))
	for _, r := range rules {
		byID[r.ID] = r
	}
	var problems []string
	for _, id := range slices.Sorted(maps.Keys(covered)) {
		switch r, ok := byID[id]; {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s covers %s, which is not in the manifest", covered[id], id))
		case r.Withdrawn:
			problems = append(problems, fmt.Sprintf("%s covers %s, which is withdrawn", covered[id], id))
		}
	}
	for _, id := range deferred {
		if test, ok := covered[id]; ok {
			problems = append(problems, fmt.Sprintf("%s is deferred, but %s covers it; remove the deferral", id, test))
		}
	}
	var missing []string
	for _, id := range enforced {
		test, ok := covered[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if !namedFor(id, test) {
			problems = append(problems, fmt.Sprintf(
				"%s is covered only by tests not named for it, %s among them; name one TestConformance_%s_...",
				id, test, strings.ReplaceAll(id, "-", "")))
		}
	}
	if len(missing) > 0 {
		msg := fmt.Sprintf("%d enforced rule(s) have no passing conformance test:\n  %s", len(missing), strings.Join(missing, "\n  "))
		if len(covered) == 0 {
			msg += fmt.Sprintf("\nno coverage records found in %s; make check records them by running the tests with %s set", display(coverDir), coverEnv)
		}
		problems = append(problems, msg)
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	if len(enforced)+len(deferred) == 0 {
		fmt.Fprintln(w, "rulecheck: coverage: no rules enforced yet")
		return nil
	}
	fmt.Fprintf(w, "rulecheck: coverage: all %d enforced rules covered, %d deferred\n", len(enforced), len(deferred))
	return nil
}
