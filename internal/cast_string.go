package internal

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/goccy/go-googlesql"

	"github.com/goccy/googlesqlite/internal/value"
)

// castFromString converts s to t, whose kind is kind, following
// GoogleSQL's STRING conversion rules for CAST. CastValue cannot apply
// them because its lenient StringValue conversions also decode stored
// row values (where "" means 0 or false), so CAST routes STRING sources
// through here instead. The kind is passed separately because t.Kind()
// is a round trip into the googlesql runtime on every row.
func castFromString(t googlesql.Googlesql_TypeNode, kind googlesql.TypeKind, s value.StringValue) (value.Value, error) {
	// shortcut: only the BigQuery types are parsed strictly; INT32, UINT32,
	// UINT64 and FLOAT still use CastValue's lenient parsing. Add them here
	// when they need GoogleSQL's rules too.
	switch kind {
	case googlesql.TypeKindTypeInt64:
		return parseInt64String(string(s))
	case googlesql.TypeKindTypeBool:
		return parseBoolString(string(s))
	case googlesql.TypeKindTypeDouble:
		return parseDoubleString(string(s))
	case googlesql.TypeKindTypeNumeric:
		return parseNumericString(string(s), false)
	case googlesql.TypeKindTypeBignumeric:
		return parseNumericString(string(s), true)
	}
	return CastValue(t, s)
}

// trimCastSpace removes the ASCII whitespace GoogleSQL ignores around
// numeric strings. Unicode spaces such as U+00A0 are not trimmed.
func trimCastSpace(s string) string {
	return strings.Trim(s, " \t\n\v\f\r")
}

func splitSign(s string) (neg bool, rest string) {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		return s[0] == '-', s[1:]
	}
	return false, s
}

func hasHexPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X')
}

// parseInt64String accepts an optionally signed decimal or 0x-prefixed hex
// integer. Explicit bases keep strconv from accepting underscores or 0b/0o
// prefixes and from reading a leading zero as octal.
func parseInt64String(s string) (value.Value, error) {
	neg, digits := splitSign(trimCastSpace(s))
	base := 10
	if hasHexPrefix(digits) {
		base, digits = 16, digits[2:]
	}
	u, err := strconv.ParseUint(digits, base, 64)
	if err != nil || neg && u > 1<<63 || !neg && u > math.MaxInt64 {
		return nil, fmt.Errorf("Bad int64 value: %s", s)
	}
	if neg {
		// Two's complement negation keeps -2^63 representable.
		return value.IntValue(int64(-u)), nil
	}
	return value.IntValue(int64(u)), nil
}

func parseBoolString(s string) (value.Value, error) {
	switch {
	case equalFoldASCII(s, "true"):
		return value.BoolValue(true), nil
	case equalFoldASCII(s, "false"):
		return value.BoolValue(false), nil
	}
	return nil, fmt.Errorf("Bad bool value: %s", s)
}

// equalFoldASCII is strings.EqualFold without Unicode case folding,
// which would accept spellings such as "falſe" (U+017F folds to 's').
func equalFoldASCII(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if lowerASCII(s[i]) != lowerASCII(t[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// parseDoubleString accepts the strtod syntax GoogleSQL uses: decimal or
// hex floats (the hex exponent is optional), and case-insensitive
// [+-]inf, [+-]infinity and [+-]nan. Out-of-range values become ±Inf.
func parseDoubleString(s string) (value.Value, error) {
	trimmed := trimCastSpace(s)
	_, body := splitSign(trimmed)
	if isNaNString(body) {
		return value.FloatValue(math.NaN()), nil
	}
	if strings.Contains(trimmed, "_") {
		return nil, fmt.Errorf("Bad double value: %s", s)
	}
	if hasHexPrefix(body) && !strings.ContainsAny(body, "pP") {
		trimmed += "p0"
	}
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return nil, fmt.Errorf("Bad double value: %s", s)
	}
	return value.FloatValue(f), nil
}

// isNaNString reports whether s is "nan" or strtod's "nan(chars)" form,
// case-insensitively.
func isNaNString(s string) bool {
	if len(s) < 3 || !equalFoldASCII(s[:3], "nan") {
		return false
	}
	rest := s[3:]
	if rest == "" {
		return true
	}
	if len(rest) < 2 || rest[0] != '(' || rest[len(rest)-1] != ')' {
		return false
	}
	for _, c := range rest[1 : len(rest)-1] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_') {
			return false
		}
	}
	return true
}

