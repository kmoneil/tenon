// Package conformance ties tests to the rules of the tenon specification.
//
// A conformance test names the rules it checks by calling Covers:
//
//	func TestConformance_NU012_DivisionRounding(t *testing.T) {
//		conformance.Covers(t, "NU-012")
//		// ...
//	}
//
// When the environment variable named by EnvDir holds an absolute directory,
// every test that called Covers and then passed records the rules it named in
// a file there. make check sets the variable, and tools/rulecheck then fails
// if a rule that conformance/active-areas.txt enforces was not covered.
// Without the variable, Covers only checks its arguments.
package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
)

// EnvDir names the environment variable holding the directory that coverage
// records are written to.
const EnvDir = "TENON_RULECOV_DIR"

// ruleID matches a rule identifier.
var ruleID = regexp.MustCompile(`^[A-Z]{2}-[0-9]{3}$`)

// Covers declares that t checks the rules with the given identifiers, such as
// "NU-012". The rules count as covered if t passes. Covers fails t at once if
// no identifier is given or one is malformed.
func Covers(t *testing.T, ids ...string) {
	t.Helper()
	covers(t, os.Getenv(EnvDir), ids)
}

// testingT is the part of *testing.T that covers uses.
type testingT interface {
	Helper()
	Name() string
	Cleanup(func())
	Failed() bool
	Skipped() bool
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// covers implements Covers, recording into dir unless dir is empty.
func covers(t testingT, dir string, ids []string) {
	t.Helper()
	if len(ids) == 0 {
		t.Fatalf("conformance.Covers: no rule identifiers given")
		return
	}
	for _, id := range ids {
		if !ruleID.MatchString(id) {
			t.Fatalf("conformance.Covers: malformed rule identifier %q; want the form XX-nnn", id)
			return
		}
	}
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("conformance.Covers: %s must be an absolute path, not %q", EnvDir, dir)
		return
	}
	test := t.Name()
	ids = append([]string(nil), ids...)
	t.Cleanup(func() {
		if t.Failed() || t.Skipped() {
			return
		}
		if err := writeRecords(dir, test, ids); err != nil {
			t.Errorf("conformance.Covers: recording coverage: %v", err)
		}
	})
}

// coverageRecord is one line of a coverage file. tools/rulecheck reads the
// same format.
type coverageRecord struct {
	Rule string `json:"rule"`
	Test string `json:"test"`
}

// files holds this process's coverage file in each directory written to. The
// files stay open until the process exits.
var files struct {
	sync.Mutex
	byDir map[string]*os.File
}

// writeRecords appends a record for each rule to this process's coverage file
// in dir.
func writeRecords(dir, test string, ids []string) error {
	var buf []byte
	for _, id := range ids {
		line, err := json.Marshal(coverageRecord{Rule: id, Test: test})
		if err != nil {
			return err
		}
		buf = append(append(buf, line...), '\n')
	}
	files.Lock()
	defer files.Unlock()
	f := files.byDir[dir]
	if f == nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		var err error
		if f, err = os.CreateTemp(dir, "cover-*.jsonl"); err != nil {
			return err
		}
		if files.byDir == nil {
			files.byDir = make(map[string]*os.File)
		}
		files.byDir[dir] = f
	}
	_, err := f.Write(buf)
	return err
}
