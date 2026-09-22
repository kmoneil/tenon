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

// manyFailures is the most diagnostics a failures compares one against. A Go
// value fails in a handful of places, among which a scan is quickest and
// allocates nothing; but every member of a slice or map can fail, and
// comparing each failure with every failure collected costs the square of
// them: a JSON array of 20,000 nulls took 3 seconds to encode.
const manyFailures = 16

// failures collects diagnostics, each once: by comparing them while there are
// few, and by a set of their keys once there are many.
type failures struct {
	list []tenon.Diagnostic
	seen map[string]struct{}
	n    int    // how many of the list's diagnostics the set holds
	buf  []byte // the key of the diagnostic being looked up, kept to be reused
}

func (f *failures) add(d tenon.Diagnostic) {
	if !f.holds(d) {
		f.list = append(f.list, d)
	}
}

// holds reports whether d is among the diagnostics collected.
func (f *failures) holds(d tenon.Diagnostic) bool {
	if f.seen == nil {
		if len(f.list) <= manyFailures {
			return slices.ContainsFunc(f.list, d.Equal)
		}
		f.seen = make(map[string]struct{}, 2*len(f.list))
	}
	for _, h := range f.list[f.n:] {
		f.seen[string(appendFailureKey(nil, h))] = struct{}{}
	}
	f.n = len(f.list)
	// The key of what is looked up is built in a buffer of its own, which a
	// lookup of a map by a string of bytes does not copy.
	f.buf = appendFailureKey(f.buf[:0], d)
	_, ok := f.seen[string(f.buf)]
	return ok
}

// appendFailureKey appends a key that two diagnostics share exactly when
// tenon.Diagnostic.Equal reports them the same: the code, the message, and
// each step of the path, written after its length so that no two of them run
// together and tagged by its kind so that an attribute and a key of the same
// text stay apart. A key is written as the value it is, without the marks it
// carries, which Equal leaves out as well; a number is written in its one
// canonical text, whatever text it was read from.
func appendFailureKey(b []byte, d tenon.Diagnostic) []byte {
	b = appendKeyPart(b, string(d.Code))
	b = appendKeyPart(b, d.Message)
	for _, s := range d.Path.Steps() {
		if s.Kind() == tenon.StepAttribute {
			b = appendKeyPart(append(b, 'a'), s.Name())
			continue
		}
		key, _ := tenon.UnmarkDeep(s.Key())
		if key.Type() == tenon.StringType() {
			b = appendKeyPart(append(b, 's'), key.AsString())
			continue
		}
		b = appendKeyPart(append(b, 'n'), key.String())
	}
	return b
}

