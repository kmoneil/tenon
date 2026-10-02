package ctytenon_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/zclconf/go-cty/cty"
)

// sensitive is the tenon mark that Terraform's sensitive mark, the string
// "sensitive" in cty, crosses as: one that redacts what it marks.
type sensitive struct{}

func (sensitive) MarkID() string                 { return "sensitive" }
func (sensitive) Propagation() tenon.Propagation { return tenon.Propagate }
func (sensitive) Redacting() bool                { return true }

// terraform returns a Bridge that maps Terraform's sensitive mark, and no
// other, to the tenon mark sensitive and back.
func terraform() ctytenon.Bridge {
	return ctytenon.Bridge{
		MarkFromCty: func(m any) (tenon.Mark, bool) {
			if m == "sensitive" {
				return sensitive{}, true
			}
			return nil, false
		},
		MarkToCty: func(m tenon.Mark) (any, bool) {
			if m == (sensitive{}) {
				return "sensitive", true
			}
			return nil, false
		},
	}
}

// A program built on cty hands a configuration to tenon, which checks it, and
// takes it back. What is not known yet, a replica count to be settled when the
// configuration is applied, crosses with what is known of it, and what is
// sensitive crosses under a mark that tenon's display withholds.
func Example() {
	b := terraform()
	config := cty.ObjectVal(map[string]cty.Value{
		"name":     cty.StringVal("web"),
		"replicas": cty.UnknownVal(cty.Number).Refine().NotNull().NumberRangeLowerBound(cty.NumberIntVal(1), true).NewValue(),
		"token":    cty.StringVal("s3cr3t").Mark("sensitive"),
	})

	v, err := b.FromCty(config)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)

	// tenon holds the configuration to what the program expects of it.
	want := tenon.ObjectWith(map[string]tenon.Field{
		"name":     tenon.Required(tenon.Exactly(tenon.StringType())),
		"replicas": tenon.Required(tenon.Exactly(tenon.NumberType())),
	}, false)
	fmt.Println(tenon.Convert(v, want, tenon.Safe).IsError())

	back, err := b.ToCty(v)
	fmt.Println(back.RawEquals(config), err)
	// Output:
	// {"name": "web", "replicas": unknown(number, not null, >= 1), "token": redacted("sensitive")}
	// false
	// true <nil>
}

// A Bridge that maps Terraform's sensitive mark carries a sensitive value to
// tenon under a redacting mark, which tenon's display withholds, and back to
// cty as it was.
func ExampleBridge_marks() {
	b := terraform()
	login := cty.ObjectVal(map[string]cty.Value{
		"user":     cty.StringVal("admin"),
		"password": cty.StringVal("hunter2").Mark("sensitive"),
	})

	v, err := b.FromCty(login)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)

	back, err := b.ToCty(v)
	fmt.Println(back.RawEquals(login), err)
	// Output:
	// {"password": redacted("sensitive"), "user": "admin"}
	// true <nil>
}

// A cty type constraint, the type of a Terraform variable, crosses as the
// tenon constraint that accepts what cty's conversion to it does: an object
// type as an open ObjectWith, since cty's conversion drops attributes the
// type does not name and tenon's keeps them.
func ExampleBridge_ConstraintFromCty() {
	var b ctytenon.Bridge
	c, err := b.ConstraintFromCty(cty.Object(map[string]cty.Type{
		"name": cty.String,
		"tags": cty.List(cty.DynamicPseudoType),
	}))
	fmt.Println(c, err)
	// Output:
	// object_with({"name": exactly(string), "tags": list_of(any)}, open) <nil>
}
