package tenon

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon/internal/decimal"
)

// Apply returns the value the path reaches within v, reading each step from
// the value the one before it reached (VA-022): an attribute step the
// attribute of an object; an index step, by a whole-number key counting from
// zero, the element of a list or a tuple and the member of a set at that
// place in its iteration order, and by a string key, the element of a map. A
// pending value holding its members is read as its tuple or object. The
// answer carries its own marks and the Propagate and redacting marks of every
// value a step read it out of.
//
// A step that reaches nothing fails with CodePathNoMember, a step of the
// wrong kind for the value it reads with CodeOperationWrongType, and a step
// from a null with CodeOperationNullOperand: an error value located at the
// path up to the step, or at a redacted value the steps went into (VA-023).
// A value not known yet answers the unknown of the member's type, or a pending
// value the pending value of the member's constraint, and a step every value
// it may turn out to be would fail fails now (VA-024). An error value met on
// the way is the answer. Apply panics on the zero Value.
func (p Path) Apply(v Value) Value {
	v.data()
	cur, at := v, Path{}
	var carried []Mark
	var redacted *Path
	for _, s := range p.Steps() {
		n := cur.data()
		if n.state == stateError {
			return withCarried(cur, carried)
		}
		if redacted == nil && n.withholds() {
			r := at
			redacted = &r
		}
		carried = append(carried, propagateMarks(n)...)
		at = at.extend(s)
		member, failure := stepFrom(cur, s)
		if failure.code != "" {
			loc, msg := at, failure.message
			if redacted != nil {
				loc, msg = *redacted, "Apply: the path goes into "+redactedBy(redactedValue(v, *redacted).data().redactingMarks())+", which withholds what it holds"
			}
			return withCarried(ErrorVal(Diagnostic{Code: failure.code, Message: msg, Path: loc}), carried)
		}
		cur = member
	}
	return withCarried(cur, carried)
}

// Lookup returns the value the path reaches within v and true, or the zero
// Value and false where applying the path fails, as Apply's answer would be an
// error value (VA-025). It is Apply for a caller asking whether anything is
// there, as a missing attribute or a null on the way leaves nothing to read.
func (p Path) Lookup(v Value) (Value, bool) {
	r := p.Apply(v)
	if r.IsError() {
		return Value{}, false
	}
	return r, true
}

// withCarried returns v carrying the marks a path's steps read on the way.
func withCarried(v Value, carried []Mark) Value {
	if len(carried) == 0 {
		return v
	}
	return WithMarks(v, carried...)
}

// redactedValue returns the value at the path r within v, which the steps
// before it read without failing.
func redactedValue(v Value, r Path) Value {
	cur := v
	for _, s := range r.Steps() {
		cur, _ = stepFrom(cur, s)
	}
	return cur
}

// stepFailure is why a step reaches nothing: its code and message.
type stepFailure struct {
	code    Code
	message string
}

// stepFrom returns what the step s reads from v, or why it reads nothing.
func stepFrom(v Value, s Step) (Value, stepFailure) {
	n := v.data()
	if n.state == stateNull || n.state == statePending && n.null == nullOnly {
		return Value{}, stepFailure{CodeOperationNullOperand, "Apply: the step " + s.String() + " reads from a null"}
	}
	if p, ok := n.held(); ok {
		if p.names != nil {
			return objectStep(s, func(name string) (Value, bool) { return p.attribute(name) })
		}
		return sequenceStep(s, "tuple", int64(len(p.vals)), true, func(i int) Value { return p.vals[i] })
	}
	if n.state == statePending {
		return pendingStep(n.constraint(), s, n)
	}
	t := n.typ
	known := n.state == stateKnown
	switch t.t.kind {
	case KindObject:
		if !known {
			if s.kind != StepAttribute {
				return Value{}, wrongStep(s, "an object")
			}
			at, ok := t.LookupAttributeType(s.name)
			if !ok {
				return Value{}, stepFailure{CodePathNoMember, "Apply: the object has no attribute " + quoted(s.name)}
			}
			return Unknown(at), stepFailure{}
		}
		return objectStep(s, func(name string) (Value, bool) { return n.attribute(name) })
	case KindMap:
		if s.kind != StepIndex || s.key.n.typ.t.kind != KindString {
			return Value{}, wrongStep(s, "a map")
		}
		if !known {
			return Unknown(t.ElementType()), stepFailure{}
		}
		key := s.key.n.data.(string)
		e, ok := findName(n.data.([]mapEntry), key, func(e mapEntry) string { return e.key })
		if !ok {
			return Value{}, stepFailure{CodePathNoMember, "Apply: the map has no key " + quoted(key)}
		}
		return e.val, stepFailure{}
	case KindList, KindSet:
		kind := "list"
		if t.t.kind == KindSet {
			kind = "set"
		}
		if !known {
			if _, failure := sequenceIndex(s, kind, -1, false); failure.code != "" {
				return Value{}, failure
			}
			if hi, bounded := v.Range().LengthMax(); bounded {
				if _, failure := sequenceIndex(s, kind, hi, true); failure.code != "" {
					return Value{}, failure
				}
			}
			return Unknown(t.ElementType()), stepFailure{}
		}
		if t.t.kind == KindSet {
			members := v.Elements()
			return sequenceStep(s, kind, int64(len(members)), true, func(i int) Value { return members[i] })
		}
		elems := n.data.([]Value)
		return sequenceStep(s, kind, int64(len(elems)), true, func(i int) Value { return elems[i] })
	case KindTuple:
		elemTypes := t.TupleElementTypes()
		if !known {
			i, failure := sequenceIndex(s, "tuple", int64(len(elemTypes)), true)
			if failure.code != "" {
				return Value{}, failure
			}
			return Unknown(elemTypes[i]), stepFailure{}
		}
		elems := n.data.([]Value)
		return sequenceStep(s, "tuple", int64(len(elems)), true, func(i int) Value { return elems[i] })
	}
	return Value{}, wrongStep(s, "a "+strings.ToLower(t.t.kind.String()))
}

