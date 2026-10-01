package tenon_test

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// TestRangeNumberBounds reads a range's bounds back as the narrowings that
// made it took them, and a known number as its own bound.
func TestRangeNumberBounds(t *testing.T) {
	num := tenon.NumberType()
	n := tenon.NumberFromInt
	unknown := func(ns ...tenon.Narrowing) tenon.Value { return tenon.Narrow(tenon.Unknown(num), ns...) }
	// side is one bound as it displays, and whether it is inclusive; "" is no
	// bound.
	type side struct {
		bound string
		incl  bool
	}
	for _, tt := range []struct {
		v        tenon.Value
		min, max side
	}{
		{tenon.Unknown(num), side{}, side{}},
		{unknown(tenon.NumberMin(n(1), true)), side{"1", true}, side{}},
		{unknown(tenon.NumberMin(n(1), false), tenon.NumberMax(tenon.NumberFromText("2.5"), true)), side{"1", false}, side{"2.5", true}},
		// The tighter bound is the one recorded, and at the same number the
		// exclusive one is the tighter.
		{unknown(tenon.NumberMax(n(9), true), tenon.NumberMax(n(5), true), tenon.NumberMax(n(5), false)), side{}, side{"5", false}},
		// A bound holds of the values other than null, so a range that holds
		// null keeps it, and one that excludes null says so apart.
		{unknown(tenon.NumberMin(n(0), true), tenon.NotNull()), side{"0", true}, side{}},
		{n(7), side{"7", true}, side{"7", true}},
		{tenon.Null(num), side{}, side{}},
	} {
		r := tt.v.Range()
		for _, s := range []struct {
			name string
			want side
			read func() (tenon.Value, bool, bool)
		}{
			{"NumberMin", tt.min, r.NumberMin},
			{"NumberMax", tt.max, r.NumberMax},
		} {
			b, incl, ok := s.read()
			switch {
			case ok != (s.want.bound != ""):
				t.Errorf("%v: %s reports a bound %t, want %t", tt.v, s.name, ok, !ok)
			case ok && (b.String() != s.want.bound || incl != s.want.incl):
				t.Errorf("%v: %s is %v, inclusive %t; want %s, inclusive %t", tt.v, s.name, b, incl, s.want.bound, s.want.incl)
			case !ok && (!b.IsZero() || incl):
				t.Errorf("%v: %s reports no bound but returns %v, %t", tt.v, s.name, b, incl)
			}
		}
	}
}

// TestRangeStringPrefix reads the prefix a range records, which the
// narrowing cut back to what may follow it, and a known string as its own.
func TestRangeStringPrefix(t *testing.T) {
	str := tenon.StringType()
	for _, tt := range []struct {
		v    tenon.Value
		want string
	}{
		{tenon.Unknown(str), ""},
		{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("v1-")), "v1-"},
		// A combining acute after the e would make it another letter
		// (UN-006), so the narrowing keeps what nothing following can change.
		{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("cafe")), "caf"},
		// Narrowing by the prefix a range reports keeps the longer one.
		{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("cafe"), tenon.StringPrefix("caf")), "caf"},
		// Nothing follows a known string, which begins with the whole of
		// itself.
		{tenon.String("cafe"), "cafe"},
		{tenon.String(""), ""},
		{tenon.Null(str), ""},
	} {
		if got := tt.v.Range().StringPrefix(); got != tt.want {
			t.Errorf("%v: StringPrefix is %q, want %q", tt.v, got, tt.want)
		}
	}
}

