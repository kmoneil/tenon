package tenon

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon/internal/decimal"
	"github.com/kmoneil/tenon/internal/uni"
)

// goWriter writes the Go syntax that builds a value, a type, or what holds
// them, for GoString. It writes the calls of the package's constructors, as
// in tenon.List(tenon.NumberType(), tenon.NumberFromInt(1)), so that a
// test's failure shows text that can be pasted back into the test. Where
// names is set it writes each type other than Bool, Number and String by the
// variable that names it, defined once, rather than in full wherever it
// appears (see goSyntax).
type goWriter struct {
	textWriter
	names *goNames
}

// goNames holds the variables a goWriter names types by: each type's
// variable, and their definitions in the order they were written, the types a
// type holds defined before it.
type goNames struct {
	vars map[*typeData]string
	defs strings.Builder
}

// plainGoLimit is how many bytes the Go syntax that writes every type in full
// may run to before goSyntax weighs it against naming each type once.
const plainGoLimit = 4096

// goSyntax returns the Go syntax that write writes, which builds a result of
// the Go type result, such as tenon.Value. It writes each type in full
// wherever one is needed, as tenon.Null(T) needs T, unless that comes to more
// than plainGoLimit bytes and more than twice what naming each type once
// does. Many members of one large type, or a type many levels deep, would
// have it grow with the members times their type, where a display form grows
// with the value (DI-010). Then it names each type in a variable, in a
// function literal called in place:
//
//	func() tenon.Value { t1 := tenon.ListType(tenon.NumberType()); return tenon.List(t1, tenon.Null(t1)) }()
//
// Each attempt at the plain form stops at its limit, so the syntax costs what
// the named form does, however long the plain one would have been.
func goSyntax(result string, write func(w *goWriter)) string {
	plain := goWriter{}
	plain.limit = plainGoLimit
	write(&plain)
	if !plain.full() {
		return plain.String()
	}
	named := goWriter{names: &goNames{vars: map[*typeData]string{}}}
	write(&named)
	if len(named.names.vars) == 0 {
		return named.String()
	}
	text := "func() " + result + " { " + named.names.defs.String() + "return " + named.String() + " }()"
	plain = goWriter{}
	plain.limit = 2 * len(text)
	write(&plain)
	if !plain.full() {
		return plain.String()
	}
	return text
}

// writeQuoted writes s as a Go string literal.
func (w *goWriter) writeQuoted(s string) {
	w.WriteString(strconv.Quote(s))
}

// writeType writes the Go syntax of t: the function that returns it, or the
// variable that names it.
func (w *goWriter) writeType(t Type) {
	d := t.t
	if isPrimitive(d.kind) {
		w.WriteString("tenon.")
		w.WriteString(d.kind.String())
		w.WriteString("Type()")
		return
	}
	if w.names != nil {
		w.WriteString(w.names.name(t))
		return
	}
	w.writeTypeCall(t)
}

// name returns the variable that names t, defining it, after the types t
// holds, where t has none yet.
func (ns *goNames) name(t Type) string {
	if v, ok := ns.vars[t.t]; ok {
		return v
	}
	def := goWriter{names: ns}
	def.writeTypeCall(t)
	v := "t" + strconv.Itoa(len(ns.vars)+1)
	ns.vars[t.t] = v
	ns.defs.WriteString(v)
	ns.defs.WriteString(" := ")
	ns.defs.WriteString(def.String())
	ns.defs.WriteString("; ")
	return v
}

// writeTypeCall writes the call that returns t, a type other than Bool,
// Number and String.
func (w *goWriter) writeTypeCall(t Type) {
	d := t.t
	switch d.kind {
	case KindList, KindSet, KindMap:
		w.WriteString("tenon.")
		w.WriteString(d.kind.String())
		w.WriteString("Type(")
		w.writeType(d.elem)
		w.WriteByte(')')
	case KindTuple:
		w.WriteString("tenon.TupleType(")
		for i, e := range d.elems {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeType(e)
		}
		w.WriteByte(')')
	case KindObject:
		w.WriteString("tenon.ObjectType(map[string]tenon.Type{")
		for i, a := range d.attrs {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeQuoted(a.name)
			w.WriteByte(':')
			w.writeType(a.typ)
		}
		w.WriteString("})")
	case KindCapsule:
		w.writeCapsule(d.capsule)
		w.WriteString(".Type()")
	}
}

