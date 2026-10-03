# `make check` is the gate: a change is not done until it passes.

# local.mk holds untracked settings, such as TENON_SPEC, the specification
# file that tools/rulecheck reads.
-include local.mk

# Tests that call conformance.Covers record the rules they cover in RULECOV.
# The tests run with -count=1 because a cached result records nothing.
RULECOV := $(CURDIR)/.rulecov

.PHONY: check check-slow determinism fuzz fuzz-parse fuzz-string fuzz-string-xtext fuzz-deserialize fuzz-convert fuzz-json release-fuzz growth bench bench-smoke rules codes report lint vuln release-notes ctytenon-released

# tools/unigen is a module of its own, which ./... does not reach, so that
# tenon requires no other module: its tests hold internal/uni to Unicode's own
# conformance tests, and to golang.org/x/text where the toolchain carries the
# same Unicode version. The gate and the linter run there too.
UNIGEN := tools/unigen

# ctytenon, the bridge to go-cty, is a module of its own for the same reason:
# tenon never requires go-cty, and only a program importing the bridge
# downloads it. The gate, the linter and govulncheck run there too.
CTYTENON := ctytenon

check:
	@test -z "$$TENON_UPDATE_VECTORS" || { echo 'check: TENON_UPDATE_VECTORS is set, which rewrites both corpora and passes; unset it'; exit 1; }
	@echo '==> gofmt'
	@test -z "$$(gofmt -l .)" || { echo 'gofmt: these files need formatting:'; gofmt -l .; exit 1; }
	@echo '==> go vet'
	go vet ./...
	cd $(UNIGEN) && go vet ./...
	cd $(CTYTENON) && go vet ./...
	@echo '==> go test -race'
	rm -rf '$(RULECOV)'
	TENON_RULECOV_DIR='$(RULECOV)' go test -race -count=1 ./...
	cd $(UNIGEN) && TENON_RULECOV_DIR='$(RULECOV)' go test -race -count=1 ./...
	cd $(CTYTENON) && go test -race -count=1 ./...
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
	cd $(UNIGEN) && TENON_SLOW=20 go test -count=1 ./...
	cd $(CTYTENON) && TENON_SLOW=20 go test -count=1 ./...
	$(MAKE) determinism

# Canonical output from two runs of the tests, which determinism compares.
EMIT := $(CURDIR)/.emit

# determinism runs the tests twice, in shuffled orders and on different numbers
# of processors, and fails unless every canonical output that they emit
# (encodings, display forms, diffs and the like) comes out the same. CI runs
# it every night (.github/workflows/determinism.yml).
determinism:
	rm -rf '$(EMIT)'
	TENON_EMIT_DIR='$(EMIT)/first' go test -count=1 -shuffle=on ./...
	TENON_EMIT_DIR='$(EMIT)/second' GOMAXPROCS=1 go test -count=1 -shuffle=on ./...
	@n="$$(find '$(EMIT)/first' -type f | wc -l | tr -d ' ')"; test "$$n" -eq 12 || { echo "determinism: the tests emitted $$n outputs, not the 12 they emit; a silenced emitter would otherwise pass"; exit 1; }
	diff -r '$(EMIT)/first' '$(EMIT)/second'
	@echo "determinism: $$(find '$(EMIT)/first' -type f | wc -l | tr -d ' ') outputs came out the same in both runs"

# fuzz runs each fuzz target for FUZZTIME, 30 minutes by default, one after
# another, or all at once with make -j5 fuzz. What the fuzzer finds that fails
# is written to the package's testdata/fuzz directory, where it joins the seeds
# that every test run replays. FuzzStringAgainstXText, in tools/unigen, holds
# string construction to golang.org/x/text, and runs only below go1.27.
FUZZTIME ?= 30m
fuzz: fuzz-parse fuzz-string fuzz-string-xtext fuzz-deserialize fuzz-convert fuzz-json
fuzz-parse:
	go test -run='^$$' -fuzz='^FuzzParse$$' -fuzztime=$(FUZZTIME) ./internal/decimal
fuzz-string:
	go test -run='^$$' -fuzz='^FuzzString$$' -fuzztime=$(FUZZTIME) .
fuzz-string-xtext:
	cd $(UNIGEN) && go test -run='^$$' -fuzz='^FuzzStringAgainstXText$$' -fuzztime=$(FUZZTIME) .
fuzz-deserialize:
	go test -run='^$$' -fuzz='^FuzzDeserialize$$' -fuzztime=$(FUZZTIME) .
fuzz-convert:
	go test -run='^$$' -fuzz='^FuzzConvert$$' -fuzztime=$(FUZZTIME) .
fuzz-json:
	go test -run='^$$' -fuzz='^FuzzParseJSON$$' -fuzztime=$(FUZZTIME) .

