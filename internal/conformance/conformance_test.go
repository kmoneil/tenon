package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeT stands in for *testing.T and records how covers uses it.
type fakeT struct {
	name            string
	failed, skipped bool
	fatal           string
	errors          []string
	cleanups        []func()
}

func (f *fakeT) Helper()           {}
func (f *fakeT) Name() string      { return f.name }
func (f *fakeT) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }
func (f *fakeT) Failed() bool      { return f.failed }
func (f *fakeT) Skipped() bool     { return f.skipped }

func (f *fakeT) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)
}

// finish runs the cleanups, as the testing package does when a test ends.
func (f *fakeT) finish() {
	for _, fn := range slices.Backward(f.cleanups) {
		fn()
	}
}

// records returns the coverage records in dir as sorted "rule test" strings.
func records(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "cover-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.Lines(string(data)) {
			var r coverageRecord
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				t.Fatalf("%s: %q: %v", path, line, err)
			}
			got = append(got, r.Rule+" "+r.Test)
		}
	}
	slices.Sort(got)
	return got
}

func TestCoversRejectsBadIdentifiers(t *testing.T) {
	for _, ids := range [][]string{nil, {"NU012"}, {"NU-12"}, {"nu-012"}, {"NU-012", "[NU-013]"}} {
		f := &fakeT{name: "TestBad"}
		covers(f, t.TempDir(), ids)
		if f.fatal == "" || len(f.cleanups) != 0 {
			t.Errorf("covers(%q): fatal %q with %d cleanups; want a failure and no recording", ids, f.fatal, len(f.cleanups))
		}
	}
}

func TestCoversNeedsAbsoluteDirectory(t *testing.T) {
	f := &fakeT{name: "TestRelative"}
	covers(f, filepath.Join("relative", "dir"), []string{"AA-001"})
	if !strings.Contains(f.fatal, "absolute") {
		t.Errorf("covers with a relative directory: fatal %q, want a complaint about absolute paths", f.fatal)
	}
}

func TestCoversWithoutDirectory(t *testing.T) {
	f := &fakeT{name: "TestNoDirectory"}
	covers(f, "", []string{"AA-001"})
	if f.fatal != "" || len(f.cleanups) != 0 {
		t.Errorf("covers without a directory: fatal %q with %d cleanups; want neither", f.fatal, len(f.cleanups))
	}
}

func TestCoversRecordsPassingTestsOnly(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []*fakeT{
		{name: "TestPassed"},
		{name: "TestFailed", failed: true},
		{name: "TestSkipped", skipped: true},
	} {
		covers(f, dir, []string{"AA-001", "AA-002"})
		f.finish()
		if f.fatal != "" || len(f.errors) != 0 {
			t.Fatalf("%s: fatal %q, errors %q", f.name, f.fatal, f.errors)
		}
	}
	want := []string{"AA-001 TestPassed", "AA-002 TestPassed"}
	if got := records(t, dir); !reflect.DeepEqual(got, want) {
		t.Errorf("records = %q, want %q", got, want)
	}
}

func TestCoversConcurrently(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			f := &fakeT{name: fmt.Sprintf("Test%02d", i)}
			covers(f, dir, []string{"AA-001"})
			f.finish()
		})
	}
	wg.Wait()
	if got := records(t, dir); len(got) != 20 {
		t.Errorf("got %d records, want 20: %q", len(got), got)
	}
}

// TestCoversInTests calls Covers from real tests, as conformance tests do.
func TestCoversInTests(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	t.Run("passes", func(t *testing.T) {
		Covers(t, "AA-001", "AA-002")
	})
	t.Run("skips", func(t *testing.T) {
		Covers(t, "AA-003")
		t.Skip("a skipped test covers nothing")
	})
	want := []string{"AA-001 TestCoversInTests/passes", "AA-002 TestCoversInTests/passes"}
	if got := records(t, dir); !reflect.DeepEqual(got, want) {
		t.Errorf("records = %q, want %q", got, want)
	}
}
