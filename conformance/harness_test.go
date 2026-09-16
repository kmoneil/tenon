package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIterations(t *testing.T) {
	for _, tt := range []struct {
		slow  string
		want  int
		fatal bool
	}{
		{"", 300, false},
		{"1", 300, false},
		{"20", 6000, false},
		{"0", 300, true},
		{"-2", 300, true},
		{"many", 300, true},
	} {
		f := &fakeT{name: "TestProperty"}
		if got := iterations(f, tt.slow, 300); got != tt.want || (f.fatal != "") != tt.fatal {
			t.Errorf("TENON_SLOW=%q: %d cases, fatal %q", tt.slow, got, f.fatal)
		}
	}
}

func TestEmit(t *testing.T) {
	dir := t.TempDir()
	f := &fakeT{name: "TestOutputs/set members: unknown"}
	emit(f, dir, "display.txt", []byte("one\n"))
	if f.fatal != "" {
		t.Fatal(f.fatal)
	}
	got, err := os.ReadFile(filepath.Join(dir, "TestOutputs_set_members__unknown", "display.txt"))
	if err != nil || string(got) != "one\n" {
		t.Errorf("emitted %q, %v", got, err)
	}

	// A name emitted twice is a mistake in the test, since one of the two
	// would hide the other.
	emit(f, dir, "display.txt", []byte("two\n"))
	if !strings.Contains(f.fatal, `emitted "display.txt" twice`) {
		t.Errorf("emitting twice: fatal %q", f.fatal)
	}

	// Without a directory nothing is written, and a relative one is refused.
	quiet := &fakeT{name: "TestQuiet"}
	emit(quiet, "", "x", []byte("x"))
	if quiet.fatal != "" {
		t.Errorf("no directory: fatal %q", quiet.fatal)
	}
	emit(quiet, "relative/dir", "x", []byte("x"))
	if !strings.Contains(quiet.fatal, "must be an absolute path") {
		t.Errorf("a relative directory: fatal %q", quiet.fatal)
	}
}
