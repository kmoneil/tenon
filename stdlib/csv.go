package stdlib

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
)

// csvReader reads CSV text by the dialect of LE-007, go-cty's, which is
// encoding/csv's with its defaults: a comma between fields, a record to a
// line end, a quoted field holding commas, line ends and doubled quotation
// marks.
type csvReader struct {
	text string
	pos  int // the byte offset of the next line
	line int // the number of the line last read, from 1
	// ended reports whether the record last read ended at a line end, not
	// at the end of the text.
	ended bool
}

// csvFailure is a failure of the text: its code, and its message.
type csvFailure struct {
	code    tenon.Code
	message string
	// open reports a quoted field the end of the text cut off, which more
	// text may yet close.
	open bool
}

// readLine returns the next line of the text, its LF included: CR LF read
// as LF, and at the end of the text, where no LF ends the line, a final CR
// left out. It reports false at the end of the text.
func (r *csvReader) readLine() (string, bool) {
	if r.pos >= len(r.text) {
		return "", false
	}
	rest := r.text[r.pos:]
	r.line++
	i := strings.IndexByte(rest, '\n')
	if i < 0 {
		r.pos = len(r.text)
		return strings.TrimSuffix(rest, "\r"), true
	}
	r.pos += i + 1
	line := rest[:i+1]
	if strings.HasSuffix(line, "\r\n") {
		line = line[:len(line)-2] + "\n"
	}
	return line, true
}

// record reads the next record, passing over lines that hold nothing,
// and returns its fields and the line it begins on; or false at the end of
// the text; or its failure.
func (r *csvReader) record() ([]string, int, bool, *csvFailure) {
	var line string
	for {
		l, ok := r.readLine()
		if !ok {
			return nil, 0, false, nil
		}
		if l != "" && l != "\n" {
			line = l
			break
		}
	}
	// cur is the line the record is read on: it moves on with a quoted
	// field's next line only where that line holds something.
	start, cur, col := r.line, r.line, 1
	var fields []string
	at := func(line, col int) string {
		return "line " + strconv.Itoa(line) + ", column " + strconv.Itoa(col)
	}
	for {
		if line == "" || line[0] != '"' {
			i := strings.IndexByte(line, ',')
			field := line
			if i >= 0 {
				field = field[:i]
			} else {
				field = strings.TrimSuffix(field, "\n")
			}
			if j := strings.IndexByte(field, '"'); j >= 0 {
				return nil, 0, true, &csvFailure{code: tenon.CodeCSVInvalidSyntax,
					message: "CSVDecode: " + at(cur, col+j) + ": a quotation mark in a field not quoted"}
			}
			fields = append(fields, field)
			if i < 0 {
				r.ended = strings.HasSuffix(line, "\n")
				return fields, start, true, nil
			}
			line, col = line[i+1:], col+i+1
			continue
		}
		// A quoted field, to the quotation mark no other doubles.
		line, col = line[1:], col+1
		var b strings.Builder
		for {
			i := strings.IndexByte(line, '"')
			switch {
			case i >= 0:
				b.WriteString(line[:i])
				line, col = line[i+1:], col+i+1
				switch {
				case strings.HasPrefix(line, `"`):
					b.WriteByte('"')
					line, col = line[1:], col+1
					continue
				case strings.HasPrefix(line, ","):
					fields = append(fields, b.String())
					line, col = line[1:], col+1
				case line == "" || line == "\n":
					fields = append(fields, b.String())
					r.ended = line == "\n"
					return fields, start, true, nil
				default:
					return nil, 0, true, &csvFailure{code: tenon.CodeCSVInvalidSyntax,
						message: "CSVDecode: " + at(cur, col-1) + ": a quotation mark ending a quoted field is followed by neither a comma nor the end of the line"}
				}
			case line != "":
				b.WriteString(line)
				col += len(line)
				next, ok := r.readLine()
				if !ok {
					return nil, 0, true, &csvFailure{code: tenon.CodeCSVInvalidSyntax, open: true,
						message: "CSVDecode: " + at(cur, col) + ": a quoted field is not closed before the end of the text"}
				}
				line = next
				if next != "" {
					cur, col = r.line, 1
				}
				continue
			default:
				return nil, 0, true, &csvFailure{code: tenon.CodeCSVInvalidSyntax, open: true,
					message: "CSVDecode: " + at(cur, col) + ": a quoted field is not closed before the end of the text"}
			}
			break
		}
	}
}

// csvHeader reads the header of the text, a leading byte order mark
// passed over, and returns the object type its names give, each a String;
// or its failure: no header at all, a name empty or given twice, or the
// text's.
func csvHeader(r *csvReader) (tenon.Type, []string, *csvFailure) {
	r.text = strings.TrimPrefix(r.text, "\U0000FEFF")
	names, _, ok, failure := r.record()
	switch {
	case failure != nil:
		return tenon.Type{}, nil, failure
	case !ok:
		return tenon.Type{}, nil, &csvFailure{code: tenon.CodeCSVMissingHeader, message: "CSVDecode: the text has no header line"}
	}
	attrs := map[string]tenon.Type{}
	var seen []string
	for k, name := range names {
		n := tenon.String(name).AsString()
		switch {
		case n == "":
			return tenon.Type{}, nil, &csvFailure{code: tenon.CodeObjectEmptyName,
				message: "CSVDecode: column " + strconv.Itoa(k+1) + " of the header has no name, and an attribute must have one"}
		case slices.Contains(seen, n):
			return tenon.Type{}, nil, &csvFailure{code: tenon.CodeObjectDuplicateName,
				message: "CSVDecode: the header names the column " + strconv.Quote(n) + " twice"}
		}
		seen = append(seen, n)
		attrs[n] = tenon.StringType()
	}
	return tenon.ObjectType(attrs), seen, nil
}

