package bench

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
)

// Each benchmark is named Workload/size=S/lib=L, which BENCHMARKS.md is
// generated from: L is json for encoding/json with map[string]any, tenon, or
// cty for go-cty, and for crossing ctytenon's bridge, fromcty or tocty.

// sink keeps what a benchmark computes from being optimized away.
var sink any

// decodeJSON decodes a document as a program reading data it did not declare
// does: into any, keeping each number's text, as tenon does, rather than the
// nearest float64.
func decodeJSON(b *testing.B, doc []byte) any {
	d := json.NewDecoder(bytes.NewReader(doc))
	d.UseNumber()
	var x any
	if err := d.Decode(&x); err != nil {
		b.Fatal(err)
	}
	return x
}

// parseTenon reads a document with ParseJSON, into what JSON implies.
func parseTenon(b *testing.B, doc []byte) tenon.Value {
	v, err := tenon.ParseJSON(doc, tenon.Any(), tenon.Safe)
	if err != nil {
		b.Fatal(err)
	}
	return v
}

func parseCty(b *testing.B, doc []byte) cty.Value {
	ty, err := ctyjson.ImpliedType(doc)
	if err != nil {
		b.Fatal(err)
	}
	v, err := ctyjson.Unmarshal(doc, ty)
	if err != nil {
		b.Fatal(err)
	}
	return v
}

// typedTenon and typedCty give a document parsed and converted to its schema,
// the value a program holds once it has checked what it read: ParseJSON reads
// it into the schema in one call.
func typedTenon(b *testing.B, doc []byte) tenon.Value {
	v, err := tenon.ParseJSON(doc, TenonSchema(), tenon.Safe)
	if err != nil {
		b.Fatal(err)
	}
	return v
}

func typedCty(b *testing.B, doc []byte) cty.Value {
	v, err := convert.Convert(parseCty(b, doc), CtySchema())
	if err != nil {
		b.Fatal(err)
	}
	return v
}

// sized runs f at each size, as a sub-benchmark named for the size and the
// library.
func sized(b *testing.B, lib string, f func(b *testing.B, doc []byte)) {
	for _, s := range Sizes {
		doc := Services(s.Services)
		b.Run("size="+s.Label+"/lib="+lib, func(b *testing.B) {
			b.ReportAllocs()
			f(b, doc)
		})
	}
}

// BenchmarkParse reads a JSON document into a value.
func BenchmarkParse(b *testing.B) {
	sized(b, "json", func(b *testing.B, doc []byte) {
		b.SetBytes(int64(len(doc)))
		for b.Loop() {
			sink = decodeJSON(b, doc)
		}
	})
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		b.SetBytes(int64(len(doc)))
		for b.Loop() {
			sink = parseTenon(b, doc)
		}
	})
	sized(b, "cty", func(b *testing.B, doc []byte) {
		b.SetBytes(int64(len(doc)))
		for b.Loop() {
			sink = parseCty(b, doc)
		}
	})
}

// BenchmarkConvert checks a parsed document against its schema and converts
// it there: tuples to lists and sets, objects to maps.
func BenchmarkConvert(b *testing.B) {
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		v, schema := parseTenon(b, doc), TenonSchema()
		if c := tenon.Convert(v, schema, tenon.Safe); c.IsError() {
			b.Fatal(c)
		}
		for b.Loop() {
			sink = tenon.Convert(v, schema, tenon.Safe)
		}
	})
	sized(b, "cty", func(b *testing.B, doc []byte) {
		v, schema := parseCty(b, doc), CtySchema()
		for b.Loop() {
			var err error
			if sink, err = convert.Convert(v, schema); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkRoundTrip encodes a typed document for another process and decodes
// it back, reporting the encoding's length as B/doc.
func BenchmarkRoundTrip(b *testing.B) {
	sized(b, "json", func(b *testing.B, doc []byte) {
		x := decodeJSON(b, doc)
		enc, err := json.Marshal(x)
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			enc, err = json.Marshal(x)
			if err != nil {
				b.Fatal(err)
			}
			sink = decodeJSON(b, enc)
		}
		b.ReportMetric(float64(len(enc)), "B/doc")
	})
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		v := typedTenon(b, doc)
		enc, err := tenon.Serialize(v)
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if enc, err = tenon.Serialize(v); err != nil {
				b.Fatal(err)
			}
			if sink, err = tenon.Deserialize(enc, tenon.Decoders{}); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(len(enc)), "B/doc")
	})
	sized(b, "cty", func(b *testing.B, doc []byte) {
		v := typedCty(b, doc)
		ty := v.Type()
		enc, err := msgpack.Marshal(v, ty)
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			if enc, err = msgpack.Marshal(v, ty); err != nil {
				b.Fatal(err)
			}
			if sink, err = msgpack.Unmarshal(enc, ty); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(len(enc)), "B/doc")
	})
}

