package gotenon_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// moment is a time that marshals itself as text.
type moment struct{ t time.Time }

func (m moment) MarshalValue() (tenon.Value, error) {
	if m.t.IsZero() {
		return tenon.Value{}, errors.New("a moment has no time")
	}
	return tenon.String(m.t.Format(time.RFC3339Nano)), nil
}

func (m *moment) UnmarshalValue(v tenon.Value, _ tenon.Policy) error {
	if !v.IsKnown() || v.IsError() || v.Type() != tenon.StringType() {
		return tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{Code: "app.not_a_moment", Message: "a moment is a known string, not " + v.String()}))
	}
	t, err := time.Parse(time.RFC3339Nano, v.AsString())
	if err != nil {
		return err
	}
	m.t = t
	return nil
}

// timeType encapsulates a time, and serializes it as text.
var timeType = tenon.NewCapsule("time", tenon.CapsuleOps[time.Time]{
	Equal: func(a, b *time.Time) bool { return a.Equal(*b) },
	Hash:  func(v *time.Time) uint64 { return uint64(v.UnixNano()) },
	Encoding: &tenon.CapsuleEncoding[time.Time]{
		ID:     "tenon.test/time",
		Type:   tenon.StringType(),
		Encode: func(v *time.Time) tenon.Value { return tenon.String(v.Format(time.RFC3339Nano)) },
		Decode: func(v tenon.Value) (*time.Time, []tenon.Diagnostic) {
			t, err := time.Parse(time.RFC3339Nano, v.AsString())
			if err != nil {
				return nil, []tenon.Diagnostic{{Code: "app.bad_time", Message: err.Error()}}
			}
			return &t, nil
		},
	},
})

// instant is a time that marshals itself as a capsule value.
type instant struct{ t time.Time }

func (i *instant) MarshalValue() (tenon.Value, error) {
	t := i.t
	return timeType.Value(&t), nil
}

func (i *instant) UnmarshalValue(v tenon.Value, _ tenon.Policy) error {
	t, ok := timeType.Of(v)
	if !ok {
		return errors.New("an instant is a time capsule")
	}
	i.t = *t
	return nil
}

type schedule struct {
	Start moment    `tenon:"start"`
	End   *moment   `tenon:"end,optional"`
	Marks []moment  `tenon:"marks"`
	At    instant   `tenon:"at"`
	Log   []instant `tenon:"log"`
}

func TestConformance_GO040_MarshalersRoundTrip(t *testing.T) {
	conformance.Covers(t, "GO-040", "GO-050")
	t1 := time.Date(2026, 9, 16, 12, 30, 45, 123456789, time.UTC)
	t2 := t1.Add(90 * time.Minute)
	x := schedule{
		Start: moment{t1}, End: &moment{t2}, Marks: []moment{{t1}, {t2}},
		At: instant{t2}, Log: []instant{{t1}},
	}
	v := encoded(t, x)
	// A moment is text and an instant a capsule value, and a slice of either
	// is a tuple, since a marshaler's values need not share a type.
	if got := v.Attribute("start"); !tenon.Identical(got, s(t1.Format(time.RFC3339Nano))) {
		t.Errorf("a moment encoded as %v", got)
	}
	if at, ok := timeType.Of(v.Attribute("at")); !ok || !at.Equal(t2) {
		t.Errorf("an instant encoded as %v", v.Attribute("at"))
	}
	if got := v.Attribute("marks").Type(); got.Kind() != tenon.KindTuple {
		t.Errorf("a slice of moments encoded as a %v", got)
	}
	back := decoded[schedule](t, v, tenon.Safe)
	if !back.Start.t.Equal(t1) || !back.End.t.Equal(t2) || len(back.Marks) != 2 || !back.Marks[1].t.Equal(t2) ||
		!back.At.t.Equal(t2) || len(back.Log) != 1 || !back.Log[0].t.Equal(t1) {
		t.Errorf("the schedule came back as %+v", back)
	}

	// The capsule's own encoding carries an instant through serialization.
	b, err := tenon.Serialize(v)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}
	restored, err := tenon.Deserialize(b, tenon.Decoders{Capsules: []tenon.Type{timeType.Type()}})
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}
	if again := decoded[schedule](t, restored, tenon.Safe); !again.At.t.Equal(t2) || !again.Log[0].t.Equal(t1) {
		t.Errorf("after serialization the schedule is %+v", again)
	}
}

