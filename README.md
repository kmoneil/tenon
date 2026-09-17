# tenon

tenon is a Go library providing a dynamic type and value system for
applications that must represent user-supplied data whose types are not known
at compile time. It is built as the value layer of a configuration language,
and is not tied to any particular syntax. The same guarantees serve other
programs that hold such values:

- **Plan and diff engines** carry values that are not known yet through a
  plan, each with a range of what it may turn out to be, and show what changes
  without showing what is secret.
- **Plugin protocols** pass values, unknown and marked ones included, across a
  process boundary in one canonical encoding.
- **Validation layers** check values against constraints, and report every
  failure with a stable code and the path to it.

Version 0.1.0 implements version 0.1.0 of the tenon specification and
satisfies every one of its rules, as `CONFORMANCE.md` reports; `CHANGELOG.md`
says what it holds.

    go get github.com/kmoneil/tenon@v0.1.0

# Unicode

tenon holds strings in Normalization Form C and measures their length in
grapheme clusters, both under Unicode 15.0.0, and its display form escapes
text by the same version. Which Unicode version is in use decides which
strings are equal and how long they are, so changing it is a breaking change.

**Known issue in 0.1.0.** The version is not held inside the module.
Normalization and display escaping read tables that follow the toolchain you
build with, so a build with Go 1.27 or later applies Unicode 17.0.0 to both,
while grapheme cluster lengths stay at 15.0.0: one build mixes two versions.
Some sequences normalize differently between them, so a value built under one
version can serialize to bytes that a build under the other refuses as
`serialize.not_canonical`. Build with Go 1.26 for the version this module
states.

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

# License

tenon is licensed under the Apache License, Version 2.0. See `LICENSE`.