# release-fuzz is the fuzzing a release asks for: every target at once for five
# minutes. The depth is CI's, which fuzzes each for thirty minutes every night
# (.github/workflows/fuzz.yml), and keeps what it found from night to night.
release-fuzz:
	$(MAKE) -j6 fuzz FUZZTIME=5m

# growth runs every benchmark once and reads the pairs among them, each a
# benchmark measured at a size and at four times that size: where the larger
# allocates more than five times the bytes of the smaller, or makes more than
# five times its allocations, the work grows faster than its input and the
# run fails. Time is reported beside them and decides nothing. CI runs this
# every night (.github/workflows/growth.yml). BENCH narrows the run to the
# benchmarks a regular expression matches, as go test -bench does. PAIRS is
# the fewest pairs a run of every benchmark reads, all of those there are, so
# that a pair dropped, or no longer run with its package, fails the run rather
# than passing on the pairs left; a narrowed run reads what it selects.
BENCH ?= .
PAIRS ?= 40
growth:
	go run ./tools/growth -bench='$(BENCH)' -pairs=$(if $(filter .,$(BENCH)),$(PAIRS),1) ./...


# bench measures what tenon costs beside encoding/json and go-cty, in the
# bench module, which has its own go.mod so that go-cty never enters tenon's,
# and writes BENCHMARKS.md and the README's summary from the medians of
# BENCHCOUNT runs of each benchmark. It takes about ten minutes; run it on a
# quiet machine before a release. CI does not run it: timing on shared
# runners is noise, and the nightly growth job guards how work scales.
BENCHCOUNT ?= 10
BENCHTIME ?= 1s
bench:
	mkdir -p .bench
	cd bench && go test -run '^$$' -bench . -benchmem -count '$(BENCHCOUNT)' -benchtime '$(BENCHTIME)' > ../.bench/raw.txt
	cd bench && go run ./cmd/benchdoc -in ../.bench/raw.txt -doc ../BENCHMARKS.md -readme ../README.md

# bench-smoke vets the bench module, runs staticcheck over it, runs its tests,
# which probe tenon with the cases of go-cty's open issues, and runs each of
# its benchmarks once, so that a change to tenon's API cannot leave the
# benchmarks broken until a release measures them. CI runs it with lint.
bench-smoke:
	cd bench && go vet ./... && go run $(STATICCHECK) ./... && go test -bench . -benchtime 1x ./...

# lint runs staticcheck and vuln runs govulncheck, each at the version named
# here through go run, so that neither enters go.mod as a dependency. CI runs
# lint on every change as a required check (.github/workflows/check.yml), and
# vuln on every change and every night (.github/workflows/vuln.yml), since a
# vulnerability can be published against code that has not changed. vuln
# fails only on a vulnerability tenon's code can reach, the standard
# library's included, so it also flags a toolchain that needs updating.
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.8.0
lint:
	go run $(STATICCHECK) ./...
	cd $(UNIGEN) && go run $(STATICCHECK) ./...
	cd $(CTYTENON) && go run $(STATICCHECK) ./...
vuln:
	go run $(GOVULNCHECK) ./...
	cd $(CTYTENON) && go run $(GOVULNCHECK) ./...

# release-notes prints the notes of VERSION, its section of CHANGELOG.md, or
# of ctytenon/CHANGELOG.md for a version of the bridge, as in
# VERSION=ctytenon/v0.1.0, which the release workflow
# (.github/workflows/release.yml) makes the GitHub Release of a pushed tag
# from. Check them before tagging: make release-notes VERSION=0.6.0.
release-notes:
	@test -n '$(VERSION)' || { echo 'release-notes: set VERSION, as in VERSION=0.6.0'; exit 1; }
	@case '$(VERSION)' in \
	  ctytenon/*) go run ./tools/relnotes -changelog $(CTYTENON)/CHANGELOG.md '$(patsubst ctytenon/%,%,$(VERSION))' ;; \
	  *) go run ./tools/relnotes '$(VERSION)' ;; \
	esac

# ctytenon-released runs the bridge's tests against the tenon its go.mod
# requires, as a program that imports the bridge gets it. In this repository
# the bridge builds beside the tenon in the working tree (go.mod's replace,
# which only this repository sees), so a release of the bridge runs this
# first: a bridge that reaches for what the tenon it requires does not have
# fails here rather than in a program that imports it.
ctytenon-released:
	@d=$$(mktemp -d) && trap 'rm -rf "$$d"' EXIT && cp -R $(CTYTENON)/. "$$d" && cd "$$d" && \
	  go mod edit -dropreplace github.com/kmoneil/tenon && \
	  GOFLAGS=-mod=mod go test -count=1 ./... && \
	  echo 'ctytenon-released: the bridge passes against the tenon its go.mod requires'
