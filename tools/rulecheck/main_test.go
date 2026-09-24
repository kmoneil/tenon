package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// cli runs rulecheck commands for a test.
type cli struct {
	t *testing.T
}

// ok runs a command that must succeed and print want.
func (c cli) ok(want string, args ...string) {
	c.t.Helper()
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		c.t.Fatalf("rulecheck %s: %v", strings.Join(args, " "), err)
	}
	if !strings.Contains(out.String(), want) {
		c.t.Fatalf("rulecheck %s printed %q, want it to contain %q", strings.Join(args, " "), out.String(), want)
	}
}

// fails runs a command that must fail with an error containing want.
func (c cli) fails(want string, args ...string) {
	c.t.Helper()
	err := run(args, io.Discard)
	if err == nil || !strings.Contains(err.Error(), want) {
		c.t.Fatalf("rulecheck %s: error %v, want one containing %q", strings.Join(args, " "), err, want)
	}
}

func TestRunManifest(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	manifest := filepath.Join(dir, "conformance", "rules.json")
	active := filepath.Join(dir, "active-areas.txt")
	registry := registryFixture(t, dir)
	conformanceReport := filepath.Join(dir, "CONFORMANCE.md")
	writeFile(t, active, []byte("AA\n"))
	writeFile(t, filepath.Join(dir, "cover", "cover-x.jsonl"),
		[]byte(`{"rule":"AA-001","test":"TestConformance_AA001_X"}`+"\n"+`{"rule":"AA-002","test":"TestConformance_AA002_X"}`+"\n"))
	t.Setenv("TENON_SPEC", "")
	c := cli{t}
	inputs := []string{"-manifest", manifest, "-active", active, "-cover", filepath.Join(dir, "cover"), "-codes", registry, "-report", conformanceReport}
	check := func(extra ...string) []string {
		return append(append([]string{"check"}, inputs...), extra...)
	}

	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One.", "", "`[AA-002]` Two."))
	c.ok("wrote the appendix", "codes", "-spec", spec, "-codes", registry)

	// Generating writes the manifest, and regenerating changes nothing.
	c.ok("wrote", "manifest", "-spec", spec, "-manifest", manifest)
	c.ok("wrote", append([]string{"report"}, inputs...)...)
	generated := readFile(t, manifest)
	c.ok("already up to date", "manifest", "-spec", spec, "-manifest", manifest)
	if !bytes.Equal(readFile(t, manifest), generated) {
		t.Fatal("regenerating changed the manifest")
	}

	// check, the default command, verifies freshness when it has a
	// specification from -spec or TENON_SPEC, and says so when it has none.
	c.ok("manifest up to date", check("-spec", spec)...)
	c.ok("TENON_SPEC not set", check()...)
	c.ok("TENON_SPEC not set", check()[1:]...)
	t.Setenv("TENON_SPEC", spec)
	c.ok("manifest up to date", check()...)

	// A new rule makes the manifest stale.
	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One.", "", "`[AA-002]` Two.", "", "`[AA-003]` Three."))
	c.fails("added AA-003", check()...)

	// Removing a rule is refused by both commands, and manifest leaves the
	// file alone.
	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One."))
	c.fails("rule AA-002 has disappeared", check()...)
	c.fails("rule AA-002 has disappeared", "manifest", "-manifest", manifest)
	if !bytes.Equal(readFile(t, manifest), generated) {
		t.Fatal("a refused regeneration changed the manifest")
	}

	t.Setenv("TENON_SPEC", "")
	c.fails("set TENON_SPEC", "manifest", "-manifest", manifest)
	c.fails("unknown command", "frobnicate")
	c.fails("unexpected arguments", check("extra")...)
}

// TestRunCoverage checks coverage end to end, from records that package
// conformance writes to the verdict of check.
func TestRunCoverage(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	manifest := filepath.Join(dir, "rules.json")
	active := filepath.Join(dir, "active-areas.txt")
	cover := filepath.Join(dir, "cover")
	t.Setenv("TENON_SPEC", "")
	c := cli{t}
	inputs := []string{"-manifest", manifest, "-active", active, "-cover", cover, "-report", filepath.Join(dir, "CONFORMANCE.md")}
	check := append([]string{"check"}, inputs...)

	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One.", "", "`[AA-002]` Two."))
	c.ok("wrote", "manifest", "-spec", spec, "-manifest", manifest)
	writeFile(t, active, []byte("AA\n"))
	c.fails("2 enforced rule(s) have no passing conformance test:\n  AA-001\n  AA-002", check...)

	t.Setenv(conformance.EnvDir, cover)
	t.Run("covers AA-001", func(t *testing.T) { conformance.Covers(t, "AA-001") })
	c.fails("1 enforced rule(s) have no passing conformance test:\n  AA-002", check...)

	t.Run("covers AA-002", func(t *testing.T) { conformance.Covers(t, "AA-002") })
	// The records the subtests wrote carry this test's name, which is not
	// named for either rule, so coverage alone is not enough.
	c.fails("AA-001 is covered only by tests not named for it, TestRunCoverage/covers_AA-001 among them; name one TestConformance_AA001_", check...)
	writeFile(t, filepath.Join(cover, "cover-named.jsonl"), []byte(
		`{"rule":"AA-001","test":"TestConformance_AA001_One"}`+"\n"+
			`{"rule":"AA-002","test":"TestConformance_AA002_Two"}`+"\n"))
	c.fails("CONFORMANCE.md is stale", check...)
	c.ok("wrote", append([]string{"report"}, inputs...)...)
	c.ok("all 2 enforced rules covered, 0 deferred", check...)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