// observer records the value it is given, whatever it is, and the policy.
type observer struct {
	got    tenon.Value
	policy tenon.Policy
}

func (o *observer) UnmarshalValue(v tenon.Value, p tenon.Policy) error {
	o.got, o.policy = v, p
	return nil
}

// The test types that decode themselves implement the interface, so that a
// change to its method fails to compile here rather than leaving them decoded
// by their kinds.
var (
	_ gotenon.ValueUnmarshaler = (*moment)(nil)
	_ gotenon.ValueUnmarshaler = (*instant)(nil)
	_ gotenon.ValueUnmarshaler = (*observer)(nil)
	_ gotenon.ValueUnmarshaler = (*callback)(nil)
	_ gotenon.ValueUnmarshaler = (*fullDisk)(nil)
	_ gotenon.ValueUnmarshaler = (*keeps)(nil)
	_ gotenon.ValueUnmarshaler = (*decodesOnly)(nil)
)

// watched holds unmarshalers in fields and slices, behind pointers and not,
// and a tenon.Value, all of which take what they are given as it is.
type watched struct {
	One    *observer   `tenon:"one"`
	Many   []*observer `tenon:"many"`
	Plain  observer    `tenon:"plain"`
	Plains []observer  `tenon:"plains"`
	Raw    tenon.Value `tenon:"raw"`
}

