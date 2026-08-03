package investapi

import (
	"strings"
	"testing"
)

// TestReadAcceptsTheArrayShape is the shape the live service sent on the first
// real run, and the shape this package refused.
//
// INVEST_BRAND_ARRAY is a PHP array: json_encode writes an object when its keys
// are sparse and an array when they are dense, so the shape follows the account's
// holdings rather than the endpoint. One run saw both — the ミニアプリ bucket keyed
// by brand id, the アプリ bucket a bare array — and there is no shape to assume.
//
// The array form carries no key, so the join to the catalogue has to fall back to
// BRAND_ID. Both stub responses use the array form here, which is what makes that
// fallback the thing under test.
func TestReadAcceptsTheArrayShape(t *testing.T) {
	got, err := serve(t, &stub{asArray: true}).Read(t.Context(), App)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one held", got.Holdings)
	}
	// Named, not just counted: an unnamed holding is refused, so a count alone
	// would pass on a join that silently matched the wrong 銘柄.
	if got.Holdings[0].Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q, want the catalogue entry with the held BRAND_ID",
			got.Holdings[0].Name)
	}
	if got.Holdings[0].Acquisition != 300000 {
		t.Errorf("acquisition = %d, want value - gain", got.Holdings[0].Acquisition)
	}
}

// TestReadAcceptsAnEmptyBucket separates "holds nothing" from "could not be read".
//
// An empty PHP array is `[]`, which is the array shape again — and refusing to
// decode it would turn every genuinely empty category into a failed run. There is
// no danger in accepting it here: an empty answer that is actually a lost session
// is caught by the envelope, and one that is actually a mis-read is caught by the
// ledger's own refusal to empty a category.
func TestReadAcceptsAnEmptyBucket(t *testing.T) {
	got, err := serve(t, &stub{noHoldings: true}).Read(t.Context(), App)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(got.Holdings) != 0 {
		t.Errorf("holdings = %+v, want none", got.Holdings)
	}
	// The totals still have to arrive: "no holdings" is not "no answer".
	if got.Total != 345678 {
		t.Errorf("total = %d, want the reported total", got.Total)
	}
}

// TestReadAcceptsQuotedNumbers is the other half of what a PHP service does to
// scalars.
//
// Two of these arrived unquoted where the page's bundle implied a string, and the
// reverse is the same coin: a value's JSON type here reflects where it came from
// on the far side, not what it means. Every number this package reads is parsed
// from either form, so finding out costs a test rather than a failed sync.
func TestReadAcceptsQuotedNumbers(t *testing.T) {
	got, err := serve(t, &stub{quoted: true}).Read(t.Context(), MiniApp)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Total != 345678 || got.Acquisition != 300000 || got.Gain != 45678 {
		t.Errorf("totals = %+v, want them parsed out of the quoted form", got)
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one held", got.Holdings)
	}
	// Joined on a quoted BRAND_ID, which is the part a lenient total would not
	// have covered.
	if got.Holdings[0].Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q", got.Holdings[0].Name)
	}
	if got.Holdings[0].Yen != 345678 {
		t.Errorf("yen = %d", got.Holdings[0].Yen)
	}
}

// TestReadRefusesANumberItCannotParse keeps the leniency from becoming a guess: a
// scalar that is neither a number nor a number in quotes is refused, not zeroed.
// A zero here would be an amount, and this program acts on amounts.
func TestReadRefusesANumberItCannotParse(t *testing.T) {
	_, err := serve(t, &stub{brokenTotal: true}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() accepted a total that is not a number")
	}
	if !strings.Contains(err.Error(), "not a number") {
		t.Errorf("error = %v, want it to say what was wrong with the value", err)
	}
}
