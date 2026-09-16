package vectors_test

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"os"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// mutation changes a vector's value into another value that diffs.json diffs
// it against, where the change applies to the value.
type mutation struct {
	name   string
	mutate func(v tenon.Value) (tenon.Value, bool)
}

// ownMarks returns w carrying the marks that v carries itself.
func ownMarks(v, w tenon.Value) tenon.Value {
	_, marks := tenon.Unmark(v)
	return tenon.WithMarks(w, marks...)
}

// entries is what a container holds, as the constructor of its kind takes it,
// with the members of a set as the set holds them.
type entries struct {
	members []tenon.Value
	names   []string
}

// contents returns what v holds, and false where v is no known container.
func contents(v tenon.Value) (entries, bool) {
	if !v.HasContent() {
		return entries{}, false
	}
	switch v.Type().Kind() {
	case tenon.KindList, tenon.KindTuple:
		return entries{members: v.Elements()}, true
	case tenon.KindSet:
		stored, _ := tenon.UnmarkDeep(v)
		return entries{members: stored.Elements()}, true
	case tenon.KindMap:
		var e entries
		for _, k := range v.MapKeys() {
			m, _ := v.MapElement(k)
			e.members, e.names = append(e.members, m), append(e.names, k)
		}
		return e, true
	case tenon.KindObject:
		var e entries
		for _, name := range v.Type().AttributeNames() {
			e.members, e.names = append(e.members, v.Attribute(name)), append(e.names, name)
		}
		return e, true
	}
	return entries{}, false
}

// rebuilt returns a container of v's kind and type holding e, with v's marks.
func rebuilt(v tenon.Value, e entries) tenon.Value {
	var w tenon.Value
	switch v.Type().Kind() {
	case tenon.KindList:
		w = tenon.ListVal(v.Type().ElementType(), e.members...)
	case tenon.KindTuple:
		w = tenon.TupleVal(e.members...)
	case tenon.KindSet:
		w = tenon.SetVal(v.Type().ElementType(), e.members...)
	case tenon.KindMap, tenon.KindObject:
		attrs := map[string]tenon.Value{}
		for i, name := range e.names {
			attrs[name] = e.members[i]
		}
		if v.Type().Kind() == tenon.KindMap {
			w = tenon.MapVal(v.Type().ElementType(), attrs)
		} else {
			w = tenon.ObjectVal(attrs)
		}
	}
	return ownMarks(v, w)
}

// filler returns a member to add to or change in v: a null of the element type
// of a collection, and a string in a tuple or an object.
func filler(v tenon.Value) tenon.Value {
	switch v.Type().Kind() {
	case tenon.KindTuple, tenon.KindObject:
		return s("new")
	}
	return tenon.NullVal(v.Type().ElementType())
}

var mutations = []mutation{
	{"marked", func(v tenon.Value) (tenon.Value, bool) { return tenon.WithMarks(v, plain), true }},
	{"unknown", func(v tenon.Value) (tenon.Value, bool) {
		if !v.IsResolved() {
			return tenon.Value{}, false
		}
		return ownMarks(v, tenon.Unknown(v.Type())), true
	}},
	{"member added", func(v tenon.Value) (tenon.Value, bool) {
		e, ok := contents(v)
		if !ok {
			return tenon.Value{}, false
		}
		e.members = append(e.members, filler(v))
		e.names = append(e.names, "zz")
		return rebuilt(v, e), true
	}},
	{"member removed", func(v tenon.Value) (tenon.Value, bool) {
		e, ok := contents(v)
		if !ok || len(e.members) == 0 {
			return tenon.Value{}, false
		}
		e.members = e.members[:len(e.members)-1]
		if e.names != nil {
			e.names = e.names[:len(e.names)-1]
		}
		return rebuilt(v, e), true
	}},
	{"member changed", func(v tenon.Value) (tenon.Value, bool) {
		e, ok := contents(v)
		if !ok || len(e.members) == 0 {
			return tenon.Value{}, false
		}
		e.members[0] = filler(v)
		return rebuilt(v, e), true
	}},
}

type diffsFile struct {
	Format int       `json:"format"`
	About  string    `json:"about"`
	Diffs  []diffOut `json:"diffs"`
}

type diffOut struct {
	Name   string `json:"name"`
	Vector string `json:"vector"`
	Hex    string `json:"hex"`
	Diff   string `json:"diff"`
}

func TestConformance_DI037_DiffCorpus(t *testing.T) {
	conformance.Covers(t, "DI-037")
	f := diffsFile{
		Format: 1,
		About: "Each diff is of the value of the valid vector named in vectors.json against the value that hex encodes, " +
			"a copy of it with the change the name gives, and is written in the display form of a diff (DI-037), " +
			"a line for each change.",
	}
	for _, vec := range valid {
		a := vec.build(rand.New(rand.NewSource(0)))
		for _, m := range mutations {
			b, ok := m.mutate(a)
			if !ok || tenon.Identical(a, b) {
				continue
			}
			bytesB, failure, ok := tenon.Serialize(b)
			if !ok {
				t.Fatalf("%s/%s: %v does not serialize: %v", vec.name, m.name, b, failure)
			}
			if back, _, ok := tenon.Deserialize(bytesB, decoders); !ok || !tenon.Identical(back, b) {
				t.Fatalf("%s/%s: %v does not come back from its encoding", vec.name, m.name, b)
			}
			f.Diffs = append(f.Diffs, diffOut{Name: vec.name + "/" + m.name, Vector: vec.name, Hex: hex.EncodeToString(bytesB), Diff: tenon.Diff(a, b).String()})
		}
	}

	var want bytes.Buffer
	line := func(v any) string { return jsonLine(t, v) }
	want.WriteString("{\n  \"format\": " + line(f.Format) + ",\n  \"about\": " + line(f.About) + ",\n  \"diffs\": [\n")
	for i, d := range f.Diffs {
		want.WriteString("    " + line(d))
		if i < len(f.Diffs)-1 {
			want.WriteByte(',')
		}
		want.WriteByte('\n')
	}
	want.WriteString("  ]\n}\n")
	conformance.Emit(t, "diffs.json", want.Bytes())

	if os.Getenv("TENON_UPDATE_VECTORS") == "1" {
		if err := os.WriteFile("diffs.json", want.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile("diffs.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v; set TENON_UPDATE_VECTORS=1 to write it", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("diffs.json is not what the vectors give; if the change is deliberate, set TENON_UPDATE_VECTORS=1 and review the diff")
	}
}
