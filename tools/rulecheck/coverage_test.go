package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

func TestCoverEnvMatchesConformance(t *testing.T) {
	if coverEnv != conformance.EnvDir {
		t.Fatalf("rulecheck reads records from $%s but package conformance writes them to $%s", coverEnv, conformance.EnvDir)
	}
}

func TestParseActive(t *testing.T) {
	got, err := parseActive([]byte("# comment\n\nAA\nBB-002\ndefer AA-003 needs values first\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := activeList{
		areas:    []string{"AA"},
		rules:    []string{"BB-002"},
		deferred: map[string]string{"AA-003": "needs values first"},
		line:     map[string]int{"AA": 3, "BB-002": 4, "AA-003": 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseActive:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseActiveProblems(t *testing.T) {
	tests := []struct {
		name, data, want string
	}{
		{"unrecognised line", "hello\n", `line 1: want an area, a rule identifier or "defer XX-nnn reason", got "hello"`},
		{"lowercase area", "# areas\naa\n", "line 2: want an area"},
		{"two entries on a line", "AA BB\n", "line 1: want an area"},
		{"deferral without a reason", "defer AA-001\n", `line 1: want "defer XX-nnn reason", got "defer AA-001"`},
		{"deferral of an area", "defer AA reason\n", `line 1: want "defer XX-nnn reason"`},
		{"area twice", "AA\nAA\n", "line 2: AA already appears at line 1"},
		{"rule both enforced and deferred", "AA-001\ndefer AA-001 reason\n", "line 2: AA-001 already appears at line 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseActive([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseActive(%q): error %v, want one containing %q", tt.data, err, tt.want)
			}
		})
	}
}

// coverageRules is the manifest that the coverage tests check against.
var coverageRules = []Rule{
	{ID: "BB-001", Area: "BB"},
	{ID: "BB-002", Area: "BB", Withdrawn: true},
	{ID: "AA-001", Area: "AA"},
	{ID: "AA-002", Area: "AA"},
	{ID: "AA-003", Area: "AA", Outline: true},
	{ID: "CC-001", Area: "CC", Outline: true},
}

// coveredBy maps each rule to a test named after it, as coverage records
// would.
func coveredBy(ids ...string) map[string]string {
	m := make(map[string]string, len(ids))
	for _, id := range ids {
		m[id] = "Test" + strings.ReplaceAll(id, "-", "")
	}
	return m
}

func TestCheckCoverage(t *testing.T) {
	tests := []struct {
		name    string
		active  string
		covered map[string]string
		wantErr bool
		want    string // a substring of the error if wantErr, else of the output
	}{
		{"nothing enforced", "", nil, false, "no rules enforced yet"},
		{"whole area covered", "AA\n", coveredBy("AA-001", "AA-002"), false, "all 2 enforced rules covered, 0 deferred"},
		{"withdrawn rule not enforced", "BB\n", coveredBy("BB-001"), false, "all 1 enforced rules covered, 0 deferred"},
		{"single rule", "AA-002\n", coveredBy("AA-002"), false, "all 1 enforced rules covered"},
		{"deferred rule", "AA\ndefer AA-002 needs values\n", coveredBy("AA-001"), false, "all 1 enforced rules covered, 1 deferred"},
		{"coverage beyond the enforced rules", "AA-001\n", coveredBy("AA-001", "BB-001"), false, "all 1 enforced rules covered"},

		{"uncovered rule", "AA\n", coveredBy("AA-002"), true, "1 enforced rule(s) have no passing conformance test:\n  AA-001"},
		{"no records at all", "AA\n", nil, true, "no coverage records found in /cover"},
		{"covered deferral", "AA\ndefer AA-002 needs values\n", coveredBy("AA-001", "AA-002"), true, "AA-002 is deferred, but TestAA002 covers it; remove the deferral"},
		{"record for a rule not in the manifest", "", coveredBy("ZZ-001"), true, "TestZZ001 covers ZZ-001, which is not in the manifest"},
		{"record for a withdrawn rule", "", coveredBy("BB-002"), true, "TestBB002 covers BB-002, which is withdrawn"},
		{"area without enforceable rules", "CC\n", nil, true, "line 1: area CC has no enforceable rules in the manifest"},
		{"rule not in the manifest", "AA-009\n", nil, true, "line 1: rule AA-009 is not in the manifest"},
		{"outline rule", "AA-003\n", nil, true, "line 1: rule AA-003 is outline and cannot be enforced"},
		{"rule inside an active area", "AA\nAA-001\n", coveredBy("AA-001", "AA-002"), true, "line 2: rule AA-001 is already enforced by area AA"},
		{"deferral outside the active areas", "defer AA-001 needs values\n", nil, true, "line 1: deferred rule AA-001 is not in an active area"},
		{"deferral of a withdrawn rule", "BB\ndefer BB-002 gone\n", coveredBy("BB-001"), true, "line 2: deferred rule BB-002 is withdrawn and is not enforced anyway"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := func() error {
				list, err := parseActive([]byte(tt.active))
				if err != nil {
					return err
				}
				enforced, deferred, err := resolveActive(coverageRules, list)
				if err != nil {
					return err
				}
				return checkCoverage(&out, coverageRules, enforced, deferred, tt.covered, "/cover")
			}()
			got := out.String()
			if err != nil {
				got = err.Error()
			}
			if (err != nil) != tt.wantErr || !strings.Contains(got, tt.want) {
				t.Fatalf("got error %t and %q; want error %t and a message containing %q", err != nil, got, tt.wantErr, tt.want)
			}
		})
	}
}
