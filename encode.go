package tenon

import (
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"tenon/internal/decimal"
	"tenon/internal/uni"
)

// DiagnosticError is the error that Decode and Encode return where they fail:
// an error value whose diagnostics say why, each located by its path.
type DiagnosticError struct {
	// Value is the error value.
	Value Value
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
func (e *DiagnosticError) Diagnostics() []Diagnostic { return e.Value.Diagnostics() }

// Encode returns the value that the Go value x maps to: a Bool, String or
// Number for Go's booleans, strings and numbers, a list for a slice or an
// array, a map for a map with string keys, an object for a struct, and for a
// tenon.Value the value itself. The type follows from T, so a nil pointer,
// slice or map encodes as the null of the type T maps to.
//
// A struct maps its exported fields to attributes, each named by its tenon tag
// or by the field's name: `tenon:"name"`, with `tenon:"name,optional"` marking
// an attribute that may be absent or null when decoding, and `tenon:"-"`
// leaving the field out. An embedded field must be named by its tag, and its
// fields are not promoted.
//
// Numbers encode exactly: a float64 is the terminating decimal it holds, not a
// rounded rendering of it. Encode fails with a *DiagnosticError where a part of
// x cannot be encoded: a NaN or an infinity (CodeEncodeNotANumber), a big.Rat
// that is not a terminating decimal (CodeEncodeInexact), a number outside the
// range of numbers (CodeNumberOutOfRange), a string that is not valid UTF-8
// (CodeStringInvalidUTF8), and a map of values whose types need not agree
// whose keys are empty or collide once normalized.
//
// Encode panics where T does not map to tenon: an interface, a channel, a
// function, a complex number, a pointer to tenon.Value, a map without string
// keys, or a type that holds itself; where a struct's tags are malformed; and
// on a required tenon.Value field, or any other tenon.Value, holding the zero
// Value.
func Encode[T any](x T) (Value, error) {
	m := mappingOf(reflect.TypeFor[T]())
	var e goEncoder
	v, ok := e.encode(m, reflect.ValueOf(&x).Elem(), Path{})
	if failure, failed := e.errs.value(); failed || !ok {
		return Value{}, &DiagnosticError{Value: failure}
	}
	return v, nil
}

// goEncoder encodes one Go value, collecting what it cannot encode.
type goEncoder struct {
	errs containerErrors
}

func (e *goEncoder) fail(p Path, code Code, message string) {
	e.errs.addDiagnostic(Diagnostic{Code: code, Message: message, Path: p})
}

// encode encodes rv, which maps as m and which p locates, reporting whether it
// could.
func (e *goEncoder) encode(m *goMapping, rv reflect.Value, p Path) (Value, bool) {
	switch m.kind {
	case goValue:
		v := rv.Interface().(Value)
		if v.n == nil {
			usagePanic("Encode: the tenon.Value at %q is the zero Value, which is not a value", p.String())
		}
		return v, true
	case goBool:
		return Bool(rv.Bool()), true
	case goString:
		v := String(rv.String())
		if v.n.state == stateError {
			e.fail(p, CodeStringInvalidUTF8, v.n.data.([]Diagnostic)[0].Message)
			return Value{}, false
		}
		return v, true
	case goInt:
		return NumberFromInt(rv.Int()), true
	case goUint:
		d, _ := decimal.FromParts(new(big.Int).SetUint64(rv.Uint()), 0)
		return numberValue(d), true
	case goFloat:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			e.fail(p, CodeEncodeNotANumber, strconv.FormatFloat(f, 'g', -1, 64)+" is not a number")
			return Value{}, false
		}
		d, _ := exactBigFloat(new(big.Float).SetFloat64(f))
		return numberValue(d), true
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
		if m.typ.t != nil {
			return NullVal(m.typ), true
		}
		return NullVal(nullType(m.elem)), true
	}
	return e.encode(m.elem, rv.Elem(), p)
}

