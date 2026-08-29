package adapter

import (
	"context"
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/assetname"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

// ManulifeID names this source in logs, in errors, and in the input names its
// credentials arrive under.
const ManulifeID = "manulife"

// acquisitionVariable is what supplies [ManulifeSource.AcquisitionYen], named
// here only so a refusal can tell somebody which one to look at.
const acquisitionVariable = "MANULIFE_ACQUISITION_YEN"

// ManulifeCategory is what マニュライフ生命's holdings are recorded under.
//
// Short because MoneyForward caps an entry's name at twenty characters and the
// prefix is kept whole; see [assetname.Scheme]. It is also the string a recorded
// entry is traced back through, so changing it orphans every row already
// written under the old one — which reconciliation then deletes.
const ManulifeCategory = "保険"

// ManulifeSource reads a contract's surrender value from マニュライフ生命.
//
// One entry, not one per 銘柄: a policy is the unit the site states a figure
// for. What makes it comparable to a brokerage holding is that the figure is a
// valuation and the cost is known, which is all [asset.Asset] asks for.
type ManulifeSource struct {
	Client *manulife.Client

	// Browser is the chromedp context the site is driven through.
	Browser context.Context

	// Codes supplies the one-time code the login needs.
	Codes otp.Source

	// AcquisitionYen is what was actually paid, in yen.
	//
	// Supplied rather than read: the contract is denominated in a foreign
	// currency and nothing on the site converts the premium at the rate it was
	// struck at. Converting it at today's rate would produce a cost that moves
	// with the exchange rate — and a holding can be down in dollars while up in
	// yen, which is the case this exists for.
	//
	// Zero means it was not configured. Left out, MoneyForward takes the cost to
	// equal the value and reports a profit of exactly zero, which is a figure
	// rather than a blank; see [asset.Asset].
	AcquisitionYen int64

	// OnLogin, if set, is told whether a challenge was presented.
	OnLogin func(challenged bool)

	// OnSkip, if set, is told about a contract that is in the list but not in
	// force.
	//
	// Separate from OnRead because a skip is not a reading: there are no figures
	// to mask. What it needs is to be said, since the row this program wrote for
	// that contract is about to be deleted and nothing else would mention it.
	OnSkip func(card manulife.Card)

	// testPages replaces the page operations. Unexported: this is how the loop
	// below is tested, and not a way to configure a run.
	testPages *manulifePages

	// OnRead, if set, is handed each contract's reading as soon as it is taken.
	//
	// It exists so the scheduled job can register the figures with the Actions
	// log masker before anything can print them. Masking after the fact does not
	// work: ::add-mask:: only affects output that comes after it.
	OnRead func(manulife.Reading)
}

// The three page operations, as fields, so the loop over contracts can be
// exercised without a browser.
//
// The loop is where this source's judgements live — which contracts to skip,
// when the browser has to be brought back to the list, whether a premium can be
// attributed — and every one of them was untestable while the calls were
// package functions. Two of the three were wrong.
//
// Defaulted rather than required: a caller that only wants the real thing says
// nothing, and cannot get half of a stand-in by forgetting a field.
type manulifePages struct {
	cards      func(context.Context) ([]manulife.Card, error)
	readCard   func(context.Context, manulife.Card) (manulife.Reading, error)
	backToList func(context.Context) error
}

// pages returns the operations this source drives, real unless overridden.
func (m ManulifeSource) pages() manulifePages {
	if m.testPages != nil {
		return *m.testPages
	}
	return manulifePages{
		cards:      manulife.Cards,
		readCard:   manulife.ReadCard,
		backToList: manulife.BackToList,
	}
}

// ID names this source.
func (m ManulifeSource) ID() string { return ManulifeID }

// SignIn logs in, obtaining a one-time code.
func (m ManulifeSource) SignIn(context.Context) error {
	result, err := m.Client.Login(m.Browser, m.Codes)
	if err != nil {
		if step := manulife.StepOf(err); step != "" {
			return fmt.Errorf("login failed at %s: %w", step, err)
		}
		return fmt.Errorf("login: %w", err)
	}
	if m.OnLogin != nil {
		m.OnLogin(result.OTPRequired)
	}
	return nil
}

// Holdings reads every contract in the list and returns one entry each.
func (m ManulifeSource) Holdings(context.Context) (asset.Holdings, error) {
	pages := m.pages()

	cards, err := pages.cards(m.Browser)
	if err != nil {
		return asset.Holdings{}, err
	}

	// The category is covered whatever the contracts turn out to say, because
	// the list was read — and would not be if the list could not be, since this
	// line would not be reached.
	//
	// That is what lets an ended contract's row be deleted, once there is
	// another contract left to record. A run whose only contract has ended
	// produces no holdings at all, and the empty-read guard upstream stops
	// before anything is deleted: it fails every weekday until somebody removes
	// the row or the source. Safe, and not self-correcting.
	holdings := asset.Holdings{Categories: []string{ManulifeCategory}}

	scheme := assetname.Scheme{Category: ManulifeCategory}
	var names assetname.Set

	// onDetailPage says the browser was navigated away from the list, which is
	// what decides whether it has to be brought back.
	//
	// Tracked rather than inferred from the loop index. "Not the first contract"
	// was standing in for it, and the two part company the moment one is
	// skipped: the run would then go back from a list to whatever preceded it,
	// and wait ninety seconds for cards that are not coming.
	onDetailPage := false

	for _, card := range cards {
		// A contract that has ended still appears in the list. Recording its
		// last surrender value would keep a policy in the portfolio that no
		// longer exists — and reading it costs a page load either way.
		//
		// Said out loud, because a skipped contract is otherwise invisible: the
		// category was covered, so its recorded row is deleted rather than left
		// alone, and nothing else in the run mentions why.
		if !card.InForce() {
			if m.OnSkip != nil {
				m.OnSkip(card)
			}
			continue
		}

		if onDetailPage {
			if err := pages.backToList(m.Browser); err != nil {
				return asset.Holdings{}, err
			}
		}

		// Set before the error check: a read that failed still navigated, and a
		// caller that recovered would otherwise click from a contract page.
		onDetailPage = true
		reading, err := pages.readCard(m.Browser, card)
		if m.OnRead != nil {
			// Before the error check: ReadCard returns what it managed to
			// gather even when it failed, and a failure is exactly when those
			// figures are about to appear in a message.
			m.OnRead(reading)
		}
		if err != nil {
			return asset.Holdings{}, err
		}
		yen, err := reading.Amount()
		if err != nil {
			return asset.Holdings{}, err
		}

		// The cost is one contract's, and there is one of it.
		//
		// A single-premium contract has one historical yen figure, supplied by
		// hand because the site states the premium in a foreign currency only.
		// Two contracts have two, and this configuration cannot express that —
		// so rather than record the same premium against both, which overstates
		// the cost of each and is a plausible figure nobody would question, it
		// stops. The alternative is silence, and the whole point of this program
		// is that a wrong amount is worse than no amount.
		if m.AcquisitionYen != 0 && len(holdings.Assets) > 0 {
			return asset.Holdings{}, fmt.Errorf("%s names one contract's "+
				"premium and this account has more than one in force; there is no way "+
				"to say which it belongs to", acquisitionVariable)
		}

		a := asset.Asset{
			Name:           scheme.For(card.Title),
			Yen:            yen,
			AcquisitionYen: m.AcquisitionYen,
			HasAcquisition: m.AcquisitionYen != 0,
			Kind:           asset.SavingsInsurance,
			Source:         ManulifeID,
		}
		if err := names.Add(a.Name, card.Title); err != nil {
			return asset.Holdings{}, err
		}
		holdings.Assets = append(holdings.Assets, a)
	}
	return holdings, nil
}
