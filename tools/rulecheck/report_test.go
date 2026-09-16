package main

import (
	"strings"
	"testing"

	"tenon/internal/uni"
)

func TestReport(t *testing.T) {
	m := Manifest{Version: "9.9.9", Rules: []Rule{
		{ID: "BB-001", Area: "BB"},
		{ID: "BB-002", Area: "BB"},
		{ID: "BB-003", Area: "BB", Withdrawn: true},
		{ID: "AA-001", Area: "AA"},
		{ID: "AA-002", Area: "AA"},
		{ID: "AA-003", Area: "AA"},
		{ID: "AA-004", Area: "AA", Outline: true},
	}}
	list, err := parseActive([]byte("AA\ndefer AA-002 waits for the parser\nBB-001\n"))
	if err != nil {
		t.Fatal(err)
	}
	enforced, _, err := resolveActive(m.Rules, list)
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]string{"AA-001": "TestOne", "BB-001": "TestTwo"}
	got := report(m, list, enforced, covered)
	for _, want := range []string{
		"| Specification version | 9.9.9 |\n",
		"| Unicode version (`ST-003`) | " + uni.UnicodeVersion + " |\n",
		"| Rules | 5 normative, 1 outline, 1 withdrawn |\n",
		"| Rules satisfied | 2 of 5 |\n",
		// Unsatisfied rules in manifest order, each with why.
		"| `BB-002` | not enforced: its area is not active |\n" +
			"| `AA-002` | deferred: waits for the parser |\n" +
			"| `AA-003` | no passing conformance test |\n",
		"## Implementation-defined orderings\n\n- **Unknown set members** (`EQ-044`)",
		"| `BB` | 2 | 1 |\n| `AA` | 3 | 1 |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "None. Every normative rule") {
		t.Error("the report says every rule is satisfied")
	}

	all := map[string]string{"AA-001": "T", "AA-002": "T", "AA-003": "T", "BB-001": "T", "BB-002": "T"}
	if got := report(m, list, enforced, all); !strings.Contains(got, "| Rules satisfied | 5 of 5 |") || !strings.Contains(got, "None. Every normative rule") {
		t.Errorf("a report with every rule covered:\n%s", got)
	}
}