// objectStep reads the step s from an object, whose attributes attr finds by
// name.
func objectStep(s Step, attr func(string) (Value, bool)) (Value, stepFailure) {
	if s.kind != StepAttribute {
		return Value{}, wrongStep(s, "an object")
	}
	a, ok := attr(s.name)
	if !ok {
		return Value{}, stepFailure{CodePathNoMember, "Apply: the object has no attribute " + quoted(s.name)}
	}
	return a, stepFailure{}
}

// sequenceStep reads the step s from a list, a tuple or a set of n members,
// the member at a place given by at.
func sequenceStep(s Step, kind string, n int64, bounded bool, at func(int) Value) (Value, stepFailure) {
	i, failure := sequenceIndex(s, kind, n, bounded)
	if failure.code != "" {
		return Value{}, failure
	}
	return at(int(i)), stepFailure{}
}

// sequenceIndex returns the place the step s names in a list, a tuple or a
// set of at most n members, where bounded, or why it names none: a step that
// is not a number key is of the wrong kind, and a key that is not a whole
// number from zero to n-1 names no member.
func sequenceIndex(s Step, kind string, n int64, bounded bool) (int64, stepFailure) {
	if s.kind != StepIndex || s.key.n.typ.t.kind != KindNumber {
		return 0, wrongStep(s, "a "+kind)
	}
	d := s.key.n.data.(decimal.Dec)
	i, whole := d.Int64()
	if _, integral := d.BigInt(); !integral {
		return 0, stepFailure{CodePathNoMember, "Apply: the key " + d.String() + " is not a whole number, and names no member of a " + kind}
	}
	switch {
	case d.Sign() < 0:
		return 0, stepFailure{CodePathNoMember, "Apply: the key " + d.String() + " is negative, and names no member of a " + kind}
	case bounded && (!whole || i >= n):
		return 0, stepFailure{CodePathNoMember, "Apply: the key " + d.String() + " is not below the " + strconv.FormatInt(n, 10) + " members of the " + kind}
	}
	return i, stepFailure{}
}

// wrongStep returns the failure of the step s on a value of the kind named.
func wrongStep(s Step, of string) stepFailure {
	reads := "an attribute step reads an object"
	if s.kind == StepIndex {
		if s.key.n.typ.t.kind == KindString {
			reads = "a string key reads a map"
		} else {
			reads = "a number key reads a list, a tuple or a set"
		}
	}
	return stepFailure{CodeOperationWrongType, "Apply: the step " + s.String() + " reads " + of + ", and " + reads}
}

// pendingStep reads the step s from a pending value of the constraint c,
// which holds no members: the pending value of the constraint c admits for
// the member, or the unknown of the type it names, and the step fails now
// where no type c admits could take it.
func pendingStep(c Constraint, s Step, n *node) (Value, stepFailure) {
	member, failure := memberConstraint(c, s)
	if failure.code != "" {
		return Value{}, failure
	}
	if s.kind == StepIndex && s.key.n.typ.t.kind == KindNumber {
		if _, hi := n.pendingLengths(); hi.set {
			if _, failure := sequenceIndex(s, "list", hi.n, true); failure.code != "" {
				return Value{}, failure
			}
		}
	}
	if member.Kind() == ConstraintExactly {
		return Unknown(member.Type()), stepFailure{}
	}
	return Pending(member), stepFailure{}
}