func TestConformance_GO040_MarshalersAtTheBoundary(t *testing.T) {
	conformance.Covers(t, "GO-040", "GO-041", "GO-042")
	// An unmarshaler is given the value as it is: unknown, marked or null.
	marked := tenon.WithMarks(tenon.Unknown(num), stamp{id: "iso", policy: tenon.Isolate})
	for _, v := range []tenon.Value{marked, tenon.NullVal(str), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull())} {
		if got := decoded[observer](t, v, tenon.Safe); !tenon.Identical(got.got, v) {
			t.Errorf("an unmarshaler was given %v, not %v", got.got, v)
		}
	}
	// So is a pointer to one, at any depth, alone, in a field or in a slice,
	// as encoding/json treats a pointer to an Unmarshaler: anything but an
	// unmarked null, a marked null among them, goes to the method of a new
	// value, and an unmarked null leaves the pointer nil.
	markedNull := tenon.WithMarks(tenon.NullVal(str), stamp{id: "m"})
	pending := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull())
	for _, v := range []tenon.Value{marked, tenon.Unknown(num), pending, markedNull} {
		if got := decoded[*observer](t, v, tenon.Safe); got == nil || !tenon.Identical(got.got, v) {
			t.Errorf("a pointer to an unmarshaler, given %v, decoded to %v", v, got)
		}
		if got := decoded[**observer](t, v, tenon.Safe); got == nil || *got == nil || !tenon.Identical((*got).got, v) {
			t.Errorf("a pointer to a pointer to an unmarshaler, given %v, decoded to %v", v, got)
		}
	}
	// Within a container, a field or a slice, each takes the member it was
	// given, before the container's conversion, which carries only
	// Propagate marks: an Isolate mark reaches it too. A container holds no
	// pending value.
	holding := func(v tenon.Value) tenon.Value {
		return obj(map[string]tenon.Value{"one": v, "many": tenon.TupleVal(v), "plain": v, "plains": tenon.TupleVal(v), "raw": v})
	}
	carried := tenon.WithMarks(tenon.Unknown(num), stamp{id: "p"})
	for _, v := range []tenon.Value{marked, carried, tenon.Unknown(num), markedNull} {
		w := decoded[watched](t, holding(v), tenon.Safe)
		if w.One == nil || !tenon.Identical(w.One.got, v) {
			t.Errorf("a field pointing to an unmarshaler, given %v, decoded to %v", v, w.One)
		}
		if len(w.Many) != 1 || w.Many[0] == nil || !tenon.Identical(w.Many[0].got, v) {
			t.Errorf("a slice of pointers to an unmarshaler, given [%v], decoded to %v", v, w.Many)
		}
		if !tenon.Identical(w.Plain.got, v) {
			t.Errorf("an unmarshaler field, given %v, was given %v", v, w.Plain.got)
		}
		if len(w.Plains) != 1 || !tenon.Identical(w.Plains[0].got, v) {
			t.Errorf("a slice of unmarshalers, given [%v], decoded to %v", v, w.Plains)
		}
		if !tenon.Identical(w.Raw, v) {
			t.Errorf("a tenon.Value field, given %v, holds %v", v, w.Raw)
		}
	}
	// Unconverted as well: under Unsafe, a tuple of a number and a string
	// converts to a list of strings, and each method takes its member as it
	// was, the number a number.
	mixed := tenon.TupleVal(n(1), s("a"))
	w := decoded[watched](t, obj(map[string]tenon.Value{"one": n(1), "many": mixed, "plain": n(1), "plains": mixed, "raw": mixed}), tenon.Unsafe)
	for i, want := range mixed.Elements() {
		if !tenon.Identical(w.Many[i].got, want) || !tenon.Identical(w.Plains[i].got, want) {
			t.Errorf("member %d of a mixed tuple reached the methods as %v and %v, want %v", i, w.Many[i].got, w.Plains[i].got, want)
		}
	}
	if !tenon.Identical(w.Raw, mixed) {
		t.Errorf("a tenon.Value field given %v holds %v", mixed, w.Raw)
	}
	for _, v := range []tenon.Value{tenon.NullVal(str), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())} {
		if got := decoded[*observer](t, v, tenon.Safe); got != nil {
			t.Errorf("a pointer to an unmarshaler, given the unmarked %v, decoded to %v, want nil", v, got)
		}
	}
	// A failure is located where the Go value is: an error that carries
	// diagnostics gives them, and any other error its text.
	wantDecodeFailures[schedule](t, "a moment that is a number", obj(map[string]tenon.Value{
		"start": n(1), "marks": tenon.TupleVal(s("not a time")), "at": s("x"), "log": tenon.TupleVal(),
	}), tenon.Safe,
		wantDiag{tenon.CodeDecodeUnmarshalFailed, ".at"},
		wantDiag{tenon.CodeDecodeUnmarshalFailed, ".marks[0]"},
		wantDiag{"app.not_a_moment", ".start"})
	wantEncodeFailure(t, "a moment with no time", schedule{At: instant{time.Now()}},
		wantDiag{tenon.CodeEncodeMarshalFailed, ".start"})
	mustPanicUsage(t, "returned the zero Value", func() { gotenon.Encode(zeroMarshaler{}) })
}

// TestConformance_GO012_CollectionsOfUnmarshalers holds a slice, array or map
// of unmarshalers to what a slice or map of tenon.Value does in decoding: it
// decodes from Any, member by member, since each member takes a value of any
// type, so members whose types differ decode under either policy, each
// method given its own.
func TestConformance_GO012_CollectionsOfUnmarshalers(t *testing.T) {
	conformance.Covers(t, "GO-012", "GO-040")
	mixed := tenon.TupleVal(n(1), s("a"), tenon.Unknown(boo))
	for _, p := range []tenon.Policy{tenon.Safe, tenon.Unsafe} {
		plain := decoded[[]observer](t, mixed, p)
		pointers := decoded[[]*observer](t, mixed, p)
		array := decoded[[3]observer](t, mixed, p)
		for i, want := range mixed.Elements() {
			if !tenon.Identical(plain[i].got, want) || pointers[i] == nil || !tenon.Identical(pointers[i].got, want) || !tenon.Identical(array[i].got, want) {
				t.Errorf("%s: member %d, %v, reached a slice, a slice of pointers and an array of unmarshalers as %v, %v and %v", p, i, want, plain[i].got, pointers[i], array[i].got)
			}
		}
		nested := decoded[[][]observer](t, tenon.TupleVal(mixed, tenon.TupleVal(n(2))), p)
		if len(nested) != 2 || len(nested[0]) != 3 || !tenon.Identical(nested[0][1].got, s("a")) || !tenon.Identical(nested[1][0].got, n(2)) {
			t.Errorf("%s: a slice of slices of unmarshalers decoded to %v", p, nested)
		}
		m := decoded[map[string]observer](t, obj(map[string]tenon.Value{"n": n(1), "s": s("a")}), p)
		if !tenon.Identical(m["n"].got, n(1)) || !tenon.Identical(m["s"].got, s("a")) {
			t.Errorf("%s: a map of unmarshalers decoded to %v", p, m)
		}
	}
	// A value of another kind still decodes into none of them.
	wantDecodeFailures[[]observer](t, "a number into a slice of unmarshalers", n(1), tenon.Safe,
		wantDiag{tenon.CodeConvertNoConversion, "."})
	wantDecodeFailures[map[string]observer](t, "a list into a map of unmarshalers", tenon.ListVal(num, n(1)), tenon.Safe,
		wantDiag{tenon.CodeConvertNoConversion, "."})
}

