package tenon

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"

	"github.com/kmoneil/tenon/internal/cbor"
	"github.com/kmoneil/tenon/internal/decimal"
	"github.com/kmoneil/tenon/internal/uni"
)

// Decoders supplies what Deserialize needs to read the capsule values and the
// marks a document holds, which only their authors can make.
type Decoders struct {
	// Capsules are the capsule types a document may name, each found by the
	// identifier its encoding declares.
	Capsules []Type
	// Marks decode marks by their identifiers.
	Marks map[string]MarkDecoder
}

// MarkDecoder returns the mark that was serialized with payload, where
// hasPayload says it was serialized with one; where it was not, payload is
// the zero Value and must not be used. A payload is a known, unmarked value
// other than a null, of whatever type the input gives, which need not be one
// the mark ever serializes with. It returns the diagnostics that say why there
// is no mark where it refuses what it is given.
type MarkDecoder func(payload Value, hasPayload bool) (Mark, []Diagnostic)

// maxDepth bounds how deeply a document may nest.
const maxDepth = 512

// Deserialize returns the value that data, a document written by Serialize,
// encodes, and true. Otherwise it returns an error value and false:
// CodeSerializeMalformed where data is not a document or describes no value,
// CodeSerializeNotCanonical where it describes a value but is not that value's
// encoding, CodeSerializeUnsupportedVersion for a document of another format
// version, CodeSerializeTooLarge where it nests more deeply than 512 levels,
// CodeSerializeUnknownCapsule and CodeSerializeUnknownMark for an identifier
// that decoders supplies nothing for, and the diagnostics a decoder reports
// where it refuses what it is given.
//
// Deserialize never panics on its input, and never allocates for a length the
// input declares before the input has shown it holds that much. The work it
// does grows no faster than n log n in the length of data, however data is
// shaped, so a caller that takes documents from outside bounds the cost of
// decoding them by bounding their length, as for any parser. The work of the
// capsule and mark decoders a caller supplies is theirs. Nesting deeper than
// 512 levels, each item, type, constraint or content counting one, is the only
// thing refused for its size.
//
// Deserialize panics if decoders names a capsule type that declares no
// encoding, or two that declare one identifier, and on a mark decoder that
// breaks its contract: one returning neither a mark nor a diagnostic, a mark of
// another identifier, or diagnostics that ErrorVal refuses.
func Deserialize(data []byte, decoders Decoders) (Value, Value, bool) {
	d := &decoder{r: cbor.NewReader(data), capsules: map[string]Type{}, marks: decoders.Marks}
	for _, t := range decoders.Capsules {
		enc := t.mustKind(KindCapsule, "Deserialize").capsule.encoding
		if enc == nil {
			usagePanic("Deserialize: capsule type %q declares no encoding", t.t.capsule.name)
		}
		if other, ok := d.capsules[enc.id]; ok && other != t {
			usagePanic("Deserialize: two capsule types declare the identifier %q", enc.id)
		}
		d.capsules[enc.id] = t
	}
	v, err := d.document()
	switch {
	case err != nil && err.diags != nil:
		return Value{}, errorValue(err.diags...), false
	case err != nil:
		return Value{}, errorValue(err.diagnostic()), false
	}
	again, failure, ok := Serialize(v)
	switch {
	case !ok:
		return Value{}, errorValue(Diagnostic{Code: CodeSerializeNotCanonical,
			Message: "the decoded value does not serialize again: " + failure.String()}), false
	case !bytes.Equal(again, data):
		return Value{}, errorValue(Diagnostic{Code: CodeSerializeNotCanonical,
			Message: fmt.Sprintf("the input is not the encoding of the value it describes, which differs from byte %d", firstDifference(again, data))}), false
	}
	return v, Value{}, true
}

