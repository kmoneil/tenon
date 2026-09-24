package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kmoneil/tenon/internal/uni"
)

// orderings says what the implementation does where the specification leaves an
// order to it (Appendix A). It describes the implementation, so it changes with
// the code that orders set members and capsule values.
var orderings = []string{
	"**Unknown set members** (`EQ-044`): after the known members, in the bytewise " +
		"order of their encodings (`SE-001`), which hold no type. Members that encode " +
		"alike only because a capsule value's type declares no encoding follow the " +
		"canonical order of those capsule values (`EQ-045`). A set holds no pending value.",
	"**Capsule values of a type that declares no ordering** (`EQ-045`): values " +
		"the type's equality reports equal together; other values by the hash the " +
		"type declares, if it declares one, and then by the order in which the run " +
		"first compared them, which holds for the rest of the run.",
	"**Capsule types of one name** (`EQ-045`): by the order in which the run " +
		"created them.",
}

// report renders the conformance report: what Appendix A requires, from the
// manifest, the active list and the rules it enforces, and the rules the
// coverage records say a passing test covers.
func report(m Manifest, list activeList, enforced []string, covered map[string]string) string {
	isEnforced := map[string]bool{}
	for _, id := range enforced {
		isEnforced[id] = true
	}
	var (
		normative, outline, withdrawn int
		unsatisfied                   []string
		areas                         []string
		inArea                        = map[string]int{}
		satisfiedInArea               = map[string]int{}
	)
	for _, r := range m.Rules {
		switch {
		case r.Withdrawn:
			withdrawn++
			continue
		case r.Outline:
			outline++
			continue
		}
		normative++
		if inArea[r.Area] == 0 {
			areas = append(areas, r.Area)
		}
		inArea[r.Area]++
		_, isCovered := covered[r.ID]
		switch reason, isDeferred := list.deferred[r.ID]; {
		case isCovered:
			satisfiedInArea[r.Area]++
		case isDeferred:
			unsatisfied = append(unsatisfied, fmt.Sprintf("| `%s` | deferred: %s |", r.ID, reason))
		case isEnforced[r.ID]:
			unsatisfied = append(unsatisfied, fmt.Sprintf("| `%s` | no passing conformance test |", r.ID))
		default:
			unsatisfied = append(unsatisfied, fmt.Sprintf("| `%s` | not enforced: its area is not active |", r.ID))
		}
	}

	var b strings.Builder
	b.WriteString("# Conformance report\n\n")
	b.WriteString("This is the conformance report that Appendix A of the tenon specification\n")
	b.WriteString("requires. `make report` generates it from the rule manifest, the rules that\n")
	b.WriteString("`make check` enforces, and a run of the conformance tests, and `make check`\n")
	b.WriteString("fails where it is stale.\n\n")
	b.WriteString("| | |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Specification version | %s |\n", m.Version)
	fmt.Fprintf(&b, "| Unicode version (`ST-003`) | %s |\n", uni.UnicodeVersion)
	fmt.Fprintf(&b, "| Rules | %d normative, %d outline, %d withdrawn |\n", normative, outline, withdrawn)
	fmt.Fprintf(&b, "| Rules satisfied | %d of %d |\n", normative-len(unsatisfied), normative)
	b.WriteString("| Optional areas omitted | none |\n\n")

	b.WriteString("## Rules not satisfied\n\n")
	if len(unsatisfied) == 0 {
		b.WriteString("None. Every normative rule is enforced, and a passing conformance test covers it.\n\n")
	} else {
		b.WriteString("| Rule | Why |\n| ---- | --- |\n")
		b.WriteString(strings.Join(unsatisfied, "\n"))
		b.WriteString("\n\n")
	}

	b.WriteString("## Implementation-defined orderings\n\n")
	for _, o := range orderings {
		b.WriteString("- " + o + "\n")
	}
	b.WriteString("\n## Optional areas\n\n")
	b.WriteString("None is omitted: capsule types (§2.5), marks (§6) and serialization (§8) are\n")
	b.WriteString("implemented, and their conformance tests run.\n\n")

	b.WriteString("## Rules by area\n\n")
	b.WriteString("| Area | Normative rules | Satisfied |\n| ---- | --------------: | --------: |\n")
	for _, a := range areas {
		fmt.Fprintf(&b, "| `%s` | %d | %d |\n", a, inArea[a], satisfiedInArea[a])
	}
	return b.String()
}

// freshReport returns the report that the manifest, the active list and the
// coverage records give.
func freshReport(p paths) (string, error) {
	data, err := os.ReadFile(p.manifest)
	if err != nil {
		return "", err
	}
	m, err := decodeManifest(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", display(p.manifest), err)
	}
	active, err := os.ReadFile(p.active)
	if err != nil {
		return "", err
	}
	list, err := parseActive(active)
	if err != nil {
		return "", fmt.Errorf("%s: %w", display(p.active), err)
	}
	enforced, _, err := resolveActive(m.Rules, list)
	if err != nil {
		return "", fmt.Errorf("%s: %w", display(p.active), err)
	}
	covered, err := readCoverage(p.cover)
	if err != nil {
		return "", err
	}
	return report(m, list, enforced, covered), nil
}

// checkReport verifies that the report file is what runReport would write.
func checkReport(w io.Writer, p paths) error {
	fresh, err := freshReport(p)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(p.report)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !bytes.Equal(current, []byte(fresh)) {
		return fmt.Errorf("%s is stale; regenerate it with `make report`", display(p.report))
	}
	fmt.Fprintf(w, "rulecheck: %s up to date\n", display(p.report))
	return nil
}

// runReport writes the report file.
func runReport(w io.Writer, p paths) error {
	fresh, err := freshReport(p)
	if err != nil {
		return err
	}
	if current, err := os.ReadFile(p.report); err == nil && bytes.Equal(current, []byte(fresh)) {
		fmt.Fprintf(w, "rulecheck: %s already up to date\n", display(p.report))
		return nil
	}
	if err := os.WriteFile(p.report, []byte(fresh), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "rulecheck: wrote %s\n", display(p.report))
	return nil
}
