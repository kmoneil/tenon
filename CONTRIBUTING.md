# Contributing to tenon

Issues and pull requests are welcome. This page says how the project works,
so that a change arrives ready to merge.

## Before you start

For anything beyond a small fix, open an issue first, saying what you want to
change and why, so that the design is settled before the code is written.

A security problem goes through private reporting, never an issue or a pull
request: see `SECURITY.md`.

## Building and testing

tenon needs Go 1.26 or later, and nothing else to build and test.

- `make check` is the gate: gofmt, `go vet`, the tests under the race
  detector, and a check that every rule has a passing conformance test. A
  change is not done until it passes. CI runs it on Go 1.26 and 1.27 for every
  pull request.
- `make lint` runs staticcheck, which CI requires as well.
- `make vuln` runs govulncheck.
- `make check-slow` runs the property tests at twenty times their cases and
  checks that the tests' output is the same in every run. Run it when you
  change how values are stored, ordered or encoded.

The other targets:

| Target | What it does |
| ------ | ------------ |
| `make bench` | Runs the benchmarks of the `bench` module, which measure tenon beside `encoding/json` and go-cty, ten times, and writes `BENCHMARKS.md` and the Performance summary from the medians. Run it on a quiet machine before a release. |
| `make check-slow` | `make check`, then every property test at twenty times its cases (`TENON_SLOW=20`), then `make determinism`. Run it before a release, and after changing how values are stored, ordered or encoded. |
| `make determinism` | Runs the tests twice, in shuffled orders and on different numbers of processors, writing the canonical output they emit (encodings, display forms, diffs, conversions) to `.emit/`, and fails unless both runs wrote the same bytes. CI does this every night. |
| `make fuzz` | Runs each fuzz target (the number parser, string construction, decoding, and conversion) for `FUZZTIME`, 30 minutes by default; `make -j4 fuzz` runs them at once. An input that fails is saved under the package's `testdata/fuzz`, where it runs with the tests from then on. CI does this every night. |
| `make release-fuzz` | Every fuzz target at once for five minutes: the fuzzing a release asks for, the depth coming from the nightly runs. |
| `make release-notes VERSION=x.y.z` | Prints that version's section of `CHANGELOG.md`, which becomes the GitHub Release when its tag is pushed. |
| `make growth` | Measures each benchmark at a size and at four times that size, and fails where the larger allocates more than five times what the smaller does: work growing faster than its input. CI does this every night. |
| `make lint` | Runs staticcheck, at the version the Makefile names, over every package. CI requires it of every change. |
| `make report` | Runs the tests, recording the rules they cover, and regenerates `CONFORMANCE.md`. |
| `make rules`, `make codes` | Regenerate `conformance/rules.json` and the specification's appendix of diagnostic codes from the specification that `TENON_SPEC` names. |
| `make vuln` | Runs govulncheck, at the version the Makefile names, and fails on a known vulnerability tenon's code can reach, in its dependencies or in the Go standard library it is built with. CI runs it on every change and every night. |

## How the code holds together

- tenon implements a specification whose rules each have an identifier, such
  as `NU-012`; `conformance/rules.json` lists them. A test that checks a rule
  is named for it, as `TestConformance_NU012_DivisionRounding`, and calls
  `conformance.Covers(t, "NU-012")`. `make check` fails where a rule has no
  passing test named for it. A change to what a rule says is the maintainers'
  to make: open an issue.
- Numbers are exact. Binary floating point never stands in for a tenon
  number, in the code or as a test's expected answer; use `math/big` or number
  text. Only gotenon handles Go's `float32` and `float64`, converting them
  exactly at its boundary.
- Data that is wrong becomes an error value with a diagnostic code. A panic is
  for a mistake in the calling program, and its message begins
  `tenon: usage:`.
- Nothing a caller can observe depends on the order Go iterates a map in.
- Values are immutable: every operation returns a new value.
- Dependencies stay few: `golang.org/x/text` and `github.com/rivo/uniseg`. A
  new one needs its reason given in the pull request.

## Pull requests

- One change per pull request, with the tests that hold it, and an entry in
  `CHANGELOG.md` under Unreleased for anything a user of the library would
  notice.
- Doc comments and the README say what the code does after the change.
- A commit message says what changed and why.
- `main` takes only pull requests, merged by rebase or squash so that history
  stays linear, and a pull request merges once it is up to date with `main`
  and passes `check` and `lint`.

## License

tenon is licensed under the Apache License, Version 2.0, and a contribution is
made under the same license (see `LICENSE`).