// TestRangeLengths reads the least and the greatest length of a range, as
// Length bounds them: a set's from its members and its element type too.
func TestRangeLengths(t *testing.T) {
	str, num, bl := tenon.StringType(), tenon.NumberType(), tenon.BoolType()
	n, s := tenon.NumberFromInt, tenon.String
	const none = -1
	for _, tt := range []struct {
		v        tenon.Value
		min, max int64
	}{
		{tenon.Unknown(str), 0, none},
		{tenon.Narrow(tenon.Unknown(str), tenon.LengthMin(2), tenon.LengthMax(5)), 2, 5},
		// A prefix requires as many clusters as it has.
		{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("v1-")), 3, none},
		// A string's length is its count of grapheme clusters, and an e with
		// a combining acute is one.
		{s("e\U00000301x"), 2, 2},
		{tenon.Narrow(tenon.Unknown(tenon.ListType(num)), tenon.LengthMin(1)), 1, none},
		{tenon.List(num, n(1), tenon.Unknown(num)), 2, 2},
		{tenon.Map(num, map[string]tenon.Value{"a": n(1), "b": tenon.Unknown(num)}), 2, 2},
		// A set holding a member that is not known may turn out one member
		// shorter, if that member is the other.
		{tenon.Set(num, n(1), tenon.Unknown(num)), 1, 2},
		{tenon.Set(num, n(1), tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(2), true))), 2, 2},
		// A set is no longer than the values its element type holds, null
		// among them, whatever its range records.
		{tenon.Unknown(tenon.SetType(bl)), 0, 3},
		{tenon.Narrow(tenon.Unknown(tenon.SetType(bl)), tenon.LengthMax(2)), 0, 2},
		{tenon.Unknown(tenon.SetType(tenon.TupleType())), 0, 2},
		// A listing requires as many members as it lists known values.
		{tenon.Narrow(tenon.Unknown(tenon.SetType(str)), tenon.Members(s("a"), s("b"))), 2, none},
		{tenon.Null(tenon.ListType(num)), 0, none},
	} {
		r := tt.v.Range()
		if got := r.LengthMin(); got != tt.min {
			t.Errorf("%v: LengthMin is %d, want %d", tt.v, got, tt.min)
		}
		got, ok := r.LengthMax()
		switch {
		case ok != (tt.max != none):
			t.Errorf("%v: LengthMax reports a bound %t, want %t", tt.v, ok, !ok)
		case ok && got != tt.max:
			t.Errorf("%v: LengthMax is %d, want %d", tt.v, got, tt.max)
		case !ok && got != 0:
			t.Errorf("%v: LengthMax reports no bound but returns %d", tt.v, got)
		}
	}
}

// TestRangeMembers reads a set's members, or the listing its range records,
// in the order a set iterates them.
func TestRangeMembers(t *testing.T) {
	str := tenon.StringType()
	set := tenon.SetType(str)
	s := tenon.String
	notNull := tenon.Narrow(tenon.Unknown(str), tenon.NotNull())
	for _, tt := range []struct {
		v    tenon.Value
		want string
	}{
		{tenon.Unknown(set), "[]"},
		// A listing holds each value once, known ones first, and keeps a
		// value whose range excludes nothing only as a least length.
		{tenon.Narrow(tenon.Unknown(set), tenon.Members(s("b"), notNull, s("a"), s("b"), tenon.Unknown(str))),
			`["a" "b" unknown(string, not null)]`},
		{tenon.Narrow(tenon.Unknown(set), tenon.Members(tenon.Null(str))), "[null(string)]"},
		{tenon.Set(str, s("b"), tenon.Unknown(str), s("a")), `["a" "b" unknown(string)]`},
		{tenon.Set(str), "[]"},
		{tenon.Null(set), "[]"},
	} {
		if got := fmt.Sprint(tt.v.Range().Members()); got != tt.want {
			t.Errorf("%v: Members is %s, want %s", tt.v, got, tt.want)
		}
	}

	// The members come in a new slice, and the range keeps its own.
	listed := tenon.Narrow(tenon.Unknown(set), tenon.Members(s("a")))
	got := listed.Range().Members()
	got[0] = s("z")
	if again := listed.Range().Members(); again[0].AsString() != "a" {
		t.Errorf("writing to the members Members returned changed the range's listing to %v", again)
	}

	// A set's deep marks come with its members, as Elements gives them, and
	// the narrowing lists them again once UnmarkDeep has taken them off.
	deep := stamp{id: "deep", deep: true}
	for _, v := range []tenon.Value{listed, tenon.Set(str, s("a"))} {
		ms := tenon.WithMarks(v, deep).Range().Members()
		if len(ms) != 1 || !tenon.HasMark(ms[0], deep) {
			t.Errorf("the members of %v with a deep mark are %v, want each carrying it", v, ms)
			continue
		}
		bare, _ := tenon.UnmarkDeep(ms[0])
		again := tenon.Narrow(tenon.Unknown(set), tenon.Members(bare)).Range().Members()
		if len(again) != 1 || !tenon.Identical(again[0], s("a")) {
			t.Errorf("listing the unmarked member of %v again gives %v", v, again)
		}
	}
}

