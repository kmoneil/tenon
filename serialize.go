package tenon

import (
	"bytes"
	"math/big"
	"slices"
	"unicode/utf8"

	"github.com/kmoneil/tenon/internal/cbor"
	"github.com/kmoneil/tenon/internal/decimal"
)

// The CBOR tags of the encoding.
const (
	tagDocument  uint64 = 1952804352
	tagUnknown   uint64 = 1952804353
	tagMarked    uint64 = 1952804354
	tagBignum    uint64 = 2
	tagNegBignum uint64 = 3
	tagDecimal   uint64 = 4
)

// formatVersion is the version of the encoding that Serialize writes.
const formatVersion = 1

// The item kinds of the encoding.
const (
	itemResolved = 0
	itemPending  = 1
	itemError    = 2
)

// Serialize returns the encoding of v, a CBOR document, and true. Every value
// has exactly one encoding, whatever way it was built: two values serialize to
// the same bytes exactly when they are Identical, so the bytes can be hashed,
// compared or used as a key in place of the value.
//
// Where v cannot be serialized, Serialize returns an error value and false.
// The error value has a diagnostic for each part of v that cannot be, located
// by its path: a capsule value, or a type or constraint naming a capsule type,
// whose capsule type declares no encoding or shares its identifier with
// another (CodeSerializeUnencodableCapsule), and a mark that does not
// implement EncodableMark (CodeSerializeUnencodableMark).
//
// Serialize panics on the zero Value, and on a capsule encoding or a mark
// payload that breaks its contract: one that is not a known, unmarked value of
// the declared type other than a null, or two unequal marks on one value that
// serialize alike.
func Serialize(v Value) ([]byte, Value, bool) {
	v.data()
	e := &encoder{ids: map[string]Type{}, implied: map[*markSet]*impliedMarks{}}
	body := e.item(nil, v)
	if failure, failed := e.errs.value(); failed {
		return nil, failure, false
	}
	doc := cbor.AppendTag(make([]byte, 0, len(body)+8), tagDocument)
	doc = cbor.AppendArray(doc, 2)
	doc = cbor.AppendUint(doc, formatVersion)
	return append(doc, body...), Value{}, true
}

// encoder encodes one value, collecting what it cannot encode.
type encoder struct {
	errs containerErrors
	// ids holds the capsule identifiers met so far, and the type using each.
	ids map[string]Type
	// implied holds what each mark set met on a container implies on the
	// values it holds, or nil where it implies nothing.
	implied map[*markSet]*impliedMarks
	// failures counts the calls to fail. It is not len(errs.diags), which
	// records one diagnostic however many times an identical one arrives, and
	// payloads that fail alike fail with the same message at the same path.
	failures int
	// recorded holds the encoding of every diagnostic recorded, by which one
	// that arrives again is known without comparing it with each before it:
	// two diagnostics are equal exactly when their encodings are.
	recorded map[string]struct{}
}

// fail records a diagnostic for what is at path p, unless an identical one is
// recorded already.
func (e *encoder) fail(p Path, code Code, message string) {
	e.failures++
	d := Diagnostic{Code: code, Message: message, Path: p}
	key := string(appendDiagnostic(nil, d))
	if _, dup := e.recorded[key]; dup {
		return
	}
	if e.recorded == nil {
		e.recorded = map[string]struct{}{}
	}
	e.recorded[key] = struct{}{}
	e.errs.diags = append(e.errs.diags, d)
}

