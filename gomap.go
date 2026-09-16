package tenon

import (
	"math/big"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"

	"tenon/internal/uni"
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
)

// goMapping is how a Go type maps to tenon: the type its values encode as,
// where it maps to one, and the constraint that decoding converts to.
type goMapping struct {
	rt   reflect.Type
	kind goKind
	// typ is the type the Go type's values encode as, and the zero Type where
	// the Go type maps to no type, since what its values encode as depends on
	// what they hold.
	typ        Type
	constraint Constraint
	elem       *goMapping // the element of a slice, array, map or pointer
	fields     []goField  // the mapped fields of a struct, in field order
}

// goField is a struct field that maps to an attribute.
type goField struct {
	index    int
	name     string // the attribute name, normalized
	optional bool
	m        *goMapping
}

var (
	valueGoType    = reflect.TypeFor[Value]()
	bigIntGoType   = reflect.TypeFor[big.Int]()
	bigFloatGoType = reflect.TypeFor[big.Float]()
	bigRatGoType   = reflect.TypeFor[big.Rat]()
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
	number := Exactly(Type{numberType})
	switch rt {
	case valueGoType:
		m.kind, m.constraint = goValue, Any()
	case bigIntGoType:
		m.kind, m.typ, m.constraint = goBigInt, Type{numberType}, number
	case bigFloatGoType:
		m.kind, m.typ, m.constraint = goBigFloat, Type{numberType}, number
	case bigRatGoType:
		m.kind, m.typ, m.constraint = goBigRat, Type{numberType}, number
	default:
		switch rt.Kind() {
		case reflect.Bool:
			m.kind, m.typ = goBool, Type{boolType}
			m.constraint = Exactly(m.typ)
		case reflect.String:
			m.kind, m.typ = goString, Type{stringType}
			m.constraint = Exactly(m.typ)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			m.kind, m.typ, m.constraint = goInt, Type{numberType}, number
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			m.kind, m.typ, m.constraint = goUint, Type{numberType}, number
		case reflect.Float32, reflect.Float64:
			m.kind, m.typ, m.constraint = goFloat, Type{numberType}, number
		case reflect.Slice, reflect.Array:
			m.kind = goSlice
			if rt.Kind() == reflect.Array {
				m.kind = goArray
			}
			m.elem = buildMapping(rt.Elem(), building)
			m.constraint = Any()
			if m.elem.typ.t != nil {
				m.typ, m.constraint = List(m.elem.typ), ListOf(m.elem.constraint)
			}
		case reflect.Map:
			if rt.Key().Kind() != reflect.String {
				usagePanic("the Go type %s has keys of kind %s, and only maps with string keys map to tenon", rt, rt.Key().Kind())
			}
			m.kind = goMap
			m.elem = buildMapping(rt.Elem(), building)
			m.constraint = Any()
			if m.elem.typ.t != nil {
				m.typ, m.constraint = Map(m.elem.typ), MapOf(m.elem.constraint)
			}
		case reflect.Struct:
			m.kind = goStruct
			structMapping(m, building)
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
	actual, _ := goMappings.LoadOrStore(rt, m)
	return actual.(*goMapping)
}

// structMapping fills in the mapping of a struct type from its exported fields
// and their tags.
func structMapping(m *goMapping, building map[reflect.Type]bool) {
	rt := m.rt
	names := map[string]string{} // normalized attribute name to the field mapped to it
	fields := map[string]Field{}
	attrs := map[string]Type{}
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
		normalized := uni.NFC(name)
		if other, ok := names[normalized]; ok {
			usagePanic("fields %s and %s of %s both map to the attribute %q", other, sf.Name, rt, normalized)
		}
		names[normalized] = sf.Name
		fm := buildMapping(sf.Type, building)
		m.fields = append(m.fields, goField{index: i, name: normalized, optional: optional, m: fm})
		fields[normalized] = Field{Constraint: fm.constraint, Required: !optional}
		if fm.typ.t == nil {
			typed = false
		} else {
			attrs[normalized] = fm.typ
		}
	}
	m.constraint = ObjectWith(fields, true)
	if typed {
		m.typ = Object(attrs)
	}
}
