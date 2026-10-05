package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// formatDate calls FormatDate with the format and the timestamp.
func formatDate(f, ts string) tenon.Value {
	return call(stdlib.FormatDateFunc, tenon.String(f), tenon.String(ts))
}

// formatsDate checks that FormatDate of f and ts answers want.
func formatsDate(t *testing.T, f, ts, want string) {
	t.Helper()
	if got := formatDate(f, ts); !got.Equal(tenon.String(want)) {
		t.Errorf("FormatDate(%q, %q) = %v, want %q", f, ts, got, want)
	}
}

// refusesTimestamp checks that FormatDate refuses ts, naming what.
func refusesTimestamp(t *testing.T, ts, what string) {
	t.Helper()
	got := formatDate("YYYY", ts)
	failsWith(t, "FormatDate(YYYY, "+ts+")", got, tenon.CodeTimeInvalidSyntax, at(1))
	if got.IsError() && !strings.Contains(got.Diagnostics()[0].Message, what) {
		t.Errorf("FormatDate(YYYY, %q) fails %q, want it to name %q", ts, got.Diagnostics()[0].Message, what)
	}
}

func TestConformance_LT001_TimestampGrammar(t *testing.T) {
	conformance.Covers(t, "LT-001")
	for _, ts := range []string{
		"2006-01-02T15:04:05Z", "2006-01-02t15:04:05z", "2006-01-02T15:04:05.5+01:00",
		"2006-01-02T15:04:05.123456789123456-08:30", "2006-01-02T15:04:05-00:00",
	} {
		formatsDate(t, "YYYY", ts, "2006")
	}
	for _, tt := range []struct{ ts, what string }{
		{"", "within its year"},
		{"bad", "byte 0"},
		{"2006-01-02 15:04:05Z", "'T' was expected at byte 10"},
		{"2006-1-02T15:04:05Z", "its month is not 2 digits"},
		{"2006-01-02T15:04:05", "offset from UTC"},
		{"2006-01-02T15:04:05.Z", "fraction of a second has no digit"},
		{"2006-01-02T15:04:05,5Z", "byte 19"},
		{"2006-01-02T15:04:05+0100", "+hh:mm or -hh:mm"},
		{" 2006-01-02T15:04:05Z", "byte 0"},
		{"2006-01-02T15:04:05Z ", "byte 19"},
		{"+2006-01-02T15:04:05Z", "byte 0"},
		{"2006-01-02T15:04:05\U0000FF3A", "byte 19"},
	} {
		refusesTimestamp(t, tt.ts, tt.what)
	}
}

func TestConformance_LT002_TimestampRanges(t *testing.T) {
	conformance.Covers(t, "LT-002")
	formatsDate(t, "YYYY-MM-DD", "0000-01-01T00:00:00Z", "0000-01-01")
	formatsDate(t, "YYYY-MM-DD", "9999-12-31T23:59:59Z", "9999-12-31")
	formatsDate(t, "YYYY-MM-DD", "2024-02-29T00:00:00Z", "2024-02-29")
	formatsDate(t, "YYYY-MM-DD", "2000-02-29T00:00:00Z", "2000-02-29")
	formatsDate(t, "ZZZZZ", "2006-01-02T15:04:05-00:00", "+00:00")
	formatsDate(t, "ZZZZZ", "2006-01-02T15:04:05+23:59", "+23:59")
	for _, tt := range []struct{ ts, what string }{
		{"10000-01-01T00:00:00Z", "byte 4"},
		{"2006-00-01T00:00:00Z", "month, 00"},
		{"2006-13-01T00:00:00Z", "month, 13"},
		{"2023-02-29T00:00:00Z", "February 2023"},
		{"1900-02-29T00:00:00Z", "February 1900"},
		{"2006-04-31T00:00:00Z", "April 2006"},
		{"2006-01-00T00:00:00Z", "day, 00"},
		{"2006-01-02T24:00:00Z", "hour, 24"},
		{"2006-01-02T15:60:00Z", "minute, 60"},
		{"2016-12-31T23:59:60Z", "leap second"},
		{"2006-01-02T15:04:61Z", "second, 61"},
		{"2006-01-02T15:04:05+24:00", "+24:00"},
		{"2006-01-02T15:04:05+01:60", "+01:60"},
	} {
		refusesTimestamp(t, tt.ts, tt.what)
	}
}

func TestConformance_LT003_FractionAndFailures(t *testing.T) {
	conformance.Covers(t, "LT-003")
	// Any number of digits; no directive writes them.
	formatsDate(t, "ss", "2006-01-02T15:04:05."+strings.Repeat("9", 100)+"Z", "05")
	// A timestamp that is none fails at it, whatever the format.
	refusesTimestamp(t, "2006-01-02", "byte 10")
}