// item appends the item of v.
func (e *encoder) item(b []byte, v Value) []byte {
	n := v.n
	if n.state.resolved() {
		b = cbor.AppendArray(b, 3)
		b = cbor.AppendUint(b, itemResolved)
		b = e.typ(b, n.typ, Path{})
		return e.content(b, v, Path{}, nil)
	}
	var inner []byte
	if n.state == statePending {
		inner = cbor.AppendArray(inner, 3)
		inner = cbor.AppendUint(inner, itemPending)
		inner = e.constraint(inner, n.data.(Constraint), Path{})
		inner = cbor.AppendUint(inner, uint64(nullnessCode(n.null)))
	} else {
		diags := n.data.([]Diagnostic)
		inner = cbor.AppendArray(inner, 2)
		inner = cbor.AppendUint(inner, itemError)
		inner = cbor.AppendArray(inner, len(diags))
		for _, d := range diags {
			inner = appendDiagnostic(inner, d)
		}
	}
	if n.marks == nil {
		return append(b, inner...)
	}
	b = cbor.AppendTag(b, tagMarked)
	b = cbor.AppendArray(b, 2)
	b = append(b, inner...)
	return e.marks(b, n.markList(), Path{})
}

// nullnessCode returns the code of a pending value's nullness fact.
func nullnessCode(null nullness) int {
	switch null {
	case nullNo:
		return 1
	case nullOnly:
		return 2
	}
	return 0
}

// appendDiagnostic appends a diagnostic: its code, its message, and its path.
func appendDiagnostic(b []byte, d Diagnostic) []byte {
	b = cbor.AppendArray(b, 3)
	b = cbor.AppendText(b, string(d.Code))
	b = cbor.AppendText(b, d.Message)
	steps := d.Path.Steps()
	b = cbor.AppendArray(b, len(steps))
	for _, s := range steps {
		switch {
		case s.kind == StepAttribute:
			b = cbor.AppendText(b, s.name)
		case s.key.n.typ.t.kind == KindString:
			b = cbor.AppendArray(b, 1)
			b = cbor.AppendText(b, s.key.n.data.(string))
		default:
			b = appendNumber(b, s.key.n.data.(decimal.Dec))
		}
	}
	return b
}

// typ appends the encoding of a type, which p locates within the value.
func (e *encoder) typ(b []byte, t Type, p Path) []byte {
	d := t.t
	switch d.kind {
	case KindBool, KindNumber, KindString:
		return cbor.AppendUint(b, uint64(d.kind))
	case KindList, KindSet, KindMap:
		b = cbor.AppendArray(b, 2)
		b = cbor.AppendUint(b, uint64(d.kind))
		return e.typ(b, d.elem, p)
	case KindTuple:
		b = cbor.AppendArray(b, 2)
		b = cbor.AppendUint(b, uint64(d.kind))
		b = cbor.AppendArray(b, len(d.elems))
		for _, elem := range d.elems {
			b = e.typ(b, elem, p)
		}
		return b
	case KindObject:
		b = cbor.AppendArray(b, 2)
		b = cbor.AppendUint(b, uint64(d.kind))
		b = cbor.AppendArray(b, len(d.attrs))
		for _, a := range d.attrs {
			b = cbor.AppendArray(b, 2)
			b = cbor.AppendText(b, a.name)
			b = e.typ(b, a.typ, p)
		}
		return b
	}
	b = cbor.AppendArray(b, 2)
	b = cbor.AppendUint(b, uint64(d.kind))
	return cbor.AppendText(b, e.capsuleID(t, p))
}

// capsuleID returns the identifier a capsule type declares, recording a
// failure where it declares none or another type met already declares it.
func (e *encoder) capsuleID(t Type, p Path) string {
	d := t.t.capsule
	if d.encoding == nil {
		e.fail(p, CodeSerializeUnencodableCapsule, "capsule type "+quoted(d.name)+" declares no encoding")
		return ""
	}
	id := d.encoding.id
	if other, ok := e.ids[id]; ok && other != t {
		e.fail(p, CodeSerializeUnencodableCapsule, "two capsule types, both named "+quoted(d.name)+
			" or "+quoted(other.t.capsule.name)+", declare the identifier "+quoted(id))
		return id
	}
	e.ids[id] = t
	return id
}

