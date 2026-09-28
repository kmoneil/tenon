package gotenon

import (
	goencoding "encoding"
	"encoding/json"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
)

// goKind is how a Go type maps to tenon.
type goKind uint8

const (
	goBool goKind = iota + 1
	goString
	goInt
	goUint
	goFloat
	goBigInt
	goBigFloat
	goBigRat
	goSlice
	goArray
	goMap
	goStruct
	goPointer
	goValue
	goJSONNumber // encoding/json.Number, a number written down [GO-034]
	goInterface  // a type whose values encode as what they hold [GO-015]
	goCustom     // a type that marshals and unmarshals itself, which needs no other mapping
	goText       // a type that marshals itself to text, or unmarshals itself from it [GO-044]
)

// ValueMarshaler is implemented by a Go type that encodes itself: Encode gives
// the value MarshalValue returns in place of the Go value's own mapping. The
// type may implement it itself or through its pointer.
type ValueMarshaler interface {
	// MarshalValue returns the value the Go value encodes as, or an error.
	// An error that is a *tenon.Error contributes its diagnostics, and so
	// does an error value returned in place of a value; any other error
	// contributes its text. Encode's error keeps it as a cause.
	MarshalValue() (tenon.Value, error)
}

// ValueUnmarshaler is implemented by a pointer to a Go type that decodes
// itself: Decode gives UnmarshalValue the value, unconverted and as it is,
// null, unknown and marked values included, in place of the Go type's own
// mapping.
type ValueUnmarshaler interface {
	// UnmarshalValue sets the Go value from v, or returns an error. It is
	// given the policy the decoding was given, for converting v as the rest
	// of the decoding does. An error that is a *tenon.Error contributes its
	// diagnostics, and any other error its text. Decode's error keeps it as
	// a cause.
	UnmarshalValue(v tenon.Value, p tenon.Policy) error
}

// goMapping is how a Go type maps to tenon: the type its values encode as,
// where it maps to one, and the constraint that decoding converts to.
type goMapping struct {
	rt   reflect.Type
	kind goKind
	// typ is the type the Go type's values encode as, and the zero Type where
	// the Go type maps to no type, since what its values encode as depends on
	// what they hold.
	typ        tenon.Type
	constraint tenon.Constraint
	elem       *goMapping // the element of a slice, array, map or pointer
	// fields is the mapped fields of a struct, in attribute-name order, so
	// that walking them reads, and fails, in member order (GO-003) and an
	// entry walk in name order finds each field without a lookup.
	fields []goField
	// marshal and unmarshal say the Go type encodes or decodes itself, in
	// place of its mapping in that direction.
	marshal, unmarshal bool
	// marshalText and unmarshalText say the Go type implements
	// goencoding.TextMarshaler, itself or through its pointer, or its pointer
	// goencoding.TextUnmarshaler: in that direction, unless it encodes or
	// decodes itself, it is a string [GO-044].
	marshalText, unmarshalText bool
	// iface is the interface type that decoding into the Go type meets
	// first, the Go type itself or one it holds, other than within a type
	// that decodes by an unmarshaler, and nil where it meets none. Decode
	// refuses such a type before it looks at the value (GO-011).
	iface reflect.Type
}

// typed reports whether the Go type maps to a type.
func (m *goMapping) typed() bool { return m.typ != (tenon.Type{}) }

// decodesAny reports whether a value of any type decodes into the Go type:
// one that maps to no type, one that decodes by an unmarshaler, itself or
// through a pointer, and a slice, array or map of such, which decodes from
// Any member by member (GO-012).
func (m *goMapping) decodesAny() bool {
	for m.kind == goPointer {
		m = m.elem
	}
	switch {
	case !m.typed(), m.unmarshal:
		return true
	case m.kind == goSlice, m.kind == goArray, m.kind == goMap:
		return m.elem.decodesAny()
	}
	return false
}

// goField is a struct field that maps to an attribute.
type goField struct {
	index    int
	name     string // the attribute name, normalized
	optional bool
	m        *goMapping
}

