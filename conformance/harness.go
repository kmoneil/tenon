package conformance

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// EnvSlow names the environment variable that multiplies the cases property
// tests run: make check-slow sets it to a positive integer.
const EnvSlow = "TENON_SLOW"

// EnvEmit names the environment variable holding the directory that tests
// write their canonical output to, so that one run of the tests can be
// compared with another: make determinism runs them twice and requires every
// file to come out the same.
const EnvEmit = "TENON_EMIT_DIR"

// Iterations returns how many cases a property test that runs n by default
// runs: n, or n times the multiplier that TENON_SLOW holds. It fails t if
// TENON_SLOW is set to anything but a positive integer.
func Iterations(t testing.TB, n int) int {
	t.Helper()
	return iterations(t, os.Getenv(EnvSlow), n)
}

func iterations(t testingT, slow string, n int) int {
	t.Helper()
	if slow == "" {
		return n
	}
	k, err := strconv.Atoi(slow)
	if err != nil || k < 1 {
		t.Fatalf("conformance.Iterations: %s must be a positive integer, not %q", EnvSlow, slow)
		return n
	}
	return n * k
}

// Emit records data as the canonical output that the test t calls name, in a
// file under the directory TENON_EMIT_DIR names, where it names one; without
// it, Emit does nothing. Output that must not vary from one run to the next,
// such as encodings and display forms, is what to emit: never a hash, which
// varies by design. A test emits each name at most once.
func Emit(t testing.TB, name string, data []byte) {
	t.Helper()
	emit(t, os.Getenv(EnvEmit), name, data)
}

func emit(t testingT, dir, name string, data []byte) {
	t.Helper()
	if dir == "" {
		return
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("conformance.Emit: %s must be an absolute path, not %q", EnvEmit, dir)
		return
	}
	path := filepath.Join(dir, fileName(t.Name()), fileName(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("conformance.Emit: %v", err)
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		t.Fatalf("conformance.Emit: %s emitted %q twice", t.Name(), name)
		return
	}
	if err != nil {
		t.Fatalf("conformance.Emit: %v", err)
		return
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		t.Fatalf("conformance.Emit: %v", err)
		return
	}
	if err := f.Close(); err != nil {
		t.Fatalf("conformance.Emit: %v", err)
	}
}

// fileName returns s as one path element: the separators of subtest names and
// anything else a file system might object to become underscores.
func fileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		}
		return '_'
	}, s)
}
