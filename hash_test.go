package tenon_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
	"tenon/conformance/values"
)

// hashable returns the values of the generator that have a hash: the known
// ones, less null, which has none.
func hashable() []tenon.Value {
	var out []tenon.Value
	for _, v := range values.Known() {
		if !tenon.IsNull(v).AsBool() {
			out = append(out, v)
		}
	}
	return out
}

func TestConformance_EQ030_IdenticalValuesHashAlike(t *testing.T) {
	conformance.Covers(t, "EQ-030")
	all := hashable()
	if len(all) < 20 {
		t.Fatalf("only %d values to hash, which is too few to say much", len(all))
	}
	alike := 0
	for i, a := range all {
		ha := tenon.Hash(a)
		if tenon.Hash(a) != ha {
			t.Errorf("%v hashes differently from one call to the next", a)
		}
		for j, b := range all {
			if !tenon.Identical(a, b) {
				continue
			}
			if i != j {
				alike++
			}
			if hb := tenon.Hash(b); hb != ha {
				t.Errorf("%v and %v are identical but hash %d and %d", a, b, ha, hb)
			}
		}
	}
	// Values that are identical without being the same node are what this is
	// really about, so there had better be some.
	if alike < 3 {
		t.Errorf("only %d identical pairs of distinct values were hashed", alike)
	}
	// A hash is for known values that are there to hash, and asking for one
	// elsewhere is a mistake in the calling program.
	str := tenon.StringType()
	mustPanicUsage(t, "and null has no hash", func() { tenon.Hash(tenon.NullVal(str)) })
	for _, tt := range []struct {
		name string
		v    tenon.Value
	}{
		{"an unknown", tenon.Unknown(str)},
		{"a pending value", tenon.Pending(tenon.Any())},
		{"an error value", tenon.String("\xff")},
		{"a list holding an unknown", tenon.ListVal(str, tenon.Unknown(str))},
	} {
		mustPanicUsage(t, "which is not a known value", func() { tenon.Hash(tt.v) })
	}
}

func TestConformance_EQ031_HashingIsStableAcrossRepresentations(t *testing.T) {
	conformance.Covers(t, "EQ-031")
	num, str := tenon.NumberType(), tenon.StringType()
	for _, tt := range []struct {
		name string
		a, b tenon.Value
	}{
		{"an integer and the same number parsed", tenon.NumberFromInt(1), tenon.NumberFromText("1.000")},
		{"a number in scientific notation", tenon.NumberFromInt(100), tenon.NumberFromText("1e2")},
		{"a negative number", tenon.NumberFromInt(-5), tenon.NumberFromText("-5.0")},
		{"zero", tenon.NumberFromInt(0), tenon.NumberFromText("0.000")},
		{"a string in two normal forms", tenon.String("e\U00000301"), tenon.String("\U000000e9")},
		{
			"a set given its members in either order",
			tenon.SetVal(str, tenon.String("a"), tenon.String("b")),
			tenon.SetVal(str, tenon.String("b"), tenon.String("a")),
		},
		{
			"a set with a member given twice",
			tenon.SetVal(str, tenon.String("a"), tenon.String("a")),
			tenon.SetVal(str, tenon.String("a")),
		},
		{
			"an object built from maps walked in either order",
			tenon.ObjectVal(map[string]tenon.Value{"a": tenon.NumberFromInt(1), "b": tenon.NumberFromText("2.0")}),
			tenon.ObjectVal(map[string]tenon.Value{"b": tenon.NumberFromInt(2), "a": tenon.NumberFromText("1.0")}),
		},
		{
			"a map whose entries were given in either order",
			tenon.MapVal(num, map[string]tenon.Value{"j": tenon.NumberFromInt(1), "k": tenon.NumberFromInt(2)}),
			tenon.MapVal(num, map[string]tenon.Value{"k": tenon.NumberFromText("2.00"), "j": tenon.NumberFromText("1.00")}),
		},
	} {
		if !tenon.Identical(tt.a, tt.b) {
			t.Errorf("%s: the two are not identical to begin with", tt.name)
			continue
		}
		if ha, hb := tenon.Hash(tt.a), tenon.Hash(tt.b); ha != hb {
			t.Errorf("%s: hashed %d and %d", tt.name, ha, hb)
		}
	}
	// Values that differ usually hash differently. They are allowed to
	// collide, but not for a reason as ordinary as these.
	seen := map[uint64]string{}
	for _, v := range []tenon.Value{
		tenon.NumberFromInt(0), tenon.NumberFromInt(1), tenon.NumberFromInt(-1),
		tenon.String(""), tenon.String("0"), tenon.String("1"),
		tenon.Bool(true), tenon.Bool(false),
		tenon.ListVal(str, tenon.String("a")), tenon.ListVal(str, tenon.String("b")),
		tenon.ListVal(str, tenon.String("a"), tenon.String("b")),
		tenon.SetVal(str, tenon.String("a")),
		tenon.TupleVal(tenon.String("a")),
	} {
		h := tenon.Hash(v)
		if other, ok := seen[h]; ok {
			t.Errorf("%v and %s hash alike", v, other)
		}
		seen[h] = v.String()
	}
}

// hashChildEnv marks the process that this test starts to hash a value for it.
const hashChildEnv = "TENON_HASH_CHILD"

func TestConformance_EQ032_HashesHoldWithinARunAndNotBetween(t *testing.T) {
	text := "the same value, in either process"
	if os.Getenv(hashChildEnv) == "1" {
		fmt.Printf("HASH %d\n", tenon.Hash(tenon.String(text)))
		return
	}
	conformance.Covers(t, "EQ-032")
	// Within a run the hash holds, however the value was built and however
	// often it is asked for.
	want := tenon.Hash(tenon.String(text))
	for i := range 100 {
		if got := tenon.Hash(tenon.String(text)); got != want {
			t.Fatalf("pass %d: the hash of one value changed within the run", i)
		}
	}
	// Between runs it does not, so nothing can come to depend on it, and
	// anything that wrote one down finds out rather than appearing to work.
	// Two other processes, so that a child reporting some fixed number would
	// be caught as well as one agreeing with this process.
	seen := map[uint64]bool{want: true}
	for i := range 2 {
		other := hashInAnotherProcess(t)
		if seen[other] {
			t.Errorf("process %d hashed the value %d, which another process had already", i, other)
		}
		seen[other] = true
	}
}

// hashInAnotherProcess runs this test binary again, asking it for the hash of
// the value that the test above hashes here.
func hashInAnotherProcess(t *testing.T) uint64 {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestConformance_EQ032_HashesHoldWithinARunAndNotBetween$")
	cmd.Env = append(os.Environ(), hashChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hashing the value in another process: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		var h uint64
		if n, err := fmt.Sscanf(line, "HASH %d", &h); n == 1 && err == nil {
			return h
		}
	}
	t.Fatalf("the other process reported no hash:\n%s", out)
	return 0
}