var (
	valueGoType    = reflect.TypeFor[tenon.Value]()
	bigIntGoType   = reflect.TypeFor[big.Int]()
	bigFloatGoType = reflect.TypeFor[big.Float]()
	bigRatGoType   = reflect.TypeFor[big.Rat]()
	// A json.Number is a number written down, not text [GO-034].
	jsonNumberGoType = reflect.TypeFor[json.Number]()

	marshalerGoType   = reflect.TypeFor[ValueMarshaler]()
	unmarshalerGoType = reflect.TypeFor[ValueUnmarshaler]()

	textMarshalerGoType   = reflect.TypeFor[goencoding.TextMarshaler]()
	textUnmarshalerGoType = reflect.TypeFor[goencoding.TextUnmarshaler]()

	// mappedItself holds the Go types that the mapping names, which map as it
	// says whatever methods they have [GO-010].
	mappedItself = map[reflect.Type]bool{
		valueGoType: true, bigIntGoType: true, bigFloatGoType: true, bigRatGoType: true, jsonNumberGoType: true,
	}
)

// direction is which way a mapping is used: to encode Go values or to decode
// into them.
type direction uint8

const (
	encoding direction = iota
	decoding
	// encodingByKind maps a type that encodes itself by its kind all the
	// same, for encoding, which the null of a nil pointer to it needs
	// (nullType); what it holds is mapped for encoding.
	encodingByKind
)

// mappingKey is what a mapping is cached by: a Go type and a direction.
type mappingKey struct {
	rt  reflect.Type
	dir direction
}

// goMappings caches the mapping of every Go type met so far, in each
// direction it was met in.
var goMappings sync.Map // mappingKey to *goMapping

// mappingOf returns the mapping of the Go type rt for dir, building it on
// first use. It panics where rt does not map to tenon in that direction
// (GO-011) or holds a struct whose tags are malformed (GO-021).
//
// Each direction is mapped on its own (GO-040): a type that encodes itself is
// not mapped by its kind to be encoded, nor one that decodes itself to be
// decoded, so a map[int]string, or a tree holding itself, that marshals
// itself encodes, and only decoding into it, where it maps by its kind, is
// refused.
func mappingOf(rt reflect.Type, dir direction) *goMapping {
	if m, ok := goMappings.Load(mappingKey{rt, dir}); ok {
		return m.(*goMapping)
	}
	return buildMapping(rt, dir, map[reflect.Type]bool{})
}

// byKind returns the mapping of rt, a type that encodes itself, by its kind
// for encoding, and false where its kind does not map to tenon, as a map
// with int keys or a type holding itself does not: nothing then decodes into
// it, and its null reads back as nothing in particular.
func byKind(rt reflect.Type) (m *goMapping, ok bool) {
	ok = unlessUsageError(func() { m = mappingOf(rt, encodingByKind) })
	return m, ok
}

// buildMapping builds the mapping of rt for dir, with building holding the
// types whose mappings are being built around it, which a finite type cannot
// meet again. A type is a leaf where its methods map it in dir; otherwise the
// table maps it by what it is (GO-010), and settle then does what its methods
// ask of dir.
func buildMapping(rt reflect.Type, dir direction, building map[reflect.Type]bool) *goMapping {
	key := mappingKey{rt, dir}
	if m, ok := goMappings.Load(key); ok {
		return m.(*goMapping)
	}
	if building[rt] {
		usagePanic("the Go type %s holds itself, and a type that maps to tenon must be finite", rt)
	}
	building[rt] = true
	defer delete(building, rt)

	m := methods(rt)
	if !m.leaf(dir) {
		if dir == encodingByKind {
			dir = encoding
		}
		m.byTable(dir, building)
		m.settle(dir)
	}
	actual, _ := goMappings.LoadOrStore(key, m)
	return actual.(*goMapping)
}

// methods returns a mapping of rt that says which of the methods tenon maps
// by rt implements, itself or through its pointer, and nothing else yet.
func methods(rt reflect.Type) *goMapping {
	m := &goMapping{rt: rt}
	if k := rt.Kind(); k != reflect.Interface && k != reflect.Pointer {
		m.marshal = rt.Implements(marshalerGoType) || reflect.PointerTo(rt).Implements(marshalerGoType)
		m.unmarshal = reflect.PointerTo(rt).Implements(unmarshalerGoType)
		m.marshalText = rt.Implements(textMarshalerGoType) || reflect.PointerTo(rt).Implements(textMarshalerGoType)
		m.unmarshalText = reflect.PointerTo(rt).Implements(textUnmarshalerGoType)
	}
	return m
}

