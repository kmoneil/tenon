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
  type to an operation.
- A value carrying a redacting mark never shows its contents in a display
  form, a diagnostic message or a JSON projection, nor in anything derived
  from it.
- `Deserialize` does not panic whatever bytes it is given, does not allocate
  for a length the input declares that the rest of the input could not hold,
  and does work that grows no faster than n log n in the length of its input,
  however the input is shaped. Bound the length of what you decode and you
  bound the cost, as with any parser.
- Number text is read only up to 10,000 characters, and refused beyond,
  before it is read: reading digits costs the square of their number, and
  text arrives from outside, as a JSON document's numbers do.

Any input that makes tenon do work out of proportion to its length, through
`Deserialize`, `String`, `NumberFromText` or `gotenon.Encode`, is a
vulnerability, and so is any input that makes it panic.

## Supported versions

Fixes go into the latest minor release. tenon is before 1.0, so a fix that
changes behaviour may come as a new minor version rather than a patch.
