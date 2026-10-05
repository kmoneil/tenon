package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// csvRows returns the list of objects of the attributes names, each row's
// fields in their order.
func csvRows(names []string, rows ...[]string) tenon.Value {
	attrs := map[string]tenon.Type{}
	for _, n := range names {
		attrs[n] = tenon.StringType()
	}
	t := tenon.ObjectType(attrs)
	var out []tenon.Value
	for _, row := range rows {
		m := map[string]tenon.Value{}
		for k, n := range names {
			m[n] = tenon.String(row[k])
		}
		out = append(out, tenon.Object(m))
	}
	return tenon.List(t, out...)
}

// decodesCSV checks that CSVDecode of text answers want.
func decodesCSV(t *testing.T, text string, want tenon.Value) {
	t.Helper()
	if got := call(stdlib.CSVDecodeFunc, tenon.String(text)); !got.Equal(want) {
		t.Errorf("CSVDecode(%q) = %v, want %v", text, got, want)
	}
}

func TestConformance_LE006_CSVDecode(t *testing.T) {
	conformance.Covers(t, "LE-006")
	ab := []string{"a", "b"}
	decodesCSV(t, "a,b\n1,2\n3,4\n", csvRows(ab, []string{"1", "2"}, []string{"3", "4"}))
	// A header alone is the empty list of its object type.
	decodesCSV(t, "a,b\n", csvRows(ab))
	// Values are strings, never numbers.
	decodesCSV(t, "n\n1.50\n", csvRows([]string{"n"}, []string{"1.50"}))
	got := call(stdlib.CSVDecodeFunc, tenon.String("a\nx\n"))
	if !notNull(got) || got.Type().Kind() != tenon.KindList {
		t.Errorf("CSVDecode = %v, want a list", got)
	}
}

func TestConformance_LE007_Dialect(t *testing.T) {
	conformance.Covers(t, "LE-007")
	ab, a := []string{"a", "b"}, []string{"a"}
	for _, tt := range []struct {
		text string
		want tenon.Value
	}{
		// CR LF ends a record as LF does; the last needs no line end; a
		// final CR is not part of it; lines holding nothing are passed
		// over.
		{"a,b\r\n1,2\r\n", csvRows(ab, []string{"1", "2"})},
		{"a,b\n1,2", csvRows(ab, []string{"1", "2"})},
		{"a,b\n1,2\r", csvRows(ab, []string{"1", "2"})},
		{"a,b\n\n1,2\n\n\n3,4\n", csvRows(ab, []string{"1", "2"}, []string{"3", "4"})},
		// Quoted fields hold commas, line ends and doubled quotation marks,
		// CR LF read as LF; a CR alone is data.
		{"a,b\n\"x,y\",\"say \"\"hi\"\"\"\n", csvRows(ab, []string{"x,y", "say \"hi\""})},
		{"a\n\"x\r\ny\"\n", csvRows(a, []string{"x\ny"})},
		{"a\n\"x\n\ny\"\n", csvRows(a, []string{"x\n\ny"})},
		{"a\nx\ry\n", csvRows(a, []string{"x\ry"})},
		// Spaces are kept; # begins no comment; a leading byte order mark
		// is passed over.
		{" a , b \n 1 , 2 \n", csvRows([]string{" a ", " b "}, []string{" 1 ", " 2 "})},
		{"#a\n1\n", csvRows([]string{"#a"}, []string{"1"})},
		{"\U0000FEFFa,b\n1,2\n", csvRows(ab, []string{"1", "2"})},
		// An empty field, quoted or not.
		{"a,b\n,\"\"\n", csvRows(ab, []string{"", ""})},
	} {
		decodesCSV(t, tt.text, tt.want)
	}
	for _, tt := range []struct{ text, at string }{
		{"a\nx\"y\n", "line 2, column 2"},
		{"a\n\"x\"y\n", "line 2, column 3"},
		{"a\n \"x\"\n", "line 2, column 2"},
		{"a\n\"x\n", "line 2, column 4"},
		{"a,b\n1,\"2\n3\n", "line 3, column 3"},
	} {
		got := call(stdlib.CSVDecodeFunc, tenon.String(tt.text))
		failsWith(t, "CSVDecode("+tt.text+")", got, tenon.CodeCSVInvalidSyntax, at(0))
		if got.IsError() && !strings.Contains(got.Diagnostics()[0].Message, tt.at) {
			t.Errorf("CSVDecode(%q) fails %q, want it to name %s", tt.text, got.Diagnostics()[0].Message, tt.at)
		}
	}
}