// leaf reports whether m's Go type is mapped by its methods in dir, whatever
// its kind, and maps it so where it is. A type that encodes itself needs no
// mapping of its kind to be encoded, and one that decodes itself none to be
// decoded [GO-040]. A type that marshals itself to text, and not by
// MarshalValue, is a string in that direction [GO-044]: ValueMarshaler and
// ValueUnmarshaler come first, being tenon's own, and so do the types the
// table maps itself, the big numbers among them, whose text methods would
// make text of numbers.
func (m *goMapping) leaf(dir direction) bool {
	switch {
	case m.marshal && m.unmarshal, dir == encoding && m.marshal, dir == decoding && m.unmarshal:
		m.kind, m.constraint = goCustom, tenon.Any()
	case mappedItself[m.rt]:
		return false
	case dir != decoding && !m.marshal && m.marshalText, dir == decoding && !m.unmarshal && m.unmarshalText:
		m.kind, m.typ, m.constraint = goText, tenon.StringType(), tenon.Exactly(tenon.StringType())
	default:
		return false
	}
	return true
}

// byTable maps m's Go type for dir as GO-010's table says: by the type
// itself where the table names it, and otherwise by its kind, building the
// mappings of what it holds.
func (m *goMapping) byTable(dir direction, building map[reflect.Type]bool) {
	rt := m.rt
	number := tenon.Exactly(tenon.NumberType())
	switch rt {
	case valueGoType:
		m.kind, m.constraint = goValue, tenon.Any()
	case bigIntGoType:
		m.kind, m.typ, m.constraint = goBigInt, tenon.NumberType(), number
	case bigFloatGoType:
		m.kind, m.typ, m.constraint = goBigFloat, tenon.NumberType(), number
	case bigRatGoType:
		m.kind, m.typ, m.constraint = goBigRat, tenon.NumberType(), number
	case jsonNumberGoType:
		m.kind, m.typ, m.constraint = goJSONNumber, tenon.NumberType(), number
	default:
		switch rt.Kind() {
		case reflect.Bool:
			m.kind, m.typ = goBool, tenon.BoolType()
			m.constraint = tenon.Exactly(m.typ)
		case reflect.String:
			m.kind, m.typ = goString, tenon.StringType()
			m.constraint = tenon.Exactly(m.typ)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			m.kind, m.typ, m.constraint = goInt, tenon.NumberType(), number
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			m.kind, m.typ, m.constraint = goUint, tenon.NumberType(), number
		case reflect.Float32, reflect.Float64:
			m.kind, m.typ, m.constraint = goFloat, tenon.NumberType(), number
		case reflect.Slice, reflect.Array:
			m.kind = goSlice
			if rt.Kind() == reflect.Array {
				m.kind = goArray
			}
			m.elem = buildMapping(rt.Elem(), dir, building)
			m.constraint = tenon.Any()
			if m.elem.typed() {
				m.typ = tenon.ListType(m.elem.typ)
				// Members that take any value decode by their own
				// conversions, as members of no type do (GO-012).
				if !m.elem.decodesAny() {
					m.constraint = tenon.ListOf(m.elem.constraint)
				}
			}
		case reflect.Map:
			if rt.Key().Kind() != reflect.String {
				usagePanic("the Go type %s has keys of kind %s, and only maps with string keys map to tenon", rt, rt.Key().Kind())
			}
			m.kind = goMap
			m.elem = buildMapping(rt.Elem(), dir, building)
			m.constraint = tenon.Any()
			if m.elem.typed() {
				m.typ = tenon.MapType(m.elem.typ)
				if !m.elem.decodesAny() {
					m.constraint = tenon.MapOf(m.elem.constraint)
				}
			}
		case reflect.Struct:
			m.kind = goStruct
			structMapping(m, dir, building)
		case reflect.Interface:
			// A value of an interface type encodes as what it holds, whose
			// type is not known until it is in hand, so the interface maps
			// to no type. Decoding into one is a usage error, which Decode
			// reports from iface before it looks at a value.
			m.kind, m.constraint = goInterface, tenon.Any()
		case reflect.Pointer:
			if rt.Elem() == valueGoType {
				usagePanic("the Go type %s is a pointer to tenon.Value, which does not map to tenon; use tenon.Value itself", rt)
			}
			m.kind = goPointer
			m.elem = buildMapping(rt.Elem(), dir, building)
			m.typ, m.constraint = m.elem.typ, m.elem.constraint
		default:
			usagePanic("the Go type %s is of kind %s, which does not map to tenon", rt, rt.Kind())
		}
	}
}

