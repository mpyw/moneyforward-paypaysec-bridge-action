package manulife

import (
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
)

func TestParseForeignAmount(t *testing.T) {
	got, err := parseForeignAmount("10,000.00 米ドル")
	if err != nil {
		t.Fatalf("parseForeignAmount = %v", err)
	}
	if got != 1000000 {
		t.Errorf("parseForeignAmount = %d, want 1000000 hundredths", got)
	}
}

func TestParseRate(t *testing.T) {
	got, err := parseRate("1 米ドル=150.25 円")
	if err != nil {
		t.Fatalf("parseRate = %v", err)
	}
	if got != 15025 {
		t.Errorf("parseRate = %d, want 15025 hundredths", got)
	}
}

// TestParseRateRefusesAQuotePerManyUnits is the check that stops a wrong figure
// looking like a right one.
//
// Quoting a rate per hundred units is a normal way to write one for a weak
// currency, and this contract's currency is chosen by the policyholder. Reading
// "100 テスト=1.50 円" as a rate per unit would put the cross-check a hundredfold
// out — and the cross-check is the only thing standing behind the figure that
// gets recorded.
func TestParseRateRefusesAQuotePerManyUnits(t *testing.T) {
	if got, err := parseRate("100 テスト=1.50 円"); err == nil {
		t.Errorf("parseRate = %d, want a refusal of a rate quoted per 100 units", got)
	}
}

func TestParseRateRefusesTheMalformed(t *testing.T) {
	for _, in := range []string{
		"150.25 円",      // no currency and no =
		"1 米ドル=150.25",  // no 円
		"米ドル=150.25 円",  // no count
		"1 米ドル=とても高い 円", // not a number
	} {
		if got, err := parseRate(in); err == nil {
			t.Errorf("parseRate(%q) = %d, want an error", in, got)
		}
	}
}

// readingOf builds a Reading from the three page strings, the way readFigures
// does, so the tests below exercise the parsing and the check together.
func readingOf(t *testing.T, yen, fcy, rate string) Reading {
	t.Helper()
	var r Reading
	rows := []Pair{
		{Label: selector.LabelSurrenderYen + "：", Value: yen},
		{Label: selector.LabelSurrenderFCY + ":", Value: fcy},
		{Label: selector.LabelRate, Value: rate},
	}
	if err := r.readFigures(rows); err != nil {
		t.Fatalf("readFigures: %v", err)
	}
	return r
}

func TestReadingAmount(t *testing.T) {
	r := readingOf(t, "1,500,000 円", "10,000.00 米ドル", "1 米ドル=150.00 円")
	got, err := r.Amount()
	if err != nil {
		t.Fatalf("Amount() = %v", err)
	}
	if got != 1500000 {
		t.Errorf("Amount() = %d, want 1500000", got)
	}
}

// TestReadingAmountAcceptsEitherRounding is the case the first live read landed
// on, and the reason the check is a range rather than an equality.
//
// 10,000.01 米ドル at 150.07 comes to 1,500,701.5007 円. The site does not say
// how it rounds, and demanding one answer would fail a correct reading by a
// single yen — which reads downstream as "the page is lying", and stops the
// whole run.
func TestReadingAmountAcceptsEitherRounding(t *testing.T) {
	for _, yen := range []string{"1,500,701 円", "1,500,702 円"} {
		r := readingOf(t, yen, "10,000.01 米ドル", "1 米ドル=150.07 円")
		if _, err := r.Amount(); err != nil {
			t.Errorf("Amount() for %s = %v — both roundings of the exact product "+
				"describe the same amount", yen, err)
		}
	}
}

func TestReadingAmountRefusesADisagreement(t *testing.T) {
	// A yen figure one outside the rounding range: not the same amount.
	r := readingOf(t, "1,500,703 円", "10,000.01 米ドル", "1 米ドル=150.07 円")
	if got, err := r.Amount(); err == nil {
		t.Errorf("Amount() = %d, want a refusal", got)
	}
}

// TestReadingAmountRefusesAnUncheckedFigure: a figure with nothing holding it
// up is not recorded.
//
// The alternative is to record it and hope, and an unverified figure in a
// financial record is indistinguishable from a correct one — which is the whole
// reason this program cross-checks anything.
func TestReadingAmountRefusesAnUncheckedFigure(t *testing.T) {
	r := readingOf(t, "1,500,000 円", "", "")
	_, err := r.Amount()
	if err == nil {
		t.Fatal("Amount() accepted a figure with nothing to check it against")
	}
	if !strings.Contains(err.Error(), selector.LabelRate) {
		t.Errorf("error = %v, want it to name what was missing", err)
	}
}

// TestReadingAmountOnAPageWithNoYenFigure is what an unauthenticated page, or a
// contract of some other kind, looks like: the label is simply not there.
func TestReadingAmountOnAPageWithNoYenFigure(t *testing.T) {
	r := readingOf(t, "", "10,000.00 米ドル", "1 米ドル=150.00 円")
	if _, err := r.Amount(); err == nil {
		t.Error("Amount() produced a figure from a page that stated none")
	}
}

// TestOptionalFieldRefusesDuplicates repeats in Go what the browser test proves
// against markup, because this is the rule and it should be pinned where it
// lives.
func TestOptionalFieldRefusesDuplicates(t *testing.T) {
	rows := []Pair{
		{Label: "積立金額", Value: "1,000,000 円"},
		{Label: "積立金額:", Value: "2,000,000 円"},
	}
	if got, err := optionalField(rows, "積立金額"); err == nil {
		t.Errorf("optionalField = %q, want a refusal — the punctuation differs but "+
			"the label does not", got)
	}
}

// TestFieldSeparatesAbsentFromEmpty: field insists on a value, optionalField
// does not, and Amount decides what an absence means.
func TestFieldRequiresAValue(t *testing.T) {
	if _, err := field(nil, "保険種類"); err == nil {
		t.Error("field() accepted a page with no such row")
	}
	if got, err := optionalField(nil, "保険種類"); err != nil || got != "" {
		t.Errorf("optionalField() = %q, %v — an absence is not an error here", got, err)
	}
}
