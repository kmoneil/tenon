package gotenon

import (
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
)

// Decode returns the Go value of type T that v decodes to. It converts v, as
// tenon.Convert does under the policy p, to the constraint that T maps to (see
// Encode for the mapping), and builds the Go value from the result: a list
// into a slice, a map into a map, an object into a struct field by field, and
// into a tenon.Value, the value as it is. What the conversion refuses, Decode
// refuses with the conversion's diagnostics. So an object decodes into a Go
// map under either policy, and a map into a struct under Unsafe only.
//
// Decode fails with a *DiagnosticError, with a diagnostic for each part of v
// that cannot be decoded, located by its path: a part that is unknown or
// pending (tenon.CodeDecodeNotKnown), or carries a mark
// (tenon.CodeDecodeMarked), other than parts decoded into a tenon.Value or by
// an unmarshaler; a null decoded into a Go type that has no nil, other than a
// field marked optional (tenon.CodeDecodeNull); a number the Go number type
// cannot hold, or an integer type a fraction (tenon.CodeDecodeOutOfRange); a
// list of another length than a Go array (tenon.CodeDecodeLengthMismatch); and
// an UnmarshalValue method's failure (tenon.CodeDecodeUnmarshalFailed, or its
// own diagnostics). An error value gives its own diagnostics. To decode a
// marked value, unmark it with tenon.UnmarkDeep first and keep the marks.
//
// A pointer, at any depth, to a type whose pointer implements
// ValueUnmarshaler decodes by that method too, as encoding/json treats a
// pointer to an Unmarshaler: a null that carries no mark leaves the pointer
// nil, and any other value, a marked null among them, goes as it is to the
// method of a new value that the pointer then points to.
//
// A null decodes into a pointer, slice or map as nil, and into an optional
// field as the field's zero value, as an absent optional attribute leaves it. A
// float64 is the nearest to the number, ties to even; a big.Float takes the
// precision big.Float.SetRat gives it, and a json.Number the number's canonical
// text.
//
// Decode panics where T does not map to tenon, as Encode does, and if p is
// not Safe or Unsafe. It panics as well where T is an interface type, or holds
// one: nothing in a value says which Go type it would take, and tenon.Value is
// the Go type that holds any value, so decode into that.
func Decode[T any](v tenon.Value, p tenon.Policy) (T, error) {
	var out T
	if v == (tenon.Value{}) {
		usagePanic("Decode called with the zero Value, which is not a value")
	}
	if p != tenon.Safe && p != tenon.Unsafe {
		usagePanic("Decode called with %s, which is neither Safe nor Unsafe", p)
	}
	m := mappingOf(reflect.TypeFor[T]())
	d := decoder{policy: p, marked: map[string]tenon.Diagnostic{}}
	d.decode(m, reflect.ValueOf(&out).Elem(), v, tenon.Path{}, false)
	if len(d.fails.list) > 0 {
		var zero T
		return zero, &DiagnosticError{Value: tenon.ErrorVal(d.fails.list...)}
	}
	return out, nil
}

// decoder decodes one value into Go, collecting what it cannot decode.
type decoder struct {
	policy tenon.Policy
	fails  failures
	// marked holds the diagnostic of each marked part, by the pathKey of its
	// path, for build to give when it comes to the part. Marked parts are found
	// by looking at a value before it is converted, since the conversion drops
	// a mark that does not propagate, and a dropped mark must still be refused.
	marked map[string]tenon.Diagnostic
	// typeTexts holds each tenon type's rendered text, once per Decode,
	// for the messages that name one.
	typeTexts map[tenon.Type]string
}

// valueText renders v for a message, cut to 32 bytes at a character
// boundary: the widest in-window number is a million digits, and a message
// is for reading, not for carrying the value.
func valueText(v tenon.Value) string { return shortText(v.String()) }