// TestRangeBoundsCarryPropagateMarks holds a bound to the marks of the value
// whose range holds it: the ones an operation over that value would carry.
func TestRangeBoundsCarryPropagateMarks(t *testing.T) {
	num := tenon.NumberType()
	five := tenon.NumberFromInt(5)
	kept := stamp{id: "kept"}
	isolated := stamp{id: "isolated", policy: tenon.Isolate}
	secret := stamp{id: "secret", policy: tenon.Isolate, redact: true}
	marksOf := func(v tenon.Value) []tenon.Mark {
		_, ms := tenon.Unmark(v)
		return ms
	}

	bounded := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(five, true))
	for _, v := range []tenon.Value{tenon.WithMarks(bounded, kept, isolated, secret), tenon.WithMarks(five, kept, isolated, secret)} {
		b, _, _ := v.Range().NumberMin()
		if !tenon.HasMark(b, kept) || tenon.HasMark(b, isolated) || !tenon.HasMark(b, secret) {
			t.Errorf("the bound of %v carries %v, want kept and secret, which propagates as a redacting mark", v, marksOf(b))
		}
		if got := b.String(); got != `redacted("secret")` {
			t.Errorf("the bound of %v displays as %s, want the placeholder", v, got)
		}
		if got, _ := b.AsInt64(); got != 5 {
			t.Errorf("the bound of %v is %d, want 5", v, got)
		}
	}

	// A bound taken from a marked value marks the value it narrows, and so
	// the bound read back.
	from := tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(tenon.WithMarks(five, kept), true))
	if b, _, _ := from.Range().NumberMax(); !tenon.HasMark(b, kept) {
		t.Errorf("the bound of %v carries %v, want kept", from, marksOf(b))
	}
	if b, _, _ := bounded.Range().NumberMin(); len(marksOf(b)) != 0 {
		t.Errorf("the bound of the unmarked %v carries %v", bounded, marksOf(b))
	}
}

// TestRangeAccessorsRefuseOtherKinds holds each accessor to the kinds its
// narrowing applies to.
func TestRangeAccessorsRefuseOtherKinds(t *testing.T) {
	str, num := tenon.String("a").Range(), tenon.NumberFromInt(1).Range()
	tuple := tenon.Tuple(tenon.String("a")).Range()
	list := tenon.Unknown(tenon.ListType(tenon.StringType())).Range()
	const lengths = "not a range of String, list, set or map values"
	mustPanicUsage(t, "NumberMin called on the range of a value of type string, not a range of Number values", func() { str.NumberMin() })
	mustPanicUsage(t, "NumberMax called on the range of a value of type string, not a range of Number values", func() { str.NumberMax() })
	mustPanicUsage(t, "StringPrefix called on the range of a value of type number, not a range of String values", func() { num.StringPrefix() })
	mustPanicUsage(t, lengths, func() { num.LengthMin() })
	mustPanicUsage(t, lengths, func() { tuple.LengthMin() })
	mustPanicUsage(t, lengths, func() { tuple.LengthMax() })
	mustPanicUsage(t, "Members called on the range of an unknown value of type list(string), not a range of Set values", func() { list.Members() })

	// A redacting mark withholds the type, and so the reason.
	red := tenon.WithMarks(tenon.String("a"), stamp{id: "secret", redact: true}).Range()
	mustPanicUsage(t, `NumberMin cannot take the range of a value redacted by "secret", for a reason its redacting marks withhold`, func() { red.NumberMin() })

	mustPanicUsage(t, "use of the zero Value", func() { tenon.Range{}.LengthMin() })
}

