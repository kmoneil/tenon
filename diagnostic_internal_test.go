package tenon

import (
	"slices"
	"strconv"
	"testing"
)

// keyedDiagnostics returns diagnostics covering every part of a diagnostic
// that decides whether two are equal: the code, the message, and the shape of
// the path, attribute steps and index steps of both key kinds among them,
// with a key written another way.
//
// Two of the codes and two of the messages run together into one text
// ("app.x" with "ab", and "app.xa" with "b"), and an attribute and a string
// key share their text, so a key that wrote no lengths or did not say which
// kind of step it holds would give one key to two diagnostics.
func keyedDiagnostics() []Diagnostic {
	paths := []Path{
		{},
		Path{}.Attribute("a"),
		Path{}.Attribute("b"),
		Path{}.Attribute("a b"),
		Path{}.Index(String("a")),
		Path{}.Index(String("a b")),
		Path{}.Index(NumberFromInt(1)),
		Path{}.Index(NumberFromInt(11)),
		// The same number written another way: equality compares the values a
		// path indexes by, not the text they were written as.
		Path{}.Index(NumberFromText("1.0")),
		Path{}.Attribute("a").Index(NumberFromInt(1)),
		Path{}.Index(NumberFromInt(1)).Attribute("a"),
		Path{}.Attribute("a").Attribute("b"),
	}
	var out []Diagnostic
	for _, code := range []Code{"app.one", "app.two", "app.x", "app.xa"} {
		for _, message := range []string{"m", "mm", "m m", "ab", "b"} {
			for _, p := range paths {
				out = append(out, Diagnostic{Code: code, Message: message, Path: p})
			}
		}
	}
	return out
}

// TestDiagnosticKeyIsEquality holds the key a diagnosticLookup uses to
// Diagnostic.Equal, which is what it stands in for: two diagnostics share a
// key exactly when they are equal.
func TestDiagnosticKeyIsEquality(t *testing.T) {
	diags := keyedDiagnostics()
	if len(diags) < 60 {
		t.Fatalf("only %d diagnostics to compare", len(diags))
	}
	pairs := 0
	for _, a := range diags {
		for _, b := range diags {
			pairs++
			if same, equal := diagnosticKey(a) == diagnosticKey(b), a.Equal(b); same != equal {
				t.Fatalf("%v and %v: one key %v, equal %v", a, b, same, equal)
			}
		}
	}
	t.Logf("%d pairs compared", pairs)
}

// TestDiagnosticLookupIsTheScan holds every answer the lookup gives to the
// answer the scan it replaces would give, on either side of the count where
// it stops comparing and starts looking up.
func TestDiagnosticLookupIsTheScan(t *testing.T) {
	diags := keyedDiagnostics()
	for _, count := range []int{0, 1, manyDiagnostics - 1, manyDiagnostics, manyDiagnostics + 1, 3 * manyDiagnostics, len(diags)} {
		if count > len(diags) {
			t.Fatalf("only %d diagnostics for a list of %d", len(diags), count)
		}
		var lookup diagnosticLookup
		var kept []Diagnostic
		for i, d := range diags[:count] {
			want := slices.ContainsFunc(kept, d.Equal)
			if got := lookup.holds(kept, d); got != want {
				t.Fatalf("collecting %d: diagnostic %d, holds = %v, the scan says %v", count, i, got, want)
			}
			if !want {
				kept = append(kept, d)
			}
		}
		// Every diagnostic is answered for as the scan would answer, whichever
		// way the lookup is answering by now, and a list that grew after the
		// set was made is still searched whole.
		for _, d := range append(diags, Diagnostic{Code: "app.late", Message: "late " + strconv.Itoa(count)}) {
			if got, want := lookup.holds(kept, d), slices.ContainsFunc(kept, d.Equal); got != want {
				t.Errorf("with %d collected, holds(%v) = %v, want %v", len(kept), d, got, want)
			}
		}
		late := Diagnostic{Code: "app.late", Message: "late " + strconv.Itoa(count)}
		grown := append(kept, late)
		if !lookup.holds(grown, late) {
			t.Errorf("with %d collected, a diagnostic added after the set was not found", len(kept))
		}
	}
}

// TestContainerFailuresAreListedOnce holds a container's error value to
// listing its members' diagnostics, each located by its step, once, on first
// asking, in member order, a later duplicate dropped as each container on
// the way up dropped it: the same failure under one key twice, and an
// unlocated member's failure that one located elsewhere repeats.
func TestContainerFailuresAreListedOnce(t *testing.T) {
	var errs containerErrors
	failed := ErrorVal(Diagnostic{Code: "app.failed", Message: "it failed"})
	for i := range 3 * manyDiagnostics {
		errs.add(indexStep(NumberFromInt(int64(i))), failed)
	}
	errs.add(indexStep(NumberFromInt(0)), failed)
	errs.add(indexStep(NumberFromText("1.0")), failed)
	errs.addUnlocated(ErrorVal(Diagnostic{Code: "app.failed", Message: "it failed", Path: Path{}.Index(NumberFromInt(2))}))
	errs.addDiagnostic(Diagnostic{Code: "app.own", Message: "the container failed"})
	v, ok := errs.value()
	if !ok {
		t.Fatal("no error value from failing members")
	}
	h := v.n.data.(*hoisted)
	got := v.Diagnostics()
	if len(got) != 3*manyDiagnostics+1 {
		t.Fatalf("%d failing members and one own failure listed %d diagnostics: %v", 3*manyDiagnostics, len(got), v)
	}
	for i, d := range got[:3*manyDiagnostics] {
		if want := (Path{}).Index(NumberFromInt(int64(i))); !d.Path.Equal(want) || d.Code != "app.failed" {
			t.Errorf("diagnostic %d is %v, want app.failed at %v", i, d, want)
		}
	}
	if last := got[len(got)-1]; last.Code != "app.own" || last.Path.Len() != 0 {
		t.Errorf("the container's own failure is listed as %v", last)
	}
	if again := h.list(); &again[0] != &h.list()[0] || len(again) != len(got) {
		t.Error("the diagnostics were listed again")
	}
}
