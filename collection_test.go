package tenon_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

func TestConformance_VA005_ValuesImmutable(t *testing.T) {
	conformance.Covers(t, "VA-005")
	str, num := tenon.StringType(), tenon.NumberType()
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)

	elems := []tenon.Value{one, two}
	list, set, tuple := tenon.List(num, elems...), tenon.Set(num, elems...), tenon.Tuple(elems...)
	elems[0] = two
	for _, v := range []tenon.Value{list, set, tuple} {
		if got := v.Elements(); len(got) != 2 || !got[0].Equal(one) {
			t.Errorf("changing the slice passed to its constructor changed %v", v)
		}
		got := v.Elements()
		got[1] = one
		if !v.Elements()[1].Equal(two) {
			t.Errorf("changing the slice from Elements changed %v", v)
		}
	}

	entries := map[string]tenon.Value{"a": one}
	m := tenon.Map(num, entries)
	entries["a"], entries["b"] = two, two
	if e, ok := m.LookupMapElement("a"); m.Len() != 1 || !ok || !e.Equal(one) {
		t.Errorf("changing the map passed to Map changed %v", m)
	}
	keys := m.MapKeys()
	keys[0] = "z"
	if _, ok := m.LookupMapElement("a"); !ok {
		t.Errorf("changing the slice from MapKeys changed %v", m)
	}

	attrs := map[string]tenon.Value{"a": tenon.String("x")}
	obj := tenon.Object(attrs)
	attrs["a"], attrs["b"] = one, one
	if obj.Len() != 1 || obj.Attribute("a").AsString() != "x" || obj.Type() != tenon.ObjectType(map[string]tenon.Type{"a": str}) {
		t.Errorf("changing the map passed to Object changed %v", obj)
	}
}

func TestConformance_TY016_MapKeys(t *testing.T) {
	conformance.Covers(t, "TY-016")
	num := tenon.NumberType()
	composed, decomposed := "caf\u00e9", "cafe\u0301"
	m := tenon.Map(num, map[string]tenon.Value{decomposed: tenon.NumberFromInt(1), "tea": tenon.NumberFromInt(2)})
	if got := m.MapKeys(); !slices.Equal(got, []string{composed, "tea"}) {
		t.Errorf("MapKeys() = %+q; want the keys normalized and sorted", got)
	}
	// Lookups normalize the key they are given.
	for _, key := range []string{composed, decomposed} {
		if v, ok := m.LookupMapElement(key); !ok || v.String() != "1" {
			t.Errorf("LookupMapElement(%+q) = %v, %t", key, v, ok)
		}
	}
	for _, key := range []string{"cafe", "caf", "\xff"} {
		if v, ok := m.LookupMapElement(key); ok {
			t.Errorf("LookupMapElement(%+q) found %v", key, v)
		}
	}

	// Keys are strings of Unicode scalar values.
	bad := tenon.Map(num, map[string]tenon.Value{"ok": tenon.NumberFromInt(1), "a\xffb": tenon.NumberFromInt(2)})
	if !bad.IsError() {
		t.Fatalf("a map with an invalid key is %v; want an error value", bad)
	}
	if d := bad.Diagnostics(); len(d) != 1 || d[0].Code != tenon.CodeStringInvalidUTF8 || !strings.HasSuffix(d[0].Message, "byte 1") {
		t.Errorf("a map with an invalid key has diagnostics %v", d)
	}
}

