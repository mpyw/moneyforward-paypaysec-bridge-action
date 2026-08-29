package money_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/money"
)

func TestParseHundredths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int64
	}{
		{"grouped with two places", "12,345.67", 1234567},
		{"no places", "8", 800},
		{"a rate", "150.25", 15025},
		{"negative", "-237.50", -23750},
		{"leading plus", "+1.00", 100},
		{"surrounding whitespace", "  99.99  ", 9999},
		{"ideographic space", "　99.99　", 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := money.ParseHundredths(tt.in)
			if err != nil {
				t.Fatalf("ParseHundredths(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseHundredths(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseHundredthsRefusesTheAmbiguous is the same discipline ParseYen
// applies to 万 notation: a figure that could mean two things is an error, not a
// guess, because the result is written into a financial record.
func TestParseHundredthsRefusesTheAmbiguous(t *testing.T) {
	for _, in := range []string{
		"123.4",       // one place: 123.40 or 123.04?
		"123.456",     // three places, and no rule for the third
		"12,34.56",    // grouping that is not thousands
		"1.23.45",     // two points
		"12,345.6,7",  // a separator after the point
		"100 200",     // two amounts with only space between them
		"12,345.67ドル", // a unit the caller was supposed to strip
	} {
		if got, err := money.ParseHundredths(in); err == nil {
			t.Errorf("ParseHundredths(%q) = %d, want an error", in, got)
		}
	}
}

func TestParseHundredthsReportsAPlaceholder(t *testing.T) {
	if _, err := money.ParseHundredths("—"); !errors.Is(err, money.ErrNoValue) {
		t.Errorf("ParseHundredths(placeholder) error = %v, want ErrNoValue", err)
	}
}

// TestConvertedYenRangeBracketsTheRounding: the source does not say how it
// rounds, so what is checked is that both figures describe the same amount.
func TestConvertedYenRange(t *testing.T) {
	// 12,345.67 米ドル at 150.25 円 = 1,854,936.9... yen.
	lo, hi := money.ConvertedYenRange(1234567, 15025)
	if lo != 1854936 || hi != 1854937 {
		t.Errorf("ConvertedYenRange = %d–%d, want 1854936–1854937", lo, hi)
	}

	// An exact product leaves no room at all.
	if lo, hi = money.ConvertedYenRange(10000, 10000); lo != 10000 || hi != 10000 {
		t.Errorf("ConvertedYenRange for an exact product = %d–%d, want 10000–10000", lo, hi)
	}
}

// TestConvertedYenRangeDoesNotInvertForNegatives guards a trap rather than a
// case that arises: truncation moves towards zero, so a naive implementation
// returns lo > hi for a negative amount and every comparison against it passes.
func TestConvertedYenRangeDoesNotInvertForNegatives(t *testing.T) {
	lo, hi := money.ConvertedYenRange(-1234567, 15025)
	if lo > hi {
		t.Errorf("ConvertedYenRange = %d–%d, which is inverted", lo, hi)
	}
	if hi != -1854936 || lo != -1854937 {
		t.Errorf("ConvertedYenRange = %d–%d, want -1854937–-1854936", lo, hi)
	}
}

func TestAgreesWithConversion(t *testing.T) {
	// Either rounding of 1,854,936.9... is accepted.
	for _, yen := range []int64{1854936, 1854937} {
		if err := money.AgreesWithConversion(yen, 1234567, 15025); err != nil {
			t.Errorf("AgreesWithConversion(%d, …) = %v, want nil", yen, err)
		}
	}
	// One yen outside it is not.
	for _, yen := range []int64{1854935, 1854938} {
		if err := money.AgreesWithConversion(yen, 1234567, 15025); err == nil {
			t.Errorf("AgreesWithConversion(%d, …) = nil, want an error", yen)
		}
	}
}

// TestAgreesWithConversionNamesEveryFigure: the message is read by someone with
// the page open beside it, so all three numbers have to be in it.
func TestAgreesWithConversionNamesEveryFigure(t *testing.T) {
	err := money.AgreesWithConversion(1, 1234567, 15025)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"12345.67", "150.25", "1854936"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}
