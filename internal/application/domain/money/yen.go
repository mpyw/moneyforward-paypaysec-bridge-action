// Package money parses the yen figures Japanese brokerage sites render.
//
// Its own package because it is pure: no browser, no network, no site. That
// makes it the one piece of this project that can be reasoned about entirely
// from its tests, which matters for the part that turns markup into the number
// recorded against someone's assets.
package money

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoValue reports a cell that holds a placeholder rather than an amount —
// the "—" the site shows for an empty holding. It is a legitimate state, not a
// parse failure, so callers can treat it as "nothing here" without conflating it
// with markup they failed to understand.
var ErrNoValue = errors.New("no value")

// Amount patterns as the sites render them.
//
// The 万 remainder is required to be exactly four digits because that is what
// every observed figure uses — 78万9012, 25万1234, 60万0000 (note the padding
// on a round million). Accepting a shorter remainder would mean guessing whether
// "1万23" is 10,023 or 12,300, and guessing about money is how a wrong balance
// gets recorded as if it were right. A rendering change should fail loudly here
// instead.
var (
	manPattern   = regexp.MustCompile(`^([+-]?)(\d+)万(\d{4})?$`)
	plainPattern = regexp.MustCompile(`^([+-]?)(\d+)$`)

	// numberRun is a maximal stretch of digits and separators, checked for
	// grouping before the separators are dropped.
	numberRun = regexp.MustCompile(`[0-9,]+`)

	// grouped is what a separator is allowed to mean.
	grouped = regexp.MustCompile(`^\d{1,3}(,\d{3})+$`)

	// digitsAcrossASpace catches two figures that whitespace was the only thing
	// separating.
	digitsAcrossASpace = regexp.MustCompile(`\d[\s\x{3000}]+\d`)
)

// placeholders are the strings the site uses for "nothing to show".
var placeholders = map[string]bool{
	"":  true,
	"—": true, // em dash
	"―": true, // horizontal bar
	"−": true, // minus signMultiplier
	"-": true,
}

// ParseYen converts a rendered amount to whole yen.
//
// It handles the site's 万 notation and the signed form used for profit and
// loss: "25万1234円" is 251234, "+3万1234円" is 31234, "-237円" is -237.
//
// It is deliberately strict. Anything it does not recognise is an error rather
// than a best effort, because the caller writes this number into a financial
// record where a plausible-but-wrong figure is indistinguishable from a correct
// one. Returns [ErrNoValue] for a placeholder.
func ParseYen(s string) (int64, error) {
	cleaned := strings.TrimSpace(s)
	if placeholders[cleaned] {
		return 0, ErrNoValue
	}

	// 円, commas and spaces used to be dropped wherever they appeared, which threw
	// away the only thing separating one figure from the next: "100円200円" became
	// "100200" and parsed cleanly. A selector that starts matching a container
	// rather than a cell is the same drift that once read an unrelated "0円" as the
	// balance of an account holding 25万1234円, so two amounts have to be refused
	// rather than concatenated into a third.
	if digitsAcrossASpace.MatchString(cleaned) {
		return 0, fmt.Errorf("parse yen %q: two runs of digits with only space between "+
			"them, so this is more than one amount", s)
	}
	switch strings.Count(cleaned, "円") {
	case 0:
	case 1:
		if !strings.HasSuffix(cleaned, "円") {
			return 0, fmt.Errorf("parse yen %q: 円 is not at the end, so this is not one amount", s)
		}
		cleaned = strings.TrimSuffix(cleaned, "円")
	default:
		return 0, fmt.Errorf("parse yen %q: more than one 円, so this is more than one amount", s)
	}

	cleaned = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', ' ', '　':
			return -1
		}
		return r
	}, cleaned)

	var err error
	if cleaned, err = ungroup(cleaned); err != nil {
		return 0, fmt.Errorf("parse yen %q: %w", s, err)
	}
	if placeholders[cleaned] {
		return 0, ErrNoValue
	}

	// 億 would need its own rule, and no sample has ever shown one. Say so
	// rather than let the 万 pattern fail with a vaguer message.
	if strings.ContainsRune(cleaned, '億') {
		return 0, fmt.Errorf("parse yen %q: 億 notation is not supported", s)
	}

	if m := manPattern.FindStringSubmatch(cleaned); m != nil {
		head, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse yen %q: %w", s, err)
		}
		var rest int64
		if m[3] != "" {
			rest, err = strconv.ParseInt(m[3], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse yen %q: %w", s, err)
			}
		}
		return signMultiplier(m[1]) * (head*10000 + rest), nil
	}

	if m := plainPattern.FindStringSubmatch(cleaned); m != nil {
		n, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse yen %q: %w", s, err)
		}
		return signMultiplier(m[1]) * n, nil
	}

	return 0, fmt.Errorf("parse yen %q: unrecognised format (cleaned to %q)", s, cleaned)
}

// ungroup removes thousands separators, having checked that is what they are.
//
// "1,00" and "1,0,0" both used to reach 100 by having their commas deleted. A
// separator in the wrong place means the text is not the number it appears to
// be, and this package's promise is to say so rather than produce something
// plausible.
func ungroup(s string) (string, error) {
	s = strings.ReplaceAll(s, "，", ",")
	if !strings.Contains(s, ",") {
		return s, nil
	}
	var bad string
	out := numberRun.ReplaceAllStringFunc(s, func(run string) string {
		if !strings.Contains(run, ",") {
			return run
		}
		if !grouped.MatchString(run) {
			bad = run
			return run
		}
		return strings.ReplaceAll(run, ",", "")
	})
	if bad != "" {
		return "", fmt.Errorf("%q is not grouped in thousands", bad)
	}
	return out, nil
}

func signMultiplier(s string) int64 {
	if s == "-" {
		return -1
	}
	return 1
}
