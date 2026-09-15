# `make check` is the gate: a change is not done until it passes.

# local.mk holds untracked settings, such as TENON_SPEC, the specification
# file that tools/rulecheck reads.
-include local.mk

# Tests that call conformance.Covers record the rules they cover in RULECOV.
# The tests run with -count=1 because a cached result records nothing.
RULECOV := $(CURDIR)/.rulecov

.PHONY: check rules

check:
	@echo '==> gofmt'
	@test -z "$$(gofmt -l .)" || { echo 'gofmt: these files need formatting:'; gofmt -l .; exit 1; }
	@echo '==> go vet'
	go vet ./...
	@echo '==> go test -race'
	rm -rf '$(RULECOV)'
	TENON_RULECOV_DIR='$(RULECOV)' go test -race -count=1 ./...
	@echo '==> rulecheck'
	go run ./tools/rulecheck -cover '$(RULECOV)'

# rules regenerates conformance/rules.json from the specification.
rules:
	go run ./tools/rulecheck manifest
