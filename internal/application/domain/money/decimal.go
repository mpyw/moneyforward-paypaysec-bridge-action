package money

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Foreign-currency amounts, and what they come to in yen.
//
// Here rather than in the package that scrapes them, for the same reason the
// three-route reconciliation is: this is arithmetic about money. Nothing below
// knows which site stated the figures, and the rule it enforces — that a
// converted amount and the source's own conversion of it agree — is the same
// judgement whoever reported them.
//
// Integer throughout. A rate times an amount is exactly representable in
// hundredths of each, and doing it in float64 would introduce an error that has
// to be absorbed by a tolerance, which then absorbs real disagreements too.

// hundredthsPattern is a decimal amount with exactly two places, or none.
//
// Two or none rather than "up to two", because the number of places is
// information: a figure rendered with them and a figure rendered without them
// come from different fields, and quietly accepting one place would turn
// "123.4" into 123.40 when it might have meant 123.04. Every observed foreign
// amount and rate on マニュライフ生命's contract page carries exactly two.
var hundredthsPattern = regexp.MustCompile(`^([+-]?)(\d+)(?:\.(\d{2}))?$`)

// ParseHundredths converts a decimal amount to hundredths of its unit:
// "12,345.67" is 1234567, "8" is 800.
//
// Strict in the same way [ParseYen] is, and for the same reason. The result is
// checked against a figure that will be recorded, so a string this does not
// recognise has to be an error rather than a best effort.
func ParseHundredths(s string) (int64, error) {
	cleaned := strings.TrimSpace(s)
	if placeholders[cleaned] {
		return 0, ErrNoValue
	}
	if digitsAcrossASpace.MatchString(cleaned) {
		return 0, fmt.Errorf("parse decimal %q: two runs of digits with only space "+
			"between them, so this is more than one amount", s)
	}
	cleaned = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', ' ', '　':
			return -1
		}
		return r
	}, cleaned)

	var err error
	if cleaned, err = ungroupBeforeThePoint(cleaned); err != nil {
		return 0, fmt.Errorf("parse decimal %q: %w", s, err)
	}

	m := hundredthsPattern.FindStringSubmatch(cleaned)
	if m == nil {
		return 0, fmt.Errorf("parse decimal %q: unrecognised format (cleaned to %q)", s, cleaned)
	}
	whole, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse decimal %q: %w", s, err)
	}
	var frac int64
	if m[3] != "" {
		if frac, err = strconv.ParseInt(m[3], 10, 64); err != nil {
			return 0, fmt.Errorf("parse decimal %q: %w", s, err)
		}
	}
	return signMultiplier(m[1]) * (whole*100 + frac), nil
}

// ungroupBeforeThePoint removes thousands separators from the integer part.
//
// Split from [ungroup] because the grouping check has to be applied to the part
// that is grouped: "12,345.67" is well-formed, and handing the whole string to a
// rule that expects groups of three all the way through rejects it.
func ungroupBeforeThePoint(s string) (string, error) {
	whole, frac, hasPoint := strings.Cut(s, ".")
	ungrouped, err := ungroup(whole)
	if err != nil {
		return "", err
	}
	if !hasPoint {
		return ungrouped, nil
	}
	if strings.Contains(frac, ",") {
		return "", fmt.Errorf("a thousands separator after the decimal point")
	}
	return ungrouped + "." + frac, nil
}

// ConvertedYenRange is what an amount in a foreign currency comes to in yen,
// as the range that rounding leaves open.
//
// Both arguments are in hundredths of their unit, which is how [ParseHundredths]
// reports them. The result is a closed interval: the exact product truncated,
// and the exact product rounded up. Any of truncation, half-up rounding or
// rounding away from zero lands inside it.
//
// A range rather than a number, because the source's rounding rule is not
// stated anywhere and guessing at it would mean either accepting a figure that
// is off by a yen or rejecting one that is right. What is worth checking is
// that the two figures describe the same amount, and that question has an exact
// answer.
func ConvertedYenRange(amountHundredths, rateHundredths int64) (lo, hi int64) {
	const scale = 10_000 // hundredths × hundredths

	product := amountHundredths * rateHundredths
	if product < 0 {
		// Truncation moves towards zero, so the ends swap for a negative
		// amount. Negative valuations do not arise here, but a function that
		// silently returns an inverted interval for one is a trap for whoever
		// uses it next.
		hi = -(-product / scale)
		lo = -((-product + scale - 1) / scale)
		return lo, hi
	}
	return product / scale, (product + scale - 1) / scale
}

// AgreesWithConversion reports whether a stated yen figure is the conversion of
// a foreign amount at a stated rate.
//
// The error names all three figures, because the point of the check is to be
// read by someone holding the page open beside it.
func AgreesWithConversion(yen, amountHundredths, rateHundredths int64) error {
	lo, hi := ConvertedYenRange(amountHundredths, rateHundredths)
	if yen >= lo && yen <= hi {
		return nil
	}
	return fmt.Errorf("the yen figure is %d, but %s at a rate of %s comes to %d–%d",
		yen, FormatHundredths(amountHundredths), FormatHundredths(rateHundredths), lo, hi)
}

// FormatHundredths renders hundredths back as a decimal: 1234567 is "12345.67".
//
// Exported because the messages here quote it, and anything a message can quote
// has to be registered with the log masker before the message can be produced.
// It was unexported, and the refusal above put an unmasked foreign-currency
// amount into a workflow log — the integer form was registered and the decimal
// form is a different string.
func FormatHundredths(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}
