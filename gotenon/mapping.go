package gotenon

import (
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
)

// ValueMarshaler is implemented by a Go type that encodes itself: Encode gives
// the value MarshalValue returns in place of the Go value's own mapping. The
// type may implement it itself or through its pointer.
type ValueMarshaler interface {
	// MarshalValue returns the value the Go value encodes as, or an error.
	// An error that is a *DiagnosticError contributes its diagnostics.
	MarshalValue() (tenon.Value, error)
}

// ValueUnmarshaler is implemented by a pointer to a Go type that decodes
// itself: Decode gives UnmarshalValue the value, unconverted and as it is,
// null, unknown and marked values included, in place of the Go type's own
// mapping.
type ValueUnmarshaler interface {
	// UnmarshalValue sets the Go value from v, or returns an error. An error
	// that is a *DiagnosticError contributes its diagnostics.
	UnmarshalValue(v tenon.Value) error
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
}

// typed reports whether the Go type maps to a type.
func (m *goMapping) typed() bool { return m.typ != (tenon.Type{}) }

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
)

// goMappings caches the mapping of every Go type met so far.
var goMappings sync.Map // reflect.Type to *goMapping

// mappingOf returns the mapping of the Go type rt, building it on first use. It
// panics where rt does not map to tenon (GO-011) or holds a struct whose tags
// are malformed (GO-021).
func mappingOf(rt reflect.Type) *goMapping {
	if m, ok := goMappings.Load(rt); ok {
		return m.(*goMapping)
	}
	return buildMapping(rt, map[reflect.Type]bool{})
}

// buildMapping builds the mapping of rt, with building holding the types whose
// mappings are being built around it, which a finite type cannot meet again.
func buildMapping(rt reflect.Type, building map[reflect.Type]bool) *goMapping {
	if m, ok := goMappings.Load(rt); ok {
		return m.(*goMapping)
	}
	if building[rt] {
		usagePanic("the Go type %s holds itself, and a type that maps to tenon must be finite", rt)
	}
	building[rt] = true
	defer delete(building, rt)

	m := &goMapping{rt: rt}
	if k := rt.Kind(); k != reflect.Interface && k != reflect.Pointer {
		m.marshal = rt.Implements(marshalerGoType) || reflect.PointerTo(rt).Implements(marshalerGoType)
		m.unmarshal = reflect.PointerTo(rt).Implements(unmarshalerGoType)
	}
	if m.marshal && m.unmarshal {
		m.kind, m.constraint = goCustom, tenon.Any()
		actual, _ := goMappings.LoadOrStore(rt, m)
		return actual.(*goMapping)
	}
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
			m.elem = buildMapping(rt.Elem(), building)
			m.constraint = tenon.Any()
			if m.elem.typed() {
				m.typ, m.constraint = tenon.List(m.elem.typ), tenon.ListOf(m.elem.constraint)
			}
		case reflect.Map:
			if rt.Key().Kind() != reflect.String {
				usagePanic("the Go type %s has keys of kind %s, and only maps with string keys map to tenon", rt, rt.Key().Kind())
			}
			m.kind = goMap
			m.elem = buildMapping(rt.Elem(), building)
			m.constraint = tenon.Any()
			if m.elem.typed() {
				m.typ, m.constraint = tenon.Map(m.elem.typ), tenon.MapOf(m.elem.constraint)
			}
		case reflect.Struct:
			m.kind = goStruct
			structMapping(m, building)
		case reflect.Interface:
			// A value of an interface type encodes as what it holds, whose
			// type is not known until it is in hand, so the interface maps
			// to no type. Decoding into one is a usage error, which the
			// decoder reports where it meets it.
			m.kind, m.constraint = goInterface, tenon.Any()
		case reflect.Pointer:
			if rt.Elem() == valueGoType {
				usagePanic("the Go type %s is a pointer to tenon.Value, which does not map to tenon; use tenon.Value itself", rt)
			}
			m.kind = goPointer
			m.elem = buildMapping(rt.Elem(), building)
			m.typ, m.constraint = m.elem.typ, m.elem.constraint
		default:
			usagePanic("the Go type %s is of kind %s, which does not map to tenon", rt, rt.Kind())
		}
	}
	// A type that encodes or decodes itself maps to no type, or to Any, in
	// that direction, and by its own kind in the other.
	if m.marshal {
		m.typ = tenon.Type{}
	}
	if m.unmarshal {
		m.constraint = tenon.Any()
	}
	actual, _ := goMappings.LoadOrStore(rt, m)
	return actual.(*goMapping)
}

// structMapping fills in the mapping of a struct type from its exported fields
// and their tags.
func structMapping(m *goMapping, building map[reflect.Type]bool) {
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
		fm := buildMapping(sf.Type, building)
		m.fields = append(m.fields, goField{index: i, name: normalized, optional: optional, m: fm})
		fields[normalized] = tenon.Field{Constraint: fm.constraint, Required: !optional}
		if !fm.typed() {
			typed = false
		} else {
			attrs[normalized] = fm.typ
		}
	}
	slices.SortFunc(m.fields, func(a, b goField) int { return strings.Compare(a.name, b.name) })
	m.constraint = tenon.ObjectWith(fields, true)
	if typed {
		m.typ = tenon.Object(attrs)
	}
}