// memberConstraint returns the constraint c admits for what the step s
// reads, or why no type c admits can take the step.
func memberConstraint(c Constraint, s Step) (Constraint, stepFailure) {
	d := c.data()
	switch d.kind {
	case ConstraintAny:
		if s.kind == StepIndex && s.key.n.typ.t.kind == KindNumber {
			if _, failure := sequenceIndex(s, "list", -1, false); failure.code != "" {
				return Constraint{}, failure
			}
		}
		return Any(), stepFailure{}
	case ConstraintExactly:
		v := Unknown(d.typ)
		m, failure := stepFrom(v, s)
		if failure.code != "" {
			return Constraint{}, failure
		}
		return Exactly(m.Type()), stepFailure{}
	case ConstraintListOf, ConstraintSetOf:
		kind := "list"
		if d.kind == ConstraintSetOf {
			kind = "set"
		}
		if _, failure := sequenceIndex(s, kind, -1, false); failure.code != "" {
			return Constraint{}, failure
		}
		return d.elem, stepFailure{}
	case ConstraintMapOf:
		if s.kind != StepIndex || s.key.n.typ.t.kind != KindString {
			return Constraint{}, wrongStep(s, "a map")
		}
		return d.elem, stepFailure{}
	case ConstraintTupleOf:
		i, failure := sequenceIndex(s, "tuple", int64(len(d.members)), true)
		if failure.code != "" {
			return Constraint{}, failure
		}
		return d.members[i], stepFailure{}
	case ConstraintObjectWith:
		if s.kind != StepAttribute {
			return Constraint{}, wrongStep(s, "an object")
		}
		for _, f := range d.fields {
			if f.name == s.name {
				return f.Constraint, stepFailure{}
			}
		}
		if d.closed {
			return Constraint{}, stepFailure{CodePathNoMember, "Apply: no object the constraint admits has the attribute " + quoted(s.name)}
		}
		return Any(), stepFailure{}
	case ConstraintOneOf:
		var admitted []Constraint
		var first stepFailure
		for _, m := range d.members {
			mc, failure := memberConstraint(m, s)
			if failure.code != "" {
				if first.code == "" {
					first = failure
				}
				continue
			}
			admitted = append(admitted, mc)
		}
		switch len(admitted) {
		case 0:
			return Constraint{}, first
		case 1:
			return admitted[0], stepFailure{}
		}
		return Any(), stepFailure{}
	}
	return Any(), stepFailure{}
}

// HasPrefix reports whether q is a prefix of p: whether p begins with q's
// steps, the empty path and p itself among its prefixes.
func (p Path) HasPrefix(q Path) bool {
	pn, qn := p.Len(), q.Len()
	if qn > pn {
		return false
	}
	node := p.last
	for range pn - qn {
		node = node.parent
	}
	return Path{last: node}.Equal(q)
}

// Parent returns p without its last step, and the empty path for the empty
// path.
func (p Path) Parent() Path {
	if p.last == nil {
		return Path{}
	}
	return Path{last: p.last.parent}
}

// Last returns the last step of p and true, or the zero Step and false for
// the empty path.
func (p Path) Last() (Step, bool) {
	if p.last == nil {
		return Step{}, false
	}
	return p.last.step, true
}