const (
	numericScale    = 9
	bigNumericScale = 38
)

var (
	numericDenom    = pow10(numericScale)
	bigNumericDenom = pow10(bigNumericScale)

	// NUMERIC is DECIMAL(38, 9): at most 38 significant digits.
	maxNumericScaled = new(big.Int).Sub(pow10(38), big.NewInt(1))
	minNumericScaled = new(big.Int).Neg(maxNumericScaled)

	// BIGNUMERIC is a 256-bit two's complement integer scaled by 10^38.
	maxBigNumericScaled = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1))
	minBigNumericScaled = new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 255))
)

// parseNumericString accepts [+-]digits[.digits][e[+-]digits] with at
// least one digit in the mantissa, rounds half away from zero to the
// type's scale, and rejects values outside the type's range.
func parseNumericString(s string, isBigNumeric bool) (value.Value, error) {
	typeName, scale, denom, lo, hi := "NUMERIC", numericScale, numericDenom, minNumericScaled, maxNumericScaled
	if isBigNumeric {
		typeName, scale, denom, lo, hi = "BIGNUMERIC", bigNumericScale, bigNumericDenom, minBigNumericScaled, maxBigNumericScaled
	}
	invalid := func() (value.Value, error) {
		return nil, fmt.Errorf("Invalid %s value: %s", typeName, s)
	}

	neg, mantissa, exp, ok := splitDecimal(trimCastSpace(s))
	if !ok {
		return invalid()
	}
	mantissa = strings.TrimLeft(mantissa, "0")
	if mantissa == "" {
		return &value.NumericValue{Rat: new(big.Rat), IsBigNumeric: isBigNumeric}, nil
	}
	// The value is mantissa * 10^shift / 10^scale before rounding. Settle
	// extreme shifts up front so a huge exponent never materializes a
	// huge big.Int: in-range scaled values have at most 77 digits
	// (2^255 < 10^77), and anything below 0.1 units rounds to zero.
	shift := exp + int64(scale)
	switch {
	case int64(len(mantissa))+shift > 80:
		return invalid()
	case int64(len(mantissa))+shift < 0:
		return &value.NumericValue{Rat: new(big.Rat), IsBigNumeric: isBigNumeric}, nil
	}
	scaled, _ := new(big.Int).SetString(mantissa, 10)
	if shift >= 0 {
		scaled.Mul(scaled, pow10(shift))
	} else {
		var rem big.Int
		divisor := pow10(-shift)
		scaled.QuoRem(scaled, divisor, &rem)
		// Round half away from zero; the sign is applied afterwards.
		if rem.Lsh(&rem, 1).Cmp(divisor) >= 0 {
			scaled.Add(scaled, big.NewInt(1))
		}
	}
	if neg {
		scaled.Neg(scaled)
	}
	if scaled.Cmp(lo) < 0 || scaled.Cmp(hi) > 0 {
		return invalid()
	}
	return &value.NumericValue{
		Rat:          new(big.Rat).SetFrac(scaled, denom),
		IsBigNumeric: isBigNumeric,
	}, nil
}

// splitDecimal splits [+-]int[.frac][e[+-]exp] into its sign, the digits
// of int and frac concatenated, and the power of ten that applies to them.
// Exponents beyond int32 saturate, which callers treat as overflow or
// underflow.
func splitDecimal(s string) (neg bool, mantissa string, exp int64, ok bool) {
	neg, s = splitSign(s)
	mant, expPart, hasExp := s, "", false
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant, expPart, hasExp = s[:i], s[i+1:], true
	}
	intPart, fracPart, _ := strings.Cut(mant, ".")
	if intPart+fracPart == "" || !isDigits(intPart) || !isDigits(fracPart) {
		return false, "", 0, false
	}
	var e int64
	if hasExp {
		expNeg, expDigits := splitSign(expPart)
		if expDigits == "" || !isDigits(expDigits) {
			return false, "", 0, false
		}
		// ParseInt saturates at MaxInt32 on overflow.
		e, _ = strconv.ParseInt(expDigits, 10, 32)
		if expNeg {
			e = -e
		}
	}
	return neg, intPart + fracPart, e - int64(len(fracPart)), true
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}