// writeCapsule writes the call that makes a capsule type of c's name and Go
// type. It is a new capsule type, declaring no operations, and so another
// type than c's, as every capsule type is (TY-040): no syntax names the
// CapsuleType a program holds, which it writes in its place.
func (w *goWriter) writeCapsule(c *capsuleData) {
	w.WriteString("tenon.NewCapsule[")
	w.WriteString(c.goType)
	w.WriteString("](")
	w.writeQuoted(c.name)
	w.WriteString(", tenon.CapsuleOps[")
	w.WriteString(c.goType)
	w.WriteString("]{})")
}

// goTypeName returns the name Go syntax gives E, as in main.point: the name
// %T gives a pointer to E, which it gives even where E is an interface type,
// without the pointer's star.
func goTypeName[E any]() string {
	return strings.TrimPrefix(fmt.Sprintf("%T", (*E)(nil)), "*")
}

// writeValue writes the Go syntax that builds v, with the marks it carries
// beyond those the container being written implies on it (DI-015). A value
// carrying a redacting mark is written as the zero Value with its redacting
// marks, withholding all else, as its display form does.
func (w *goWriter) writeValue(v Value) {
	n := v.n
	if n == nil {
		w.WriteString("tenon.Value{}")
		return
	}
	if n.marks == nil {
		w.writeUnmarked(n)
		return
	}
	if n.state != stateError {
		if rs := n.redactingMarks(); rs != nil {
			w.WriteString("tenon.WithMarks(tenon.Value{} /* redacted */")
			w.writeMarks(rs)
			w.WriteByte(')')
			return
		}
	}
	var listed []Mark
	if w.implied != nil {
		listed = w.implied.listed(n.marks)
	} else {
		listed = n.markList()
	}
	if len(listed) == 0 {
		w.writeUnmarked(n)
		return
	}
	w.WriteString("tenon.WithMarks(")
	w.writeUnmarked(n)
	w.writeMarks(listed)
	w.WriteByte(')')
}

// writeMarks writes each of ms after a comma, as %#v writes it: a mark is the
// caller's Go value, which its type's GoString method, if it has one, writes.
func (w *goWriter) writeMarks(ms []Mark) {
	for _, m := range ms {
		if w.full() {
			return
		}
		w.WriteString(", ")
		w.WriteString(fmt.Sprintf("%#v", m))
	}
}

// writeUnmarked writes the Go syntax that builds n without its marks.
func (w *goWriter) writeUnmarked(n *node) {
	switch n.state {
	case stateError:
		w.WriteString("tenon.ErrorVal(")
		for i, d := range n.diagnostics() {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeDiagnostic(d)
		}
		w.WriteByte(')')
		return
	case statePending:
		if p, ok := n.held(); ok {
			// The Tuple or Object call that makes it, its members in place,
			// without the deep marks it implies on them.
			defer w.within(n)()
			if p.names == nil {
				w.WriteString("tenon.Tuple(")
			} else {
				w.WriteString("tenon.Object(map[string]tenon.Value{")
			}
			for i, val := range p.vals {
				if w.full() {
					return
				}
				if i > 0 {
					w.WriteString(", ")
				}
				if p.names != nil {
					w.writeQuoted(p.names[i])
					w.WriteByte(':')
				}
				w.writeValue(val)
			}
			if p.names == nil {
				w.WriteByte(')')
			} else {
				w.WriteString("})")
			}
			return
		}
		var ns []string
		switch n.null {
		case nullNo:
			ns = append(ns, "tenon.NotNull()")
		case nullOnly:
			ns = append(ns, "tenon.NullOnly()")
		}
		lo, hi := n.pendingLengths()
		if lo > 0 {
			ns = append(ns, "tenon.LengthMin("+strconv.FormatInt(lo, 10)+")")
		}
		if hi.set {
			ns = append(ns, "tenon.LengthMax("+strconv.FormatInt(hi.n, 10)+")")
		}
		if ns != nil {
			w.WriteString("tenon.Narrow(")
		}
		w.WriteString("tenon.Pending(")
		w.writeConstraint(n.constraint())
		w.WriteByte(')')
		if ns != nil {
			w.WriteString(", ")
			w.WriteString(strings.Join(ns, ", "))
			w.WriteByte(')')
		}
		return
	case stateNull:
		w.WriteString("tenon.Null(")
		w.writeType(n.typ)
		w.WriteByte(')')
		return
	case stateUnknown:
		r := n.data.(*rangeData)
		if !r.hasFacts() {
			w.WriteString("tenon.Unknown(")
			w.writeType(n.typ)
			w.WriteByte(')')
			return
		}
		w.WriteString("tenon.Narrow(tenon.Unknown(")
		w.writeType(n.typ)
		w.WriteByte(')')
		for _, nw := range r.narrowings() {
			if w.full() {
				return
			}
			w.WriteString(", ")
			w.writeNarrowing(nw)
		}
		w.WriteByte(')')
		return
	}
	switch n.typ.t.kind {
	case KindBool:
		w.WriteString("tenon.Bool(")
		w.WriteString(strconv.FormatBool(n.data.(bool)))
		w.WriteByte(')')
	case KindNumber:
		w.writeNumber(n.data.(decimal.Dec))
	case KindString:
		w.WriteString("tenon.String(")
		w.writeQuoted(n.data.(string))
		w.WriteByte(')')
	case KindCapsule:
		w.writeCapsule(n.typ.t.capsule)
		w.WriteString(".Value(")
		w.WriteString(fmt.Sprintf("%#v", n.data))
		w.WriteByte(')')
	default:
		w.writeContainer(n)
	}
}