// constraint appends the encoding of a constraint.
func (e *encoder) constraint(b []byte, c Constraint, p Path) []byte {
	d := c.c
	switch d.kind {
	case ConstraintExactly:
		b = cbor.AppendArray(b, 2)
		b = cbor.AppendUint(b, uint64(d.kind))
		return e.typ(b, d.typ, p)
	case ConstraintAny:
		b = cbor.AppendArray(b, 1)
		return cbor.AppendUint(b, uint64(d.kind))
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		b = cbor.AppendArray(b, 2)
		b = cbor.AppendUint(b, uint64(d.kind))
		return e.constraint(b, d.elem, p)
	case ConstraintObjectWith:
		b = cbor.AppendArray(b, 3)
		b = cbor.AppendUint(b, uint64(d.kind))
		b = cbor.AppendArray(b, len(d.fields))
		for _, f := range d.fields {
			b = cbor.AppendArray(b, 3)
			b = cbor.AppendText(b, f.name)
			b = cbor.AppendBool(b, f.Required)
			b = e.constraint(b, f.Constraint, p)
		}
		return cbor.AppendBool(b, d.closed)
	}
	b = cbor.AppendArray(b, 2)
	b = cbor.AppendUint(b, uint64(d.kind))
	b = cbor.AppendArray(b, len(d.members))
	for _, m := range d.members {
		b = e.constraint(b, m, p)
	}
	return b
}

// impliedMarks is what a container carrying deep marks implies on the values
// it holds: they carry those marks because the container does, so [SE-031]
// does not list them again. The marks a value lists for itself follow from
// the mark set it holds, so they are decided once per set rather than once
// per value: a container's members commonly share one set, the one the deep
// marks were attached to them through.
type impliedMarks struct {
	deep map[Mark]bool       // the container's deep marks
	own  map[*markSet][]Mark // what a value holding that set lists for itself
}

// listed returns the marks a value holding held lists for itself, which are
// those the container does not imply on it. The result is read, never
// appended to: it is often held's own list, which the mark set shares.
func (im *impliedMarks) listed(held *markSet) []Mark {
	if own, ok := im.own[held]; ok {
		return own
	}
	own := held.list
	for i, m := range held.list {
		if !im.deep[m] {
			continue
		}
		own = slices.Clone(held.list[:i:i])
		for _, m := range held.list[i+1:] {
			if !im.deep[m] {
				own = append(own, m)
			}
		}
		break
	}
	if im.own == nil {
		im.own = map[*markSet][]Mark{}
	}
	im.own[held] = own
	return own
}

// implies returns what a container carrying the marks in ms implies on the
// values it holds, or nil where it implies nothing. Containers that carry the
// same marks share one answer, which holds what the values under them list.
func (e *encoder) implies(ms *markSet) *impliedMarks {
	if ms == nil {
		return nil
	}
	if im, ok := e.implied[ms]; ok {
		return im
	}
	var im *impliedMarks
	for _, m := range ms.list {
		if !isDeep(m) {
			continue
		}
		if im == nil {
			im = &impliedMarks{deep: map[Mark]bool{}}
		}
		im.deep[m] = true
	}
	e.implied[ms] = im
	return im
}

// content appends the content of the resolved value v, which p locates. The
// value is held by a container implying deep marks on it, which v carries
// because the container does, and which are therefore not listed on v.
func (e *encoder) content(b []byte, v Value, p Path, implied *impliedMarks) []byte {
	own := v.n.markList()
	if own != nil && implied != nil {
		own = implied.listed(v.n.marks)
	}
	if len(own) == 0 {
		return e.bare(b, v, p)
	}
	b = cbor.AppendTag(b, tagMarked)
	b = cbor.AppendArray(b, 2)
	b = e.bare(b, v, p)
	return e.marks(b, own, p)
}