// shortText returns s, or its first 32 bytes to a character boundary and an
// ellipsis where s is longer.
func shortText(s string) string {
	const limit = 32
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// typeText renders a tenon type for a message, once for the run of one
// Decode however many parts name it: a value holding many unknown members
// of one large type would otherwise pay the type's text for every one.
func (d *decoder) typeText(t tenon.Type) string {
	if d.typeTexts == nil {
		d.typeTexts = map[tenon.Type]string{}
	}
	s, ok := d.typeTexts[t]
	if !ok {
		s = t.String()
		d.typeTexts[t] = s
	}
	return s
}

func (d *decoder) fail(p tenon.Path, code tenon.Code, message string) {
	d.fails.add(tenon.Diagnostic{Code: code, Message: message, Path: p})
}

// decode decodes v, which p locates, into dst: it looks for the marked parts,
// converts v to the constraint m maps to, and builds the Go value from the
// result. optional says dst is a field marked optional.
func (d *decoder) decode(m *goMapping, dst reflect.Value, v tenon.Value, p tenon.Path, optional bool) {
	if m.kind == goValue {
		dst.Set(reflect.ValueOf(v))
		return
	}
	if v.IsError() {
		d.fails.within(p, v.Diagnostics())
		return
	}
	if byUnmarshaler(m) {
		d.build(m, dst, v, p, optional)
		return
	}
	var marked []tenon.Diagnostic
	scanMarks(m, v, p, &marked)
	c := tenon.Convert(v, m.constraint, d.policy)
	if c.IsError() {
		d.fails.within(p, c.Diagnostics())
		return
	}
	for _, diag := range marked {
		d.marked[pathKey(diag.Path)] = diag
	}
	d.build(m, dst, c, p, optional)
}

// scanMarks adds a diagnostic to found for each part of v that carries a mark,
// located as the part is once v is converted to the constraint m maps to, and
// looks no further within a part that does. It passes over what m takes as it
// is and the members of a collection that are decoded by their own
// conversions, which look at their own marks.
func scanMarks(m *goMapping, v tenon.Value, p tenon.Path, found *[]tenon.Diagnostic) {
	if m.kind == goValue || byUnmarshaler(m) {
		return
	}
	if _, marks := tenon.Unmark(v); len(marks) > 0 {
		*found = append(*found, tenon.Diagnostic{Code: tenon.CodeDecodeMarked, Path: p,
			Message: "the value carries marks, which a Go " + m.rt.String() + " cannot hold; unmark it first"})
		return
	}
	if !v.HasContent() {
		return
	}
	switch kind := v.Type().Kind(); {
	case m.kind == goPointer:
		scanMarks(m.elem, v, p, found)
	case (m.kind == goSlice || m.kind == goArray) && m.typed() &&
		(kind == tenon.KindList || kind == tenon.KindSet || kind == tenon.KindTuple):
		for i, e := range v.Elements() {
			scanMarks(m.elem, e, p.Index(tenon.NumberFromInt(int64(i))), found)
		}
	case m.kind == goMap && m.typed():
		// An object converts to a map, whose elements are located by key.
		entries(v, func(name string, e tenon.Value) {
			scanMarks(m.elem, e, p.Index(tenon.String(name)), found)
		})
	case m.kind == goStruct:
		// A map converts to an object, whose attributes are located by name.
		// The entries come in name order, as the fields are held, so the
		// field for each entry is found by walking both together.
		i := 0
		entries(v, func(name string, e tenon.Value) {
			for i < len(m.fields) && m.fields[i].name < name {
				i++
			}
			if i < len(m.fields) && m.fields[i].name == name {
				scanMarks(m.fields[i].m, e, p.Attribute(name), found)
			}
		})
	}
}

// entries calls each with the name and member of each entry of a map or
// attribute of an object, in name order, and does nothing for other values.
func entries(v tenon.Value, each func(name string, member tenon.Value)) {
	switch v.Type().Kind() {
	case tenon.KindMap:
		for _, key := range v.MapKeys() {
			e, _ := v.MapElement(key)
			each(key, e)
		}
	case tenon.KindObject:
		for _, name := range v.Type().AttributeNames() {
			each(name, v.Attribute(name))
		}
	}
}

// pathKey returns a text that tells p apart from every other path, which
// Path.String does not quite do: it writes an attribute whose name is not an
// identifier as it writes an index by a String key.
func pathKey(p tenon.Path) string {
	var b strings.Builder
	for _, s := range p.Steps() {
		if s.Kind() == tenon.StepAttribute {
			b.WriteString("." + strconv.Quote(s.Name()))
		} else {
			b.WriteString(s.String())
		}
	}
	return b.String()
}

// byUnmarshaler reports whether m decodes by an unmarshaler: its Go type's
// pointer implements ValueUnmarshaler, or it is a pointer, at any depth, to a
// type whose pointer does (GO-041). Such a mapping takes values as they are,
// unknown, pending and marked ones among them.
func byUnmarshaler(m *goMapping) bool {
	for m.kind == goPointer {
		m = m.elem
	}
	return m.unmarshal
}

// unmarkedNull reports whether v is a null, or a pending value known to be
// null, that carries no mark: what a pointer decodes as nil even where its
// element decodes by an unmarshaler.
func unmarkedNull(v tenon.Value) bool {
	if _, marks := tenon.Unmark(v); len(marks) > 0 {
		return false
	}
	if v.IsPending() {
		isNull := tenon.IsNull(v)
		return isNull.IsKnown() && isNull.AsBool()
	}
	return v.IsKnown() && !v.HasContent()
}

// unmarshal decodes v into dst by the UnmarshalValue method of dst's pointer,
// giving it v as it is.
func (d *decoder) unmarshal(dst reflect.Value, v tenon.Value, p tenon.Path) {
	u := dst.Addr().Interface().(ValueUnmarshaler)
	if err := u.UnmarshalValue(v); err != nil {
		d.fails.withError(p, tenon.CodeDecodeUnmarshalFailed, err)
	}
}

// build builds the Go value in dst from v, which is converted already to the
// constraint m maps to, and which p locates.
func (d *decoder) build(m *goMapping, dst reflect.Value, v tenon.Value, p tenon.Path, optional bool) {
	if m.kind == goValue {
		dst.Set(reflect.ValueOf(v))
		return
	}
	if m.unmarshal {
		d.unmarshal(dst, v, p)
		return
	}
	if m.kind == goPointer && byUnmarshaler(m) {
		// A pointer to what decodes by an unmarshaler decodes by it too
		// (GO-041): an unmarked null leaves it nil, as it leaves any pointer,
		// and anything else, a marked null among them, goes as it is to the
		// method of a new value.
		if unmarkedNull(v) {
			dst.SetZero()
			return
		}
		ptr := reflect.New(m.elem.rt)
		d.build(m.elem, ptr.Elem(), v, p, false)
		dst.Set(ptr)
		return
	}
	if len(d.marked) > 0 {
		if diag, ok := d.marked[pathKey(p)]; ok {
			d.fails.add(diag)
			return
		}
	}
	// A known value without content is a null, and a pending value known to
	// be null decodes as one.
	null := v.IsKnown() && !v.HasContent()
	if v.IsPending() {
		isNull := tenon.IsNull(v)
		null = isNull.IsKnown() && isNull.AsBool()
	}
	switch {
	case null:
		if m.kind == goPointer || m.kind == goSlice || m.kind == goMap || optional {
			dst.SetZero()
			return
		}
		d.fail(p, tenon.CodeDecodeNull, "a null cannot be decoded into a Go "+m.rt.String()+", which has no nil")
		return
	case v.IsPending():
		d.fail(p, tenon.CodeDecodeNotKnown, "a pending value cannot be decoded into a Go "+m.rt.String())
		return
	case !v.HasContent():
		d.fail(p, tenon.CodeDecodeNotKnown, "an unknown value of type "+d.typeText(v.Type())+" cannot be decoded into a Go "+m.rt.String())
		return
	}
	switch m.kind {
	case goInterface:
		// Nothing in a value says which Go type it would take [GO-011].
		usagePanic("Decode: the value at %q is decoded into a Go %s, an interface, which says nothing of the Go type it would take; decode into tenon.Value, which holds any value", p.String(), m.rt)
	case goBool:
		dst.SetBool(v.AsBool())
	case goString:
		dst.SetString(v.AsString())
	case goInt, goUint, goBigInt:
		d.integer(m, dst, v, p)
	case goFloat:
		bits := 64
		if m.rt.Kind() == reflect.Float32 {
			bits = 32
		}
		f, err := strconv.ParseFloat(v.String(), bits)
		if err != nil {
			d.fail(p, tenon.CodeDecodeOutOfRange, "the number "+valueText(v)+" is beyond the range of a Go "+m.rt.String())
			return
		}
		dst.SetFloat(f)
	case goJSONNumber:
		// The canonical text of the number [GO-034], which Value.String
		// gives for a Number.
		dst.SetString(v.String())
	case goBigRat:
		dst.Set(reflect.ValueOf(v.AsBigRat()).Elem())
	case goBigFloat:
		dst.Set(reflect.ValueOf(new(big.Float).SetRat(v.AsBigRat())).Elem())
	case goSlice, goArray:
		d.sequence(m, dst, v, p)
	case goMap:
		d.mapping(m, dst, v, p)
	case goStruct:
		// Attributes are taken in name order, so that failures read in member
		// order; the fields are held in that order, so the field for each
		// attribute is found by walking both together.
		i := 0
		for _, name := range v.Type().AttributeNames() {
			for i < len(m.fields) && m.fields[i].name < name {
				i++
			}
			if i < len(m.fields) && m.fields[i].name == name {
				f := m.fields[i]
				d.build(f.m, dst.Field(f.index), v.Attribute(name), p.Attribute(name), f.optional)
			}
		}
	case goPointer:
		ptr := reflect.New(m.elem.rt)
		d.build(m.elem, ptr.Elem(), v, p, false)
		dst.Set(ptr)
	default:
		usagePanic("Decode met the mapping kind %d of %s, which no arm of build handles; this is a defect in gotenon, not in the caller", m.kind, m.rt)
	}
}

// integer decodes a number into a Go integer type or a big.Int, which needs it
// to be an integer the type holds.
func (d *decoder) integer(m *goMapping, dst reflect.Value, v tenon.Value, p tenon.Path) {
	fail := func() {
		d.fail(p, tenon.CodeDecodeOutOfRange, "the number "+valueText(v)+" is not an integer that a Go "+m.rt.String()+" holds")
	}
	switch m.kind {
	case goInt:
		i, ok := v.AsInt64()
		if !ok || dst.OverflowInt(i) {
			fail()
			return
		}
		dst.SetInt(i)
	case goUint:
		// The canonical text of an integer that a uint64 holds is its digits.
		u, err := strconv.ParseUint(v.String(), 10, 64)
		if err != nil || dst.OverflowUint(u) {
			fail()
			return
		}
		dst.SetUint(u)
	default:
		i, ok := v.AsBigInt()
		if !ok {
			fail()
			return
		}
		dst.Set(reflect.ValueOf(i).Elem())
	}
}

// sequence decodes a list into a slice or an array. A slice of elements that
// map to no type decodes from a list, a set or a tuple, each member by its own
// conversion.
func (d *decoder) sequence(m *goMapping, dst reflect.Value, v tenon.Value, p tenon.Path) {
	dynamic := !m.typed()
	switch k := v.Type().Kind(); {
	case !dynamic && k != tenon.KindList, dynamic && k != tenon.KindList && k != tenon.KindSet && k != tenon.KindTuple:
		d.fail(p, tenon.CodeConvertNoConversion, d.typeText(v.Type())+" does not decode into a Go "+m.rt.String())
		return
	}
	members := v.Elements()
	target := dst
	if m.kind == goSlice {
		target = reflect.MakeSlice(m.rt, len(members), len(members))
	} else if len(members) != m.rt.Len() {
		d.fail(p, tenon.CodeDecodeLengthMismatch, "a list of "+strconv.Itoa(len(members))+" members does not decode into a Go "+m.rt.String())
		return
	}
	for i, member := range members {
		at := p.Index(tenon.NumberFromInt(int64(i)))
		if dynamic {
			d.decode(m.elem, target.Index(i), member, at, false)
		} else {
			d.build(m.elem, target.Index(i), member, at, false)
		}
	}
	if m.kind == goSlice {
		dst.Set(target)
	}
}

// mapping decodes a map into a Go map. A map of elements that map to no type
// decodes from a map or an object, each member by its own conversion.
func (d *decoder) mapping(m *goMapping, dst reflect.Value, v tenon.Value, p tenon.Path) {
	dynamic := !m.typed()
	var names []string
	var members []tenon.Value
	var steps []tenon.Path
	switch k := v.Type().Kind(); {
	case k == tenon.KindMap:
		for _, key := range v.MapKeys() {
			e, _ := v.MapElement(key)
			names, members, steps = append(names, key), append(members, e), append(steps, p.Index(tenon.String(key)))
		}
	case dynamic && k == tenon.KindObject:
		for _, name := range v.Type().AttributeNames() {
			names, members, steps = append(names, name), append(members, v.Attribute(name)), append(steps, p.Attribute(name))
		}
	default:
		d.fail(p, tenon.CodeConvertNoConversion, d.typeText(v.Type())+" does not decode into a Go "+m.rt.String())
		return
	}
	target := reflect.MakeMapWithSize(m.rt, len(members))
	for i, member := range members {
		elem := reflect.New(m.rt.Elem()).Elem()
		if dynamic {
			d.decode(m.elem, elem, member, steps[i], false)
		} else {
			d.build(m.elem, elem, member, steps[i], false)
		}
		target.SetMapIndex(reflect.ValueOf(names[i]).Convert(m.rt.Key()), elem)
	}
	dst.Set(target)
}
