package bench

import (
	"bytes"
	"strconv"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// Size is a document size the suite measures: a label for the benchmark's
// name and the number of services the document holds.
type Size struct {
	Label    string
	Services int
}

// Sizes are the document sizes the suite measures: about a kilobyte, 32
// kilobytes and a megabyte of JSON.
var Sizes = []Size{{"1KB", 6}, {"32KB", 200}, {"1MB", 6400}}

// Services returns a configuration document of n services, as JSON. Each
// service has a name, a replica count, a fractional CPU share, a list of
// ports, a map of environment variables and a list of tags, as a deployment
// configuration does. Service i of every document is the same, so documents
// differ only in length.
func Services(n int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"services":[`)
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		id := strconv.Itoa(i)
		b.WriteString(`{"name":"svc-` + id + `","replicas":` + strconv.Itoa(i%7+1) +
			`,"cpu":` + strconv.Itoa(i%4) + `.5,"ports":[80,443,` + strconv.Itoa(8000+i) +
			`],"env":{"LOG":"info","REGION":"us-east-` + strconv.Itoa(i%3) + `","ID":"` + id +
			`"},"tags":["web","prod","team-` + strconv.Itoa(i%10) + `"],"enabled":true}`)
	}
	b.WriteString(`]}`)
	return b.Bytes()
}

// TenonSchema is what a document must be: an object holding a list of
// services, each an object of exactly these attributes.
func TenonSchema() tenon.Constraint {
	str, num := tenon.Exactly(tenon.StringType()), tenon.Exactly(tenon.NumberType())
	service := tenon.ObjectWith(map[string]tenon.Field{
		"name":     tenon.Required(str),
		"replicas": tenon.Required(num),
		"cpu":      tenon.Required(num),
		"ports":    tenon.Required(tenon.ListOf(num)),
		"env":      tenon.Required(tenon.MapOf(str)),
		"tags":     tenon.Required(tenon.ListOf(str)),
		"enabled":  tenon.Required(tenon.Exactly(tenon.BoolType())),
	}, true)
	return tenon.ObjectWith(map[string]tenon.Field{"services": tenon.Required(tenon.ListOf(service))}, true)
}

// CtySchema is TenonSchema in go-cty's terms.
func CtySchema() cty.Type {
	service := cty.Object(map[string]cty.Type{
		"name":     cty.String,
		"replicas": cty.Number,
		"cpu":      cty.Number,
		"ports":    cty.List(cty.Number),
		"env":      cty.Map(cty.String),
		"tags":     cty.List(cty.String),
		"enabled":  cty.Bool,
	})
	return cty.Object(map[string]cty.Type{"services": cty.List(service)})
}