// TestRangeAccessorsAreImplied narrows every value of the corpus, and every
// value within one, by what its range's accessors say, which must change
// nothing: they say what the range already says. It also holds the lengths
// to Length's answer for the same value.
func TestRangeAccessorsAreImplied(t *testing.T) {
	checked := 0
	var visit func(v tenon.Value)
	visit = func(v tenon.Value) {
		if !v.IsResolved() {
			return
		}
		checked++
		checkRangeAccessors(t, v)
		if !v.HasContent() {
			return
		}
		switch v.Type().Kind() {
		case tenon.KindList, tenon.KindSet, tenon.KindTuple:
			for _, e := range v.Elements() {
				visit(e)
			}
		case tenon.KindMap:
			for _, k := range v.MapKeys() {
				e, _ := v.LookupMapElement(k)
				visit(e)
			}
		case tenon.KindObject:
			for _, name := range v.Type().AttributeNames() {
				visit(v.Attribute(name))
			}
		}
	}
	for _, v := range values.All() {
		visit(v)
	}
	if checked < 100 {
		t.Errorf("checked %d values, want the corpus's", checked)
	}
}

// TestRangeAccessorsSayWhatNarrowingSaid narrows unknown values at random,
// and holds the accessors to saying no more than the narrowings did: what
// they say, narrowed by the narrowings again, is the value they narrowed.
// That they say nothing the range does not, checkRangeAccessors holds.
func TestRangeAccessorsSayWhatNarrowingSaid(t *testing.T) {
	r := rand.New(rand.NewSource(20261001))
	str, num := tenon.StringType(), tenon.NumberType()
	types := []tenon.Type{
		num, str, tenon.ListType(num),
		tenon.SetType(tenon.BoolType()), tenon.SetType(num), tenon.SetType(str),
	}
	numbers := []string{"-1", "0", "0.5", "1", "2", "10"}
	prefixes := []string{"a", "ab-", "cafe", "e\U00000301", "v1-", "\U0001F1EB\U0001F1F7"}
	member := func(t tenon.Type) tenon.Value {
		switch r.Intn(4) {
		case 0:
			return tenon.Null(t)
		case 1:
			return tenon.Narrow(tenon.Unknown(t), tenon.NotNull())
		}
		switch t.Kind() {
		case tenon.KindBool:
			return tenon.Bool(r.Intn(2) == 0)
		case tenon.KindNumber:
			return tenon.NumberFromText(numbers[r.Intn(len(numbers))])
		}
		return tenon.String(prefixes[r.Intn(len(prefixes))])
	}
	cases, contradicted := conformance.Iterations(t, 4000), 0
	for range cases {
		typ := types[r.Intn(len(types))]
		var ns []tenon.Narrowing
		for range r.Intn(5) {
			switch k := typ.Kind(); {
			case r.Intn(5) == 0:
				ns = append(ns, tenon.NotNull())
			case k == tenon.KindNumber && r.Intn(2) == 0:
				ns = append(ns, tenon.NumberMin(tenon.NumberFromText(numbers[r.Intn(len(numbers))]), r.Intn(2) == 0))
			case k == tenon.KindNumber:
				ns = append(ns, tenon.NumberMax(tenon.NumberFromText(numbers[r.Intn(len(numbers))]), r.Intn(2) == 0))
			case k == tenon.KindString && r.Intn(2) == 0:
				ns = append(ns, tenon.StringPrefix(prefixes[r.Intn(len(prefixes))]))
			case k == tenon.KindSet && r.Intn(2) == 0:
				ms := make([]tenon.Value, 1+r.Intn(3))
				for i := range ms {
					ms[i] = member(typ.ElementType())
				}
				ns = append(ns, tenon.Members(ms...))
			case r.Intn(2) == 0:
				ns = append(ns, tenon.LengthMin(int64(r.Intn(5))))
			default:
				ns = append(ns, tenon.LengthMax(int64(r.Intn(5))))
			}
		}
		v := tenon.Narrow(tenon.Unknown(typ), ns...)
		if v.IsError() {
			contradicted++
			continue
		}
		checkRangeAccessors(t, v)
		said := tenon.Narrow(tenon.Unknown(typ), rangeNarrowings(v.Range())...)
		if got := tenon.Narrow(said, ns...); !tenon.Identical(got, v) {
			t.Errorf("%v narrowed by %v is %v; what its range's accessors say, %v, narrowed by them again is %v",
				tenon.Unknown(typ), ns, v, said, got)
		}
	}
	if contradicted == cases {
		t.Errorf("all %d cases were contradictions", cases)
	}
}

