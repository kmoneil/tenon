package tenon_test

import (
	"errors"
	"fmt"

	"github.com/kmoneil/tenon"
)

// sensitive marks a value that must not be shown. A mark crosses a process
// boundary only if its type says how it is encoded, and the receiver must be
// given a decoder for it, so a mark is never lost quietly on the way.
type sensitive struct{}

func (sensitive) MarkID() string                   { return "acme/sensitive" }
func (sensitive) Propagation() tenon.Propagation   { return tenon.Propagate }
func (sensitive) Redacting() bool                  { return true }
func (sensitive) MarkPayload() (tenon.Value, bool) { return tenon.Value{}, false }
func (sensitive) String() string                   { return "sensitive" }

// A plugin protocol: a host and a plugin in separate processes pass values as
// bytes, and what is not known yet, what it is bounded by, and what must stay
// secret all survive the crossing.
func Example_pluginProtocol() {
	// The host's side. The address is not known until the resource is
	// created, though the host knows which network it will be on.
	resource := tenon.Object(map[string]tenon.Value{
		"name":    tenon.String("web"),
		"address": tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix("10.")),
		"token":   tenon.WithMarks(tenon.String("hunter2"), sensitive{}),
	})
	wire, err := tenon.Serialize(resource)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(wire), "bytes on the wire")

	// The plugin's side. It is given a decoder for each mark it understands.
	decoders := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
		"acme/sensitive": func(tenon.Value, bool) (tenon.Mark, error) { return sensitive{}, nil },
	}}
	received, err := tenon.Deserialize(wire, decoders)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(received)
	fmt.Println("same value:", tenon.Identical(received, resource))

	// One value has one encoding, so a cache or a comparison can work on the
	// bytes without unpacking them.
	again, _ := tenon.Serialize(received)
	fmt.Println("same bytes:", string(again) == string(wire))

	// A receiver that does not know a mark is told so, rather than being
	// handed a value whose secret has quietly stopped being one.
	// The error is a *tenon.Error, whose diagnostics say why.
	_, err = tenon.Deserialize(wire, tenon.Decoders{})
	var refused *tenon.Error
	if errors.As(err, &refused) {
		fmt.Println("refused:", refused.Diagnostics()[0].Code)
	}
	// Output:
	// 88 bytes on the wire
	// {"address": unknown(string, not null, prefix "10.", length >= 3), "name": "web", "token": redacted("acme/sensitive")}
	// same value: true
	// same bytes: true
	// refused: serialize.unknown_mark
}