func TestConformance_TY017_DuplicateMapKeys(t *testing.T) {
	conformance.Covers(t, "TY-017")
	num := tenon.NumberType()
	composed, decomposed := "caf\u00e9", "cafe\u0301"
	v := tenon.Map(num, map[string]tenon.Value{
		composed:   tenon.NumberFromInt(1),
		decomposed: tenon.NumberFromInt(2),
		"x":        tenon.NumberFromInt(3),
	})
	if !v.IsError() {
		t.Fatalf("a map with two spellings of one key is %v; want an error value", v)
	}
	d := v.Diagnostics()
	if len(d) != 1 || d[0].Code != tenon.CodeMapDuplicateKey ||
		!strings.Contains(d[0].Message, strconv.QuoteToASCII(composed)) || !strings.Contains(d[0].Message, strconv.QuoteToASCII(decomposed)) {
		t.Errorf("diagnostics %v; want one duplicate-key diagnostic naming both spellings", d)
	}

	// A shared key is reported whatever its entries hold, after the
	// diagnostics of their elements, which the key's normalized form locates.
	// Every problem comes in the order of the keys: an invalid key's own
	// diagnostic beside its element's, which keep their paths, and entries
	// that share a key by their keys as given.
	one := tenon.NumberFromInt(1)
	composedE, decomposedE := "\U000000E9", "e\U00000301"
	at := func(key string) string { return tenon.Path{}.Index(tenon.String(key)).String() }
	sharedE := "map keys " + strconv.QuoteToASCII(decomposedE) + " and " + strconv.QuoteToASCII(composedE) +
		" are the same key after normalization"
	for _, tt := range []struct {
		name    string
		entries map[string]tenon.Value
		want    []string
	}{
		{
			"an error element beside a number",
			map[string]tenon.Value{composedE: failed("bad"), decomposedE: one},
			[]string{"bad at " + at(composedE), sharedE},
		},
		{
			"two error elements",
			map[string]tenon.Value{composedE: failed("composed"), decomposedE: failed("decomposed")},
			[]string{"decomposed at " + at(composedE), "composed at " + at(composedE), sharedE},
		},
		{
			"every kind of problem",
			map[string]tenon.Value{
				"a\xff": failed("under a bad key"), decomposedE: failed("one"), composedE: one, "f": failed("two"),
				"\xffz": failed("under the last key"),
			},
			[]string{
				`map key "a\xff" is not well-formed UTF-8 at byte 1`, "under a bad key", "two at " + at("f"), "one at " + at(composedE),
				`map key "\xffz" is not well-formed UTF-8 at byte 0`, "under the last key", sharedE,
			},
		},
	} {
		if got := located(tenon.Map(num, tt.entries)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: diagnostics %q, want %q", tt.name, got, tt.want)
		}
	}

	// With no elements that fail, the invalid keys come first, then the keys
	// that normalize alike.
	many := tenon.Map(num, map[string]tenon.Value{
		"\xffz": one, "a\xff": one,
		composed: one, decomposed: one,
		"\u00c5": one, "A\u030a": one,
		"fine": one,
	})
	var got []string
	for _, d := range many.Diagnostics() {
		got = append(got, string(d.Code)+": "+d.Message)
	}
	want := []string{
		`string.invalid_utf8: map key "a\xff" is not well-formed UTF-8 at byte 1`,
		`string.invalid_utf8: map key "\xffz" is not well-formed UTF-8 at byte 0`,
		"map.duplicate_key: map keys " + strconv.QuoteToASCII(decomposed) + " and " + strconv.QuoteToASCII(composed) + " are the same key after normalization",
		"map.duplicate_key: map keys " + strconv.QuoteToASCII("A\u030a") + " and " + strconv.QuoteToASCII("\u00c5") + " are the same key after normalization",
	}
	if !slices.Equal(got, want) {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestConformance_TY017_AnyEntriesAnySpelling builds a map from every choice
// of entries over five keys, two of them spellings of one key and two not
// well-formed UTF-8, one sorting before that key and one after it, each
// absent or holding a number or one of two error values. The map is an error
// value exactly when an entry is a problem; a shared key is reported exactly
// when both its spellings are there, whatever they hold, after everything
// else; and where no key is shared, spelling the one key the other way
// changes nothing.
func TestConformance_TY017_AnyEntriesAnySpelling(t *testing.T) {
	conformance.Covers(t, "TY-016", "TY-017", "ER-008")
	num := tenon.NumberType()
	composedE, decomposedE := "\U000000E9", "e\U00000301"
	keys := []string{"a\xff", "f", decomposedE, composedE, "\xffz"}
	elems := []tenon.Value{tenon.NumberFromInt(1), failed("x"), failed("y")}
	respelled := map[string]string{composedE: decomposedE, decomposedE: composedE}
	for choice := range 1 << (2 * len(keys)) {
		entries := map[string]tenon.Value{}
		for i, k := range keys {
			if pick := choice >> (2 * i) & 3; pick > 0 {
				entries[k] = elems[pick-1]
			}
		}
		_, hasComposed := entries[composedE]
		_, hasDecomposed := entries[decomposedE]
		_, firstInvalid := entries["a\xff"]
		_, lastInvalid := entries["\xffz"]
		shared := hasComposed && hasDecomposed
		problem := shared || firstInvalid || lastInvalid
		for _, e := range entries {
			problem = problem || e.IsError()
		}
		v := tenon.Map(num, entries)
		if v.IsError() != problem {
			t.Errorf("Map(%q) = %v", entries, v)
			continue
		}
		if !v.IsError() {
			continue
		}
		ds := v.Diagnostics()
		var dups []int
		for i, d := range ds {
			if d.Code == tenon.CodeMapDuplicateKey {
				dups = append(dups, i)
			}
		}
		switch {
		case shared && !slices.Equal(dups, []int{len(ds) - 1}):
			t.Errorf("Map(%q) gave %q, want the shared key reported once, last", entries, located(v))
		case !shared && len(dups) > 0:
			t.Errorf("Map(%q) gave %q, reporting a key that nothing shares", entries, located(v))
		case !shared:
			other := map[string]tenon.Value{}
			for k, e := range entries {
				if r, ok := respelled[k]; ok {
					k = r
				}
				other[k] = e
			}
			if w := tenon.Map(num, other); !tenon.Identical(v, w) {
				t.Errorf("Map(%q) gave %q, but spelled the other way %q", entries, located(v), located(w))
			}
		}
	}
}

func TestContainerValues(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	a, b, one := tenon.String("a"), tenon.String("b"), tenon.NumberFromInt(1)

	list := tenon.List(str, a, b)
	if list.Type() != tenon.ListType(str) || list.Len() != 2 || !list.Index(0).Equal(a) || !list.Index(1).Equal(b) {
		t.Errorf("List gave %v", list)
	}
	if empty := tenon.List(num); empty.Type() != tenon.ListType(num) || empty.Len() != 0 || len(empty.Elements()) != 0 {
		t.Errorf("an empty List gave %v", empty)
	}
	// A set holds its members in the order it iterates them in, which is not
	// the order they were given in.
	if set := tenon.Set(str, b, a); set.Type() != tenon.SetType(str) || !slices.EqualFunc(set.Elements(), []tenon.Value{a, b}, tenon.Value.Equal) {
		t.Errorf("Set gave %v", set)
	}
	tuple := tenon.Tuple(a, one, tenon.Bool(true))
	if tuple.Type() != tenon.TupleType(str, num, tenon.BoolType()) || tuple.Len() != 3 || !tuple.Index(2).Equal(tenon.Bool(true)) {
		t.Errorf("Tuple gave %v", tuple)
	}
	m := tenon.Map(str, map[string]tenon.Value{"y": b, "x": a})
	if e, ok := m.LookupMapElement("y"); m.Type() != tenon.MapType(str) || !slices.Equal(m.MapKeys(), []string{"x", "y"}) || !ok || !e.Equal(b) {
		t.Errorf("Map gave %v", m)
	}
	obj := tenon.Object(map[string]tenon.Value{"name": a, "count": one, "caf\u00e9": b})
	if obj.Type() != tenon.ObjectType(map[string]tenon.Type{"name": str, "count": num, "caf\u00e9": str}) || obj.Len() != 3 ||
		!obj.Attribute("name").Equal(a) || !obj.Attribute("count").Equal(one) || !obj.Attribute("cafe\u0301").Equal(b) {
		t.Errorf("Object gave %v", obj)
	}
	if nested := tenon.List(tenon.TupleType(str), tenon.Tuple(a)); !nested.Index(0).Index(0).Equal(a) {
		t.Errorf("a nested value gave %v", nested)
	}

	mustPanicUsage(t, "List: element 0 has type number, not string", func() { tenon.List(str, one) })
	mustPanicUsage(t, "element 1 is a pending value, which has no type", func() { tenon.Set(str, a, tenon.Pending(tenon.Any())) })
	mustPanicUsage(t, `the element of key "k" has type string, not number`, func() { tenon.Map(num, map[string]tenon.Value{"k": a}) })
	// A host's own mistake panics before any problem the data has is reported,
	// under a key that is not well-formed UTF-8 too.
	mustPanicUsage(t, `the element of key "\xff" has type string, not number`, func() {
		tenon.Map(num, map[string]tenon.Value{"\xff": a, "k": tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "m"})})
	})
	mustPanicUsage(t, "zero Type", func() { tenon.List(tenon.Type{}) })
	mustPanicUsage(t, "Index(2) called on a value with 2 elements", func() { list.Index(2) })
	mustPanicUsage(t, "not a list or tuple value", func() { m.Index(0) })
	mustPanicUsage(t, "not a list, set or tuple value", func() { obj.Elements() })
	mustPanicUsage(t, `which has no attribute "missing"`, func() { obj.Attribute("missing") })
	mustPanicUsage(t, "not a value of kind Map", func() { list.MapKeys() })
	mustPanicUsage(t, "not a collection or structural value", func() { a.Len() })
}

