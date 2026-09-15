package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	manifest := filepath.Join(dir, "conformance", "rules.json")
	t.Setenv("TENON_SPEC", "")

	runOK := func(want string, args ...string) {
		t.Helper()
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatalf("rulecheck %s: %v", strings.Join(args, " "), err)
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("rulecheck %s printed %q, want it to contain %q", strings.Join(args, " "), out.String(), want)
		}
	}
	runErr := func(want string, args ...string) {
		t.Helper()
		err := run(args, io.Discard)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("rulecheck %s: error %v, want one containing %q", strings.Join(args, " "), err, want)
		}
	}

	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One.", "", "`[AA-002]` Two."))

	// Generating writes the manifest, and regenerating changes nothing.
	runOK("wrote", "manifest", "-spec", spec, "-manifest", manifest)
	generated := readFile(t, manifest)
	runOK("already up to date", "manifest", "-spec", spec, "-manifest", manifest)
	if !bytes.Equal(readFile(t, manifest), generated) {
		t.Fatal("regenerating changed the manifest")
	}

	// check is the default command. It verifies freshness when it has a
	// specification, from -spec or TENON_SPEC, and says so when it has none.
	runOK("manifest up to date", "check", "-spec", spec, "-manifest", manifest)
	runOK("TENON_SPEC not set", "-manifest", manifest)
	t.Setenv("TENON_SPEC", spec)
	runOK("manifest up to date", "-manifest", manifest)

	// A new rule makes the manifest stale.
	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One.", "", "`[AA-002]` Two.", "", "`[AA-003]` Three."))
	runErr("added AA-003", "check", "-manifest", manifest)

	// Removing a rule is refused by both commands, and manifest leaves the
	// file alone.
	writeFile(t, spec, fixture("## 2. Alpha", "", "`[AA-001]` One."))
	runErr("rule AA-002 has disappeared", "check", "-manifest", manifest)
	runErr("rule AA-002 has disappeared", "manifest", "-manifest", manifest)
	if !bytes.Equal(readFile(t, manifest), generated) {
		t.Fatal("a refused regeneration changed the manifest")
	}

	t.Setenv("TENON_SPEC", "")
	runErr("set TENON_SPEC", "manifest", "-manifest", manifest)
	runErr("unknown command", "frobnicate")
	runErr("unexpected arguments", "check", "-manifest", manifest, "extra")
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
