# Security

## Reporting a vulnerability

Please report it privately, through GitHub's private vulnerability reporting:
the **Report a vulnerability** button on this repository's **Security** tab.
Do not open a public issue for it.

A report is most useful with a program or an input that shows the problem,
and the version it was found in.

## What counts

tenon makes promises about what data cannot make it do, and a way around one
of them is a vulnerability:

- Data that is wrong becomes an error value, never a panic. A panic is kept
  for a mistake in the calling program, such as passing a value of the wrong
  type to an operation. The names of an object's attributes are data where
  they reach a value: `Object`, like `Map`, gives an error value for a
  name that is empty, not UTF-8, or the same as another once normalized.
  `ObjectType`, `ObjectWith` and `Path.Attribute` take names the program
  writes and panic on such a name; a program building them from names it did not
  write checks the names with `CheckAttributeNames` first.
- A value carrying a redacting mark never shows its contents, the keys of a
  map and the attribute names of an object among them, in a display form, a
  diagnostic's message or path, a usage panic's message, or a JSON
  projection, nor in anything derived from it. A panic names such a value by
  its marks alone, and withholds why the call could not take it where the
  reason would say what the marks withhold. A collection's declared type is
  the collection's own, not a member's: a list the program declared as a
  list of objects shows that element type, attribute names included, beside
  a redacted member as beside any other, and a type that tenon takes from a
  redacted value's attribute names, as a conversion does, carries its mark.
- `Deserialize` does not panic whatever bytes it is given, does not allocate
  for a length the input declares that the rest of the input could not hold,
  and does work that grows no faster than n log n in the length of its input,
  however the input is shaped. Bound the length of what you decode and you
  bound the cost, as with any parser.
- Number text is read only up to 10,000 characters, and refused beyond,
  before it is read: reading digits costs the square of their number, and
  text arrives from outside, as a JSON document's numbers do.

Any input that makes tenon do work out of proportion to its length, through
`Deserialize`, `String`, `NumberFromText`, `gotenon.Encode`, `Convert`, `Diff`
or `Unify`, is a vulnerability, and so is any input that makes it panic. Two
of them have a bound of their own. `Convert`'s work is in proportion to its
input and to its result, which the conversion defines: objects of distinct
attributes converted to one collection each gain the others' attributes, so
the result can be the square of the input, as `Convert`'s documentation says.
`Unify` refuses with `CodeUnifyTooLarge` a unification whose pairs would weigh
more than a fixed multiple of what it is given, and otherwise works in
proportion to that. The length of a Go value given to `gotenon.Encode` is the
tree it describes: a slice, map or pointer reached from two places counts at
each, as `encoding/json` counts it, so a value built by sharing one part many
times is as long as what it spells out.

`gotenon.Decode` into a `big.Int`, a `big.Rat` or a `big.Float` is exact, and
an exact number costs work in proportion to how large or small it is, not to
how long it is written: `1e-999999` is nine characters, and a hundred bytes
of such numbers take about a second to decode into a `[]big.Float`. Decode
numbers that come from outside into `tenon.Value`, or into a fixed-size type
such as `int64` or `float64`, whose cost is bounded.

## Supported versions

Fixes go into the latest minor release. tenon is before 1.0, so a fix that
changes behaviour may come as a new minor version rather than a patch.