func TestContainerString(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	a, b := tenon.String("a"), tenon.String("b")
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	for _, tt := range []struct {
		v    tenon.Value
		want string
	}{
		{tenon.List(str, a, b), `list(string)["a", "b"]`},
		{tenon.List(num), "list(number)[]"},
		{tenon.Set(num, one), "set(number)[1]"},
		{tenon.Tuple(a, one), `["a", 1]`},
		{tenon.Tuple(), "[]"},
		{tenon.Map(num, map[string]tenon.Value{"b": one, "a": two}), `map(number){"a": 2, "b": 1}`},
		{tenon.Object(map[string]tenon.Value{"z": one, "k": tenon.List(str)}), `{"k": list(string)[], "z": 1}`},
		{tenon.Object(nil), "{}"},
	} {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("String() = %s, want %s", got, tt.want)
		}
	}
}

func TestContainersHoldMembersThatAreNotKnown(t *testing.T) {
	str := tenon.StringType()
	unknown, null := tenon.Unknown(str), tenon.Null(str)
	l := tenon.List(str, unknown, null, tenon.String("x"))
	if l.Len() != 3 || !l.Index(0).Equal(unknown) || !l.Index(1).Equal(null) {
		t.Errorf("a list did not keep the members it was given: %v", l)
	}
	if l.Type() != tenon.ListType(str) {
		t.Errorf("the list has type %v, want %v", l.Type(), tenon.ListType(str))
	}
	if want := `list(string)[unknown, null, "x"]`; l.String() != want {
		t.Errorf("the list reads as %s, want %s", l, want)
	}
	// A tuple and an object take their type from members that are not known,
	// which have types like any other resolved value.
	if got, want := tenon.Tuple(unknown, null).Type(), tenon.TupleType(str, str); got != want {
		t.Errorf("a tuple of unknown and null has type %v, want %v", got, want)
	}
	obj := tenon.Object(map[string]tenon.Value{"a": unknown})
	if got, want := obj.Type(), tenon.ObjectType(map[string]tenon.Type{"a": str}); got != want {
		t.Errorf("an object with an unknown attribute has type %v, want %v", got, want)
	}
	if v, ok := tenon.Map(str, map[string]tenon.Value{"k": unknown}).LookupMapElement("k"); !ok || !v.Equal(unknown) {
		t.Errorf("a map did not keep the unknown element it was given")
	}
	// The element type is still checked, and a member with no type at all is
	// still a mistake.
	mustPanicUsage(t, "List: element 0 has type number, not string", func() {
		tenon.List(str, tenon.Unknown(tenon.NumberType()))
	})
	mustPanicUsage(t, "element 0 is a pending value, which has no type; Resolve it to one first", func() {
		tenon.List(str, tenon.Pending(tenon.Any()))
	})
	// An error member is still hoisted out of the container.
	if got := tenon.List(str, tenon.String("\xff"), unknown); !got.IsError() {
		t.Errorf("a list with an error member is %v, want an error value", got)
	}
}

