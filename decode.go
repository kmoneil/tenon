package tenon

import (
	"math/big"
	"reflect"
	"strconv"

	"tenon/internal/decimal"
)

// Decode returns the Go value of type T that v decodes to. It converts v, as
// Convert does under the policy p, to the constraint that T maps to (see
// Encode for the mapping), and builds the Go value from the result: a list
// into a slice, a map into a map, an object into a struct field by field, and
// into a tenon.Value, the value as it is. What the conversion refuses, Decode
// refuses with the conversion's diagnostics. So an object decodes into a Go
// map under either policy, and a map into a struct under Unsafe only.
//
// Decode fails with a *DiagnosticError, with a diagnostic for each part of v
// that cannot be decoded, located by its path: a part that is unknown or
// pending (CodeDecodeNotKnown), or carries a mark (CodeDecodeMarked), other
// than parts decoded into a tenon.Value; a null decoded into a Go type that has
// no nil, other than a field marked optional (CodeDecodeNull); a number the Go
// number type cannot hold, or an integer type a fraction
// (CodeDecodeOutOfRange); and a list of another length than a Go array
// (CodeDecodeLengthMismatch). An error value gives its own diagnostics. To
// decode a marked value, unmark it with UnmarkDeep first and keep the marks.
//
// A null decodes into a pointer, slice or map as nil, and into an optional
// field as the field's zero value, as an absent optional attribute leaves it. A
// float64 is the nearest to the number, ties to even; a big.Float takes the
// precision big.Float.SetRat gives it.
//
// Decode panics where T does not map to tenon, as Encode does, and if p is
// not Safe or Unsafe.
func Decode[T any](v Value, p Policy) (T, error) {
	var out T
	v.data()
	if p != Safe && p != Unsafe {
		usagePanic("Decode called with %s, which is neither Safe nor Unsafe", p)
	}
	m := mappingOf(reflect.TypeFor[T]())
	d := goDecoder{policy: p}
	d.decode(m, reflect.ValueOf(&out).Elem(), v, Path{}, false)
	if failure, failed := d.errs.value(); failed {
		var zero T
		return zero, &DiagnosticError{Value: failure}
	}
	return out, nil
}

// goDecoder decodes one value into Go, collecting what it cannot decode.
type goDecoder struct {
	policy Policy
	errs   containerErrors
}

func (d *goDecoder) fail(p Path, code Code, message string) {
	d.errs.addDiagnostic(Diagnostic{Code: code, Message: message, Path: p})
}

// failWith records diagnostics that arose within the part at p.
func (d *goDecoder) failWith(p Path, diags []Diagnostic) {
	for _, diag := range diags {
		at := p
		for _, s := range diag.Path.Steps() {
			at = at.extend(s)
		}
		diag.Path = at
		d.errs.addDiagnostic(diag)
	}
}

// decode converts v, which p locates, to the constraint m maps to, and builds
// the Go value in dst from the result. optional says dst is a field marked
// optional.
func (d *goDecoder) decode(m *goMapping, dst reflect.Value, v Value, p Path, optional bool) {
	n := v.n
	if m.kind == goValue {
		dst.Set(reflect.ValueOf(v))
		return
	}
	if n.state == stateError {
		d.failWith(p, n.data.([]Diagnostic))
		return
	}
	if m.unmarshal {
		d.unmarshal(dst, v, p)
		return
	}
	// The conversion keeps every mark on what it converts, so that a mark the
	// decoding must refuse is not dropped by the conversion unseen.
	c := converter{policy: d.policy, keepMarks: true}.value(v, m.constraint)
	if ms := n.markList(); ms != nil {
		c = WithMarks(c, ms...)
	}
	if c.n.state == stateError {
		d.failWith(p, c.n.data.([]Diagnostic))
		return
	}
	d.build(m, dst, c, p, optional)
}

// build builds the Go value in dst from v, which is converted already to the
// constraint m maps to, and which p locates.
func (d *goDecoder) build(m *goMapping, dst reflect.Value, v Value, p Path, optional bool) {
	n := v.n
	if m.kind == goValue {
		dst.Set(reflect.ValueOf(v))
		return
	}
	if m.unmarshal {
		d.unmarshal(dst, v, p)
		return
	}
	nilable := m.kind == goPointer || m.kind == goSlice || m.kind == goMap
	if n.marks != nil {
		d.fail(p, CodeDecodeMarked, "the value carries marks, which a Go "+m.rt.String()+" cannot hold; unmark it first")
		return
	}
	switch n.state {
	case statePending:
		if n.null == nullOnly && (nilable || optional) {
			dst.SetZero()
			return
		}
		d.fail(p, CodeDecodeNotKnown, "a pending value cannot be decoded into a Go "+m.rt.String())
		return
	case stateUnknown:
		d.fail(p, CodeDecodeNotKnown, "an unknown value of type "+n.typ.String()+" cannot be decoded into a Go "+m.rt.String())
		return
	case stateNull:
		if nilable || optional {
			dst.SetZero()
			return
		}
		d.fail(p, CodeDecodeNull, "a null cannot be decoded into a Go "+m.rt.String()+", which has no nil")
		return
	}
	switch m.kind {
	case goBool:
		dst.SetBool(n.data.(bool))
	case goString:
		dst.SetString(n.data.(string))
	case goInt, goUint, goBigInt:
		d.integer(m, dst, n.data.(decimal.Dec), p)
	case goFloat:
		bits := 64
		if m.rt.Kind() == reflect.Float32 {
			bits = 32
		}
		f, err := strconv.ParseFloat(n.data.(decimal.Dec).String(), bits)
		if err != nil {
			d.fail(p, CodeDecodeOutOfRange, "the number "+valueText(v)+" is beyond the range of a Go "+m.rt.String())
			return
		}
		dst.SetFloat(f)
	case goBigRat:
		dst.Set(reflect.ValueOf(n.data.(decimal.Dec).Rat()).Elem())
	case goBigFloat:
		dst.Set(reflect.ValueOf(new(big.Float).SetRat(n.data.(decimal.Dec).Rat())).Elem())
	case goSlice, goArray:
		d.sequence(m, dst, v, p)
	case goMap:
		d.mapping(m, dst, v, p)
	case goStruct:
		attrs := n.data.([]Value)
		for i, a := range n.typ.t.attrs {
			for _, f := range m.fields {
				if f.name == a.name {
					d.build(f.m, dst.Field(f.index), attrs[i], p.Attribute(f.name), f.optional)
				}
			}
		}
	case goPointer:
		ptr := reflect.New(m.elem.rt)
		d.build(m.elem, ptr.Elem(), v, p, false)
		dst.Set(ptr)
	}
}