// bare appends the content of the resolved value v without its marks.
func (e *encoder) bare(b []byte, v Value, p Path) []byte {
	n := v.n
	switch n.state {
	case stateNull:
		return cbor.AppendNull(b)
	case stateUnknown:
		b = cbor.AppendTag(b, tagUnknown)
		return e.rng(b, n.data.(*rangeData), p)
	}
	switch n.typ.t.kind {
	case KindBool:
		return cbor.AppendBool(b, n.data.(bool))
	case KindNumber:
		return appendNumber(b, n.data.(decimal.Dec))
	case KindString:
		return cbor.AppendText(b, n.data.(string))
	case KindList, KindTuple:
		members := n.data.([]Value)
		implied := e.implies(n.marks)
		b = cbor.AppendArray(b, len(members))
		for i, m := range members {
			b = e.content(b, m, p.extend(indexStep(NumberFromInt(int64(i)))), implied)
		}
		return b
	case KindSet:
		// The members of a set carry no marks in storage ([MK-006]), so a
		// deep mark on the set implies nothing on them: it stays on the set.
		members := n.data.([]Value)
		return e.members(b, members, func(i int) Path { return p.extend(indexStep(NumberFromInt(int64(i)))) })
	case KindMap:
		entries := n.data.([]mapEntry)
		implied := e.implies(n.marks)
		b = cbor.AppendArray(b, len(entries))
		for _, entry := range entries {
			b = cbor.AppendArray(b, 2)
			b = cbor.AppendText(b, entry.key)
			b = e.content(b, entry.val, p.extend(indexStep(String(entry.key))), implied)
		}
		return b
	case KindObject:
		attrs := n.data.([]Value)
		implied := e.implies(n.marks)
		b = cbor.AppendArray(b, len(attrs))
		for i, m := range attrs {
			b = e.content(b, m, p.extend(attributeStep(n.typ.t.attrs[i].name)), implied)
		}
		return b
	}
	return e.capsule(b, n, p)
}

// members appends the members of a set, or those a range records, in the
// bytewise order of their encodings. at locates member i.
func (e *encoder) members(b []byte, members []Value, at func(i int) Path) []byte {
	encoded := make([][]byte, len(members))
	for i, m := range members {
		encoded[i] = e.content(nil, m, at(i), nil)
	}
	slices.SortFunc(encoded, bytes.Compare)
	b = cbor.AppendArray(b, len(encoded))
	for _, enc := range encoded {
		b = append(b, enc...)
	}
	return b
}

// capsule appends the content of a known capsule value: the value its type
// serializes it as.
func (e *encoder) capsule(b []byte, n *node, p Path) []byte {
	d := n.typ.t.capsule
	if d.encoding == nil {
		e.fail(p, CodeSerializeUnencodableCapsule, "capsule type "+quoted(d.name)+" declares no encoding")
		return cbor.AppendNull(b)
	}
	payload := d.encoding.encode(n.data)
	if payload.n == nil || !payload.n.isKnown() || payload.n.state == stateNull || payload.n.isMarked() || payload.n.typ != d.encoding.typ {
		usagePanic("capsule type %q serialized a value as %s, not a known, unmarked value of %s other than null",
			d.name, payload, d.encoding.typ)
	}
	b = cbor.AppendArray(b, 2)
	b = e.typ(b, d.encoding.typ, p)
	return e.content(b, payload, p, nil)
}

// rng appends a range: a map from keys to the narrowings it records.
func (e *encoder) rng(b []byte, r *rangeData, p Path) []byte {
	keys := 0
	for _, set := range []bool{r.null == nullNo, r.lo.set, r.hi.set, r.pfx != "", r.lenLo > 0, r.lenHi.set, len(r.members) > 0} {
		if set {
			keys++
		}
	}
	b = cbor.AppendMap(b, keys)
	if r.null == nullNo {
		b = cbor.AppendUint(b, 0)
		b = cbor.AppendBool(b, true)
	}
	for i, bd := range []bound{r.lo, r.hi} {
		if bd.set {
			b = cbor.AppendUint(b, uint64(1+i))
			b = cbor.AppendArray(b, 2)
			b = appendNumber(b, bd.v)
			b = cbor.AppendBool(b, bd.incl)
		}
	}
	if r.pfx != "" {
		b = cbor.AppendUint(b, 3)
		b = cbor.AppendText(b, r.pfx)
	}
	if r.lenLo > 0 {
		b = cbor.AppendUint(b, 4)
		b = cbor.AppendUint(b, uint64(r.lenLo))
	}
	if r.lenHi.set {
		b = cbor.AppendUint(b, 5)
		b = cbor.AppendUint(b, uint64(r.lenHi.n))
	}
	if len(r.members) > 0 {
		b = cbor.AppendUint(b, 6)
		b = e.members(b, r.members, func(int) Path { return p })
	}
	return b
}