// usagePanicMessage runs f and returns the message it panics with, or "" if it
// does not panic.
func usagePanicMessage(f func()) (msg string) {
	defer func() { msg, _ = recover().(string) }()
	f()
	return ""
}

// TestConformance_ER001_ContainerConstructorsNameTheMemberAtFault pins the
// whole message of every panic a container constructor gives for a member it
// cannot hold, in each way it names one: an element by its index, an attribute
// by its name as a display form quotes it, and a map element by its key in
// ASCII, a name or a key shortened past 32 bytes.
func TestConformance_ER001_ContainerConstructorsNameTheMemberAtFault(t *testing.T) {
	conformance.Covers(t, "ER-001")
	str, num := tenon.StringType(), tenon.NumberType()
	a, one := tenon.String("a"), tenon.NumberFromInt(1)
	pending := tenon.Pending(tenon.Any())
	long := strings.Repeat("x", 30) + "yz-and-more"
	const noType = " is a pending value, which has no type; Resolve it to one first"
	for _, tt := range []struct {
		want string
		f    func()
	}{
		{"List: element 1 has type number, not string", func() { tenon.List(str, a, one) }},
		{"List: element 2" + noType, func() { tenon.List(str, a, a, pending) }},
		{"Set: element 1 has type number, not string", func() { tenon.Set(str, a, one) }},
		{"Set: element 0" + noType, func() { tenon.Set(str, pending) }},
		{`Map: the element of key "k" has type string, not number`, func() { tenon.Map(num, map[string]tenon.Value{"k": a}) }},
		{`Map: the element of key "\xff" has type string, not number`, func() { tenon.Map(num, map[string]tenon.Value{"\xff": a}) }},
		{`Map: the element of key "k"` + noType, func() { tenon.Map(num, map[string]tenon.Value{"k": pending}) }},
		{
			`Map: the element of key "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxyz"... has type string, not number`,
			func() { tenon.Map(num, map[string]tenon.Value{long: a}) },
		},
		{
			`Map: the element of key "caf` + "\\" + `u00e9" has type string, not number`,
			func() { tenon.Map(num, map[string]tenon.Value{"caf\xc3\xa9": a}) },
		},
	} {
		if got, want := usagePanicMessage(tt.f), "tenon: usage: "+tt.want; got != want {
			t.Errorf("got the panic %q, want %q", got, want)
		}
	}
	// A tuple or an object of a pending member is the pending value holding
	// it (UN-025), and no mistake.
	for _, v := range []tenon.Value{tenon.Tuple(a, pending), tenon.Object(map[string]tenon.Value{long: pending, "caf\xc3\xa9": one})} {
		if !v.IsPending() || !v.HasMembers() {
			t.Errorf("%v is not a pending value holding its members", v)
		}
	}
}