// settle finishes m for dir once the table has mapped it. For encoding, a
// type that encodes itself, mapped by its kind all the same for the null of
// a nil pointer to it (encodingByKind), maps to no type [GO-010, GO-040]:
// what its values encode as is what the method gives. For decoding, the type
// maps as the table says, whatever it does to encode itself, and Decode
// refuses the interface types it meets (iface).
func (m *goMapping) settle(dir direction) {
	switch {
	case dir == decoding:
		m.iface = decodedInterface(m)
	case m.marshal:
		m.typ = tenon.Type{}
	}
}

// decodedInterface returns the interface type that decoding into m's Go type
// meets first: the type itself where it is one, and otherwise the first that
// its element or its fields, in name order, meet. The mappings m holds are
// built already, and a type that decodes by an unmarshaler meets none.
func decodedInterface(m *goMapping) reflect.Type {
	switch {
	case m.kind == goInterface:
		return m.rt
	case m.elem != nil:
		return m.elem.iface
	}
	for _, f := range m.fields {
		if f.m.iface != nil {
			return f.m.iface
		}
	}
	return nil
}

// structMapping fills in the mapping of a struct type from its exported fields
// and their tags.
func structMapping(m *goMapping, dir direction, building map[reflect.Type]bool) {
	rt := m.rt
	names := map[string]string{} // normalized attribute name to the field mapped to it
	fields := map[string]tenon.Field{}
	attrs := map[string]tenon.Type{}
	typed := true
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag, tagged := sf.Tag.Lookup("tenon")
		parts := strings.Split(tag, ",")
		name, options := parts[0], parts[1:]
		if name == "-" {
			if len(options) > 0 {
				usagePanic("field %s of %s has the tag %q, which skips it and so takes no options", sf.Name, rt, tag)
			}
			continue
		}
		if sf.Anonymous && (!tagged || name == "") {
			usagePanic("embedded field %s of %s must be named by its tenon tag", sf.Name, rt)
		}
		if name == "" {
			name = sf.Name
		}
		optional := false
		for _, opt := range options {
			if opt != "optional" {
				usagePanic("field %s of %s has the tag %q, whose option %q is not optional", sf.Name, rt, tag, opt)
			}
			optional = true
		}
		if !utf8.ValidString(name) {
			usagePanic("field %s of %s has a tag naming an attribute that is not valid UTF-8", sf.Name, rt)
		}
		normalized := tenon.String(name).AsString()
		if other, ok := names[normalized]; ok {
			usagePanic("fields %s and %s of %s both map to the attribute %q", other, sf.Name, rt, normalized)
		}
		names[normalized] = sf.Name
		fm := buildMapping(sf.Type, dir, building)
		m.fields = append(m.fields, goField{index: i, name: normalized, optional: optional, m: fm})
		fields[normalized] = tenon.Field{Constraint: fm.constraint, Required: !optional}
		if !fm.typed() {
			typed = false
		} else {
			attrs[normalized] = fm.typ
		}
	}
	// A struct whose state is all in unexported fields, as time.Time's and
	// netip.Addr's is, would encode as an empty object and decode to its
	// zero value, losing what it holds without a word: it must marshal
	// itself instead [GO-011]. A struct of no fields holds nothing to lose.
	if len(m.fields) == 0 && hasUnexported(rt) {
		usagePanic("the Go type %s holds its state in unexported fields, which do not map to tenon; implement ValueMarshaler and ValueUnmarshaler, or encoding.TextMarshaler and encoding.TextUnmarshaler", rt)
	}
	slices.SortFunc(m.fields, func(a, b goField) int { return strings.Compare(a.name, b.name) })
	m.constraint = tenon.ObjectWith(fields, true)
	if typed {
		m.typ = tenon.ObjectType(attrs)
	}
}

// hasUnexported reports whether the struct type rt has an unexported field.
func hasUnexported(rt reflect.Type) bool {
	for i := range rt.NumField() {
		if !rt.Field(i).IsExported() {
			return true
		}
	}
	return false
}
