package gotenon

import (
	"errors"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
)

// DiagnosticError is the error that Decode and Encode return where they fail:
// an error value whose diagnostics say why, each located by its path.
type DiagnosticError struct {
	// Value is the error value.
	Value tenon.Value
}

// Error renders the diagnostics, as in "decode.null: ... at .name".
func (e *DiagnosticError) Error() string {
	var b strings.Builder
	for i, d := range e.Value.Diagnostics() {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(string(d.Code))
		b.WriteString(": ")
		b.WriteString(d.Message)
		if d.Path.Len() > 0 {
			b.WriteString(" at ")
			b.WriteString(d.Path.String())
		}
	}
	return b.String()
}

// Diagnostics returns the diagnostics of the failure, in order, in a new
// slice.
func (e *DiagnosticError) Diagnostics() []tenon.Diagnostic { return e.Value.Diagnostics() }

// failures collects diagnostics, each once.
type failures []tenon.Diagnostic

func (f *failures) add(d tenon.Diagnostic) {
	if !slices.ContainsFunc(*f, d.Equal) {
		*f = append(*f, d)
	}
}

// within adds diagnostics that arose within the part at p.
func (f *failures) within(p tenon.Path, diags []tenon.Diagnostic) {
	for _, d := range diags {
		d.Path = join(p, d.Path)
		f.add(d)
	}
}

// join returns p followed by the steps of q.
func join(p, q tenon.Path) tenon.Path {
	for _, s := range q.Steps() {
		if s.Kind() == tenon.StepAttribute {
			p = p.Attribute(s.Name())
		} else {
			p = p.Index(s.Key())
		}
	}
	return p
}

// withError adds err, which a marshaler or unmarshaler returned, for the part
// at p: the diagnostics of a *DiagnosticError located within the part, and
// otherwise a diagnostic of code whose message is the error's text.
func (f *failures) withError(p tenon.Path, code tenon.Code, err error) {
	var de *DiagnosticError
	if errors.As(err, &de) && de.Value != (tenon.Value{}) && de.Value.IsError() {
		f.within(p, de.Value.Diagnostics())
		return
	}
	message := strings.ToValidUTF8(err.Error(), "\U0000FFFD")
	if message == "" {
		message = "the method returned an error with no text"
	}
	f.add(tenon.Diagnostic{Code: code, Message: message, Path: p})
}

// Encode returns the value that the Go value x maps to: a Bool, String or
// Number for Go's booleans, strings and numbers, a list for a slice or an
// array, a map for a map with string keys, an object for a struct, and for a
// tenon.Value the value itself. The type follows from T, so a nil pointer,
// slice or map encodes as the null of the type T maps to. A slice or map whose
// elements need not share a type, as tenon.Value and marshaler types need not,
// encodes as a tuple or an object.
//
// A struct maps its exported fields to attributes, each named by its tenon tag
// or by the field's name: `tenon:"name"`, with `tenon:"name,optional"` marking
// an attribute that may be absent or null when decoding, and `tenon:"-"`
// leaving the field out. An embedded field must be named by its tag, and its
// fields are not promoted.
//
// Numbers encode exactly: a float64 is the terminating decimal it holds, not a
// rounded rendering of it. Encode fails with a *DiagnosticError where a part of
// x cannot be encoded: a NaN or an infinity (tenon.CodeEncodeNotANumber), a big.Rat
// that is not a terminating decimal (tenon.CodeEncodeInexact), a number outside the
// range of numbers (tenon.CodeNumberOutOfRange), a string that is not valid
// UTF-8 (tenon.CodeStringInvalidUTF8), a map of values whose types need not
// agree whose keys are empty or collide once normalized, and a MarshalValue
// method's failure (tenon.CodeEncodeMarshalFailed, or its own diagnostics).
//
// Encode panics where T does not map to tenon: an interface, a channel, a
// function, a complex number, a pointer to tenon.Value, a map without string
// keys, or a type that holds itself; where a struct's tags are malformed; on a
// required tenon.Value field, or any other tenon.Value, holding the zero
// Value; and on a MarshalValue method returning the zero Value.
func Encode[T any](x T) (tenon.Value, error) {
	m := mappingOf(reflect.TypeFor[T]())
	var e encoder
	v, ok := e.encode(m, reflect.ValueOf(&x).Elem(), tenon.Path{})
	if len(e.fails) > 0 || !ok {
		return tenon.Value{}, &DiagnosticError{Value: tenon.ErrorVal(e.fails...)}
	}
	return v, nil
}

// encoder encodes one Go value, collecting what it cannot encode.
type encoder struct {
	fails failures
}

func (e *encoder) fail(p tenon.Path, code tenon.Code, message string) {
	e.fails.add(tenon.Diagnostic{Code: code, Message: message, Path: p})
}