// TestConformance_ER001_AConstructorNamesAMemberOnlyToPanic holds the
// container constructors to naming a member only for a panic that uses the
// name. Each checks every member it is given, nearly every check passes, and
// a name built for each is an allocation or two for each that nothing reads:
// a list of 100 numbers made 106 allocations, 100 of them names.
func TestConformance_ER001_AConstructorNamesAMemberOnlyToPanic(t *testing.T) {
	conformance.Covers(t, "ER-001")
	num := tenon.NumberType()
	const size = 100
	members := make([]tenon.Value, size)
	named := make(map[string]tenon.Value, size)
	for i := range members {
		members[i] = tenon.NumberFromInt(int64(i))
		named["a"+strconv.Itoa(1000+i)] = members[i]
	}
	for _, tt := range []struct {
		name string
		most float64 // what it made before, less the names: 106, 112, 225, 241 and 218
		f    func()
	}{
		{"List", 6, func() { tenon.List(num, members...) }},
		{"Tuple", 12, func() { tenon.Tuple(members...) }},
		{"Set", 125, func() { tenon.Set(num, members...) }},
		{"Object", 41, func() { tenon.Object(named) }},
		{"Map", 18, func() { tenon.Map(num, named) }},
	} {
		if got := testing.AllocsPerRun(100, tt.f); got > tt.most {
			t.Errorf("%s of %d members makes %v allocations, want at most %v", tt.name, size, got, tt.most)
		}
	}
}
