package gotenon_test

import (
	"errors"
	"testing"
	"time"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/gotenon"
)

// moment is a time that marshals itself as text.
type moment struct{ t time.Time }

func (m moment) MarshalValue() (tenon.Value, error) {
	if m.t.IsZero() {
		return tenon.Value{}, errors.New("a moment has no time")
	}
	return tenon.String(m.t.Format(time.RFC3339Nano)), nil
}

func (m *moment) UnmarshalValue(v tenon.Value) error {
	if !v.IsKnown() || v.IsError() || v.Type() != tenon.StringType() {
		return &gotenon.DiagnosticError{Value: tenon.ErrorVal(tenon.Diagnostic{Code: "app.not_a_moment", Message: "a moment is a known string, not " + v.String()})}
	}
	t, err := time.Parse(time.RFC3339Nano, v.AsString())
	if err != nil {
		return err
	}
	m.t = t
	return nil
}

// timeType encapsulates a time, and serializes it as text.
var timeType = tenon.Capsule("time", tenon.CapsuleOps[time.Time]{
	Equals: func(a, b *time.Time) bool { return a.Equal(*b) },
	Hash:   func(v *time.Time) uint64 { return uint64(v.UnixNano()) },
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
	return tenon.CapsuleVal(timeType, &t), nil
}

func (i *instant) UnmarshalValue(v tenon.Value) error {
	if !v.IsKnown() || v.Type() != timeType {
		return errors.New("an instant is a time capsule")
	}
	i.t = *tenon.CapsuleValue[time.Time](v)
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
	if got := v.Attribute("at"); got.Type() != timeType || !tenon.CapsuleValue[time.Time](got).Equal(t2) {
		t.Errorf("an instant encoded as %v", got)
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
	b, failure, ok := tenon.Serialize(v)
	if !ok {
		t.Fatalf("Serialize failed: %v", failure)
	}
	restored, failure, ok := tenon.Deserialize(b, tenon.Decoders{Capsules: []tenon.Type{timeType}})
	if !ok {
		t.Fatalf("Deserialize failed: %v", failure)
	}
	if again := decoded[schedule](t, restored, tenon.Safe); !again.At.t.Equal(t2) || !again.Log[0].t.Equal(t1) {
		t.Errorf("after serialization the schedule is %+v", again)
	}
}

// observer records the value it is given, whatever it is.
type observer struct{ got tenon.Value }

func (o *observer) UnmarshalValue(v tenon.Value) error {
	o.got = v
	return nil
}

func TestConformance_GO040_MarshalersAtTheBoundary(t *testing.T) {
	conformance.Covers(t, "GO-040")
	// An unmarshaler is given the value as it is: unknown, marked or null.
	marked := tenon.WithMarks(tenon.Unknown(num), stamp{id: "iso", policy: tenon.Isolate})
	for _, v := range []tenon.Value{marked, tenon.NullVal(str), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull())} {
		if got := decoded[observer](t, v, tenon.Safe); !tenon.Identical(got.got, v) {
			t.Errorf("an unmarshaler was given %v, not %v", got.got, v)
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
	wantValue(t, "encoding an observer by its mapping", encoded(t, observer{}), obj(nil))
}
