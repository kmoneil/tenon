package tenon

import (
	"strconv"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/kmoneil/tenon/internal/uni"
)

// ParseJSON reads data, JSON text (RFC 8259), into a value that c admits,
// converting what the text is read as to c under the policy p as Convert does
// (JS-021). The text is read as JSON implies (JS-020): a number as the Number
// its text writes, exactly; a string as the String of its characters,
// normalized; true and false as Bools; an array as the tuple of its members,
// and an object as the object of its members; and null as the pending value
// known to be null, which the conversion makes the null of the type c admits
// there, or of the type the members beside it in a list, set or map settle.
// Read with Any(), a value is what JSON implies.
//
// The text is RFC 8259's grammar, strictly: one value, with whitespace before
// and after it and nothing else, no byte order mark, no comment, no trailing
// comma, and no number with a leading zero (JS-002). ParseJSON fails with a
// [*Error] whose diagnostics are located by the paths of the parts that fail:
//   - text that is not JSON, with CodeJSONInvalidSyntax, its message naming
//     the byte offset and what was expected there, and nothing more is read
//     (JS-003);
//   - arrays and objects nested more than 512 levels deep, with
//     CodeJSONTooDeep, at the 513th, where the reading stops too;
//   - a string that is not well-formed UTF-8, or an escape naming a surrogate
//     that no escaped pair completes, with CodeStringInvalidUTF8: no
//     replacement character is put in its place (JS-010);
//   - a number beyond the range of numbers, with CodeNumberOutOfRange, and
//     one whose text is longer than 10,000 bytes, with CodeNumberTooLong
//     (JS-011);
//   - an empty name, with CodeObjectEmptyName, and two members of one object
//     with one name, as written or after normalization, with
//     CodeObjectDuplicateName, located at the object, as Object reports them,
//     whatever the object becomes (JS-020);
//
// every one of them but the first two reported together, in the order of the
// text (JS-022), and where there are none, what the conversion fails with.
//
// Reading the text does work in proportion to its length, n log n at most,
// however it is shaped; the conversion does what Convert does, which for some
// values is more, so a program reading text it did not write bounds its
// length (JS-004). It panics on the zero Constraint, as Convert does.
func ParseJSON(data []byte, c Constraint, p Policy) (Value, error) {
	c.data()
	r := jsonReader{data: data}
	v := r.document()
	switch {
	case r.stopped != nil:
		return Value{}, asError(errorValue(*r.stopped))
	case len(r.diags) > 0:
		return Value{}, asError(errorValue(r.diags...))
	}
	out := Convert(v, c, p)
	if out.n.state == stateError {
		return Value{}, asError(out)
	}
	return out, nil
}

// jsonMaxDepth is how deep a text's arrays and objects may nest (JS-003).
const jsonMaxDepth = 512

// jsonNull is what a JSON null is read as: the pending value of Any known to
// be null (JS-020).
var jsonNull = sync.OnceValue(func() Value { return Narrow(Pending(Any()), NullOnly()) })

// jsonReader reads one JSON text. Its failures are collected as it reads,
// located by the path of the part it is in (steps), and it builds no value
// once one has failed, reading on only to find the others; a failure of the
// text itself stops the reading.
type jsonReader struct {
	data    []byte
	i       int
	depth   int
	steps   []jsonStep
	diags   []Diagnostic
	stopped *Diagnostic
}

// jsonStep is a step into the member being read: an array's element by its
// index, or an object's member by its name, where the name is an attribute
// name a path can step to (named), and otherwise no step a path can take.
type jsonStep struct {
	index int
	name  string
	kind  uint8
}

const (
	stepIndexed = iota
	stepNamed
	stepUnnamed
)

// failed reports whether the reading has failed, so that no value is built.
func (r *jsonReader) failed() bool { return r.stopped != nil || len(r.diags) > 0 }