// writeNumber writes the call that makes the number d: NumberFromInt where it
// is an integer an int64 holds, and NumberFromText otherwise.
func (w *goWriter) writeNumber(d decimal.Dec) {
	if i, ok := d.Int64(); ok {
		w.WriteString("tenon.NumberFromInt(")
		w.WriteString(strconv.FormatInt(i, 10))
		w.WriteByte(')')
		return
	}
	w.WriteString("tenon.NumberFromText(")
	w.writeQuoted(d.String())
	w.WriteByte(')')
}

// writeContainer writes the call that builds n, a known collection or
// structural value: the members of a list, set or map after the element type,
// which a tuple and an object take from their members instead.
func (w *goWriter) writeContainer(n *node) {
	defer w.within(n)()
	d := n.typ.t
	switch d.kind {
	case KindList, KindSet:
		w.WriteString("tenon.")
		w.WriteString(d.kind.String())
		w.WriteByte('(')
		w.writeType(d.elem)
		for _, e := range n.data.([]Value) {
			if w.full() {
				return
			}
			w.WriteString(", ")
			w.writeValue(e)
		}
		w.WriteByte(')')
	case KindTuple:
		w.WriteString("tenon.Tuple(")
		for i, e := range n.data.([]Value) {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeValue(e)
		}
		w.WriteByte(')')
	case KindMap:
		w.WriteString("tenon.Map(")
		w.writeType(d.elem)
		w.WriteString(", map[string]tenon.Value{")
		for i, e := range n.data.([]mapEntry) {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeQuoted(e.key)
			w.WriteByte(':')
			w.writeValue(e.val)
		}
		w.WriteString("})")
	case KindObject:
		w.WriteString("tenon.Object(map[string]tenon.Value{")
		for i, val := range n.data.([]Value) {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeQuoted(d.attrs[i].name)
			w.WriteByte(':')
			w.writeValue(val)
		}
		w.WriteString("})")
	}
}

// writeDiagnostic writes d as a composite literal, leaving out an empty path.
func (w *goWriter) writeDiagnostic(d Diagnostic) {
	w.WriteString("tenon.Diagnostic{Code:")
	w.writeQuoted(string(d.Code))
	w.WriteString(", Message:")
	w.writeQuoted(d.Message)
	if d.Path.Len() > 0 {
		w.WriteString(", Path:")
		w.writePath(d.Path)
	}
	w.WriteByte('}')
}

// writePath writes the calls that extend the empty path to p.
func (w *goWriter) writePath(p Path) {
	w.WriteString("tenon.Path{}")
	for _, s := range p.Steps() {
		if w.full() {
			return
		}
		w.writeStep(s)
	}
}

// writeStep writes the call that extends a path by s.
func (w *goWriter) writeStep(s Step) {
	switch s.kind {
	case StepAttribute:
		w.WriteString(".Attribute(")
		w.writeQuoted(s.name)
	case StepIndex:
		w.WriteString(".Index(")
		w.writeValue(s.key)
	}
	w.WriteByte(')')
}

