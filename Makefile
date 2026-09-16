# `make check` is the gate: a change is not done until it passes.

# local.mk holds untracked settings, such as TENON_SPEC, the specification
# file that tools/rulecheck reads.
-include local.mk

# Tests that call conformance.Covers record the rules they cover in RULECOV.
# The tests run with -count=1 because a cached result records nothing.
RULECOV := $(CURDIR)/.rulecov

.PHONY: check check-slow determinism fuzz rules codes report

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

# codes regenerates the specification's appendix of diagnostic codes from
# codes.go.
codes:
	go run ./tools/rulecheck codes

# report regenerates CONFORMANCE.md, the conformance report, from a run of the
# tests that records coverage.
report:
	rm -rf '$(RULECOV)'
	TENON_RULECOV_DIR='$(RULECOV)' go test -count=1 ./...
	go run ./tools/rulecheck report -cover '$(RULECOV)'

# check-slow runs the gate, then every property test at twenty times its
# cases, then the determinism harness.
check-slow: check
	TENON_SLOW=20 go test -count=1 ./...
	$(MAKE) determinism

# Canonical output from two runs of the tests, which determinism compares.
EMIT := $(CURDIR)/.emit

# determinism runs the tests twice, in shuffled orders and on different numbers
# of processors, and fails unless every canonical output that they emit
# (encodings, display forms, diffs and the like) comes out the same.
determinism:
	rm -rf '$(EMIT)'
	TENON_EMIT_DIR='$(EMIT)/first' go test -count=1 -shuffle=on ./...
	TENON_EMIT_DIR='$(EMIT)/second' GOMAXPROCS=1 go test -count=1 -shuffle=on ./...
	@test -n "$$(find '$(EMIT)/first' -type f)" || { echo 'determinism: the tests emitted nothing'; exit 1; }
	diff -r '$(EMIT)/first' '$(EMIT)/second'
	@echo "determinism: $$(find '$(EMIT)/first' -type f | wc -l | tr -d ' ') outputs came out the same in both runs"

# fuzz runs each fuzz target for FUZZTIME. What the fuzzer finds that fails is
# written to the package's testdata/fuzz directory, where it joins the seeds.
FUZZTIME ?= 30m
fuzz:
	go test -run='^$$' -fuzz='^FuzzParse$$' -fuzztime=$(FUZZTIME) ./internal/decimal
	go test -run='^$$' -fuzz='^FuzzString$$' -fuzztime=$(FUZZTIME) .
	go test -run='^$$' -fuzz='^FuzzDeserialize$$' -fuzztime=$(FUZZTIME) .