func TestConformance_LT004_Format(t *testing.T) {
	conformance.Covers(t, "LT-004")
	ts := "2006-01-02T15:04:05Z"
	formatsDate(t, "YYYY-MM-DD'T'hh:mm:ssZ", ts, "2006-01-02T15:04:05Z")
	formatsDate(t, "'It''s' D MMM, '' \U000000E9 -", ts, "It's 2 Jan, ' \U000000E9 -")
	formatsDate(t, "''", ts, "'")
	formatsDate(t, "", ts, "")
	// A format of any length is read whole.
	long := strings.Repeat("x", 70000)
	formatsDate(t, "YYYY'"+long+"'", ts, "2006"+long)
	for _, tt := range []struct{ f, what string }{
		{"T", "the letter 'T'"},
		{"YYY", "YY or YYYY"},
		{"MMMMM", "M, MM, MMM or MMMM"},
		{"ZZ", "Z, ZZZ, ZZZZ or ZZZZZ"},
		{"a", "aa"},
		{"'abc", "no quotation mark closes"},
		{"'abc''", "no quotation mark closes"},
		{strings.Repeat("Y", 70000), "has no meaning"},
	} {
		got := formatDate(tt.f, ts)
		failsWith(t, "FormatDate("+tt.f[:min(len(tt.f), 20)]+")", got, tenon.CodeTimeInvalidSyntax, at(0))
		if got.IsError() && !strings.Contains(got.Diagnostics()[0].Message, tt.what) {
			t.Errorf("FormatDate(%.20q) fails %q, want it to say %q", tt.f, got.Diagnostics()[0].Message, tt.what)
		}
	}
	if got := call(stdlib.FormatDateFunc, tenon.String("YYYY"), tenon.Unknown(tenon.StringType())); !notNull(got) {
		t.Errorf("FormatDate of an unknown timestamp = %v, want not null", got)
	}
}

func TestConformance_LT005_Directives(t *testing.T) {
	conformance.Covers(t, "LT-005")
	// 2017-03-02 is a Thursday; the fields are the timestamp's own, not
	// moved to UTC.
	ts := "2017-03-02T13:04:05.5-08:30"
	for _, tt := range []struct{ f, want string }{
		{"YY YYYY", "17 2017"},
		{"M MM MMM MMMM", "3 03 Mar March"},
		{"D DD", "2 02"},
		{"EEE EEEE", "Thu Thursday"},
		{"h hh H HH", "13 13 1 01"},
		{"AA aa", "PM pm"},
		{"m mm s ss", "4 04 5 05"},
		{"Z ZZZ ZZZZ ZZZZZ", "-08:30 -0830 -0830 -08:30"},
	} {
		formatsDate(t, tt.f, ts, tt.want)
	}
	formatsDate(t, "YY YYYY h H HH AA Z ZZZ ZZZZ ZZZZZ EEEE", "0005-01-01T00:07:09Z", "05 0005 0 12 12 AM Z UTC +0000 +00:00 Saturday")
	formatsDate(t, "H AA", "2006-01-02T12:00:00+05:45", "12 PM")
	formatsDate(t, "EEEE", "1600-02-29T00:00:00Z", "Tuesday")
}

func TestConformance_LT006_NotKnown(t *testing.T) {
	conformance.Covers(t, "LT-006")
	unknown := tenon.Unknown(tenon.StringType())
	got := call(stdlib.FormatDateFunc, tenon.String("'On' D, YYYY"), unknown)
	if got.IsKnown() || !notNull(got) || got.Range().StringPrefix() != "On " {
		t.Errorf("FormatDate(format, unknown) = %v, want the unknown string beginning %q", got, "On ")
	}
	got = call(stdlib.FormatDateFunc, tenon.Narrow(unknown, tenon.StringPrefix("2: YYYY")), tenon.String("2006-01-02T15:04:05Z"))
	if got.IsKnown() || !notNull(got) || got.Range().StringPrefix() != "2: " {
		t.Errorf("FormatDate(unknown format, timestamp) = %v, want the unknown string beginning %q", got, "2: ")
	}
	// What is known fails now.
	failsWith(t, "FormatDate(T, unknown)", call(stdlib.FormatDateFunc, tenon.String("T"), unknown), tenon.CodeTimeInvalidSyntax, at(0))
	failsWith(t, "FormatDate(unknown, bad)", call(stdlib.FormatDateFunc, unknown, tenon.String("bad")), tenon.CodeTimeInvalidSyntax, at(1))
}