// writeConstraint writes the call that returns c.
func (w *goWriter) writeConstraint(c Constraint) {
	d := c.c
	switch d.kind {
	case ConstraintAny:
		w.WriteString("tenon.Any()")
	case ConstraintExactly:
		w.WriteString("tenon.Exactly(")
		w.writeType(d.typ)
		w.WriteByte(')')
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		w.WriteString("tenon.")
		w.WriteString(d.kind.String())
		w.WriteByte('(')
		w.writeConstraint(d.elem)
		w.WriteByte(')')
	case ConstraintObjectWith:
		w.WriteString("tenon.ObjectWith(map[string]tenon.Field{")
		for i, f := range d.fields {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeQuoted(f.name)
			if f.Required {
				w.WriteString(":tenon.Required(")
			} else {
				w.WriteString(":tenon.Optional(")
			}
			w.writeConstraint(f.Constraint)
			w.WriteByte(')')
		}
		w.WriteString("}, ")
		w.WriteString(strconv.FormatBool(d.closed))
		w.WriteByte(')')
	case ConstraintTupleOf, ConstraintOneOf:
		w.WriteString("tenon.")
		w.WriteString(d.kind.String())
		w.WriteByte('(')
		for i, m := range d.members {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeConstraint(m)
		}
		w.WriteByte(')')
	}
}

// narrowings returns the narrowings that record r, in the order of DI-017:
// narrowing an unknown value of its type by them gives r.
func (r *rangeData) narrowings() []Narrowing {
	var ns []Narrowing
	if r.null == nullNo {
		ns = append(ns, NotNull())
	}
	if r.lo.set {
		ns = append(ns, Narrowing{kind: narrowNumberMin, num: r.lo.v, incl: r.lo.incl})
	}
	if r.hi.set {
		ns = append(ns, Narrowing{kind: narrowNumberMax, num: r.hi.v, incl: r.hi.incl})
	}
	if r.pfx != "" {
		ns = append(ns, Narrowing{kind: narrowPrefix, str: r.pfx})
	}
	if r.lenLo > 0 {
		ns = append(ns, Narrowing{kind: narrowLengthMin, n: r.lenLo})
	}
	if r.lenHi.set {
		ns = append(ns, Narrowing{kind: narrowLengthMax, n: r.lenHi.n})
	}
	if len(r.members) > 0 {
		ns = append(ns, Narrowing{kind: narrowMembers, members: r.members})
	}
	return ns
}

// writeNarrowing writes the call that returns nw. A bound is written as the
// value it was taken from, marks and all, so a redacting one withholds the
// number, as the narrowing's display does.
func (w *goWriter) writeNarrowing(nw Narrowing) {
	switch nw.kind {
	case narrowNotNull:
		w.WriteString("tenon.NotNull()")
	case narrowNull:
		w.WriteString("tenon.NullOnly()")
	case narrowNumberMin, narrowNumberMax:
		if nw.kind == narrowNumberMin {
			w.WriteString("tenon.NumberMin(")
		} else {
			w.WriteString("tenon.NumberMax(")
		}
		bound := numberValue(nw.num)
		if nw.marks != nil {
			bound = WithMarks(bound, nw.marks...)
		}
		w.writeValue(bound)
		w.WriteString(", ")
		w.WriteString(strconv.FormatBool(nw.incl))
		w.WriteByte(')')
	case narrowPrefix:
		w.writePrefix(nw.str)
	case narrowLengthMin, narrowLengthMax:
		if nw.kind == narrowLengthMin {
			w.WriteString("tenon.LengthMin(")
		} else {
			w.WriteString("tenon.LengthMax(")
		}
		w.WriteString(strconv.FormatInt(nw.n, 10))
		w.WriteByte(')')
	case narrowMembers:
		w.WriteString("tenon.Members(")
		for i, m := range nw.members {
			if w.full() {
				return
			}
			if i > 0 {
				w.WriteString(", ")
			}
			w.writeValue(m)
		}
		w.WriteByte(')')
	}
}

// writePrefix writes the StringPrefix narrowing that records the prefix p.
// StringPrefix keeps none of its text that a character following it could
// change, so a prefix ending in a character that one could is written with an
// x after it, as in tenon.StringPrefix("caf" + "x"). The x is a character
// that composes with what follows it and with nothing before it, so
// StringPrefix keeps p whole and cuts the x.
func (w *goWriter) writePrefix(p string) {
	w.WriteString("tenon.StringPrefix(")
	w.writeQuoted(p)
	if uni.StablePrefix(p) != p {
		w.WriteString(` + "x"`)
	}
	w.WriteByte(')')
}