// zeroMarshaler breaks the contract of a marshaler.
type zeroMarshaler struct{}

func (zeroMarshaler) MarshalValue() (tenon.Value, error) { return tenon.Value{}, nil }

// encodesOnly marshals itself and decodes by its struct mapping.
type encodesOnly struct {
	N int `tenon:"n"`
}

func (e encodesOnly) MarshalValue() (tenon.Value, error) { return tenon.NumberFromInt(int64(e.N)), nil }

func TestConformance_GO040_MarshalingOneWay(t *testing.T) {
	conformance.Covers(t, "GO-040")
	wantValue(t, "encoding by the method", encoded(t, encodesOnly{N: 3}), n(3))
	if got := decoded[encodesOnly](t, obj(map[string]tenon.Value{"n": n(4)}), tenon.Safe); got.N != 4 {
		t.Errorf("decoding by the struct mapping gave %+v", got)
	}
	// And the other way about.
	wantValue(t, "encoding by the struct mapping", encoded(t, decodesOnly{N: 5}), obj(map[string]tenon.Value{"n": n(5)}))
	if got := decoded[decodesOnly](t, n(6), tenon.Safe); got.N != 6 {
		t.Errorf("decoding by the method gave %+v", got)
	}
	// A struct whose state is all unexported has no mapping of its kind to
	// fall back on [GO-011].
	mustPanicUsage(t, "holds its state in unexported fields", func() { gotenon.Encode(observer{}) })
}

// decodesOnly decodes itself, and encodes by the struct mapping.
type decodesOnly struct {
	N int `tenon:"n"`
}

func (d *decodesOnly) UnmarshalValue(v tenon.Value, _ tenon.Policy) error {
	n, _ := v.AsInt64()
	d.N = int(n)
	return nil
}

// intKeyed marshals itself, where its kind, a map with int keys, maps to
// nothing, so it encodes and nothing decodes into it.
type intKeyed map[int]string

func (k intKeyed) MarshalValue() (tenon.Value, error) { return tenon.NumberFromInt(int64(len(k))), nil }

// tree marshals itself, where its kind holds itself, as a tree does.
type tree struct {
	Value int     `tenon:"value"`
	Kids  []*tree `tenon:"kids"`
}

func (t tree) MarshalValue() (tenon.Value, error) {
	n := int64(1)
	for _, k := range t.Kids {
		v, _ := k.MarshalValue()
		m, _ := v.AsInt64()
		n += m
	}
	return tenon.NumberFromInt(n), nil
}

// callback unmarshals itself, where its kind, a function, maps to nothing, so
// it decodes and nothing encodes from it.
type callback func() int

func (c *callback) UnmarshalValue(v tenon.Value, _ tenon.Policy) error {
	n, _ := v.AsInt64()
	*c = func() int { return int(n) }
	return nil
}

