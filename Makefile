# `make check` is the gate: a change is not done until it passes.

# local.mk holds untracked settings, such as TENON_SPEC, the specification
# file that tools/rulecheck reads.
-include local.mk

.PHONY: check rules

check:
	@echo '==> gofmt'
	@test -z "$$(gofmt -l .)" || { echo 'gofmt: these files need formatting:'; gofmt -l .; exit 1; }
	@echo '==> go vet'
	go vet ./...
	@echo '==> go test -race'
	go test -race ./...
	@echo '==> rulecheck'
	go run ./tools/rulecheck

# rules regenerates conformance/rules.json from the specification.
rules:
	go run ./tools/rulecheck manifest
