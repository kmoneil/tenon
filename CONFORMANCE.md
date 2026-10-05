# Conformance report

This is the conformance report that Appendix A of the tenon specification
requires. `make report` generates it from the rule manifest, the rules that
`make check` enforces, and a run of the conformance tests, and `make check`
fails where it is stale.

| | |
| --- | --- |
| Specification version | 0.13.1 |
| Unicode version (`ST-003`) | 15.0.0 |
| Rules | 245 normative, 0 outline, 0 withdrawn |
| Rules satisfied | 240 of 245 |
| Optional areas omitted | none |

## Rules not satisfied

| Rule | Why |
| ---- | --- |
| `LB-012` | deferred: partly known collections are first read by the collection functions |
| `LB-013` | deferred: sequences are first taken by the collection functions |
| `LB-021` | deferred: conversions between arguments first come with Coalesce |
| `LB-030` | deferred: argument domains are first refused by the rounding functions and ParseInt |
| `LB-031` | deferred: the first bounded results are Range's and SetProduct's |

## Implementation-defined orderings

- **Unknown set members** (`EQ-044`): after the known members, in the bytewise order of their encodings (`SE-001`), which hold no type. Members that encode alike only because a capsule value's type declares no encoding follow the canonical order of those capsule values (`EQ-045`). A set holds no pending value.
- **Capsule values of a type that declares no ordering** (`EQ-045`): values the type's equality reports equal together; other values by the hash the type declares, if it declares one; values whose hashes collide by their encodings in the canonical order, where the type declares an encoding, and otherwise by the order in which the run first compared them, which holds for the rest of the run.
- **Capsule types of one name** (`EQ-045`): by the order in which the run created them.
- **Ties among unknown members** (`EQ-044`): members that the orders above leave together, which only members told apart by nothing but capsule values their type reports equal can be, keep the order the set was given them in.

## Optional areas

None is omitted: capsule types (§2.5), marks (§6) and serialization (§8) are
implemented, and their conformance tests run.

## Rules by area

| Area | Normative rules | Satisfied |
| ---- | --------------: | --------: |
| `TY` | 24 | 24 |
| `NU` | 16 | 16 |
| `ST` | 5 | 5 |
| `BO` | 1 | 1 |
| `VA` | 7 | 7 |
| `UN` | 17 | 17 |
| `ER` | 8 | 8 |
| `MK` | 11 | 11 |
| `EQ` | 19 | 19 |
| `CV` | 26 | 26 |
| `SE` | 24 | 24 |
| `GO` | 24 | 24 |
| `DI` | 20 | 20 |
| `JS` | 9 | 9 |
| `FN` | 17 | 17 |
| `LB` | 11 | 6 |
| `LN` | 6 | 6 |