// TestConformance_GO040_EachDirectionMapsOnItsOwn holds a type implementing
// one marshaler interface to being mapped by its kind only in the other
// direction, and only when a value goes that way: a map with int keys and a
// tree holding itself that marshal themselves encode, and decoding into
// either is the usage error; a function type that unmarshals itself decodes,
// and encoding from it is. A nil pointer to a type that encodes itself still
// encodes as the null decoding reads back as nil.
func TestConformance_GO040_EachDirectionMapsOnItsOwn(t *testing.T) {
	conformance.Covers(t, "GO-040", "GO-011", "GO-013")
	wantValue(t, "a map with int keys that marshals itself", encoded(t, intKeyed{1: "a", 2: "b"}), n(2))
	mustPanicUsage(t, "keys of kind int", func() { gotenon.Decode[intKeyed](n(2), tenon.Safe) })
	leaf := &tree{Value: 2}
	wantValue(t, "a tree that marshals itself", encoded(t, tree{Value: 1, Kids: []*tree{leaf, leaf}}), n(3))
	mustPanicUsage(t, "holds itself", func() { gotenon.Decode[tree](n(3), tenon.Safe) })
	if got := decoded[callback](t, n(7), tenon.Safe); got() != 7 {
		t.Errorf("a function that unmarshals itself decoded to one giving %d", got())
	}
	mustPanicUsage(t, "of kind func", func() { gotenon.Encode(callback(func() int { return 0 })) })

	var nothing *encodesOnly
	null := encoded(t, nothing)
	wantValue(t, "a nil pointer to a struct that marshals itself", null, tenon.NullVal(tenon.Object(map[string]tenon.Type{"n": num})))
	if back := decoded[*encodesOnly](t, null, tenon.Safe); back != nil {
		t.Errorf("the null of a nil pointer decoded back as %+v", back)
	}
	var none *intKeyed
	wantValue(t, "a nil pointer to a map with int keys that marshals itself", encoded(t, none), tenon.NullVal(tenon.Object(nil)))
}

// errFull is what fullDisk's methods fail with, wrapped.
var errFull = errors.New("the disk is full")

// fullDisk is a Go type whose methods fail with a Go error: encoding with
// errFull wrapped in text, and decoding with errFull behind a *tenon.Error
// holding diagnostics of its own.
type fullDisk struct{}

func (fullDisk) MarshalValue() (tenon.Value, error) {
	return tenon.Value{}, fmt.Errorf("writing the log: %w", errFull)
}

func (*fullDisk) UnmarshalValue(tenon.Value, tenon.Policy) error {
	return tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{Code: "app.full", Message: "no room"}), errFull)
}

// TestConformance_GO040_FailuresKeepTheirCauses holds the error that Encode
// and Decode fail with to keeping the errors the methods returned as its
// causes, so errors.Is finds what a method failed with through it, while its
// diagnostics say where: the text of a plain error, or the diagnostics of a
// *tenon.Error, located where the Go value is.
func TestConformance_GO040_FailuresKeepTheirCauses(t *testing.T) {
	conformance.Covers(t, "GO-040", "GO-003")
	_, err := gotenon.Encode([]fullDisk{{}, {}})
	var failed *tenon.Error
	if !errors.As(err, &failed) || !errors.Is(err, errFull) || len(failed.Unwrap()) != 2 {
		t.Fatalf("encoding two failing values gave %v, want a *tenon.Error with a cause for each", err)
	}
	wantErrors(t, "encoding", failed.Value(),
		wantDiag{tenon.CodeEncodeMarshalFailed, ".[0]"}, wantDiag{tenon.CodeEncodeMarshalFailed, ".[1]"})
	if got := failed.Diagnostics()[0].Message; got != "writing the log: the disk is full" {
		t.Errorf("the diagnostic's message is %q, want the error's text", got)
	}

	_, err = gotenon.Decode[struct {
		Disk fullDisk `tenon:"disk"`
	}](obj(map[string]tenon.Value{"disk": s("x")}), tenon.Safe)
	if !errors.As(err, &failed) || !errors.Is(err, errFull) {
		t.Fatalf("decoding into a failing value gave %v, want a *tenon.Error with errFull behind it", err)
	}
	wantErrors(t, "decoding", failed.Value(), wantDiag{"app.full", ".disk"})
}