// path returns the path of the part being read, up to a member whose name no
// path can step to, where it stops at the object holding it.
func (r *jsonReader) path() Path {
	p := Path{}
	for _, s := range r.steps {
		switch s.kind {
		case stepIndexed:
			p = p.Index(NumberFromInt(int64(s.index)))
		case stepNamed:
			p = p.Attribute(s.name)
		default:
			return p
		}
	}
	return p
}

// fail adds the diagnostics of the error value e, located at the part being
// read.
func (r *jsonReader) fail(e Value) {
	at := r.path()
	for _, d := range e.n.diagnostics() {
		d.Path = at
		r.diags = append(r.diags, d)
	}
}

// stop ends the reading with code, its message saying where and why.
func (r *jsonReader) stop(code Code, message string) {
	if r.stopped == nil {
		r.stopped = &Diagnostic{Code: code, Message: message, Path: r.path()}
	}
}

// unexpected ends the reading at the byte offset r.i, which does not fit the
// grammar, where want was expected.
func (r *jsonReader) unexpected(want string) {
	found := "the end of the text"
	if r.i < len(r.data) {
		b := r.data[r.i]
		switch {
		case b >= 0x20 && b < 0x7f:
			found = strconv.QuoteRune(rune(b))
		default:
			found = "byte 0x" + strconv.FormatUint(uint64(b), 16)
		}
	}
	r.stop(CodeJSONInvalidSyntax, "the text is not JSON at byte "+strconv.Itoa(r.i)+": "+want+" was expected, and "+found+" was found")
}

// space skips whitespace.
func (r *jsonReader) space() {
	for r.i < len(r.data) {
		switch r.data[r.i] {
		case ' ', '\t', '\n', '\r':
			r.i++
		default:
			return
		}
	}
}

// document reads the text: one value, and whitespace around it.
func (r *jsonReader) document() Value {
	r.space()
	v := r.value()
	if r.stopped != nil {
		return Value{}
	}
	r.space()
	if r.i < len(r.data) {
		r.unexpected("the end of the text")
		return Value{}
	}
	return v
}

// value reads one value at r.i.
func (r *jsonReader) value() Value {
	if r.i >= len(r.data) {
		r.unexpected("a value")
		return Value{}
	}
	switch b := r.data[r.i]; {
	case b == '{':
		return r.object()
	case b == '[':
		return r.array()
	case b == '"':
		// Read and judged whatever failed before, so that every failure is
		// reported (JS-022).
		s, ok := r.string()
		if !ok {
			return Value{}
		}
		v := String(s)
		if v.n.state == stateError {
			r.fail(v)
			return Value{}
		}
		return v
	case b == '-' || b >= '0' && b <= '9':
		return r.number()
	case b == 't':
		return r.literal("true", Bool(true))
	case b == 'f':
		return r.literal("false", Bool(false))
	case b == 'n':
		return r.literal("null", jsonNull())
	}
	r.unexpected("a value")
	return Value{}
}

// literal reads the literal word, the value v.
func (r *jsonReader) literal(word string, v Value) Value {
	for j := 0; j < len(word); j++ {
		if r.i >= len(r.data) || r.data[r.i] != word[j] {
			r.unexpected(strconv.Quote(word))
			return Value{}
		}
		r.i++
	}
	return v
}

// number reads a number: RFC 8259's grammar, which has no leading zero, then
// the number its text writes, exactly (JS-011).
func (r *jsonReader) number() Value {
	start := r.i
	if r.data[r.i] == '-' {
		r.i++
	}
	switch {
	case r.i < len(r.data) && r.data[r.i] == '0':
		r.i++
	case r.digits() == 0:
		r.unexpected("a digit")
		return Value{}
	}
	if r.i < len(r.data) && r.data[r.i] == '.' {
		r.i++
		if r.digits() == 0 {
			r.unexpected("a digit")
			return Value{}
		}
	}
	if r.i < len(r.data) && (r.data[r.i] == 'e' || r.data[r.i] == 'E') {
		r.i++
		if r.i < len(r.data) && (r.data[r.i] == '+' || r.data[r.i] == '-') {
			r.i++
		}
		if r.digits() == 0 {
			r.unexpected("a digit")
			return Value{}
		}
	}
	v := NumberFromText(string(r.data[start:r.i]))
	if v.n.state == stateError {
		r.fail(v)
		return Value{}
	}
	return v
}

