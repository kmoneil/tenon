# `make check` is the gate: a change is not done until it passes.

# local.mk holds untracked settings, such as TENON_SPEC, the specification
# file that tools/rulecheck reads.
-include local.mk

# Tests that call conformance.Covers record the rules they cover in RULECOV.
# The tests run with -count=1 because a cached result records nothing.
RULECOV := $(CURDIR)/.rulecov

.PHONY: check check-slow determinism fuzz fuzz-parse fuzz-string fuzz-deserialize release-fuzz growth rules codes report

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

# fuzz runs each fuzz target for FUZZTIME, 30 minutes by default, one after
# another, or all at once with make -j3 fuzz. What the fuzzer finds that fails
# is written to the package's testdata/fuzz directory, where it joins the seeds
# that every test run replays.
FUZZTIME ?= 30m
fuzz: fuzz-parse fuzz-string fuzz-deserialize
fuzz-parse:
	go test -run='^$$' -fuzz='^FuzzParse$$' -fuzztime=$(FUZZTIME) ./internal/decimal
fuzz-string:
	go test -run='^$$' -fuzz='^FuzzString$$' -fuzztime=$(FUZZTIME) .
fuzz-deserialize:
	go test -run='^$$' -fuzz='^FuzzDeserialize$$' -fuzztime=$(FUZZTIME) .

# release-fuzz is the fuzzing a release asks for: every target at once for five
# minutes. The depth is CI's, which fuzzes each for thirty minutes every night
# (.github/workflows/fuzz.yml), and keeps what it found from night to night.
release-fuzz:
	$(MAKE) -j3 fuzz FUZZTIME=5m

# growth runs every benchmark once and reads the pairs among them, each a
# benchmark measured at a size and at four times that size: where the larger
# allocates more than five times the bytes of the smaller, or makes more than
# five times its allocations, the work grows faster than its input and the
# run fails. Time is reported beside them and decides nothing. CI runs this
# every night (.github/workflows/growth.yml). BENCH narrows the run to the
# benchmarks a regular expression matches, as go test -bench does.
BENCH ?= .
growth:
	go run ./tools/growth -bench='$(BENCH)' ./...