// firstDifference returns the offset of the first byte at which a and b
// differ, or the length of the shorter.
func firstDifference(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// decodeError is why a document does not decode, at a byte offset.
type decodeError struct {
	code    Code
	offset  int
	message string
	diags   []Diagnostic // what a supplied decoder reported, in place of the rest
}

func (e *decodeError) diagnostic() Diagnostic {
	return Diagnostic{Code: e.code, Message: fmt.Sprintf("at byte %d: %s", e.offset, e.message)}
}

// decoder reads one document.
type decoder struct {
	r        *cbor.Reader
	capsules map[string]Type
	marks    map[string]MarkDecoder
	depth    int
	// deferred says a deep mark was read, which the decoder gives only the
	// value it is listed on until the value read is settled (settleDeep).
	deferred bool
}

// malformed returns the error of input that is not a document of a value, at
// offset at.
func (d *decoder) malformed(at int, format string, args ...any) *decodeError {
	return &decodeError{code: CodeSerializeMalformed, offset: at, message: fmt.Sprintf(format, args...)}
}

// cborError converts the codec's refusal.
func (d *decoder) cborError(err error) *decodeError {
	var ce *cbor.Error
	if errors.As(err, &ce) {
		code := CodeSerializeMalformed
		if ce.Canonical {
			code = CodeSerializeNotCanonical
		}
		return &decodeError{code: code, offset: ce.Offset, message: ce.Message}
	}
	return &decodeError{code: CodeSerializeMalformed, offset: d.r.Offset(), message: err.Error()}
}

// enter records one more level of nesting, refusing to go beyond maxDepth.
func (d *decoder) enter() *decodeError {
	d.depth++
	if d.depth > maxDepth {
		return &decodeError{code: CodeSerializeTooLarge, offset: d.r.Offset(),
			message: fmt.Sprintf("the document nests more than %d levels deep", maxDepth)}
	}
	return nil
}

func (d *decoder) leave() { d.depth-- }

// array reads the head of an array of want items.
func (d *decoder) array(want int, what string) *decodeError {
	at := d.r.Offset()
	n, err := d.r.ReadArray()
	if err != nil {
		return d.cborError(err)
	}
	if n != want {
		return d.malformed(at, "%s is an array of %d items, not %d", what, n, want)
	}
	return nil
}

// arrayOf reads the head of the array holding the content of a value of type
// t, which has want items. The type is rendered only where the head is not
// that array: a document states a type once and holds many values of it, and
// rendering it for each of them would cost the type's text per value.
func (d *decoder) arrayOf(want int, t Type) *decodeError {
	at := d.r.Offset()
	n, err := d.r.ReadArray()
	if err != nil {
		return d.cborError(err)
	}
	if n != want {
		return d.malformed(at, "the content of %s is an array of %d items, not %d", t, n, want)
	}
	return nil
}

// kind reads an unsigned integer naming a kind.
func (d *decoder) kind(what string) (uint64, int, *decodeError) {
	at := d.r.Offset()
	k, err := d.r.ReadUint()
	if err != nil {
		return 0, at, d.cborError(err)
	}
	return k, at, nil
}

// document reads a whole document.
func (d *decoder) document() (Value, *decodeError) {
	at := d.r.Offset()
	tag, err := d.r.ReadTag()
	if err != nil || tag != tagDocument {
		return Value{}, d.malformed(at, "the input does not begin with the document tag")
	}
	if err := d.array(2, "a document"); err != nil {
		return Value{}, err
	}
	at = d.r.Offset()
	version, err := d.r.ReadUint()
	if err != nil {
		return Value{}, d.cborError(err)
	}
	if version != formatVersion {
		return Value{}, &decodeError{code: CodeSerializeUnsupportedVersion, offset: at,
			message: fmt.Sprintf("the document is of format version %d, and this decoder reads version %d", version, formatVersion)}
	}
	v, derr := d.item()
	if derr != nil {
		return Value{}, derr
	}
	if d.r.Remaining() != 0 {
		return Value{}, d.malformed(d.r.Offset(), "%d bytes follow the document", d.r.Remaining())
	}
	return v, nil
}

// item reads an item.
func (d *decoder) item() (Value, *decodeError) {
	if err := d.enter(); err != nil {
		return Value{}, err
	}
	defer d.leave()
	at := d.r.Offset()
	if h, err := d.r.PeekHead(); err == nil && h.Major == cbor.MajorTag {
		if tag, _ := d.r.ReadTag(); tag != tagMarked {
			return Value{}, d.malformed(at, "tag %d where an item was expected", tag)
		}
		if err := d.array(2, "a marked item"); err != nil {
			return Value{}, err
		}
		inner := d.r.Offset()
		v, err := d.item()
		if err != nil {
			return Value{}, err
		}
		if v.n.state.resolved() || v.n.marks != nil {
			return Value{}, d.malformed(inner, "a marked item holds a resolved or marked value, whose marks go on its content")
		}
		marks, err := d.markList()
		if err != nil {
			return Value{}, err
		}
		return WithMarks(v, marks...), nil
	}
	n, err := d.r.ReadArray()
	if err != nil {
		return Value{}, d.cborError(err)
	}
	k, kat, derr := d.kind("an item")
	if derr != nil {
		return Value{}, derr
	}
	switch {
	case k == itemResolved && n == 3:
		t, err := d.typ()
		if err != nil {
			return Value{}, err
		}
		v, err := d.content(t)
		if err != nil || !d.deferred {
			return v, err
		}
		// Each part was given the marks listed on it and no more; one pass
		// gives every value the deep marks of the values above it.
		return Value{settleDeep(v.n, nil)}, nil
	case k == itemPending && n == 3:
		c, err := d.constraint()
		if err != nil {
			return Value{}, err
		}
		nat := d.r.Offset()
		null, rerr := d.r.ReadUint()
		switch {
		case rerr != nil:
			return Value{}, d.cborError(rerr)
		case null == 0:
			return Pending(c), nil
		case null == 1:
			return Narrow(Pending(c), NotNull()), nil
		case null == 2:
			return Narrow(Pending(c), Null()), nil
		}
		return Value{}, d.malformed(nat, "nullness %d is not 0, 1 or 2", null)
	case k == itemError && n == 2:
		return d.errorValue()
	}
	return Value{}, d.malformed(kat, "an item of kind %d with %d parts", k, n)
}

// errorValue reads the diagnostics of an error value.
func (d *decoder) errorValue() (Value, *decodeError) {
	at := d.r.Offset()
	n, err := d.r.ReadArray()
	if err != nil {
		return Value{}, d.cborError(err)
	}
	if n == 0 {
		return Value{}, d.malformed(at, "an error value with no diagnostics")
	}
	var diags []Diagnostic
	for range n {
		diag, err := d.diagnostic()
		if err != nil {
			return Value{}, err
		}
		diags = append(diags, diag)
	}
	return errorValue(diags...), nil
}

// diagnostic reads a diagnostic.
func (d *decoder) diagnostic() (Diagnostic, *decodeError) {
	if err := d.array(3, "a diagnostic"); err != nil {
		return Diagnostic{}, err
	}
	at := d.r.Offset()
	code, err := d.r.ReadText()
	if err != nil {
		return Diagnostic{}, d.cborError(err)
	}
	if !validCode(Code(code)) {
		return Diagnostic{}, d.malformed(at, "%s is not a diagnostic code", quotedASCII(code))
	}
	at = d.r.Offset()
	message, err := d.r.ReadText()
	if err != nil {
		return Diagnostic{}, d.cborError(err)
	}
	if message == "" {
		return Diagnostic{}, d.malformed(at, "a diagnostic with no message")
	}
	n, err := d.r.ReadArray()
	if err != nil {
		return Diagnostic{}, d.cborError(err)
	}
	var p Path
	for range n {
		at := d.r.Offset()
		h, err := d.r.PeekHead()
		if err != nil {
			return Diagnostic{}, d.cborError(err)
		}
		switch h.Major {
		case cbor.MajorText:
			name, _ := d.r.ReadText()
			if name == "" {
				return Diagnostic{}, d.malformed(at, "a path step naming the empty attribute")
			}
			p = p.Attribute(name)
		case cbor.MajorArray:
			if err := d.array(1, "a String key"); err != nil {
				return Diagnostic{}, err
			}
			key, err := d.r.ReadText()
			if err != nil {
				return Diagnostic{}, d.cborError(err)
			}
			p = p.Index(String(key))
		default:
			num, err := d.number()
			if err != nil {
				return Diagnostic{}, err
			}
			p = p.Index(num)
		}
	}
	return Diagnostic{Code: Code(code), Message: message, Path: p}, nil
}

// typ reads a type.
func (d *decoder) typ() (Type, *decodeError) {
	if err := d.enter(); err != nil {
		return Type{}, err
	}
	defer d.leave()
	at := d.r.Offset()
	h, err := d.r.PeekHead()
	if err != nil {
		return Type{}, d.cborError(err)
	}
	if h.Major == cbor.MajorUint {
		k, _ := d.r.ReadUint()
		switch Kind(k) {
		case KindBool:
			return Type{boolType}, nil
		case KindNumber:
			return Type{numberType}, nil
		case KindString:
			return Type{stringType}, nil
		}
		return Type{}, d.malformed(at, "type %d", k)
	}
	if err := d.array(2, "a type"); err != nil {
		return Type{}, err
	}
	k, kat, derr := d.kind("a type")
	if derr != nil {
		return Type{}, derr
	}
	switch Kind(k) {
	case KindList, KindSet, KindMap:
		elem, err := d.typ()
		if err != nil {
			return Type{}, err
		}
		return collection(Kind(k), elem), nil
	case KindTuple:
		n, err := d.r.ReadArray()
		if err != nil {
			return Type{}, d.cborError(err)
		}
		var elems []Type
		for range n {
			t, err := d.typ()
			if err != nil {
				return Type{}, err
			}
			elems = append(elems, t)
		}
		return Tuple(elems...), nil
	case KindObject:
		n, err := d.r.ReadArray()
		if err != nil {
			return Type{}, d.cborError(err)
		}
		attrs := map[string]Type{}
		for range n {
			if err := d.array(2, "an attribute"); err != nil {
				return Type{}, err
			}
			name, err := readName(d, attrs)
			if err != nil {
				return Type{}, err
			}
			t, err := d.typ()
			if err != nil {
				return Type{}, err
			}
			attrs[name] = t
		}
		return Object(attrs), nil
	case KindCapsule:
		at := d.r.Offset()
		id, err := d.r.ReadText()
		if err != nil {
			return Type{}, d.cborError(err)
		}
		t, ok := d.capsules[id]
		if !ok {
			return Type{}, &decodeError{code: CodeSerializeUnknownCapsule, offset: at,
				message: "no capsule type was supplied for the identifier " + quoted(id)}
		}
		return t, nil
	}
	return Type{}, d.malformed(kat, "type kind %d", k)
}

// readName reads an attribute or field name that is not yet in seen,
// normalized.
func readName[V any](d *decoder, seen map[string]V) (string, *decodeError) {
	at := d.r.Offset()
	s, err := d.r.ReadText()
	if err != nil {
		return "", d.cborError(err)
	}
	if s == "" {
		return "", d.malformed(at, "an empty name")
	}
	s = uni.NFC(s)
	if _, dup := seen[s]; dup {
		return "", d.malformed(at, "the name %s appears twice", quoted(s))
	}
	return s, nil
}

// constraint reads a constraint.
func (d *decoder) constraint() (Constraint, *decodeError) {
	if err := d.enter(); err != nil {
		return Constraint{}, err
	}
	defer d.leave()
	n, err := d.r.ReadArray()
	if err != nil {
		return Constraint{}, d.cborError(err)
	}
	k, kat, derr := d.kind("a constraint")
	if derr != nil {
		return Constraint{}, derr
	}
	switch kind := ConstraintKind(k); {
	case kind == ConstraintExactly && n == 2:
		t, err := d.typ()
		if err != nil {
			return Constraint{}, err
		}
		return Exactly(t), nil
	case kind == ConstraintAny && n == 1:
		return Any(), nil
	case (kind == ConstraintListOf || kind == ConstraintSetOf || kind == ConstraintMapOf) && n == 2:
		elem, err := d.constraint()
		if err != nil {
			return Constraint{}, err
		}
		return elementConstraint(kind, elem), nil
	case kind == ConstraintObjectWith && n == 3:
		m, err := d.r.ReadArray()
		if err != nil {
			return Constraint{}, d.cborError(err)
		}
		fields := map[string]Field{}
		for range m {
			if err := d.array(3, "a field"); err != nil {
				return Constraint{}, err
			}
			fname, err := readName(d, fields)
			if err != nil {
				return Constraint{}, err
			}
			required, rerr := d.r.ReadBool()
			if rerr != nil {
				return Constraint{}, d.cborError(rerr)
			}
			c, err := d.constraint()
			if err != nil {
				return Constraint{}, err
			}
			fields[fname] = Field{Constraint: c, Required: required}
		}
		closed, rerr := d.r.ReadBool()
		if rerr != nil {
			return Constraint{}, d.cborError(rerr)
		}
		return ObjectWith(fields, closed), nil
	case (kind == ConstraintTupleOf || kind == ConstraintOneOf) && n == 2:
		m, err := d.r.ReadArray()
		if err != nil {
			return Constraint{}, d.cborError(err)
		}
		var members []Constraint
		for range m {
			c, err := d.constraint()
			if err != nil {
				return Constraint{}, err
			}
			members = append(members, c)
		}
		return membersConstraint(kind, members), nil
	}
	return Constraint{}, d.malformed(kat, "a constraint of kind %d with %d parts", k, n)
}

// content reads the content of a resolved value of type t.
func (d *decoder) content(t Type) (Value, *decodeError) {
	if err := d.enter(); err != nil {
		return Value{}, err
	}
	defer d.leave()
	at := d.r.Offset()
	if d.r.ReadNull() {
		return NullVal(t), nil
	}
	h, err := d.r.PeekHead()
	if err != nil {
		return Value{}, d.cborError(err)
	}
	if h.Major == cbor.MajorTag && (h.Arg == tagUnknown || h.Arg == tagMarked) {
		d.r.ReadTag()
		if h.Arg == tagUnknown {
			return d.unknown(t, at)
		}
		if err := d.array(2, "a marked value"); err != nil {
			return Value{}, err
		}
		v, derr := d.content(t)
		if derr != nil {
			return Value{}, derr
		}
		marks, derr := d.markList()
		if derr != nil {
			return Value{}, derr
		}
		// A deep mark reaches the values within v when the value read is
		// settled, which merges each value's marks once, rather than here,
		// which would merge them again at every level a nest has.
		if !d.deferred && slices.ContainsFunc(marks, isDeep) {
			d.deferred = true
		}
		return withOwnMarks(v, marks), nil
	}
	switch t.t.kind {
	case KindBool:
		b, err := d.r.ReadBool()
		if err != nil {
			return Value{}, d.cborError(err)
		}
		return Bool(b), nil
	case KindNumber:
		return d.number()
	case KindString:
		s, err := d.r.ReadText()
		if err != nil {
			return Value{}, d.cborError(err)
		}
		return String(s), nil
	case KindList, KindSet, KindTuple:
		return d.sequence(t, at)
	case KindMap:
		n, err := d.r.ReadArray()
		if err != nil {
			return Value{}, d.cborError(err)
		}
		entries := map[string]Value{}
		for range n {
			if err := d.array(2, "a map entry"); err != nil {
				return Value{}, err
			}
			kat := d.r.Offset()
			key, err := d.r.ReadText()
			if err != nil {
				return Value{}, d.cborError(err)
			}
			if _, dup := entries[key]; dup {
				return Value{}, d.malformed(kat, "the key %s appears twice", quoted(key))
			}
			v, derr := d.content(t.t.elem)
			if derr != nil {
				return Value{}, derr
			}
			entries[key] = v
		}
		m := MapVal(t.t.elem, entries)
		if m.n.state == stateError {
			return Value{}, d.malformed(at, "a map whose keys are the same after normalization")
		}
		return m, nil
	case KindObject:
		attrs := t.t.attrs
		if err := d.arrayOf(len(attrs), t); err != nil {
			return Value{}, err
		}
		vals := make([]Value, len(attrs))
		for i, a := range attrs {
			v, err := d.content(a.typ)
			if err != nil {
				return Value{}, err
			}
			vals[i] = v
		}
		// The type says what the attributes are called and what order they
		// come in, so the object is built from it rather than from a map
		// whose names would be normalized and whose type would be interned
		// again, once for every object a document holds.
		return objectOf(t, vals), nil
	}
	return d.capsule(t, at)
}

// sequence reads the members of a list, set or tuple.
func (d *decoder) sequence(t Type, at int) (Value, *decodeError) {
	n, err := d.r.ReadArray()
	if err != nil {
		return Value{}, d.cborError(err)
	}
	if t.t.kind == KindTuple && n != len(t.t.elems) {
		return Value{}, d.malformed(at, "a tuple of %d elements holding %d", len(t.t.elems), n)
	}
	var members []Value
	for i := range n {
		mt := t.t.elem
		if t.t.kind == KindTuple {
			mt = t.t.elems[i]
		}
		mat := d.r.Offset()
		v, derr := d.content(mt)
		if derr != nil {
			return Value{}, derr
		}
		if t.t.kind == KindSet && v.n.isMarked() {
			return Value{}, d.malformed(mat, "a set member that carries marks")
		}
		members = append(members, v)
	}
	switch t.t.kind {
	case KindList:
		return ListVal(t.t.elem, members...), nil
	case KindSet:
		return SetVal(t.t.elem, members...), nil
	}
	return TupleVal(members...), nil
}

// capsule reads the content of a known capsule value of type t.
func (d *decoder) capsule(t Type, at int) (Value, *decodeError) {
	enc := t.t.capsule.encoding
	if err := d.array(2, "a capsule value"); err != nil {
		return Value{}, err
	}
	pt, err := d.typ()
	if err != nil {
		return Value{}, err
	}
	if pt != enc.typ {
		return Value{}, d.malformed(at, "capsule type %s is serialized as %s, not %s", quoted(t.t.capsule.name), enc.typ, pt)
	}
	payload, err := d.content(pt)
	if err != nil {
		return Value{}, err
	}
	if !payload.n.isKnown() || payload.n.state == stateNull || payload.n.isMarked() {
		return Value{}, d.malformed(at, "a capsule value serialized as a value that is null, not known, or marked")
	}
	p, diags := enc.decode(payload)
	if len(diags) > 0 {
		ErrorVal(diags...) // a decoder's diagnostics must be ones ErrorVal accepts
		return Value{}, &decodeError{offset: at, diags: diags}
	}
	if p == nil {
		usagePanic("the Decode of capsule type %q returned neither a value nor a diagnostic", t.t.capsule.name)
	}
	if !t.t.capsule.accepts(p) {
		usagePanic("the Decode of capsule type %q returned a pointer the type does not encapsulate", t.t.capsule.name)
	}
	return Value{&node{state: stateKnown, typ: t, data: p}}, nil
}

// unknown reads the range of an unknown value of type t.
func (d *decoder) unknown(t Type, at int) (Value, *decodeError) {
	n, err := d.r.ReadMap()
	if err != nil {
		return Value{}, d.cborError(err)
	}
	var ns []Narrowing
	for range n {
		kat := d.r.Offset()
		key, err := d.r.ReadUint()
		if err != nil {
			return Value{}, d.cborError(err)
		}
		nw, derr := d.narrowing(t, key, kat)
		if derr != nil {
			return Value{}, derr
		}
		ns = append(ns, nw)
	}
	for _, nw := range ns {
		if !nw.appliesTo(t) {
			return Value{}, d.malformed(at, "a range of %s recording %s", t, nw)
		}
	}
	v := Narrow(Unknown(t), ns...)
	if v.n.state == stateError {
		return Value{}, d.malformed(at, "a range that no value lies in")
	}
	return v, nil
}

// narrowing reads the narrowing a range records under key.
func (d *decoder) narrowing(t Type, key uint64, at int) (Narrowing, *decodeError) {
	switch key {
	case 0:
		b, err := d.r.ReadBool()
		if err != nil || !b {
			return Narrowing{}, d.malformed(at, "a range key 0 that is not true")
		}
		return NotNull(), nil
	case 1, 2:
		if err := d.array(2, "a bound"); err != nil {
			return Narrowing{}, err
		}
		num, derr := d.number()
		if derr != nil {
			return Narrowing{}, derr
		}
		incl, err := d.r.ReadBool()
		if err != nil {
			return Narrowing{}, d.cborError(err)
		}
		if key == 1 {
			return NumberMin(num, incl), nil
		}
		return NumberMax(num, incl), nil
	case 3:
		s, err := d.r.ReadText()
		if err != nil {
			return Narrowing{}, d.cborError(err)
		}
		if uni.NFC(s) != s {
			return Narrowing{}, &decodeError{code: CodeSerializeNotCanonical, offset: at,
				message: "a recorded prefix that is not in Normalization Form C"}
		}
		// The prefix is restored as the range recorded it. StringPrefix would
		// cut it back again, as though more text had followed it, and "a",
		// recorded from "ab", would come back as nothing.
		return Narrowing{kind: narrowPrefix, str: s}, nil
	case 4, 5:
		length, err := d.r.ReadUint()
		if err != nil {
			return Narrowing{}, d.cborError(err)
		}
		if length > math.MaxInt64 {
			return Narrowing{}, d.malformed(at, "a length of %d", length)
		}
		if key == 4 {
			return LengthMin(int64(length)), nil
		}
		return LengthMax(int64(length)), nil
	case 6:
		if t.t.kind != KindSet {
			return Narrowing{}, d.malformed(at, "a range of %s recording members", t)
		}
		n, err := d.r.ReadArray()
		if err != nil {
			return Narrowing{}, d.cborError(err)
		}
		var members []Value
		for range n {
			mat := d.r.Offset()
			v, derr := d.content(t.t.elem)
			if derr != nil {
				return Narrowing{}, derr
			}
			if v.n.isMarked() {
				return Narrowing{}, d.malformed(mat, "a recorded member that carries marks")
			}
			members = append(members, v)
		}
		return Members(members...), nil
	}
	return Narrowing{}, d.malformed(at, "range key %d", key)
}

// number reads a number: an integer, a decimal fraction, or a bignum.
func (d *decoder) number() (Value, *decodeError) {
	at := d.r.Offset()
	small, c, exp, derr := d.numberParts()
	if derr != nil {
		return Value{}, derr
	}
	var dec decimal.Dec
	var err error
	if c == nil {
		dec, err = decimal.FromInt64Parts(small, exp)
	} else {
		dec, err = decimal.FromParts(c, exp)
	}
	if err != nil {
		return Value{}, d.malformed(at, "a number outside the range of numbers")
	}
	return numberValue(dec), nil
}

// numberParts reads a number's coefficient and exponent. The coefficient is
// in small where c is nil, as it is wherever it fits an int64, so that a
// number needing no big.Int is read without one.
func (d *decoder) numberParts() (small int64, c *big.Int, exp int64, derr *decodeError) {
	at := d.r.Offset()
	h, err := d.r.PeekHead()
	if err != nil {
		return 0, nil, 0, d.cborError(err)
	}
	switch {
	case h.Major == cbor.MajorUint || h.Major == cbor.MajorNeg:
		small, c, derr = d.integer()
		return small, c, 0, derr
	case h.Major == cbor.MajorTag && (h.Arg == tagBignum || h.Arg == tagNegBignum):
		small, c, derr = d.integer()
		return small, c, 0, derr
	case h.Major == cbor.MajorTag && h.Arg == tagDecimal:
		d.r.ReadTag()
		if err := d.array(2, "a decimal fraction"); err != nil {
			return 0, nil, 0, err
		}
		eat := d.r.Offset()
		neg, arg, err := d.r.ReadInt()
		if err != nil {
			return 0, nil, 0, d.cborError(err)
		}
		if arg > math.MaxInt64 {
			return 0, nil, 0, d.malformed(eat, "a number outside the range of numbers")
		}
		exp = int64(arg)
		if neg {
			exp = -1 - exp
		}
		small, c, derr = d.integer()
		return small, c, exp, derr
	}
	return 0, nil, 0, d.malformed(at, "expected a number")
}

// integer reads a CBOR integer or a bignum: into small where it is a CBOR
// integer that fits an int64, with c nil, and into c otherwise.
func (d *decoder) integer() (small int64, c *big.Int, derr *decodeError) {
	at := d.r.Offset()
	h, err := d.r.PeekHead()
	if err != nil {
		return 0, nil, d.cborError(err)
	}
	if h.Major == cbor.MajorTag {
		tag, _ := d.r.ReadTag()
		if tag != tagBignum && tag != tagNegBignum {
			return 0, nil, d.malformed(at, "expected an integer")
		}
		p, err := d.r.ReadBytes()
		if err != nil {
			return 0, nil, d.cborError(err)
		}
		c := new(big.Int).SetBytes(p)
		if tag == tagNegBignum {
			c.Neg(c.Add(c, big.NewInt(1)))
		}
		return 0, c, nil
	}
	neg, arg, err := d.r.ReadInt()
	if err != nil {
		return 0, nil, d.cborError(err)
	}
	if arg <= math.MaxInt64 {
		if neg {
			// The argument of a negative integer n is -1 - n.
			return -1 - int64(arg), nil, nil
		}
		return int64(arg), nil, nil
	}
	c = new(big.Int).SetUint64(arg)
	if neg {
		c.Neg(c.Add(c, big.NewInt(1)))
	}
	return 0, c, nil
}

// markList reads a list of marks, which is not empty.
func (d *decoder) markList() ([]Mark, *decodeError) {
	at := d.r.Offset()
	n, err := d.r.ReadArray()
	if err != nil {
		return nil, d.cborError(err)
	}
	if n == 0 {
		return nil, d.malformed(at, "an empty list of marks")
	}
	var marks []Mark
	for range n {
		m, derr := d.mark()
		if derr != nil {
			return nil, derr
		}
		marks = append(marks, m)
	}
	return marks, nil
}

// mark reads a mark and decodes it with the decoder supplied for its
// identifier.
func (d *decoder) mark() (Mark, *decodeError) {
	at := d.r.Offset()
	n, err := d.r.ReadArray()
	if err != nil {
		return nil, d.cborError(err)
	}
	if n != 1 && n != 3 {
		return nil, d.malformed(at, "a mark of %d parts", n)
	}
	id, err := d.r.ReadText()
	if err != nil {
		return nil, d.cborError(err)
	}
	decode, ok := d.marks[id]
	if !ok {
		return nil, &decodeError{code: CodeSerializeUnknownMark, offset: at,
			message: "no decoder was supplied for the mark " + quoted(id)}
	}
	var payload Value
	if n == 3 {
		t, derr := d.typ()
		if derr != nil {
			return nil, derr
		}
		v, derr := d.content(t)
		if derr != nil {
			return nil, derr
		}
		if !v.n.isKnown() || v.n.state == stateNull || v.n.isMarked() {
			return nil, d.malformed(at, "the mark %s is serialized with a value that is null, not known, or marked", quoted(id))
		}
		payload = v
	}
	m, diags := decode(payload, n == 3)
	switch {
	case len(diags) > 0:
		ErrorVal(diags...) // a decoder's diagnostics must be ones ErrorVal accepts
		return nil, &decodeError{offset: at, diags: diags}
	case m == nil:
		usagePanic("the decoder of the mark %q returned neither a mark nor a diagnostic", id)
	case m.MarkID() != id:
		usagePanic("the decoder of the mark %q returned the mark %q", id, m.MarkID())
	}
	return m, nil
}