// csvRows reads the records after the header into objects of the type t,
// by the names, each record holding as many fields as the header; at the
// end of the text, or, where whole is false, at the last record that a
// line end closes. It returns them, or the failure of the text.
func csvRows(r *csvReader, t tenon.Type, names []string, whole bool) ([]tenon.Value, *csvFailure) {
	var rows []tenon.Value
	for {
		fields, start, ok, failure := r.record()
		switch {
		case failure != nil:
			return nil, failure
		case !ok:
			return rows, nil
		case !whole && !r.ended:
			// The text may go on to lengthen this record's last field.
			return rows, nil
		case len(fields) != len(names):
			return nil, &csvFailure{code: tenon.CodeCSVFieldCount,
				message: "CSVDecode: the record on line " + strconv.Itoa(start) + " has " + strconv.Itoa(len(fields)) +
					" fields, and the header " + strconv.Itoa(len(names))}
		}
		attrs := make(map[string]tenon.Value, len(names))
		for k, name := range names {
			attrs[name] = tenon.String(fields[k])
		}
		rows = append(rows, tenon.Object(attrs))
	}
}

// csvFailed returns the failure f located at the argument.
func csvFailed(f *csvFailure) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: f.code, Message: f.message, Path: argument(0)})
}

// csvPrefix reads what the recorded prefix p of text not known yet
// settles: the type its header gives, where a line end closes it, and how
// many records after it a line end closes; or the failure those lines
// already make, whatever follows.
func csvPrefix(p string) (t tenon.Type, rows int, settled bool, failure *csvFailure) {
	r := &csvReader{text: p}
	t, names, f := csvHeader(r)
	switch {
	case f != nil && (f.open || f.code == tenon.CodeCSVMissingHeader):
		return tenon.Type{}, 0, false, nil
	case f != nil && f.code == tenon.CodeCSVInvalidSyntax:
		return tenon.Type{}, 0, false, f
	case !r.ended:
		// The header's last name may go on, and a name empty or given
		// twice so far may not be when it does.
		return tenon.Type{}, 0, false, nil
	case f != nil:
		return tenon.Type{}, 0, false, f
	}
	got, f := csvRows(r, t, names, false)
	if f != nil && !f.open {
		return tenon.Type{}, 0, false, f
	}
	return t, len(got), true, nil
}

// CSVDecodeFunc reads CSV text into a list of objects: the first record
// names the attributes, each a String, and each later record is an object
// of its fields by those names. The dialect is go-cty's, encoding/csv's
// with its defaults: a comma between fields, a record ending at LF or CR
// LF, a field quoted with " holding commas, line ends (CR LF read as LF)
// and "" for a quotation mark, lines holding nothing passed over, spaces
// kept. A leading byte order mark is not part of the first name, where
// go-cty's keeps it. A header name empty fails with
// tenon.CodeObjectEmptyName, where go-cty's makes an attribute of it, and
// one given twice with tenon.CodeObjectDuplicateName; text with no header
// with tenon.CodeCSVMissingHeader; a record of another field count with
// tenon.CodeCSVFieldCount; and text that is not CSV with
// tenon.CodeCSVInvalidSyntax, naming the line and the column, each located
// at the argument. Text not known yet whose recorded prefix holds the
// header and a line end after it answers a list of that object type, at
// least as long as the records a line end closes there.
var CSVDecodeFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "CSVDecode",
	Description: "Parses the given string as Comma Separated Values (as defined by RFC 4180) and returns a list of objects representing the table of data, using the first row as a header row to define the object attributes.",
	Params:      []tenon.Param{stringParam("str", "The CSV text to decode.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if !args[0].IsKnown() {
			t, _, settled, failure := csvPrefix(args[0].Range().StringPrefix())
			switch {
			case failure != nil:
				return tenon.Constraint{}, tenon.NewError(csvFailed(failure))
			case settled:
				return tenon.Exactly(tenon.ListType(t)), nil
			}
			return tenon.ListOf(tenon.ObjectWith(map[string]tenon.Field{}, false)), nil
		}
		t, _, failure := csvHeader(&csvReader{text: args[0].AsString()})
		if failure != nil {
			return tenon.Constraint{}, tenon.NewError(csvFailed(failure))
		}
		return tenon.Exactly(tenon.ListType(t)), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		if !args[0].IsKnown() {
			_, rows, settled, _ := csvPrefix(args[0].Range().StringPrefix())
			answer := unknownOf(rc)
			if settled && rows > 0 {
				answer = tenon.Narrow(answer, tenon.LengthMin(int64(rows)))
			}
			return answer, nil
		}
		r := &csvReader{text: args[0].AsString()}
		t, names, failure := csvHeader(r)
		if failure != nil {
			return csvFailed(failure), nil
		}
		rows, failure := csvRows(r, t, names, true)
		if failure != nil {
			return csvFailed(failure), nil
		}
		return tenon.List(t, rows...), nil
	},
})
