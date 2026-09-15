package tenon_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

func TestConformance_VA005_ValuesImmutable(t *testing.T) {
	conformance.Covers(t, "VA-005")
	str, num := tenon.StringType(), tenon.NumberType()
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)

	elems := []tenon.Value{one, two}
	list, set, tuple := tenon.ListVal(num, elems...), tenon.SetVal(num, elems...), tenon.TupleVal(elems...)
	elems[0] = two
	for _, v := range []tenon.Value{list, set, tuple} {
		if got := v.Elements(); len(got) != 2 || got[0] != one {
			t.Errorf("changing the slice passed to its constructor changed %v", v)
		}
		got := v.Elements()
		got[1] = one
		if v.Elements()[1] != two {
			t.Errorf("changing the slice from Elements changed %v", v)
		}
	}

	entries := map[string]tenon.Value{"a": one}
	m := tenon.MapVal(num, entries)
	entries["a"], entries["b"] = two, two
	if e, ok := m.MapElement("a"); m.Len() != 1 || !ok || e != one {
		t.Errorf("changing the map passed to MapVal changed %v", m)
	}
	keys := m.MapKeys()
	keys[0] = "z"
	if _, ok := m.MapElement("a"); !ok {
		t.Errorf("changing the slice from MapKeys changed %v", m)
	}

	attrs := map[string]tenon.Value{"a": tenon.String("x")}
	obj := tenon.ObjectVal(attrs)
	attrs["a"], attrs["b"] = one, one
	if obj.Len() != 1 || obj.Attribute("a").AsString() != "x" || obj.Type() != tenon.Object(map[string]tenon.Type{"a": str}) {
		t.Errorf("changing the map passed to ObjectVal changed %v", obj)
	}
}

func TestConformance_TY016_MapKeys(t *testing.T) {
	conformance.Covers(t, "TY-016")
	num := tenon.NumberType()
	composed, decomposed := "caf\u00e9", "cafe\u0301"
	m := tenon.MapVal(num, map[string]tenon.Value{decomposed: tenon.NumberFromInt(1), "tea": tenon.NumberFromInt(2)})
	if got := m.MapKeys(); !slices.Equal(got, []string{composed, "tea"}) {
		t.Errorf("MapKeys() = %+q; want the keys normalized and sorted", got)
	}
	// Lookups normalize the key they are given.
	for _, key := range []string{composed, decomposed} {
		if v, ok := m.MapElement(key); !ok || v.String() != "1" {
			t.Errorf("MapElement(%+q) = %v, %t", key, v, ok)
		}
	}
	for _, key := range []string{"cafe", "caf", "\xff"} {
		if v, ok := m.MapElement(key); ok {
			t.Errorf("MapElement(%+q) found %v", key, v)
		}
	}

	// Keys are strings of Unicode scalar values.
	bad := tenon.MapVal(num, map[string]tenon.Value{"ok": tenon.NumberFromInt(1), "a\xffb": tenon.NumberFromInt(2)})
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
	v := tenon.MapVal(num, map[string]tenon.Value{
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

	// Every problem is reported, invalid keys first, in a fixed order.
	one := tenon.NumberFromInt(1)
	many := tenon.MapVal(num, map[string]tenon.Value{
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

func TestContainerValues(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	a, b, one := tenon.String("a"), tenon.String("b"), tenon.NumberFromInt(1)

	list := tenon.ListVal(str, a, b)
	if list.Type() != tenon.List(str) || list.Len() != 2 || list.Index(0) != a || list.Index(1) != b {
		t.Errorf("ListVal gave %v", list)
	}
	if empty := tenon.ListVal(num); empty.Type() != tenon.List(num) || empty.Len() != 0 || len(empty.Elements()) != 0 {
		t.Errorf("an empty ListVal gave %v", empty)
	}
	if set := tenon.SetVal(str, b, a); set.Type() != tenon.Set(str) || !slices.Equal(set.Elements(), []tenon.Value{b, a}) {
		t.Errorf("SetVal gave %v", set)
	}
	tuple := tenon.TupleVal(a, one, tenon.Bool(true))
	if tuple.Type() != tenon.Tuple(str, num, tenon.BoolType()) || tuple.Len() != 3 || tuple.Index(2) != tenon.Bool(true) {
		t.Errorf("TupleVal gave %v", tuple)
	}
	m := tenon.MapVal(str, map[string]tenon.Value{"y": b, "x": a})
	if e, ok := m.MapElement("y"); m.Type() != tenon.Map(str) || !slices.Equal(m.MapKeys(), []string{"x", "y"}) || !ok || e != b {
		t.Errorf("MapVal gave %v", m)
	}
	obj := tenon.ObjectVal(map[string]tenon.Value{"name": a, "count": one, "caf\u00e9": b})
	if obj.Type() != tenon.Object(map[string]tenon.Type{"name": str, "count": num, "caf\u00e9": str}) || obj.Len() != 3 ||
		obj.Attribute("name") != a || obj.Attribute("count") != one || obj.Attribute("cafe\u0301") != b {
		t.Errorf("ObjectVal gave %v", obj)
	}
	if nested := tenon.ListVal(tenon.Tuple(str), tenon.TupleVal(a)); nested.Index(0).Index(0) != a {
		t.Errorf("a nested value gave %v", nested)
	}

	mustPanicUsage(t, "ListVal: element 0 has type number, not string", func() { tenon.ListVal(str, one) })
	mustPanicUsage(t, "element 1 is a pending value, not a resolved value", func() { tenon.SetVal(str, a, tenon.Pending(tenon.Any())) })
	mustPanicUsage(t, `the element of key "k" has type string, not number`, func() { tenon.MapVal(num, map[string]tenon.Value{"k": a}) })
	mustPanicUsage(t, "the same name after normalization", func() {
		tenon.ObjectVal(map[string]tenon.Value{"caf\u00e9": a, "cafe\u0301": b})
	})
	mustPanicUsage(t, "zero Type", func() { tenon.ListVal(tenon.Type{}) })
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
		{tenon.ListVal(str, a, b), `list(string)["a", "b"]`},
		{tenon.ListVal(num), "list(number)[]"},
		{tenon.SetVal(num, one), "set(number)[1]"},
		{tenon.TupleVal(a, one), `["a", 1]`},
		{tenon.TupleVal(), "[]"},
		{tenon.MapVal(num, map[string]tenon.Value{"b": one, "a": two}), `map(number){"a": 2, "b": 1}`},
		{tenon.ObjectVal(map[string]tenon.Value{"z": one, "k": tenon.ListVal(str)}), `{"k": list(string)[], "z": 1}`},
		{tenon.ObjectVal(nil), "{}"},
	} {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("String() = %s, want %s", got, tt.want)
		}
	}
}