// digits reads a run of decimal digits, and returns how many.
func (r *jsonReader) digits() int {
	start := r.i
	for r.i < len(r.data) && r.data[r.i] >= '0' && r.data[r.i] <= '9' {
		r.i++
	}
	return r.i - start
}

// string reads a string at r.i, its quotation marks included, and returns the
// characters it writes, decoded, and false where the text stops there. A
// string that is ill-formed (JS-010) adds its failure and gives what is
// written, which the caller does not build a value of.
func (r *jsonReader) string() (string, bool) {
	r.i++
	start := r.i
	for r.i < len(r.data) {
		switch b := r.data[r.i]; {
		case b == '"':
			s := string(r.data[start:r.i])
			r.i++
			return s, true
		case b == '\\':
			return r.escaped(start)
		case b < 0x20:
			r.unexpected("a character, which below U+0020 must be escaped,")
			return "", false
		default:
			r.i++
		}
	}
	r.unexpected("'\"'")
	return "", false
}

// escaped reads the rest of a string that holds an escape, from r.i, the
// string's characters beginning at start.
func (r *jsonReader) escaped(start int) (string, bool) {
	buf := append([]byte(nil), r.data[start:r.i]...)
	for r.i < len(r.data) {
		b := r.data[r.i]
		switch {
		case b == '"':
			r.i++
			return string(buf), true
		case b < 0x20:
			r.unexpected("a character, which below U+0020 must be escaped,")
			return "", false
		case b != '\\':
			buf = append(buf, b)
			r.i++
			continue
		}
		at := r.i
		r.i++
		if r.i >= len(r.data) {
			break
		}
		switch e := r.data[r.i]; e {
		case '"', '\\', '/':
			buf = append(buf, e)
		case 'b':
			buf = append(buf, '\b')
		case 'f':
			buf = append(buf, '\f')
		case 'n':
			buf = append(buf, '\n')
		case 'r':
			buf = append(buf, '\r')
		case 't':
			buf = append(buf, '\t')
		case 'u':
			r.i++
			c, ok := r.hex4()
			if !ok {
				return "", false
			}
			if utf16.IsSurrogate(c) {
				// A high surrogate is the first of a pair, and the escape after
				// it must be the second; any other is one alone (JS-010).
				lo := rune(-1)
				if c < 0xdc00 && r.i+1 < len(r.data) && r.data[r.i] == '\\' && r.data[r.i+1] == 'u' {
					save := r.i
					r.i += 2
					if l, ok := r.hex4(); ok && l >= 0xdc00 && l <= 0xdfff {
						lo = l
					} else if !ok {
						return "", false
					} else {
						r.i = save
					}
				}
				if lo < 0 {
					r.fail(errorValue(Diagnostic{Code: CodeStringInvalidUTF8,
						Message: "the escape at byte " + strconv.Itoa(at) + " names the surrogate U+" + strconv.FormatInt(int64(c), 16) + ", which no escaped pair completes"}))
					buf = utf8.AppendRune(buf, utf8.RuneError)
					continue
				}
				c = utf16.DecodeRune(c, lo)
			}
			buf = utf8.AppendRune(buf, c)
			continue
		default:
			r.unexpected("an escape, one of '\"', '\\\\', '/', 'b', 'f', 'n', 'r', 't' or 'u',")
			return "", false
		}
		r.i++
	}
	r.unexpected("'\"'")
	return "", false
}

// hex4 reads the four hexadecimal digits of a \u escape.
func (r *jsonReader) hex4() (rune, bool) {
	var c rune
	for range 4 {
		if r.i >= len(r.data) {
			r.unexpected("a hexadecimal digit")
			return 0, false
		}
		b := r.data[r.i]
		switch {
		case b >= '0' && b <= '9':
			c = c<<4 | rune(b-'0')
		case b >= 'a' && b <= 'f':
			c = c<<4 | rune(b-'a'+10)
		case b >= 'A' && b <= 'F':
			c = c<<4 | rune(b-'A'+10)
		default:
			r.unexpected("a hexadecimal digit")
			return 0, false
		}
		r.i++
	}
	return c, true
}