// encode encodes rv, which maps as m and which p locates, reporting whether it
// could.
func (e *encoder) encode(m *goMapping, rv reflect.Value, p tenon.Path) (tenon.Value, bool) {
	if m.marshal {
		return e.marshal(m, rv, p)
	}
	switch m.kind {
	case goValue:
		v := rv.Interface().(tenon.Value)
		if v == (tenon.Value{}) {
			usagePanic("Encode: the tenon.Value at %q is the zero Value, which is not a value", p.String())
		}
		return v, true
	case goBool:
		return tenon.Bool(rv.Bool()), true
	case goString:
		return e.fromData(tenon.String(rv.String()), p)
	case goInt:
		return tenon.NumberFromInt(rv.Int()), true
	case goUint:
		return tenon.NumberFromText(strconv.FormatUint(rv.Uint(), 10)), true
	case goFloat:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			e.fail(p, tenon.CodeEncodeNotANumber, strconv.FormatFloat(f, 'g', -1, 64)+" is not a number")
			return tenon.Value{}, false
		}
		text, _ := exactBigFloat(new(big.Float).SetFloat64(f))
		return tenon.NumberFromText(text), true
	case goBigInt, goBigFloat, goBigRat:
		return e.bigNumber(m, rv, p)
	case goSlice, goArray:
		return e.sequence(m, rv, p)
	case goMap:
		return e.mapping(m, rv, p)
	case goStruct:
		return e.structure(m, rv, p)
	}
	if rv.IsNil() {
		if m.typed() {
			return tenon.NullVal(m.typ), true
		}
		return tenon.NullVal(nullType(m.elem)), true
	}
	return e.encode(m.elem, rv.Elem(), p)
}

// fromData returns v, or records its diagnostics where it is an error value
// that data made.
func (e *encoder) fromData(v tenon.Value, p tenon.Path) (tenon.Value, bool) {
	if v.IsError() {
		e.fails.within(p, v.Diagnostics())
		return tenon.Value{}, false
	}
	return v, true
}

// marshal encodes rv by its MarshalValue method, which the Go type has itself or
// through its pointer.
func (e *encoder) marshal(m *goMapping, rv reflect.Value, p tenon.Path) (tenon.Value, bool) {
	var mv ValueMarshaler
	if rv.Type().Implements(marshalerGoType) {
		mv = rv.Interface().(ValueMarshaler)
	} else {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		mv = ptr.Interface().(ValueMarshaler)
	}
	v, err := mv.MarshalValue()
	if err != nil {
		e.fails.withError(p, tenon.CodeEncodeMarshalFailed, err)
		return tenon.Value{}, false
	}
	if v == (tenon.Value{}) {
		usagePanic("the MarshalValue method of %s returned the zero Value, which is not a value", m.rt)
	}
	return v, true
}

// nullType returns the type whose null a nil of the Go type that m maps
// encodes as: the type m maps to, and for a Go type that maps to no type, the
// least type that converts to the constraint it maps to, so that decoding reads
// the null back as nil. That is the empty tuple for a slice or an array, the
// object of the required fields for a struct, and the empty object for anything
// else.
func nullType(m *goMapping) tenon.Type {
	if m.typed() {
		return m.typ
	}
	switch m.kind {
	case goPointer:
		return nullType(m.elem)
	case goSlice, goArray:
		return tenon.Tuple()
	case goStruct:
		attrs := map[string]tenon.Type{}
		for _, f := range m.fields {
			if !f.optional {
				attrs[f.name] = nullType(f.m)
			}
		}
		return tenon.Object(attrs)
	}
	return tenon.Object(nil)
}

// bigNumber encodes a big.Int, a big.Float or a big.Rat exactly.
func (e *encoder) bigNumber(m *goMapping, rv reflect.Value, p tenon.Path) (tenon.Value, bool) {
	// A copy is made where rv is not addressable, as a map's elements are not;
	// it shares what the original holds, which is only read.
	ptr := reflect.New(m.rt)
	ptr.Elem().Set(rv)
	var (
		text string
		ok   bool
	)
	switch x := ptr.Interface().(type) {
	case *big.Int:
		text, ok = x.String(), true
	case *big.Float:
		if x.IsInf() {
			e.fail(p, tenon.CodeEncodeNotANumber, x.String()+" is not a number")
			return tenon.Value{}, false
		}
		text, ok = exactBigFloat(x)
	case *big.Rat:
		var exact bool
		text, exact, ok = exactRat(x)
		if !exact {
			e.fail(p, tenon.CodeEncodeInexact, "the rational "+x.String()+" is not a terminating decimal")
			return tenon.Value{}, false
		}
	}
	if !ok {
		e.fail(p, tenon.CodeNumberOutOfRange, "the number is outside the range of numbers")
		return tenon.Value{}, false
	}
	return e.fromData(tenon.NumberFromText(text), p)
}

// maxBinaryPlaces bounds how many binary places a number in the digit window
// can need: 2^3321929 exceeds 10^1000000.
const maxBinaryPlaces = 3321929

