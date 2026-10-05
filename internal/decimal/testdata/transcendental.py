#!/usr/bin/env python3
"""Writes transcendental.json: logarithms, exponentials and powers of
decimals, each correctly rounded to 96 significant digits, half to even.

Each value is computed by Python's decimal module, decimal arithmetic and
not binary floating point, at 300 significant digits, whose ln and exp are
correctly rounded, then rounded to 96. A case whose 300-digit value lies
within 10^-100 of a unit in the 96th digit of a midpoint between two 96-digit
numbers is refused: there 300 digits might not decide the rounding.

Run from this directory: python3 transcendental.py > transcendental.json
"""

import json
from decimal import Decimal, Context, ROUND_HALF_EVEN, ROUND_DOWN

WIDE = Context(prec=300, rounding=ROUND_HALF_EVEN, Emax=10**7, Emin=-10**7)
NARROW = Context(prec=96, rounding=ROUND_HALF_EVEN, Emax=10**7, Emin=-10**7)
DOWN = Context(prec=96, rounding=ROUND_DOWN, Emax=10**7, Emin=-10**7)


def rounded(v):
    """v rounded to 96 digits, refusing a v too near a midpoint."""
    if v == 0:
        return "0"
    down = DOWN.plus(v)
    ulp = Decimal(1).scaleb(down.adjusted() - 95)
    frac = WIDE.divide(abs(v - down), ulp)
    if abs(frac - Decimal("0.5")) < Decimal("1e-100"):
        raise SystemExit(f"{v} is too near a rounding midpoint")
    return str(NARROW.plus(v))


LN = ["2", "3", "10", "0.5", "1.5", "7", "1.0000000001", "0.9999999999",
      "123456789.123456789", "1e-400", "1e-999999", "9.99e999999", "0.001",
      "2.718281828459045235360287471352662497757247093699959574966967627724076630353547594571382178525166427"]
EXP = ["1", "-1", "0.5", "100", "-100", "1e-50", "1e-400", "2300000",
       "-2300000", "0.693147180559945309417232121458176568", "12.5", "-0.001"]
LOG = [("1000", "10"), ("2", "8"), ("8", "2"), ("2", "10"), ("1e-400", "10"),
       ("10", "2"), ("7", "0.5"), ("0.5", "0.25"), ("1e999999", "10")]
POW = [("2", "0.5"), ("10", "23"), ("3", "40"), ("0.1", "2"),
       ("1.0000001", "1e9"), ("2", "-0.5"), ("10", "-400"), ("7", "1.5"),
       ("0.5", "0.3333333333"), ("1e-10", "-0.5"), ("9", "0.5"),
       ("2", "1000000")]


def main():
    cases = []
    for x in LN:
        cases.append({"op": "ln", "args": [x], "want": rounded(WIDE.ln(Decimal(x)))})
    for x in EXP:
        cases.append({"op": "exp", "args": [x], "want": rounded(WIDE.exp(Decimal(x)))})
    for x, b in LOG:
        v = WIDE.divide(WIDE.ln(Decimal(x)), WIDE.ln(Decimal(b)))
        cases.append({"op": "log", "args": [x, b], "want": rounded(v)})
    for x, y in POW:
        v = WIDE.power(Decimal(x), Decimal(y))
        cases.append({"op": "pow", "args": [x, y], "want": rounded(v)})
    print(json.dumps({"about": __doc__.strip().splitlines()[0], "cases": cases}, indent=1))


main()
