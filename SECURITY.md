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
- `Deserialize` does not panic whatever bytes it is given, and does not
  allocate for a length the input declares that the rest of the input could
  not hold.

How much work decoding may do is not yet bounded for every input. Until it
is, limit the size of input taken from a source you do not trust.

## Supported versions

Fixes go into the latest minor release. tenon is before 1.0, so a fix that
changes behaviour may come as a new minor version rather than a patch.