// exactBigFloat returns the decimal text of the value f, which is finite,
// holds exactly: a binary fraction always terminates in decimal. It reports
// false for a number too far outside the digit window to be worth computing.
func exactBigFloat(f *big.Float) (string, bool) {
	if f.Sign() == 0 {
		return "0", true
	}
	mant := new(big.Float)
	exp := int64(f.MantExp(mant))
	prec := int64(f.MinPrec())
	// f is c × 2^k, with c an odd integer of prec bits.
	c, _ := new(big.Float).SetMantExp(mant, int(prec)).Int(nil)
	k := exp - prec
	if exp > maxBinaryPlaces || -k > maxBinaryPlaces {
		return "", false
	}
	if k >= 0 {
		return c.Lsh(c, uint(k)).String(), true
	}
	// c × 2^k is c × 5^-k × 10^k.
	c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(-k), nil))
	return c.String() + "e" + strconv.FormatInt(k, 10), true
}

// exactRat returns the decimal text of r, whether r is a terminating decimal,
// one whose denominator has no prime factor but two and five, and false for a
// denominator too large to be worth dividing.
func exactRat(r *big.Rat) (text string, exact, ok bool) {
	den := new(big.Int).Set(r.Denom())
	if int64(den.BitLen()) > maxBinaryPlaces+1 {
		return "", true, false
	}
	twos := int64(den.TrailingZeroBits())
	den.Rsh(den, uint(twos))
	var fives int64
	q, rem := new(big.Int), new(big.Int)
	for _, step := range []int64{27, 1} {
		divisor := new(big.Int).Exp(big.NewInt(5), big.NewInt(step), nil)
		for {
			q.QuoRem(den, divisor, rem)
			if rem.Sign() != 0 {
				break
			}
			den.Set(q)
			fives += step
		}
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		return "", false, true
	}
	n := max(twos, fives)
	c := new(big.Int).Set(r.Num())
	c.Lsh(c, uint(n-twos))
	c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(n-fives), nil))
	return c.String() + "e-" + strconv.FormatInt(n, 10), true, true
}

// sequence encodes a slice or an array.
func (e *encoder) sequence(m *goMapping, rv reflect.Value, p tenon.Path) (tenon.Value, bool) {
	if m.kind == goSlice && rv.IsNil() {
		return tenon.NullVal(nullType(m)), true
	}
	members := make([]tenon.Value, rv.Len())
	ok := true
	for i := range members {
		v, good := e.encode(m.elem, rv.Index(i), p.Index(tenon.NumberFromInt(int64(i))))
		members[i], ok = v, ok && good
	}
	switch {
	case !ok:
		return tenon.Value{}, false
	case m.typed():
		return tenon.ListVal(m.elem.typ, members...), true
	}
	return tenon.TupleVal(members...), true
}

// mapping encodes a map with string keys.
func (e *encoder) mapping(m *goMapping, rv reflect.Value, p tenon.Path) (tenon.Value, bool) {
	if rv.IsNil() {
		return tenon.NullVal(nullType(m)), true
	}
	keys := make([]string, 0, rv.Len())
	for _, k := range rv.MapKeys() {
		keys = append(keys, k.String())
	}
	slices.Sort(keys)
	entries := make(map[string]tenon.Value, len(keys))
	normalized := map[string]string{}
	ok := true
	for _, key := range keys {
		sv := tenon.String(key)
		if sv.IsError() {
			e.fail(p, tenon.CodeStringInvalidUTF8, "the map key "+strconv.QuoteToASCII(key)+" is not well-formed UTF-8")
			ok = false
			continue
		}
		canonical := sv.AsString()
		if other, dup := normalized[canonical]; dup {
			e.fail(p, tenon.CodeMapDuplicateKey, "the map keys "+strconv.QuoteToASCII(other)+" and "+strconv.QuoteToASCII(key)+" are the same key after normalization")
			ok = false
			continue
		}
		normalized[canonical] = key
		at := p.Index(tenon.String(canonical))
		if !m.typed() && canonical == "" {
			e.fail(at, tenon.CodeConvertUnexpectedAttribute, `the map key "" cannot be an attribute name`)
			ok = false
			continue
		}
		v, good := e.encode(m.elem, rv.MapIndex(reflect.ValueOf(key).Convert(m.rt.Key())), at)
		entries[canonical], ok = v, ok && good
	}
	switch {
	case !ok:
		return tenon.Value{}, false
	case m.typed():
		return tenon.MapVal(m.elem.typ, entries), true
	}
	return tenon.ObjectVal(entries), true
}

// structure encodes a struct.
func (e *encoder) structure(m *goMapping, rv reflect.Value, p tenon.Path) (tenon.Value, bool) {
	attrs := make(map[string]tenon.Value, len(m.fields))
	ok := true
	for _, f := range m.fields {
		fv := rv.Field(f.index)
		if f.m.kind == goValue && fv.Interface().(tenon.Value) == (tenon.Value{}) {
			if f.optional {
				continue
			}
			usagePanic("Encode: required field %s of %s is a tenon.Value holding the zero Value", m.rt.Field(f.index).Name, m.rt)
		}
		v, good := e.encode(f.m, fv, p.Attribute(f.name))
		attrs[f.name], ok = v, ok && good
	}
	if !ok {
		return tenon.Value{}, false
	}
	return tenon.ObjectVal(attrs), true
}
