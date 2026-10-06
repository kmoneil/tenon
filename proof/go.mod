module github.com/kmoneil/tenon/proof

go 1.26.0

require (
	github.com/hashicorp/hcl/v2 v2.25.0
	github.com/kmoneil/tenon v0.18.0
	github.com/kmoneil/tenon/ctytenon v0.3.0
	github.com/zclconf/go-cty v1.19.0
)

require (
	github.com/agext/levenshtein v1.2.1 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/apparentlymart/go-textseg/v17 v17.0.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace (
	github.com/kmoneil/tenon => ../
	github.com/kmoneil/tenon/ctytenon => ../ctytenon
)
