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

// A Bridge that maps Terraform's sensitive mark carries a sensitive value to
// tenon under a redacting mark, which tenon's display withholds, and back to
// cty as it was.
func ExampleBridge_marks() {
	b := ctytenon.Bridge{
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