// marks appends a list of marks, in the bytewise order of their encodings.
//
// A payload that does not encode has already been recorded as a failure, and
// what its bytes hold is a placeholder rather than an encoding. Two such marks
// leave the same placeholder, so they are left out of the duplicate check
// below: what [SE-041] forbids is two unequal marks with one encoding, and
// these have none.
func (e *encoder) marks(b []byte, marks []Mark, p Path) []byte {
	encoded := make([][]byte, 0, len(marks))
	for _, m := range marks {
		id := m.MarkID()
		if !utf8.ValidString(id) {
			usagePanic("a mark of type %T has an identifier that is not valid UTF-8", m)
		}
		em, ok := m.(EncodableMark)
		if !ok {
			e.fail(p, CodeSerializeUnencodableMark, "the mark "+quoted(id)+" declares no encoding")
			continue
		}
		payload, has := em.MarkPayload()
		var enc []byte
		if !has {
			enc = cbor.AppendArray(enc, 1)
			enc = cbor.AppendText(enc, id)
		} else {
			if payload.n == nil || !payload.n.isKnown() || payload.n.state == stateNull || payload.n.isMarked() {
				usagePanic("the mark %q serialized with %s, not a known, unmarked value other than a null", id, payload)
			}
			before := e.failures
			enc = cbor.AppendArray(enc, 3)
			enc = cbor.AppendText(enc, id)
			enc = e.typ(enc, payload.n.typ, p)
			enc = e.content(enc, payload, p, nil)
			if e.failures > before {
				continue
			}
		}
		encoded = append(encoded, enc)
	}
	slices.SortFunc(encoded, bytes.Compare)
	for i := 1; i < len(encoded); i++ {
		if bytes.Equal(encoded[i-1], encoded[i]) {
			usagePanic("two unequal marks on one value serialize alike: %x", encoded[i])
		}
	}
	b = cbor.AppendArray(b, len(encoded))
	for _, enc := range encoded {
		b = append(b, enc...)
	}
	return b
}

// appendNumber appends a number: an integer where it is one that CBOR's
// integers hold, and otherwise a decimal fraction.
func appendNumber(b []byte, d decimal.Dec) []byte {
	small, c, exp := d.Parts()
	if c == nil {
		c = big.NewInt(small)
	}
	// Every integer that CBOR's integers hold has fewer than 21 digits.
	if exp >= 0 && exp <= 20 {
		v := new(big.Int).Mul(c, new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil))
		if ok, out := appendBigInt(b, v); ok {
			return out
		}
	}
	b = cbor.AppendTag(b, tagDecimal)
	b = cbor.AppendArray(b, 2)
	b = cbor.AppendInt(b, exp)
	if ok, out := appendBigInt(b, c); ok {
		return out
	}
	if c.Sign() > 0 {
		b = cbor.AppendTag(b, tagBignum)
		return cbor.AppendBytes(b, c.Bytes())
	}
	b = cbor.AppendTag(b, tagNegBignum)
	return cbor.AppendBytes(b, new(big.Int).Sub(new(big.Int).Neg(c), big.NewInt(1)).Bytes())
}

// appendBigInt appends v as a CBOR integer where it lies from -2^64 to
// 2^64 - 1, and reports whether it did.
func appendBigInt(b []byte, v *big.Int) (bool, []byte) {
	if v.Sign() >= 0 {
		if v.IsUint64() {
			return true, cbor.AppendUint(b, v.Uint64())
		}
		return false, b
	}
	// A negative integer n is written as the argument -1 - n.
	arg := new(big.Int).Sub(new(big.Int).Neg(v), big.NewInt(1))
	if arg.IsUint64() {
		return true, cbor.AppendHead(b, cbor.MajorNeg, arg.Uint64())
	}
	return false, b
}
