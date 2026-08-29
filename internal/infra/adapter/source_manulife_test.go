package adapter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
)

// The loop over contracts, which is where this source's judgements are: which
// to skip, when the browser has to be brought back to the list, and whether a
// premium can be attributed to anything.
//
// It had no test at all, and two of those three were wrong. The site is stood
// in for here; what the pages themselves say is covered against a browser in
// internal/infra/manulife.

// card builds a list entry, punctuated the way the live list punctuates it.
func card(title, number, status string) manulife.Card {
	return manulife.Card{
		Title: title,
		Fields: []manulife.Pair{
			{Label: selector.LabelPolicyNumber + "：", Value: number},
			{Label: selector.LabelStatus + " :", Value: status},
		},
	}
}

// reading is a contract page that agrees with itself: 10,000.00 at 150.00.
func reading(number string) manulife.Reading {
	return manulife.Reading{
		Number:         number,
		PolicyType:     "通貨選択型一時払終身保険",
		Yen:            1500000,
		HasYen:         true,
		FCYHundredths:  1000000,
		HasFCY:         true,
		RateHundredths: 15000,
		HasRate:        true,
	}
}

// recorder stands in for the site and records what the loop asked it to do.
type recorder struct {
	cards []manulife.Card
	calls []string
	back  error
}

func (r *recorder) pages() *manulifePages {
	return &manulifePages{
		cards: func(context.Context) ([]manulife.Card, error) {
			r.calls = append(r.calls, "list")
			return r.cards, nil
		},
		readCard: func(_ context.Context, c manulife.Card) (manulife.Reading, error) {
			number, _ := c.Number()
			r.calls = append(r.calls, "read:"+number)
			return reading(number), nil
		},
		backToList: func(context.Context) error {
			r.calls = append(r.calls, "back")
			return r.back
		},
	}
}

// TestHoldingsGoesBackOnlyAfterLeavingTheList is the bug the loop index hid.
//
// "Not the first contract" stood in for "the browser is on a contract page",
// and the two part company the moment one is skipped. Going back from the list
// lands on whatever preceded it — the sign-in flow — and the run then waits out
// the navigation budget for cards that are not coming, every weekday.
func TestHoldingsGoesBackOnlyAfterLeavingTheList(t *testing.T) {
	site := &recorder{cards: []manulife.Card{
		card("失効した契約", "000-0000001", "消滅"),
		card("生きている契約", "000-0000002", selector.StatusInForce),
	}}
	src := ManulifeSource{testPages: site.pages()}

	held, err := src.Holdings(t.Context())
	if err != nil {
		t.Fatalf("Holdings() = %v", err)
	}
	if len(held.Assets) != 1 {
		t.Fatalf("assets = %+v, want just the contract in force", held.Assets)
	}
	want := []string{"list", "read:000-0000002"}
	if strings.Join(site.calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v — nothing was on a contract page to come back from",
			site.calls, want)
	}
}

// TestHoldingsGoesBackBetweenContracts is the other half: two live contracts
// means one navigation away from the list, and one return to it.
func TestHoldingsGoesBackBetweenContracts(t *testing.T) {
	site := &recorder{cards: []manulife.Card{
		card("契約A", "000-0000001", selector.StatusInForce),
		card("契約B", "000-0000002", selector.StatusInForce),
	}}
	src := ManulifeSource{testPages: site.pages()}

	if _, err := src.Holdings(t.Context()); err != nil {
		t.Fatalf("Holdings() = %v", err)
	}
	want := []string{"list", "read:000-0000001", "back", "read:000-0000002"}
	if strings.Join(site.calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v", site.calls, want)
	}
}

