package gotenon

import (
	"slices"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
)

// secret is a redacting mark, for a path key that carries one.
type secret struct{}

func (secret) MarkID() string                 { return "secret" }
func (secret) Propagation() tenon.Propagation { return tenon.Propagate }
func (secret) Redacting() bool                { return true }

// keyedFailures returns diagnostics covering every part of a diagnostic that
// decides whether two are equal: the code, the message, and the shape of the
// path, attribute steps and index steps of both key kinds among them, with a
// key that carries a redacting mark and one written another way.
func keyedFailures() []tenon.Diagnostic {
	marked := tenon.WithMarks(tenon.NumberFromInt(1), secret{})
	paths := []tenon.Path{
		{},
		tenon.Path{}.Attribute("a"),
		tenon.Path{}.Attribute("b"),
		tenon.Path{}.Attribute("a b"),
		tenon.Path{}.Index(tenon.String("a")),
		tenon.Path{}.Index(tenon.String("a b")),
		tenon.Path{}.Index(tenon.NumberFromInt(1)),
		tenon.Path{}.Index(tenon.NumberFromInt(11)),
		tenon.Path{}.Index(tenon.NumberFromText("1.0")),
		tenon.Path{}.Index(marked),
		tenon.Path{}.Attribute("a").Index(tenon.NumberFromInt(1)),
		tenon.Path{}.Index(tenon.NumberFromInt(1)).Attribute("a"),
		tenon.Path{}.Attribute("a").Attribute("b"),
	}
	var out []tenon.Diagnostic
	for _, code := range []tenon.Code{"app.one", "app.two", "app.x", "app.xa"} {
		for _, message := range []string{"m", "mm", "m m", "ab", "b"} {
			for _, p := range paths {
				out = append(out, tenon.Diagnostic{Code: code, Message: message, Path: p})
			}
		}
	}
	return out
}

// TestFailureKeyIsEquality holds the key that failures looks a diagnostic up
// by to tenon.Diagnostic.Equal, which is what it stands in for: two
// diagnostics share a key exactly when they are equal.
func TestFailureKeyIsEquality(t *testing.T) {
	diags := keyedFailures()
	if len(diags) < 60 {
		t.Fatalf("only %d diagnostics to compare", len(diags))
	}
	for _, a := range diags {
		for _, b := range diags {
			if same, equal := string(appendFailureKey(nil, a)) == string(appendFailureKey(nil, b)), a.Equal(b); same != equal {
				t.Fatalf("%v and %v: one key %v, equal %v", a, b, same, equal)
			}
		}
	}
}

// TestFailuresCollectAsTheScan holds every answer failures gives to the
// answer the scan it replaces would give, on either side of the count where
// it stops comparing and starts looking up.
func TestFailuresCollectAsTheScan(t *testing.T) {
	diags := keyedFailures()
	for _, count := range []int{0, 1, manyFailures - 1, manyFailures, manyFailures + 1, 3 * manyFailures, len(diags)} {
		var f failures
		var kept []tenon.Diagnostic
		for i, d := range diags[:count] {
			want := slices.ContainsFunc(kept, d.Equal)
			if got := f.holds(d); got != want {
				t.Fatalf("collecting %d: diagnostic %d, holds = %v, the scan says %v", count, i, got, want)
			}
			f.add(d)
			if !want {
				kept = append(kept, d)
			}
		}
		if !slices.EqualFunc(f.list, kept, tenon.Diagnostic.Equal) {
			t.Fatalf("collecting %d gave %d diagnostics, the scan gives %d", count, len(f.list), len(kept))
		}
		late := tenon.Diagnostic{Code: "app.late", Message: "late " + strconv.Itoa(count)}
		for _, d := range append(diags, late) {
			if got, want := f.holds(d), slices.ContainsFunc(f.list, d.Equal); got != want {
				t.Errorf("with %d collected, holds(%v) = %v, want %v", len(f.list), d, got, want)
			}
		}
		// A diagnostic added after the set was made is found there too.
		f.add(late)
		if !f.holds(late) {
			t.Errorf("with %d collected, a diagnostic added after the set was not found", len(f.list))
		}
	}
}

// TestFailuresPastAHandfulAreLookedUp holds failures to its set of keys past
// manyFailures: each diagnostic is then looked for by its key rather than
// compared with every one collected. The comparisons a scan makes are the
// package's own, and nothing outside it can count them, so this asks failures
// what it holds, which a collection comparing each diagnostic with every one
// leaves without a set (T-1601). The failures are alike but for where they
// are, as those of a JSON array of nulls are.
func TestFailuresPastAHandfulAreLookedUp(t *testing.T) {
	var f failures
	for i := range 3 * manyFailures {
		f.add(tenon.Diagnostic{Code: "app.failed", Message: "it failed", Path: tenon.Path{}.Index(tenon.NumberFromInt(int64(i)))})
	}
	if len(f.list) != 3*manyFailures || f.seen == nil {
		t.Errorf("%d failures were collected as %d, looked up by key: %v", 3*manyFailures, len(f.list), f.seen != nil)
	}
}