// appendKeyPart appends one part of a key, after its length in bytes.
func appendKeyPart(b []byte, part string) []byte {
	b = strconv.AppendInt(b, int64(len(part)), 10)
	return append(append(b, ':'), part...)
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
// elements need not share a type, as tenon.Value, interface and marshaler
// types need not, encodes as a tuple or an object.
//
// A value of an interface type encodes as what it holds, by that value's own
// type, so the map[string]any that encoding/json gives encodes as an object of
// whatever the document held. A nil interface holds no value, and no type
// follows from it, so it fails with tenon.CodeEncodeUntypedNil: which null a
// JSON null is is the caller's to say. A json.Number is the number its text
// spells, so a document read with UseNumber keeps its own distinction between
// 8080 and "8080".
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
// range of numbers (tenon.CodeNumberOutOfRange), a json.Number whose text
// spells no number (tenon.CodeNumberInvalidSyntax), a nil interface
// (tenon.CodeEncodeUntypedNil), a string that is not valid
// UTF-8 (tenon.CodeStringInvalidUTF8), a map of values whose types need not
// agree whose keys are empty or collide once normalized, and a MarshalValue
// method's failure (tenon.CodeEncodeMarshalFailed, or its own diagnostics).
//
// Encode panics where T does not map to tenon: a channel, a
// function, a complex number, a pointer to tenon.Value, a map without string
// keys, or a type that holds itself, whether T is that type or holds it in an
// interface; where a struct's tags are malformed; on a
// required tenon.Value field, or any other tenon.Value, holding the zero
// Value; and on a MarshalValue method returning the zero Value.
func Encode[T any](x T) (tenon.Value, error) {
	m := mappingOf(reflect.TypeFor[T]())
	var e encoder
	v, ok := e.encode(m, reflect.ValueOf(&x).Elem(), tenon.Path{})
	if len(e.fails.list) > 0 || !ok {
		return tenon.Value{}, &DiagnosticError{Value: tenon.ErrorVal(e.fails.list...)}
	}
	return v, nil
}

// encoder encodes one Go value, collecting what it cannot encode.
type encoder struct {
	fails failures
	// within holds the Go containers the encoder is inside, so that a value
	// that holds itself is told apart from one that holds another copy of
	// the same thing.
	within map[cycle]bool
}

// cycle identifies a Go container by what it holds rather than by its own
// address alone: two slices over one array share an address, and differ in
// the members they hold.
type cycle struct {
	ptr uintptr
	len int
	typ reflect.Type
}

// cycleOf returns the identity of rv, and whether rv is a container that can
// hold itself at all: a map, a slice or a pointer.
func cycleOf(rv reflect.Value) (cycle, bool) {
	switch rv.Kind() {
	case reflect.Map, reflect.Slice:
		return cycle{rv.Pointer(), rv.Len(), rv.Type()}, true
	case reflect.Pointer:
		return cycle{rv.Pointer(), 0, rv.Type()}, true
	}
	return cycle{}, false
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
	case goInterface:
		// What an interface holds encodes by its own type [GO-015]. A nil
		// one holds nothing, and no type follows from nothing.
		if rv.IsNil() {
			e.fail(p, tenon.CodeEncodeUntypedNil, "a nil "+m.rt.String()+" holds no value, and no type follows from it")
			return tenon.Value{}, false
		}
		held := rv.Elem()
		if c, container := cycleOf(held); container {
			// An interface is the only way a Go value holds itself, a type
			// that holds itself having no mapping at all [GO-011].
			if e.within[c] {
				usagePanic("Encode: the Go value at %q holds itself, and a value that maps to tenon must be finite", p.String())
			}
			if e.within == nil {
				e.within = map[cycle]bool{}
			}
			e.within[c] = true
			defer delete(e.within, c)
		}
		return e.encode(mappingOf(held.Type()), held, p)
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
		// A finite float64 is always inside the digit window.
		c, exp, _ := exactBigFloat(new(big.Float).SetFloat64(f))
		return decimalNumber(c, exp), true
	case goBigInt, goBigFloat, goBigRat:
		return e.bigNumber(m, rv, p)
	case goJSONNumber:
		// The text of a json.Number is the text a number parses from, and
		// text that parses to no number is data that is wrong [GO-034].
		return e.fromData(tenon.NumberFromText(rv.String()), p)
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
		c   *big.Int
		exp int64
		ok  bool
	)
	switch x := ptr.Interface().(type) {
	case *big.Int:
		c, ok = x, true
	case *big.Float:
		if x.IsInf() {
			e.fail(p, tenon.CodeEncodeNotANumber, x.String()+" is not a number")
			return tenon.Value{}, false
		}
		c, exp, ok = exactBigFloat(x)
	case *big.Rat:
		var exact bool
		c, exp, exact, ok = exactRat(x)
		if !exact {
			e.fail(p, tenon.CodeEncodeInexact, "the rational "+x.String()+" is not a terminating decimal")
			return tenon.Value{}, false
		}
	}
	if !ok {
		e.fail(p, tenon.CodeNumberOutOfRange, "the number is outside the range of numbers")
		return tenon.Value{}, false
	}
	return e.fromData(decimalNumber(c, exp), p)
}

// decimalNumber returns the Number c × 10^exp, built from the coefficient and
// a power of ten rather than read from text: a number can have more digits
// than NumberFromText reads, and it is exact either way. The coefficients
// exactBigFloat and exactRat give end in a digit other than zero, so the
// number is out of range exactly where 10^exp is, and saying so is right.
func decimalNumber(c *big.Int, exp int64) tenon.Value {
	n := tenon.NumberFromBigInt(c)
	if exp == 0 || n.IsError() {
		return n
	}
	return tenon.Mul(n, tenon.NumberFromText("1e"+strconv.FormatInt(exp, 10)))
}

// maxBinaryPlaces bounds how many binary places a number in the digit window
// can need: 2^3321929 exceeds 10^1000000.
const maxBinaryPlaces = 3321929