// BenchmarkEqual compares two copies of a document, each built on its own, so
// that no comparison can stop at a part the two share.
func BenchmarkEqual(b *testing.B) {
	sized(b, "json", func(b *testing.B, doc []byte) {
		x, y := decodeJSON(b, doc), decodeJSON(b, doc)
		for b.Loop() {
			if !reflect.DeepEqual(x, y) {
				b.Fatal("the copies differ")
			}
		}
	})
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		x, y := typedTenon(b, doc), typedTenon(b, doc)
		for b.Loop() {
			if eq := tenon.Equals(x, y); !eq.IsKnown() || !eq.AsBool() {
				b.Fatal("the copies differ")
			}
		}
	})
	sized(b, "cty", func(b *testing.B, doc []byte) {
		x, y := typedCty(b, doc), typedCty(b, doc)
		for b.Loop() {
			if !x.Equals(y).True() {
				b.Fatal("the copies differ")
			}
		}
	})
}

// BenchmarkLookup reads one environment variable of the middle service.
func BenchmarkLookup(b *testing.B) {
	sized(b, "json", func(b *testing.B, doc []byte) {
		x := decodeJSON(b, doc)
		i := len(x.(map[string]any)["services"].([]any)) / 2
		for b.Loop() {
			sink = x.(map[string]any)["services"].([]any)[i].(map[string]any)["env"].(map[string]any)["REGION"]
		}
	})
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		v := typedTenon(b, doc)
		i := v.Attribute("services").Len() / 2
		for b.Loop() {
			sink, _ = v.Attribute("services").Index(i).Attribute("env").LookupMapElement("REGION")
		}
	})
	sized(b, "cty", func(b *testing.B, doc []byte) {
		v := typedCty(b, doc)
		i := cty.NumberIntVal(int64(v.GetAttr("services").LengthInt() / 2))
		region := cty.StringVal("REGION")
		for b.Loop() {
			sink = v.GetAttr("services").Index(i).GetAttr("env").Index(region)
		}
	})
}

// BenchmarkDiff reports what changed between two versions of a document that
// differ in one service's replica count, as a plan engine shows a change. It
// is tenon's alone: go-cty leaves diffing to its callers.
func BenchmarkDiff(b *testing.B) {
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		before := typedTenon(b, doc)
		changed := bytes.Replace(doc, []byte(`"name":"svc-0","replicas":1`), []byte(`"name":"svc-0","replicas":`+strconv.Itoa(2)), 1)
		after := typedTenon(b, changed)
		for b.Loop() {
			if d := tenon.Diff(before, after); len(d) != 1 {
				b.Fatalf("the diff has %d changes, want 1", len(d))
			}
		}
	})
}

// BenchmarkCross carries the converted document across ctytenon's bridge,
// from go-cty to tenon and from tenon to go-cty, as a program moving from
// one to the other a piece at a time does.
func BenchmarkCross(b *testing.B) {
	var bridge ctytenon.Bridge
	sized(b, "fromcty", func(b *testing.B, doc []byte) {
		v := typedCty(b, doc)
		for b.Loop() {
			var err error
			if sink, err = bridge.FromCty(v); err != nil {
				b.Fatal(err)
			}
		}
	})
	sized(b, "tocty", func(b *testing.B, doc []byte) {
		v := typedTenon(b, doc)
		for b.Loop() {
			var err error
			if sink, err = bridge.ToCty(v); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCall calls a two-number function once per service of the
// document, through each library's own calling convention: tenon's Call
// converts each argument to its parameter's constraint under the policy
// inside the call, where go-cty checks conformance only and every caller
// converts beforehand, so each measures the whole of what its callers do
// per call.
func BenchmarkCall(b *testing.B) {
	sized(b, "tenon", func(b *testing.B, doc []byte) {
		num := tenon.Exactly(tenon.NumberType())
		scale := tenon.NewFunction(tenon.FunctionSpec{
			Name:   "Scale",
			Params: []tenon.Param{{Name: "count", Constraint: num}, {Name: "by", Constraint: num}},
			Result: num,
			Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
				return tenon.Mul(args[0], args[1]), nil
			},
		})
		two := tenon.NumberFromInt(2)
		var args [][]tenon.Value
		for _, svc := range typedTenon(b, doc).Attribute("services").Elements() {
			args = append(args, []tenon.Value{svc.Attribute("replicas"), two})
		}
		for b.Loop() {
			for _, a := range args {
				sink = tenon.Call(scale, a, tenon.Safe)
			}
		}
	})
	sized(b, "cty", func(b *testing.B, doc []byte) {
		scale := function.New(&function.Spec{
			Params: []function.Parameter{
				{Name: "count", Type: cty.Number},
				{Name: "by", Type: cty.Number},
			},
			Type: function.StaticReturnType(cty.Number),
			Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
				return args[0].Multiply(args[1]), nil
			},
		})
		two := cty.NumberIntVal(2)
		var args [][]cty.Value
		it := typedCty(b, doc).GetAttr("services").ElementIterator()
		for it.Next() {
			_, svc := it.Element()
			args = append(args, []cty.Value{svc.GetAttr("replicas"), two})
		}
		for b.Loop() {
			for _, a := range args {
				r, err := scale.Call(a)
				if err != nil {
					b.Fatal(err)
				}
				sink = r
			}
		}
	})
}