func TestConformance_LE008_Header(t *testing.T) {
	conformance.Covers(t, "LE-008")
	failsWith(t, "CSVDecode of nothing", call(stdlib.CSVDecodeFunc, tenon.String("")), tenon.CodeCSVMissingHeader, at(0))
	failsWith(t, "CSVDecode of empty lines", call(stdlib.CSVDecodeFunc, tenon.String("\n\r\n\n")), tenon.CodeCSVMissingHeader, at(0))
	failsWith(t, "CSVDecode of an empty first name", call(stdlib.CSVDecodeFunc, tenon.String(",a\n1,2\n")), tenon.CodeObjectEmptyName, at(0))
	failsWith(t, "CSVDecode of a trailing comma", call(stdlib.CSVDecodeFunc, tenon.String("a,\n1,2\n")), tenon.CodeObjectEmptyName, at(0))
	failsWith(t, "CSVDecode of a name twice", call(stdlib.CSVDecodeFunc, tenon.String("a,b,a\n1,2,3\n")), tenon.CodeObjectDuplicateName, at(0))
	failsWith(t, "CSVDecode of a name twice after normalization", call(stdlib.CSVDecodeFunc, tenon.String("e\U00000301,\U000000E9\n1,2\n")), tenon.CodeObjectDuplicateName, at(0))
}

func TestConformance_LE009_FieldCount(t *testing.T) {
	conformance.Covers(t, "LE-009")
	for _, text := range []string{"a,b\n1\n", "a,b\n1,2\n3,4,5\n", "a\n\n\n1,2\n"} {
		failsWith(t, "CSVDecode("+text+")", call(stdlib.CSVDecodeFunc, tenon.String(text)), tenon.CodeCSVFieldCount, at(0))
	}
	got := call(stdlib.CSVDecodeFunc, tenon.String("a,b\n1,2\n\n3\n"))
	if !got.IsError() || !strings.Contains(got.Diagnostics()[0].Message, "line 4") {
		t.Errorf("CSVDecode of a short record on line 4 = %v, want it named", got)
	}
}

func TestConformance_LE010_Derivation(t *testing.T) {
	conformance.Covers(t, "LE-010")
	c, err := tenon.ResultConstraint(stdlib.CSVDecodeFunc, []tenon.Value{tenon.String("a,b\n1\n")}, tenon.Safe)
	if want := tenon.Exactly(csvRows([]string{"a", "b"}).Type()); err != nil || !c.Equal(want) {
		t.Errorf("CSVDecode's derivation = %v, %v, want %v: the header settles it, a record not", c, err, want)
	}
	if _, err := tenon.ResultConstraint(stdlib.CSVDecodeFunc, []tenon.Value{tenon.String("a,a\n")}, tenon.Safe); err == nil {
		t.Errorf("CSVDecode's derivation of a name twice does not fail")
	}
}

func TestConformance_LE011_NotKnown(t *testing.T) {
	conformance.Covers(t, "LE-011")
	text := func(prefix string) tenon.Value {
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix(prefix))
	}
	listOf := csvRows([]string{"a", "b"}).Type()
	for _, tt := range []struct {
		prefix string
		least  int64
	}{
		{"a,b\n", 0},
		{"a,b\n1,2\n3", 1},
		{"a,b\r\n1,2\r\n3,4\r\n", 2},
		{"a,b\n\"1\n", 0},
	} {
		got := call(stdlib.CSVDecodeFunc, text(tt.prefix))
		if got.IsKnown() || got.IsPending() || !got.Type().Equal(listOf) || !notNull(got) || got.Range().LengthMin() != tt.least {
			t.Errorf("CSVDecode(unknown %q) = %v, want an unknown list of the header's type, at least %d long", tt.prefix, got, tt.least)
		}
	}
	// No header a line end ends: a list of some objects.
	for _, p := range []string{"", "a,b", "a,\"b\nc"} {
		got := call(stdlib.CSVDecodeFunc, text(p))
		if !got.IsPending() || got.Constraint().Kind() != tenon.ConstraintListOf || !notNull(got) {
			t.Errorf("CSVDecode(unknown %q) = %v, want a pending list, not null", p, got)
		}
	}
	// What the lines already make fail fails now.
	failsWith(t, "CSVDecode(unknown a,a LF)", call(stdlib.CSVDecodeFunc, text("a,a\n")), tenon.CodeObjectDuplicateName, at(0))
	failsWith(t, "CSVDecode(unknown with a short record)", call(stdlib.CSVDecodeFunc, text("a,b\n1\n2")), tenon.CodeCSVFieldCount, at(0))
	failsWith(t, "CSVDecode(unknown with a bare quote)", call(stdlib.CSVDecodeFunc, text("a,b\n1,x\"y")), tenon.CodeCSVInvalidSyntax, at(0))
	failsWith(t, "CSVDecode(unknown with a bare quote in the header)", call(stdlib.CSVDecodeFunc, text("a\"b")), tenon.CodeCSVInvalidSyntax, at(0))
}
