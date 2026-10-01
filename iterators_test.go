package tenon_test

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// TestIteratorsGiveWhatTheReadsGive holds each iterator to the reads it
// stands beside, over every container of the corpus and every one within: the
// same members, in the same order, carrying the same marks.
func TestIteratorsGiveWhatTheReadsGive(t *testing.T) {
	counts := map[tenon.Kind]int{}
	var visit func(v tenon.Value)
	visit = func(v tenon.Value) {
		if !v.HasContent() {
			return
		}
		var members []tenon.Value
		switch k := v.Type().Kind(); k {
		case tenon.KindList, tenon.KindSet, tenon.KindTuple:
			members = v.Elements()
			var got []tenon.Value
			for e := range v.ElementsSeq() {
				got = append(got, e)
			}
			sameValues(t, v, "ElementsSeq", got, members)
			counts[k]++
		case tenon.KindMap:
			keys := v.MapKeys()
			i := 0
			for key, e := range v.MapEntries() {
				want, _ := v.LookupMapElement(key)
				if i >= len(keys) || key != keys[i] || !tenon.Identical(e, want) {
					t.Errorf("%v: MapEntries gives %q: %v as entry %d, want the keys %q in order", v, key, e, i, keys)
				}
				members = append(members, e)
				i++
			}
			if i != len(keys) {
				t.Errorf("%v: MapEntries gives %d entries, want %d", v, i, len(keys))
			}
			counts[k]++
		case tenon.KindObject:
			names := v.Type().AttributeNames()
			i := 0
			for name, a := range v.Attributes() {
				if i >= len(names) || name != names[i] || !tenon.Identical(a, v.Attribute(name)) {
					t.Errorf("%v: Attributes gives %q: %v as attribute %d, want the names %q in order", v, name, a, i, names)
				}
				members = append(members, a)
				i++
			}
			if i != len(names) {
				t.Errorf("%v: Attributes gives %d attributes, want %d", v, i, len(names))
			}
			counts[k]++
		}
		for _, m := range members {
			visit(m)
		}
	}
	for _, v := range values.All() {
		visit(v)
	}
	for _, k := range []tenon.Kind{tenon.KindList, tenon.KindSet, tenon.KindTuple, tenon.KindMap, tenon.KindObject} {
		if counts[k] == 0 {
			t.Errorf("the corpus holds no %s value with content to iterate", k)
		}
	}
}

// TestElementsSeqCarriesASetsDeepMarks attaches a set's deep marks to each
// member as the iterator gives it, as Elements does, and holds a list's
// elements to the marks they carry already.
func TestElementsSeqCarriesASetsDeepMarks(t *testing.T) {
	str := tenon.StringType()
	deep := stamp{id: "deep", deep: true}
	shallow := stamp{id: "shallow"}
	for _, v := range []tenon.Value{
		tenon.Set(str, tenon.String("a"), tenon.Unknown(str), tenon.Null(str)),
		tenon.List(str, tenon.String("a"), tenon.Unknown(str), tenon.Null(str)),
	} {
		marked := tenon.WithMarks(v, deep, shallow)
		n := 0
		for e := range marked.ElementsSeq() {
			if !tenon.HasMark(e, deep) || tenon.HasMark(e, shallow) {
				t.Errorf("%v gives the element %v, want it carrying the deep mark alone", marked, e)
			}
			n++
		}
		if n != 3 {
			t.Errorf("%v gives %d elements, want 3", marked, n)
		}
	}
}

// TestIteratorsStopAndRunAgain stops each iterator where the loop does, and
// runs each again from the start.
func TestIteratorsStopAndRunAgain(t *testing.T) {
	num := tenon.NumberType()
	n := tenon.NumberFromInt
	list := tenon.List(num, n(1), n(2), n(3))
	set := tenon.WithMarks(tenon.Set(num, n(1), n(2), n(3)), stamp{id: "deep", deep: true})
	m := tenon.Map(num, map[string]tenon.Value{"a": n(1), "b": n(2), "c": n(3)})
	obj := tenon.Object(map[string]tenon.Value{"a": n(1), "b": n(2), "c": n(3)})
	for _, v := range []tenon.Value{list, set} {
		seq := v.ElementsSeq()
		for range 2 {
			var got []int64
			for e := range seq {
				i, _ := e.AsInt64()
				got = append(got, i)
				if len(got) == 2 {
					break
				}
			}
			if len(got) != 2 || got[0] != 1 || got[1] != 2 {
				t.Errorf("%v: ElementsSeq broken off after two gives %v, want 1 and 2", v, got)
			}
		}
	}
	for _, tt := range []struct {
		v   tenon.Value
		seq func() func(func(string, tenon.Value) bool)
	}{
		{m, func() func(func(string, tenon.Value) bool) { return m.MapEntries() }},
		{obj, func() func(func(string, tenon.Value) bool) { return obj.Attributes() }},
	} {
		seq := tt.seq()
		for range 2 {
			var got string
			for k := range seq {
				got += k
				if k == "b" {
					break
				}
			}
			if got != "ab" {
				t.Errorf("%v: broken off at b, the iterator gives %q, want ab", tt.v, got)
			}
		}
	}
}