// maxDecimalPlaces is how many decimal places a number in tenon's digit
// window can have: a digit below the 10^-999999 place lies outside it, and
// TestConformance_GO030_NumbersAtTheEdgeOfTheWindow holds this to what tenon
// accepts. A binary fraction c × 2^k with c odd has exactly -k of them, since
// c × 5^-k is odd and the last digit sits at 10^k; a rational whose
// denominator is 2^a × 5^b has max(a, b).
const maxDecimalPlaces = 999999

// exactBigFloat returns the decimal text of the value f, which is finite,
// holds exactly: a binary fraction always terminates in decimal. It reports
// false for a number too far outside the digit window to be worth computing.
func exactBigFloat(f *big.Float) (*big.Int, int64, bool) {
	if f.Sign() == 0 {
		return new(big.Int), 0, true
	}
	mant := new(big.Float)
	exp := int64(f.MantExp(mant))
	prec := int64(f.MinPrec())
	// f is c × 2^k, with c an odd integer of prec bits.
	c, _ := new(big.Float).SetMantExp(mant, int(prec)).Int(nil)
	k := exp - prec
	// The last digit of c × 2^k sits at 10^k, so the places below the point
	// say whether the number is in the window before 5^-k is built.
	if exp > maxBinaryPlaces || -k > maxDecimalPlaces {
		return nil, 0, false
	}
	if k >= 0 {
		return c.Lsh(c, uint(k)), 0, true
	}
	// c × 2^k is c × 5^-k × 10^k.
	c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(-k), nil))
	return c, k, true
}

// exactRat returns the decimal text of r, whether r is a terminating decimal,
// one whose denominator has no prime factor but two and five, and false for a
// denominator too large to be worth dividing.
func exactRat(r *big.Rat) (c *big.Int, exp int64, exact, ok bool) {
	// A denominator of more bits than the window has decimal places, however
	// it factors, puts the last digit below the window. It is refused before
	// it is so much as copied.
	if int64(r.Denom().BitLen()) > maxBinaryPlaces+1 {
		return nil, 0, true, false
	}
	den := new(big.Int).Set(r.Denom())
	twos := int64(den.TrailingZeroBits())
	den.Rsh(den, uint(twos))
	fives, rest := valuation(den, 5, maxDecimalPlaces)
	// A terminating decimal with 2^a × 5^b beneath it has max(a, b) places,
	// so past the window the last digit is below it whatever is left over,
	// and the counting stops rather than dividing the rest of the way out.
	if twos > maxDecimalPlaces || fives > maxDecimalPlaces {
		return nil, 0, true, false
	}
	if rest.Cmp(big.NewInt(1)) != 0 {
		return nil, 0, false, true
	}
	n := max(twos, fives)
	c = new(big.Int).Set(r.Num())
	c.Lsh(c, uint(n-twos))
	c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(n-fives), nil))
	return c, -n, true, true
}

// valuation returns how many times p divides d, and what is left of d once
// they are taken out, stopping once the count passes limit, where what is
// left is what it had reached. It divides by p, then by p squared, then by
// that squared, while each divides, and then takes the smaller powers out on
// the way down, so a denominator of p^n costs the logarithm of n divisions
// rather than n of them.
func valuation(d *big.Int, p, limit int64) (int64, *big.Int) {
	rest := new(big.Int).Set(d)
	powers := []*big.Int{big.NewInt(p)}
	steps := []int64{1}
	q, rem := new(big.Int), new(big.Int)
	var n int64
	for i := 0; n <= limit; i++ {
		if q.QuoRem(rest, powers[i], rem); rem.Sign() != 0 {
			break
		}
		rest.Set(q)
		n += steps[i]
		if i+1 == len(powers) {
			powers = append(powers, new(big.Int).Mul(powers[i], powers[i]))
			steps = append(steps, 2*steps[i])
		}
	}
	// Each power above the one that stopped it is too large to divide what is
	// left, and each below it divides at most once, the powers being squares.
	for i := len(powers) - 2; i >= 0 && n <= limit; i-- {
		if q.QuoRem(rest, powers[i], rem); rem.Sign() == 0 {
			rest.Set(q)
			n += steps[i]
		}
	}
	return n, rest
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
		if !m.typed() {
			if canonical == "" {
				e.fail(at, tenon.CodeConvertUnexpectedAttribute, `the map key "" cannot be an attribute name`)
				ok = false
				continue
			}
			// A map whose members' types need not agree encodes as an object
			// [GO-012], so what is in it is located as an attribute of one,
			// which is where the reader of a diagnostic looks for it.
			at = p.Attribute(canonical)
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