// enter reads past the bracket that opens an array or object at r.i, one
// level deeper, and reports whether the text may nest that deep (JS-003).
func (r *jsonReader) enter() bool {
	r.depth++
	if r.depth > jsonMaxDepth {
		r.stop(CodeJSONTooDeep, "the text nests arrays and objects more than "+strconv.Itoa(jsonMaxDepth)+" levels deep, at byte "+strconv.Itoa(r.i))
		return false
	}
	r.i++
	r.space()
	return true
}

// array reads an array, the tuple of its members.
func (r *jsonReader) array() Value {
	if !r.enter() {
		return Value{}
	}
	defer func() { r.depth-- }()
	var vals []Value
	if r.i < len(r.data) && r.data[r.i] == ']' {
		r.i++
		return Tuple()
	}
	for index := 0; ; index++ {
		r.steps = append(r.steps, jsonStep{index: index, kind: stepIndexed})
		v := r.value()
		r.steps = r.steps[:len(r.steps)-1]
		if r.stopped != nil {
			return Value{}
		}
		vals = append(vals, v)
		r.space()
		if r.i < len(r.data) && r.data[r.i] == ',' {
			r.i++
			r.space()
			continue
		}
		if r.i >= len(r.data) || r.data[r.i] != ']' {
			r.unexpected("',' or ']'")
			return Value{}
		}
		r.i++
		break
	}
	if r.failed() {
		return Value{}
	}
	return Tuple(vals...)
}

// object reads an object, the object of its members, whose names are judged
// as Object judges them, a name given twice among them, once the object ends
// (JS-020, JS-022).
func (r *jsonReader) object() Value {
	if !r.enter() {
		return Value{}
	}
	defer func() { r.depth-- }()
	var entries []namedEntry[Value]
	if r.i < len(r.data) && r.data[r.i] == '}' {
		r.i++
		return Object(nil)
	}
	for {
		if r.i >= len(r.data) || r.data[r.i] != '"' {
			r.unexpected("a name, which is a string,")
			return Value{}
		}
		name, ok := r.string()
		if !ok {
			return Value{}
		}
		r.space()
		if r.i >= len(r.data) || r.data[r.i] != ':' {
			r.unexpected("':'")
			return Value{}
		}
		r.i++
		r.space()
		step := jsonStep{name: name, kind: stepUnnamed}
		if normalized, err := uni.Canonical(name); err == nil && normalized != "" {
			step.name, step.kind = normalized, stepNamed
		}
		r.steps = append(r.steps, step)
		v := r.value()
		r.steps = r.steps[:len(r.steps)-1]
		if r.stopped != nil {
			return Value{}
		}
		entries = append(entries, namedEntry[Value]{original: name, value: v})
		r.space()
		if r.i < len(r.data) && r.data[r.i] == ',' {
			r.i++
			r.space()
			continue
		}
		if r.i >= len(r.data) || r.data[r.i] != '}' {
			r.unexpected("',' or '}'")
			return Value{}
		}
		r.i++
		break
	}
	entries, shared := checkNames(entries, attributeNames)
	// The path is made only for a failure: made for every object, it would
	// cost each its depth, and a deep text the square of its length.
	var at *Path
	located := func(d Diagnostic) Diagnostic {
		if at == nil {
			p := r.path()
			at = &p
		}
		d.Path = *at
		return d
	}
	for _, e := range entries {
		if e.fault != "" {
			r.diags = append(r.diags, located(nameFault(attributeNames, e.fault, e.original)))
		}
	}
	for _, group := range shared {
		r.diags = append(r.diags, located(sharedName(attributeNames, group)))
	}
	if r.failed() {
		return Value{}
	}
	return objectOfEntries(entries)
}
