package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// timeAdd calls TimeAdd with the timestamp and the duration.
func timeAdd(ts, d string) tenon.Value {
	return call(stdlib.TimeAddFunc, tenon.String(ts), tenon.String(d))
}

// adds checks that TimeAdd of ts and d answers want.
func adds(t *testing.T, ts, d, want string) {
	t.Helper()
	if got := timeAdd(ts, d); !got.Equal(tenon.String(want)) {
		t.Errorf("TimeAdd(%q, %q) = %v, want %q", ts, d, got, want)
	}
}

func TestConformance_LT007_TimeAdd(t *testing.T) {
	conformance.Covers(t, "LT-007")
	adds(t, "2020-01-01T00:00:00Z", "1h30m", "2020-01-01T01:30:00Z")
	adds(t, "2020-01-01T00:00:00+05:30", "-90m", "2019-12-31T22:30:00+05:30")
	adds(t, "2020-01-01t00:00:00z", "1s", "2020-01-01T00:00:01Z")
	failsWith(t, "TimeAdd(bad, 1h)", timeAdd("bad", "1h"), tenon.CodeTimeInvalidSyntax, at(0))
}

func TestConformance_LT008_Duration(t *testing.T) {
	conformance.Covers(t, "LT-008")
	ts := "2020-01-01T00:00:00Z"
	for _, tt := range []struct{ d, want string }{
		{"0", ts}, {"+0", ts}, {"-0", ts}, {"1.5h", "2020-01-01T01:30:00Z"}, {"1.h", "2020-01-01T01:00:00Z"},
		{".5h", "2020-01-01T00:30:00Z"}, {"-2h5m", "2019-12-31T21:55:00Z"}, {"+1h", "2020-01-01T01:00:00Z"},
		{"1ns", "2020-01-01T00:00:00.000000001Z"}, {"1us", "2020-01-01T00:00:00.000001Z"},
		{"1\U000000B5s", "2020-01-01T00:00:00.000001Z"}, {"1\U000003BCs", "2020-01-01T00:00:00.000001Z"},
		{"1ms", "2020-01-01T00:00:00.001Z"}, {"1h1m1s1ms1us1ns", "2020-01-01T01:01:01.001001001Z"},
	} {
		adds(t, ts, tt.d, tt.want)
	}
	for _, tt := range []struct{ d, what string }{
		{"", "no number"}, {"1", "no unit"}, {"1d", `"d"`}, {"1H", `"H"`}, {"1e3s", `"e"`},
		{"1h-30m", `"h-"`}, {" 1h", "byte 0"}, {"1 h", `" h"`}, {".s", "byte 0"}, {"-", "no number"},
	} {
		got := timeAdd(ts, tt.d)
		failsWith(t, "TimeAdd(ts, "+tt.d+")", got, tenon.CodeTimeInvalidSyntax, at(1))
		if got.IsError() && !strings.Contains(got.Diagnostics()[0].Message, tt.what) {
			t.Errorf("TimeAdd(ts, %q) fails %q, want it to name %s", tt.d, got.Diagnostics()[0].Message, tt.what)
		}
	}
	failsWith(t, "TimeAdd of a duration past 10,000 bytes", timeAdd(ts, "1"+strings.Repeat("0", 10000)+"ns"), tenon.CodeFunctionTooLarge, at(1))
	if got := timeAdd(ts, "0."+strings.Repeat("0", 9990)+"1s"); got.IsError() {
		t.Errorf("TimeAdd of a duration of 9,996 bytes = %v", got)
	}
}

func TestConformance_LT009_Exact(t *testing.T) {
	conformance.Covers(t, "LT-009")
	ts := "2020-01-01T00:00:00Z"
	// No float in the arithmetic, no truncation of a component, no limit
	// on the duration but the answer's years.
	adds(t, ts, "0.3333333333333333333h", "2020-01-01T00:19:59.99999999999999988Z")
	adds(t, ts, "0.999999999999999999s", "2020-01-01T00:00:00.999999999999999999Z")
	adds(t, ts, "0.5ns0.5ns", "2020-01-01T00:00:00.000000001Z")
	adds(t, ts, "2562048h", "2312-04-12T00:00:00Z")
	adds(t, ts, "0.0000000001ns", "2020-01-01T00:00:00.0000000000000000001Z")
	// The calendar is the proleptic Gregorian, with no leap second.
	adds(t, "2020-02-28T23:00:00Z", "1h", "2020-02-29T00:00:00Z")
	adds(t, "2100-02-28T23:00:00Z", "1h", "2100-03-01T00:00:00Z")
	adds(t, "2016-12-31T23:59:59Z", "1s", "2017-01-01T00:00:00Z")
	adds(t, "1970-01-01T00:00:00Z", "-1ns", "1969-12-31T23:59:59.999999999Z")
	adds(t, "0000-03-01T00:00:00Z", "-24h", "0000-02-29T00:00:00Z")
	// The timestamp's own fraction is part of its instant.
	adds(t, "2020-01-01T00:00:00.123Z", "-0.123s", ts)
	adds(t, "2020-01-01T00:00:00.9Z", "0s", "2020-01-01T00:00:00.9Z")
}

func TestConformance_LT010_Answer(t *testing.T) {
	conformance.Covers(t, "LT-010")
	adds(t, "2020-01-01T00:00:00Z", "500ms", "2020-01-01T00:00:00.5Z")
	adds(t, "2020-01-01T00:00:00.500Z", "0s", "2020-01-01T00:00:00.5Z")
	adds(t, "2020-01-01T00:00:00.5Z", "0.5s", "2020-01-01T00:00:01Z")
	adds(t, "2020-01-01T00:00:00-00:00", "0s", "2020-01-01T00:00:00Z")
	adds(t, "2020-01-01T00:00:00-08:00", "8h", "2020-01-01T08:00:00-08:00")
	adds(t, "0000-01-01T00:00:00Z", "1s", "0000-01-01T00:00:01Z")
	adds(t, "9999-12-31T23:59:58Z", "1.999999999s", "9999-12-31T23:59:59.999999999Z")
	for _, tt := range [][2]string{
		{"9999-12-31T23:00:00Z", "1h"}, {"0000-01-01T00:00:00Z", "-1ns"}, {"2020-01-01T00:00:00Z", "87600000h"},
	} {
		failsWith(t, "TimeAdd("+tt[0]+", "+tt[1]+")", timeAdd(tt[0], tt[1]), tenon.CodeTimeOutOfRange, at(1))
	}
}

func TestConformance_LT011_NotKnown(t *testing.T) {
	conformance.Covers(t, "LT-011")
	unknown := tenon.Unknown(tenon.StringType())
	for _, args := range [][]tenon.Value{
		{unknown, tenon.String("1h")}, {tenon.String("2020-01-01T00:00:00Z"), unknown}, {unknown, unknown},
	} {
		if got := call(stdlib.TimeAddFunc, args...); got.IsKnown() || !got.Type().Equal(tenon.StringType()) || !notNull(got) {
			t.Errorf("TimeAdd(%v) = %v, want the unknown string, not null", args, got)
		}
	}
	failsWith(t, "TimeAdd(unknown, 1d)", call(stdlib.TimeAddFunc, unknown, tenon.String("1d")), tenon.CodeTimeInvalidSyntax, at(1))
	failsWith(t, "TimeAdd(bad, unknown)", call(stdlib.TimeAddFunc, tenon.String("bad"), unknown), tenon.CodeTimeInvalidSyntax, at(0))
}