// nullType returns the type whose null a nil of the Go type that m maps
// encodes as: the type m maps to, and for a Go type that maps to no type, the
// least type that converts to the constraint it maps to, so that decoding reads
// the null back as nil. That is the empty tuple for a slice or an array, the
// object of the required fields for a struct, and the empty object for anything
// else.
func nullType(m *goMapping) Type {
	if m.typ.t != nil {
		return m.typ
	}
	switch m.kind {
	case goPointer:
		return nullType(m.elem)
	case goSlice, goArray:
		return Tuple()
	case goStruct:
		attrs := map[string]Type{}
		for _, f := range m.fields {
			if !f.optional {
				attrs[f.name] = nullType(f.m)
			}
		}
		return Object(attrs)
	}
	return Object(nil)
}

// bigNumber encodes a big.Int, a big.Float or a big.Rat exactly.
func (e *goEncoder) bigNumber(m *goMapping, rv reflect.Value, p Path) (Value, bool) {
	// A copy is made where rv is not addressable, as a map's elements are not;
	// it shares what the original holds, which is only read.
	ptr := reflect.New(m.rt)
	ptr.Elem().Set(rv)
	var (
		d   decimal.Dec
		err error
	)
	switch x := ptr.Interface().(type) {
	case *big.Int:
		d, err = decimal.FromParts(x, 0)
	case *big.Float:
		if x.IsInf() {
			e.fail(p, CodeEncodeNotANumber, x.String()+" is not a number")
			return Value{}, false
		}
		d, err = exactBigFloat(x)
	case *big.Rat:
		var exact bool
		d, exact, err = exactRat(x)
		if !exact {
			e.fail(p, CodeEncodeInexact, "the rational "+x.String()+" is not a terminating decimal")
			return Value{}, false
		}
	}
	if err != nil {
		e.fail(p, CodeNumberOutOfRange, "the number is outside the range of numbers")
		return Value{}, false
	}
	return numberValue(d), true
}

// maxBinaryPlaces bounds how many binary places a number in the digit window
// can need: 2^3321929 exceeds 10^1000000.
const maxBinaryPlaces = 3321929

// exactBigFloat returns the decimal that f, which is finite, holds exactly: a
// binary fraction always terminates in decimal. It refuses a number outside the
// digit window before computing it.
func exactBigFloat(f *big.Float) (decimal.Dec, error) {
	if f.Sign() == 0 {
		return decimal.Dec{}, nil
	}
	mant := new(big.Float)
	exp := int64(f.MantExp(mant))
	prec := int64(f.MinPrec())
	// f is c × 2^k, with c an odd integer of prec bits.
	c, _ := new(big.Float).SetMantExp(mant, int(prec)).Int(nil)
	k := exp - prec
	if exp > maxBinaryPlaces || -k > maxBinaryPlaces {
		return decimal.Dec{}, decimal.ErrOutOfRange
	}
	if k >= 0 {
		return decimal.FromParts(c.Lsh(c, uint(k)), 0)
	}
	// c × 2^k is c × 5^-k × 10^k, and an odd c times a power of five is no
	// multiple of ten.
	five := new(big.Int).Exp(big.NewInt(5), big.NewInt(-k), nil)
	return decimal.FromParts(c.Mul(c, five), k)
}

