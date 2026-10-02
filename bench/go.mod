module github.com/kmoneil/tenon/bench

go 1.26.0

require (
	github.com/kmoneil/tenon v0.13.0
	github.com/kmoneil/tenon/ctytenon v0.0.0-00010101000000-000000000000
	github.com/zclconf/go-cty v1.19.0
)

require (
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/apparentlymart/go-textseg/v17 v17.0.1 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/kmoneil/tenon => ../

replace github.com/kmoneil/tenon/ctytenon => ../ctytenon
