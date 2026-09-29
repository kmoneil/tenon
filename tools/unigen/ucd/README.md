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

unigen generates `internal/uni/grapheme_tables.go` from the first two, which
neither Go's `unicode` package nor `golang.org/x/text` carries. The other two
are Unicode's conformance tests for normalization and segmentation, which this
module's tests hold `internal/uni` to on every toolchain. They are here rather
than in tenon's module so that its download does not carry them.

Moving to another Unicode version means putting that version's files here, and
unigen and the tests refuse files that do not say they are for the version
they are asked for.