// checkRangeAccessors holds the accessors of v's range to being implied by
// it, and its lengths to Length's answer.
func checkRangeAccessors(t *testing.T, v tenon.Value) {
	t.Helper()
	r := v.Range()
	if got := tenon.Narrow(v, rangeNarrowings(r)...); !tenon.Identical(got, v) {
		t.Errorf("%v narrowed by what its range's accessors say, %v, is %v", v, rangeNarrowings(r), got)
	}
	if !slices.Contains([]tenon.Kind{tenon.KindString, tenon.KindList, tenon.KindSet, tenon.KindMap}, v.Type().Kind()) {
		return
	}
	length := tenon.Length(v)
	if length.IsError() {
		return // null, which has no length
	}
	lo, _, _ := length.Range().NumberMin()
	least, _ := lo.AsInt64()
	var most int64
	hi, _, bounded := length.Range().NumberMax()
	if bounded {
		most, _ = hi.AsInt64()
	}
	if got, ok := r.LengthMax(); r.LengthMin() != least || ok != bounded || got != most {
		t.Errorf("%v: lengths from %d to %d, bounded %t, where Length is %v", v, r.LengthMin(), got, ok, length)
	}
}

// rangeNarrowings returns the narrowings that r's accessors say hold of it.
func rangeNarrowings(r tenon.Range) []tenon.Narrowing {
	var ns []tenon.Narrowing
	if !r.AllowsNull() {
		ns = append(ns, tenon.NotNull())
	}
	switch r.Type().Kind() {
	case tenon.KindNumber:
		if b, incl, ok := r.NumberMin(); ok {
			ns = append(ns, tenon.NumberMin(b, incl))
		}
		if b, incl, ok := r.NumberMax(); ok {
			ns = append(ns, tenon.NumberMax(b, incl))
		}
		return ns
	case tenon.KindString:
		if p := r.StringPrefix(); p != "" {
			ns = append(ns, tenon.StringPrefix(p))
		}
	case tenon.KindSet:
		ms := r.Members()
		for i, m := range ms {
			ms[i], _ = tenon.UnmarkDeep(m)
		}
		if len(ms) > 0 {
			ns = append(ns, tenon.Members(ms...))
		}
	case tenon.KindList, tenon.KindMap:
	default:
		return ns
	}
	if n := r.LengthMin(); n > 0 {
		ns = append(ns, tenon.LengthMin(n))
	}
	if n, ok := r.LengthMax(); ok {
		ns = append(ns, tenon.LengthMax(n))
	}
	return ns
}
