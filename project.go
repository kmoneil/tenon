package tenon

import (
	"tenon/internal/decimal"
)

// ProjectJSON returns v rendered as JSON text, and true. The projection is
// one-way and lossy: types do not survive it, and neither do marks, the
// difference between a list, a set and a tuple, or that between a map and an
// object. No function reads the JSON back into a value.
//
// Null is null, a Bool is true or false, and a Number is its canonical text,
// exact to the last digit. A String is a JSON string escaping only what JSON
// requires: quotation mark, reverse solidus and the characters below U+0020.
// Lists, sets and tuples are arrays, a set's members in the order it
// iterates, and maps and objects are objects with their names in order. A
// capsule value is the display form its type declares, as a string. The text
// has no whitespace between tokens, so one value always projects to the same
// bytes.
//
// Where v cannot be projected, ProjectJSON returns an error value and false,
// with a diagnostic for each part of v that cannot be, located by its path: a
// value that is unknown or pending (CodeSerializeNotKnown), a value carrying a
// redacting mark, whose contents are then not looked at
// (CodeSerializeRedacted), and a capsule value whose type declares no display
// form (CodeSerializeUnencodableCapsule). An error value gives itself. To
// project what a redacting mark withholds, unmark the value first.
//
// ProjectJSON panics on the zero Value.
func ProjectJSON(v Value) ([]byte, Value, bool) {
	if v.data().state == stateError {
		return nil, v, false
	}
	var p projector
	b := p.value(nil, v, Path{})
	if failure, failed := p.errs.value(); failed {
		return nil, failure, false
	}
	return b, Value{}, true
}

// projector projects one value, collecting what it cannot project.
type projector struct {
	errs containerErrors
}

func (p *projector) fail(at Path, code Code, message string) {
	p.errs.addDiagnostic(Diagnostic{Code: code, Message: message, Path: at})
}

// value appends the projection of v, which at locates.
func (p *projector) value(b []byte, v Value, at Path) []byte {
	n := v.n
	if ms := n.redactingMarks(); ms != nil {
		p.fail(at, CodeSerializeRedacted, "the value carries the redacting mark "+redactedText(ms)+", and is not projected")
		return b
	}
	switch n.state {
	case statePending:
		p.fail(at, CodeSerializeNotKnown, "a pending value has no content to project")
		return b
	case stateUnknown:
		p.fail(at, CodeSerializeNotKnown, "an unknown value of type "+n.typ.String()+" has no content to project")
		return b
	case stateNull:
		return append(b, "null"...)
	}
	switch n.typ.t.kind {
	case KindBool:
		if n.data.(bool) {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case KindNumber:
		return append(b, n.data.(decimal.Dec).String()...)
	case KindString:
		return appendJSONString(b, n.data.(string))
	case KindList, KindSet, KindTuple:
		b = append(b, '[')
		for i, m := range n.data.([]Value) {
			if i > 0 {
				b = append(b, ',')
			}
			b = p.value(b, m, at.extend(indexStep(NumberFromInt(int64(i)))))
		}
		return append(b, ']')
	case KindMap:
		b = append(b, '{')
		for i, e := range n.data.([]mapEntry) {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendJSONString(b, e.key)
			b = append(b, ':')
			b = p.value(b, e.val, at.extend(indexStep(String(e.key))))
		}
		return append(b, '}')
	case KindObject:
		b = append(b, '{')
		for i, m := range n.data.([]Value) {
			if i > 0 {
				b = append(b, ',')
			}
			name := n.typ.t.attrs[i].name
			b = appendJSONString(b, name)
			b = append(b, ':')
			b = p.value(b, m, at.extend(attributeStep(name)))
		}
		return append(b, '}')
	}
	d := n.typ.t.capsule
	if d.display == nil {
		p.fail(at, CodeSerializeUnencodableCapsule, "capsule type "+quoted(d.name)+" declares no display form")
		return b
	}
	return appendJSONString(b, d.display(n.data))
}

// appendJSONString appends s as a JSON string: quotation mark and reverse
// solidus escaped, the characters below U+0020 escaped by their short forms
// where JSON has one and as \u00 and two lowercase hexadecimal digits where it
// does not, and every other character as its UTF-8 bytes.
func appendJSONString(b []byte, s string) []byte {
	const hexDigits = "0123456789abcdef"
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\t':
			b = append(b, '\\', 't')
		case '\n':
			b = append(b, '\\', 'n')
		case '\f':
			b = append(b, '\\', 'f')
		case '\r':
			b = append(b, '\\', 'r')
		default:
			if c < 0x20 {
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
				continue
			}
			b = append(b, c)
		}
	}
	return append(b, '"')
}