// TestIteratorsAllocateNothingPerMember holds the iterators to what they are
// for: ranging over a container allocates as much for a thousand members as
// for ten, where Elements copies them all.
func TestIteratorsAllocateNothingPerMember(t *testing.T) {
	num := tenon.NumberType()
	const runs = 50
	perRun := func(f func()) uint64 {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range runs {
			f()
		}
		runtime.ReadMemStats(&after)
		return (after.TotalAlloc - before.TotalAlloc) / runs
	}
	build := func(size int) []tenon.Value {
		elems := make([]tenon.Value, size)
		entries := make(map[string]tenon.Value, size)
		for i := range elems {
			elems[i] = tenon.NumberFromInt(int64(i))
			entries["a"+strconv.Itoa(i)] = elems[i]
		}
		return []tenon.Value{tenon.List(num, elems...), tenon.Set(num, elems...), tenon.Tuple(elems...),
			tenon.Map(num, entries), tenon.Object(entries)}
	}
	ranging := func(v tenon.Value) func() {
		switch v.Type().Kind() {
		case tenon.KindMap:
			return func() {
				for range v.MapEntries() {
				}
			}
		case tenon.KindObject:
			return func() {
				for range v.Attributes() {
				}
			}
		}
		return func() {
			for range v.ElementsSeq() {
			}
		}
	}
	small, large := build(10), build(1000)
	// The measure sees a copy: Elements of a thousand copies a thousand.
	if copied := perRun(func() { large[0].Elements() }); copied < 8000 {
		t.Fatalf("Elements of a list of 1,000 allocates %d bytes a run, too few to tell a copy from none", copied)
	}
	for i := range small {
		few, many := perRun(ranging(small[i])), perRun(ranging(large[i]))
		if many > few+64 || many > 256 {
			t.Errorf("ranging over a %s allocates %d bytes a run for 10 members and %d for 1,000, want as many for either",
				small[i].Type().Kind(), few, many)
		}
	}
}

// TestIteratorsRefuseWhatTheReadsRefuse panics where the read beside each
// iterator does, as it is called, before anything ranges over it.
func TestIteratorsRefuseWhatTheReadsRefuse(t *testing.T) {
	str := tenon.StringType()
	list := tenon.ListType(str)
	mustPanicUsage(t, "ElementsSeq called on a value of type string, not a list, set or tuple value", func() { tenon.String("a").ElementsSeq() })
	mustPanicUsage(t, "ElementsSeq called on a value of type map(string), not a list, set or tuple value",
		func() { tenon.Map(str, map[string]tenon.Value{"a": tenon.String("a")}).ElementsSeq() })
	mustPanicUsage(t, "ElementsSeq called on the null value of type list(string), which has no content", func() { tenon.Null(list).ElementsSeq() })
	mustPanicUsage(t, "ElementsSeq called on an unknown value of type list(string), which has no content", func() { tenon.Unknown(list).ElementsSeq() })
	mustPanicUsage(t, "MapEntries called on a value of type list(string), not a value of kind Map",
		func() { tenon.List(str, tenon.String("a")).MapEntries() })
	mustPanicUsage(t, "MapEntries called on the null value of type map(string), which has no content", func() { tenon.Null(tenon.MapType(str)).MapEntries() })
	mustPanicUsage(t, "Attributes called on a value of type string, not a value of kind Object", func() { tenon.String("a").Attributes() })
	mustPanicUsage(t, "ElementsSeq called on an error value", func() { tenon.String("\xff").ElementsSeq() })

	// A redacting mark withholds the type, and so the reason.
	red := tenon.WithMarks(tenon.String("a"), stamp{id: "secret", redact: true})
	mustPanicUsage(t, `ElementsSeq cannot take a value redacted by "secret", for a reason its redacting marks withhold`, func() { red.ElementsSeq() })
	mustPanicUsage(t, `MapEntries cannot take a value redacted by "secret"`, func() { red.MapEntries() })
	mustPanicUsage(t, `Attributes cannot take a value redacted by "secret"`, func() { red.Attributes() })
	mustPanicUsage(t, "use of the zero Value", func() { tenon.Value{}.ElementsSeq() })
}

// sameValues fails t unless got and want hold identical values in the same
// order. v and what name the container and the read for the message.
func sameValues(t *testing.T, v tenon.Value, what string, got, want []tenon.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%v: %s gives %d values, want %d", v, what, len(got), len(want))
		return
	}
	for i := range got {
		if !tenon.Identical(got[i], want[i]) {
			t.Errorf("%v: %s gives %v as value %d, want %v", v, what, got[i], i, want[i])
		}
	}
}