// unmarshal decodes v into dst by the UnmarshalValue method of dst's pointer,
// giving it v as it is.
func (d *goDecoder) unmarshal(dst reflect.Value, v Value, p Path) {
	u := dst.Addr().Interface().(ValueUnmarshaler)
	if err := u.UnmarshalValue(v); err != nil {
		failWithError(&d.errs, p, CodeDecodeUnmarshalFailed, err)
	}
}

// integer decodes a number into a Go integer type or a big.Int, which needs it
// to be an integer the type holds.
func (d *goDecoder) integer(m *goMapping, dst reflect.Value, num decimal.Dec, p Path) {
	small, c, exp := num.Parts()
	fail := func() {
		d.fail(p, CodeDecodeOutOfRange, "the number "+num.String()+" is not an integer that a Go "+m.rt.String()+" holds")
	}
	if exp < 0 || m.kind != goBigInt && exp > 20 {
		fail()
		return
	}
	if c == nil {
		c = big.NewInt(small)
	}
	value := new(big.Int).Mul(c, new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil))
	switch m.kind {
	case goBigInt:
		dst.Set(reflect.ValueOf(value).Elem())
	case goInt:
		if !value.IsInt64() || dst.OverflowInt(value.Int64()) {
			fail()
			return
		}
		dst.SetInt(value.Int64())
	default:
		if !value.IsUint64() || dst.OverflowUint(value.Uint64()) {
			fail()
			return
		}
		dst.SetUint(value.Uint64())
	}
}

// sequence decodes a list into a slice or an array. A slice of elements that
// map to no type decodes from a list, a set or a tuple, each member by its own
// conversion.
func (d *goDecoder) sequence(m *goMapping, dst reflect.Value, v Value, p Path) {
	n := v.n
	dynamic := m.typ.t == nil
	switch k := n.typ.t.kind; {
	case !dynamic && k != KindList, dynamic && k != KindList && k != KindSet && k != KindTuple:
		d.fail(p, CodeConvertNoConversion, n.typ.String()+" does not decode into a Go "+m.rt.String())
		return
	}
	members := n.data.([]Value)
	if n.typ.t.kind == KindSet {
		members = n.retrievedMembers()
	}
	target := dst
	if m.kind == goSlice {
		target = reflect.MakeSlice(m.rt, len(members), len(members))
	} else if len(members) != m.rt.Len() {
		d.fail(p, CodeDecodeLengthMismatch, "a list of "+strconv.Itoa(len(members))+" members does not decode into a Go "+m.rt.String())
		return
	}
	for i, member := range members {
		at := p.Index(NumberFromInt(int64(i)))
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
func (d *goDecoder) mapping(m *goMapping, dst reflect.Value, v Value, p Path) {
	n := v.n
	dynamic := m.typ.t == nil
	var names []string
	var members []Value
	switch k := n.typ.t.kind; {
	case k == KindMap:
		for _, e := range n.data.([]mapEntry) {
			names = append(names, e.key)
			members = append(members, e.val)
		}
	case dynamic && k == KindObject:
		members = n.data.([]Value)
		for _, a := range n.typ.t.attrs {
			names = append(names, a.name)
		}
	default:
		d.fail(p, CodeConvertNoConversion, n.typ.String()+" does not decode into a Go "+m.rt.String())
		return
	}
	target := reflect.MakeMapWithSize(m.rt, len(members))
	for i, member := range members {
		at := p.Index(String(names[i]))
		if n.typ.t.kind == KindObject {
			at = p.Attribute(names[i])
		}
		elem := reflect.New(m.rt.Elem()).Elem()
		if dynamic {
			d.decode(m.elem, elem, member, at, false)
		} else {
			d.build(m.elem, elem, member, at, false)
		}
		target.SetMapIndex(reflect.ValueOf(names[i]).Convert(m.rt.Key()), elem)
	}
	dst.Set(target)
}
