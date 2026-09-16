# tenon

tenon is a Go library providing a dynamic type and value system for
applications that must represent user-supplied data whose types are not known
at compile time. Its primary intended use is as the value layer of a
configuration language, but it is not tied to any particular syntax.

tenon is under active development and not yet ready for use.

# Why this exists

When I need something that I need and don't want to modify an existing library, I usually write it myself. If you find
useful, awesome. If you find where it might be lacking, let me know. Find a bug, post an issue.

# Development

`make check` is the gate: a change is not done until it passes. It checks
formatting, runs `go vet` and the tests under the race detector, and then
checks the conformance suite against the specification's rules: every rule has
a passing conformance test, the diagnostic codes agree with the specification,
and `CONFORMANCE.md` is current.

The other targets:

| Target | What it does |
| ------ | ------------ |
| `make check-slow` | `make check`, then every property test at twenty times its cases (`TENON_SLOW=20`), then `make determinism`. Run it before a release, and after changing how values are stored, ordered or encoded. |
| `make determinism` | Runs the tests twice, in shuffled orders and on different numbers of processors, writing the canonical output they emit (encodings, display forms, diffs, conversions) to `.emit/`, and fails unless both runs wrote the same bytes. |
| `make fuzz` | Runs each fuzz target (the number parser, string construction, and decoding) for `FUZZTIME`, 30 minutes by default. An input that fails is saved under the package's `testdata/fuzz`, where it runs with the tests from then on. |
| `make report` | Runs the tests, recording the rules they cover, and regenerates `CONFORMANCE.md`. |
| `make rules`, `make codes` | Regenerate `conformance/rules.json` and the specification's appendix of diagnostic codes from the specification that `TENON_SPEC` names. |