// exactRat returns the decimal that r holds, and whether r is a terminating
// decimal: one whose denominator has no prime factor but two and five.
func exactRat(r *big.Rat) (decimal.Dec, bool, error) {
	den := new(big.Int).Set(r.Denom())
	if int64(den.BitLen()) > maxBinaryPlaces+1 {
		// Either the value does not terminate or its last digit is below the
		// window; telling which is not worth the arithmetic.
		return decimal.Dec{}, true, decimal.ErrOutOfRange
	}
	twos := int64(den.TrailingZeroBits())
	den.Rsh(den, uint(twos))
	var fives int64
	five27 := new(big.Int).Exp(big.NewInt(5), big.NewInt(27), nil)
	q, rem := new(big.Int), new(big.Int)
	for {
		q.QuoRem(den, five27, rem)
		if rem.Sign() != 0 {
			break
		}
		den.Set(q)
		fives += 27
	}
	five := big.NewInt(5)
	for {
		q.QuoRem(den, five, rem)
		if rem.Sign() != 0 {
			break
		}
		den.Set(q)
		fives++
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		return decimal.Dec{}, false, nil
	}
	n := max(twos, fives)
	c := new(big.Int).Set(r.Num())
	c.Lsh(c, uint(n-twos))
	c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(n-fives), nil))
	d, err := decimal.FromParts(c, -n)
	return d, true, err
}

// sequence encodes a slice or an array.
func (e *goEncoder) sequence(m *goMapping, rv reflect.Value, p Path) (Value, bool) {
	if m.kind == goSlice && rv.IsNil() {
		if m.typ.t != nil {
			return NullVal(m.typ), true
		}
		return NullVal(nullType(m)), true
	}
	members := make([]Value, rv.Len())
	ok := true
	for i := range members {
		v, good := e.encode(m.elem, rv.Index(i), p.Index(NumberFromInt(int64(i))))
		members[i], ok = v, ok && good
	}
	switch {
	case !ok:
		return Value{}, false
	case m.typ.t != nil:
		return ListVal(m.elem.typ, members...), true
	}
	return TupleVal(members...), true
}

// mapping encodes a map with string keys.
func (e *goEncoder) mapping(m *goMapping, rv reflect.Value, p Path) (Value, bool) {
	if rv.IsNil() {
		if m.typ.t != nil {
			return NullVal(m.typ), true
		}
		return NullVal(nullType(m)), true
	}
	keys := make([]string, 0, rv.Len())
	for _, k := range rv.MapKeys() {
		keys = append(keys, k.String())
	}
	slices.Sort(keys)
	entries := make(map[string]Value, len(keys))
	normalized := map[string]string{}
	ok := true
	for _, key := range keys {
		canonical, err := uni.Canonical(key)
		if err != nil {
			e.fail(p, CodeStringInvalidUTF8, "the map key "+quotedASCII(key)+" is not well-formed UTF-8")
			ok = false
			continue
		}
		if other, dup := normalized[canonical]; dup {
			e.fail(p, CodeMapDuplicateKey, "the map keys "+quotedASCII(other)+" and "+quotedASCII(key)+" are the same key after normalization")
			ok = false
			continue
		}
		normalized[canonical] = key
		at := p.Index(String(canonical))
		if m.typ.t == nil && canonical == "" {
			e.fail(at, CodeConvertUnexpectedAttribute, `the map key "" cannot be an attribute name`)
			ok = false
			continue
		}
		v, good := e.encode(m.elem, rv.MapIndex(reflect.ValueOf(key).Convert(m.rt.Key())), at)
		entries[canonical], ok = v, ok && good
	}
	switch {
	case !ok:
		return Value{}, false
	case m.typ.t != nil:
		return MapVal(m.elem.typ, entries), true
	}
	return ObjectVal(entries), true
}

// structure encodes a struct.
func (e *goEncoder) structure(m *goMapping, rv reflect.Value, p Path) (Value, bool) {
	attrs := make(map[string]Value, len(m.fields))
	ok := true
	for _, f := range m.fields {
		fv := rv.Field(f.index)
		if f.m.kind == goValue && fv.Interface().(Value).n == nil {
			if f.optional {
				continue
			}
			usagePanic("Encode: required field %s of %s is a tenon.Value holding the zero Value", m.rt.Field(f.index).Name, m.rt)
		}
		v, good := e.encode(f.m, fv, p.Attribute(f.name))
		attrs[f.name], ok = v, ok && good
	}
	if !ok {
		return Value{}, false
	}
	return ObjectVal(attrs), true
}
