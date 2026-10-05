# Unicode 15.0.0 data

These files are Unicode's own, copied unchanged from
<https://www.unicode.org/Public/15.0.0/ucd/>, under the Unicode License V3
(`LICENSE`, from <https://www.unicode.org/license.txt>).

| File | Where it is published | SHA-256 |
| ---- | --------------------- | ------- |
| `GraphemeBreakProperty.txt` | `auxiliary/GraphemeBreakProperty.txt` | `5a0f8748575432f8ff95e1dd5bfaa27bda1a844809e17d6939ee912bba6568a1` |
| `emoji-data.txt` | `emoji/emoji-data.txt` | `29071dba22c72c27783a73016afb8ffaeb025866740791f9c2d0b55cc45a3470` |
| `GraphemeBreakTest.txt` | `auxiliary/GraphemeBreakTest.txt` | `0d2080d0def294a4b7660801cc03ddfe5866ff300c789c2cc1b50fd7802b2d97` |
| `NormalizationTest.txt` | `NormalizationTest.txt` | `fb9ac8cc154a80cad6caac9897af55a4e75176af6f4e2bb6edc2bf8b1d57f326` |
| `UnicodeData.txt` | `UnicodeData.txt` | `806e9aed65037197f1ec85e12be6e8cd870fc5608b4de0fffd990f689f376a73` |
| `SpecialCasing.txt` | `SpecialCasing.txt` | `78b29c64b5840d25c11a9f31b665ee551b8a499eca6c70d770fcad7dd710f494` |
| `DerivedCoreProperties.txt` | `DerivedCoreProperties.txt` | `d367290bc0867e6b484c68370530bdd1a08b6b32404601b8c7accaf83e05628d` |
| `PropList.txt` | `PropList.txt` | `e05c0a2811d113dae4abd832884199a3ea8d187ee1b872d8240a788a96540bfd` |
| `Scripts.txt` | `Scripts.txt` | `cca85d830f46aece2e7c1459ef1249993dca8f2e46d51e869255be140d7ea4b0` |
| `CaseFolding.txt` | `CaseFolding.txt` | `cdd49e55eae3bbf1f0a3f6580c974a0263cb86a6a08daa10fbf705b4808a56f7` |
| `PropertyValueAliases.txt` | `PropertyValueAliases.txt` | `13a7666843abea5c6b7eb8c057c57ab9bb2ba96cfc936e204224dd67d71cafad` |

unigen generates `internal/uni/grapheme_tables.go` from the first two, which
neither Go's `unicode` package nor `golang.org/x/text` carries, and
`internal/uni/case_tables.go` from the last four: the simple case mappings of
`UnicodeData.txt`, the unconditional entries of `SpecialCasing.txt`, the Cased
and Case_Ignorable properties of `DerivedCoreProperties.txt`, and White_Space
of `PropList.txt`; and `internal/uni/pattern_tables.go` from `UnicodeData.txt`,
`Scripts.txt`, `CaseFolding.txt` and `PropertyValueAliases.txt`: the general
categories, the scripts, the simple case-folding orbits and the categories'
aliases that patterns read. `UnicodeData.txt` names no version, so unigen refuses it
unless its SHA-256 is the one above. `GraphemeBreakTest.txt` and
`NormalizationTest.txt` are Unicode's conformance tests for normalization and
segmentation, which this module's tests hold `internal/uni` to on every
toolchain. They are here rather than in tenon's module so that its download
does not carry them.

Moving to another Unicode version means putting that version's files here, and
unigen and the tests refuse files that do not say they are for the version
they are asked for.