// TestHoldingsRefusesToShareOnePremiumBetweenContracts.
//
// MANULIFE_ACQUISITION_YEN names one contract's premium. Recording it against
// two overstates the cost of each, and an overstated cost is a plausible
// profit that nothing downstream can question — the read-back confirms the
// figure that was sent, not the figure that was right.
func TestHoldingsRefusesToShareOnePremiumBetweenContracts(t *testing.T) {
	site := &recorder{cards: []manulife.Card{
		card("契約A", "000-0000001", selector.StatusInForce),
		card("契約B", "000-0000002", selector.StatusInForce),
	}}
	src := ManulifeSource{testPages: site.pages(), AcquisitionYen: 4000000}

	_, err := src.Holdings(t.Context())
	if err == nil {
		t.Fatal("Holdings() recorded one premium against two contracts")
	}
	if !strings.Contains(err.Error(), acquisitionVariable) {
		t.Errorf("error = %v, want it to name the variable to look at", err)
	}
}

// TestHoldingsCarriesThePremiumForASingleContract: the case it is for.
func TestHoldingsCarriesThePremiumForASingleContract(t *testing.T) {
	site := &recorder{cards: []manulife.Card{
		card("契約A", "000-0000001", selector.StatusInForce),
		card("失効した契約", "000-0000002", "消滅"),
	}}
	src := ManulifeSource{testPages: site.pages(), AcquisitionYen: 4000000}

	held, err := src.Holdings(t.Context())
	if err != nil {
		t.Fatalf("Holdings() = %v", err)
	}
	if len(held.Assets) != 1 {
		t.Fatalf("assets = %+v", held.Assets)
	}
	a := held.Assets[0]
	if !a.HasAcquisition || a.AcquisitionYen != 4000000 {
		t.Errorf("acquisition = %d (recorded=%v); without it the ledger reports a "+
			"profit of exactly zero", a.AcquisitionYen, a.HasAcquisition)
	}
	if a.Kind != asset.SavingsInsurance {
		t.Errorf("kind = %v", a.Kind)
	}
}

// TestHoldingsSaysWhenItSkipsAContract.
//
// A skipped contract is otherwise invisible: the list was read, so the category
// counts as covered, so the row this program wrote for that contract is deleted
// rather than left alone. A deletion nobody was told about is the failure this
// project keeps closing.
func TestHoldingsSaysWhenItSkipsAContract(t *testing.T) {
	site := &recorder{cards: []manulife.Card{
		card("失効した契約", "000-0000001", "消滅"),
		card("生きている契約", "000-0000002", selector.StatusInForce),
	}}
	var skipped []string
	src := ManulifeSource{
		testPages: site.pages(),
		OnSkip:    func(c manulife.Card) { skipped = append(skipped, c.Title) },
	}

	if _, err := src.Holdings(t.Context()); err != nil {
		t.Fatalf("Holdings() = %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "失効した契約" {
		t.Errorf("skipped = %v, want the contract that is not in force", skipped)
	}
}

// TestHoldingsCoversItsCategoryEvenWithNothingToRecord.
//
// Covered means "the list was read", and it has to be reported even when every
// contract was skipped — otherwise the rows under it are unverified rather than
// stale, and the coverage check refuses to remove them.
func TestHoldingsCoversItsCategory(t *testing.T) {
	site := &recorder{cards: []manulife.Card{card("失効した契約", "000-0000001", "消滅")}}
	src := ManulifeSource{testPages: site.pages()}

	held, err := src.Holdings(t.Context())
	if err != nil {
		t.Fatalf("Holdings() = %v", err)
	}
	if len(held.Categories) != 1 || held.Categories[0] != ManulifeCategory {
		t.Errorf("categories = %v, want %q", held.Categories, ManulifeCategory)
	}
	if len(held.Assets) != 0 {
		t.Errorf("assets = %+v, want none", held.Assets)
	}
}

// TestHoldingsFailsWhenItCannotGetBack: a failure to return to the list is not
// something to carry on past — the next click would be aimed at whatever page
// this one left behind.
func TestHoldingsFailsWhenItCannotGetBack(t *testing.T) {
	site := &recorder{
		cards: []manulife.Card{
			card("契約A", "000-0000001", selector.StatusInForce),
			card("契約B", "000-0000002", selector.StatusInForce),
		},
		back: errors.New("the list did not come back"),
	}
	src := ManulifeSource{testPages: site.pages()}

	if _, err := src.Holdings(t.Context()); err == nil {
		t.Fatal("Holdings() carried on after failing to return to the list")
	}
}