// ComparePaths returns -1, 0 or 1 as p orders before, with or after q in the
// canonical order of paths (VA-026): a path before every path it is a prefix
// of, and otherwise by the first step at which they differ, an attribute step
// before an index step, attributes by name and keys numbers before strings,
// numbers numerically and strings by Unicode scalar values. It is the order a
// walk visits a value's members in.
func ComparePaths(p, q Path) int {
	ps, qs := p.Steps(), q.Steps()
	for i := range min(len(ps), len(qs)) {
		if c := compareSteps(ps[i], qs[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(ps) < len(qs):
		return -1
	case len(ps) > len(qs):
		return 1
	}
	return 0
}

// compareSteps orders two steps as ComparePaths does.
func compareSteps(a, b Step) int {
	switch {
	case a.kind != b.kind:
		if a.kind == StepAttribute {
			return -1
		}
		return 1
	case a.kind == StepAttribute:
		return strings.Compare(a.name, b.name)
	}
	ak, bk := a.key.n.typ.t.kind, b.key.n.typ.t.kind
	switch {
	case ak != bk:
		if ak == KindNumber {
			return -1
		}
		return 1
	case ak == KindNumber:
		return a.key.n.data.(decimal.Dec).Cmp(b.key.n.data.(decimal.Dec))
	}
	return strings.Compare(a.key.n.data.(string), b.key.n.data.(string))
}

// ParsePath reads a path from its display form (DI-013, DI-038), as
// Path.String writes it: "." for the empty path, then steps, an attribute
// step a "." and a name, as it is where it is an identifier and quoted
// otherwise, and an index step "[", a number or a quoted string, and "]",
// with a "." before a first step that is an index. A quoted name or key reads
// the escapes of DI-012. Text that is no path fails with an *Error of code
// CodePathInvalidSyntax naming the byte where reading stopped.
func ParsePath(text string) (Path, error) {
	fail := func(at int, why string) (Path, error) {
		return Path{}, NewError(ErrorVal(Diagnostic{
			Code:    CodePathInvalidSyntax,
			Message: "the text is no path at byte " + strconv.Itoa(at) + ": " + why,
		}))
	}
	if !utf8.ValidString(text) {
		return fail(0, "it is not well-formed UTF-8")
	}
	if text == "." {
		return Path{}, nil
	}
	if !strings.HasPrefix(text, ".") {
		return fail(0, "a path begins with \".\"")
	}
	var p Path
	i := 0
	if strings.HasPrefix(text, ".[") {
		i = 1
	}
	for i < len(text) {
		switch text[i] {
		case '.':
			i++
			var name string
			switch {
			case i < len(text) && text[i] == '"':
				s, n, why := readQuoted(text[i:])
				if why != "" {
					return fail(i+n, why)
				}
				name, i = s, i+n
			default:
				j := i
				for j < len(text) && (text[j] == '_' || text[j] >= 'a' && text[j] <= 'z' || text[j] >= 'A' && text[j] <= 'Z' || j > i && text[j] >= '0' && text[j] <= '9') {
					j++
				}
				if j == i {
					return fail(i, "an attribute name, an identifier or quoted, was expected")
				}
				name, i = text[i:j], j
			}
			if err := CheckAttributeNames(name); err != nil {
				return fail(i, "the attribute name "+quoted(name)+" is one tenon refuses")
			}
			p = p.Attribute(name)
		case '[':
			i++
			var key Value
			switch {
			case i < len(text) && text[i] == '"':
				s, n, why := readQuoted(text[i:])
				if why != "" {
					return fail(i+n, why)
				}
				key, i = String(s), i+n
			default:
				j := strings.IndexByte(text[i:], ']')
				if j <= 0 {
					return fail(i, "a number or a quoted key, and \"]\", were expected")
				}
				key = NumberFromText(text[i : i+j])
				if key.IsError() {
					return fail(i, "the key "+quoted(text[i:i+j])+" is no number")
				}
				i += j
			}
			if i >= len(text) || text[i] != ']' {
				return fail(i, "\"]\" was expected")
			}
			i++
			p = p.Index(key)
		default:
			return fail(i, "a step, \".\" or \"[\", was expected")
		}
	}
	return p, nil
}

// readQuoted reads the quoted text s begins with, by the escapes of DI-012,
// and returns its content and its length in bytes, or why it is not one and
// where in s reading stopped.
func readQuoted(s string) (string, int, string) {
	var b strings.Builder
	for i := 1; i < len(s); {
		switch c := s[i]; c {
		case '"':
			return b.String(), i + 1, ""
		case '\\':
			if i+1 >= len(s) {
				return "", i, "the escape ends the text"
			}
			switch s[i+1] {
			case '"', '\\':
				b.WriteByte(s[i+1])
				i += 2
			case 't':
				b.WriteByte('\t')
				i += 2
			case 'n':
				b.WriteByte('\n')
				i += 2
			case 'r':
				b.WriteByte('\r')
				i += 2
			case 'u':
				end := strings.IndexByte(s[i:], '}')
				if !strings.HasPrefix(s[i:], `\u{`) || end < 0 {
					return "", i, "an escape \\u{...} was expected"
				}
				hex := s[i+3 : i+end]
				r, err := strconv.ParseUint(hex, 16, 32)
				if err != nil || len(hex) < 4 || !utf8.ValidRune(rune(r)) {
					return "", i, "the escape " + quoted(s[i:i+end+1]) + " names no code point"
				}
				b.WriteRune(rune(r))
				i += end + 1
			default:
				return "", i, "\\" + string(s[i+1]) + " is no escape"
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", len(s), "the quoted text is not closed"
}
