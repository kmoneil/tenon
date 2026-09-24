# Conformance report

This is the conformance report that Appendix A of the tenon specification
requires. `make report` generates it from the rule manifest, the rules that
`make check` enforces, and a run of the conformance tests, and `make check`
fails where it is stale.

| | |
| --- | --- |
| Specification version | 0.4.0 |
| Unicode version (`ST-003`) | 15.0.0 |
| Rules | 195 normative, 0 outline, 0 withdrawn |
| Rules satisfied | 195 of 195 |
| Optional areas omitted | none |

## Rules not satisfied

None. Every normative rule is enforced, and a passing conformance test covers it.

## Implementation-defined orderings

- **Unknown set members** (`EQ-044`): after the known members, in the bytewise order of their encodings (`SE-001`), which hold no type. Members that encode alike only because a capsule value's type declares no encoding follow the canonical order of those capsule values (`EQ-045`). A set holds no pending value.
- **Capsule values of a type that declares no ordering** (`EQ-045`): values the type's equality reports equal together; other values by the hash the type declares, if it declares one, and then by the order in which the run first compared them, which holds for the rest of the run.
- **Capsule types of one name** (`EQ-045`): by the order in which the run created them.

## Optional areas

None is omitted: capsule types (§2.5), marks (§6) and serialization (§8) are
implemented, and their conformance tests run.

## Rules by area

| Area | Normative rules | Satisfied |
| ---- | --------------: | --------: |
| `TY` | 23 | 23 |
| `NU` | 16 | 16 |
| `ST` | 5 | 5 |
| `BO` | 1 | 1 |
| `VA` | 7 | 7 |
| `UN` | 14 | 14 |
| `ER` | 8 | 8 |
| `MK` | 11 | 11 |
| `EQ` | 19 | 19 |
| `CV` | 25 | 25 |
| `SE` | 24 | 24 |
| `GO` | 23 | 23 |
| `DI` | 19 | 19 |
