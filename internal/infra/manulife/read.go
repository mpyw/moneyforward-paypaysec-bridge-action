package manulife

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/money"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
)

// One budget per wait, rather than one around all of them.
//
// A single deadline over "click, then navigate, then extract" expires without
// saying which of the three it was waiting for, or where the browser ended up —
// it arrives as a bare context deadline exceeded. This project has been here
// before, on the PayPay 証券 side, where one 30-second budget wrapped two
// 20-second settles.
//
// Their sum stays well under the job's own deadline, so the inner wait is
// always the one that expires and always the one that gets to explain itself.
const (
	// listTimeout covers waiting for the contract list and reading it.
	listTimeout = 30 * time.Second

	// readyTimeout covers waiting for the page's own click handler to exist.
	// Its scripts load after the cards do; see [selector.ContractOpenerReady].
	readyTimeout = 30 * time.Second

	// clickTimeout covers finding the card and clicking it.
	clickTimeout = 20 * time.Second

	// navigateTimeout covers the detail page arriving after that click. The
	// site posts back, redirects, and renders through an Akamai edge, so this is
	// the slow one.
	navigateTimeout = 90 * time.Second

	// extractTimeout covers reading the detail page once it is up.
	extractTimeout = 30 * time.Second
)

// Pair is one labelled value, exactly as the page wrote both.
//
// Labels are kept unnormalised on purpose. This site punctuates the same label
// three ways across two pages, and normalising at the edge would hide that from
// the code that has to cope with it; [selector.TrimLabel] does it once, where a
// test can hold it to the observed forms.
type Pair struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Card is one contract as the list shows it.
type Card struct {
	Title  string `json:"title"`
	Fields []Pair `json:"fields"`
}

// Number is the 種類-証券番号 the card states.
//
// The identity a detail page is checked against, because it is the one thing
// both pages carry. The product name is not: the list gives a brand name and
// the detail page gives a policy type, and neither would tell two contracts of
// the same product apart.
func (c Card) Number() (string, error) { return field(c.Fields, selector.LabelPolicyNumber) }

// InForce reports whether the contract is still running.
//
// Read rather than assumed: a lapsed or paid-out contract should stop being
// recorded, and this is the only place the site says so. An unrecognised status
// is not treated as in force — the list is where that judgement is cheapest.
func (c Card) InForce() bool {
	status, err := field(c.Fields, selector.LabelStatus)
	return err == nil && status == selector.StatusInForce
}

// contractList is what the page script reports for the list page.
type contractList struct {
	Cards []Card `json:"cards"`
}

// policyPage is what the page script reports for a detail page.
type policyPage struct {
	Summary []Pair `json:"summary"`
	Rows    []Pair `json:"rows"`
}

// Cards reads the contract list off the page the browser is already on.
//
// It does not navigate, and that is the point. A completed sign-in lands on the
// list — [Client.Login] waits for a card before it returns — and asking the site
// for /Home again from there produced an error page instead. This is a stateful
// Visualforce application reached through an Akamai edge; the previous
// rendering is not something it will simply hand out a second time.
//
// It is also the rule this package already follows everywhere else. A contract
// has no address, so the way through this site is to click what is on the
// screen. Fetching a URL directly was the one place that rule was broken, and
// it broke immediately.
func Cards(ctx context.Context) ([]Card, error) {
	js, err := selector.ExtractContracts()
	if err != nil {
		return nil, stepErr(StepReadList, err)
	}

	tctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	if err := chromedp.Run(tctx,
		chromedp.WaitVisible(selector.ContractCard, chromedp.ByQuery),
	); err != nil {
		return nil, stepErr(StepReadList, browser.PageOf(tctx).WithLocation(
			fmt.Errorf("waiting for the contract list: %w", err)))
	}

	var list contractList
	if err := chromedp.Run(tctx, chromedp.Evaluate(js, &list)); err != nil {
		return nil, stepErr(StepReadList, err)
	}
	if len(list.Cards) == 0 {
		// Not "no contracts": the wait above found a card, so an empty result
		// means the extraction and the page disagree about what a card is.
		return nil, stepErr(StepReadList, errors.New(
			"the contract list rendered but no card could be read — the page's markup "+
				"has moved away from the selectors"))
	}
	return list.Cards, nil
}

// BackToList returns to the contract list from a contract's detail page.
//
// Through the browser's own history rather than by fetching /Home, for the
// reason [Cards] gives: this site does not answer a direct request for a page it
// has already rendered. History replays the navigation the site itself
// performed.
//
// Only ever needed between contracts, and the account this was built against
// holds one — so unlike everything else here, this path has not been run. It is
// written the careful way rather than the convenient one for that reason.
func BackToList(ctx context.Context) error {
	tctx, cancel := context.WithTimeout(ctx, navigateTimeout)
	defer cancel()
	if err := chromedp.Run(tctx,
		chromedp.NavigateBack(),
		chromedp.WaitVisible(selector.ContractCard, chromedp.ByQuery),
	); err != nil {
		return stepErr(StepReadList, browser.PageOf(tctx).WithLocation(
			fmt.Errorf("going back to the contract list: %w", err)))
	}
	return nil
}

// Reading is everything one contract's detail page offered, plus what Go made
// of it.
//
// The text fields are kept beside the numbers so a recorded figure can be
// traced back to the words it came from, and so a parse failure can quote them.
type Reading struct {
	// Card is the list entry this reading was reached through.
	Card Card

	// Number is the 種類-証券番号 both pages agreed on.
	Number string

	// PolicyType is 保険種類, the kind of insurance as the detail page names it.
	PolicyType string

	// The three figures, as the page wrote them.
	YenText  string
	FCYText  string
	RateText string

	// Parsed. Yen is what gets recorded; the other two exist to check it.
	Yen    int64
	HasYen bool

	// FCYHundredths is the contract-currency amount in hundredths of its unit,
	// RateHundredths the stated conversion rate in hundredths of a yen.
	FCYHundredths  int64
	HasFCY         bool
	RateHundredths int64
	HasRate        bool
}

// Amounts is every yen figure this reading could name in a message.
//
// Beside the messages that use them: a figure added to a refusal without being
// added here reaches a log in the clear.
func (r Reading) Amounts() []int64 {
	lo, hi := money.ConvertedYenRange(r.FCYHundredths, r.RateHundredths)
	return []int64{r.Yen, r.FCYHundredths, lo, hi}
}

// Texts is every figure as a message here could spell it.
//
// The page's own words, and the decimal forms the reconciliation quotes. Those
// are a third spelling of the same amounts: [Reading.Amounts] registers
// 1234567, the page says "12,345.67 米ドル", and a refusal says "12345.67" — a
// value masked in two spellings is unmasked in the third.
func (r Reading) Texts() []string {
	return []string{
		r.YenText, r.FCYText, r.RateText,
		money.FormatHundredths(r.FCYHundredths),
		money.FormatHundredths(r.RateHundredths),
	}
}

// Amount is the figure to record, once the two routes agree.
//
// There is only one yen figure on this page — 解約時お支払金額（円支払）— because
// the contract is denominated in a foreign currency and nothing else on it is
// converted. So the cross-check cannot be a second yen figure; it is the
// contract-currency amount and the rate the page states beside it, which the
// page itself says the yen figure was computed from.
//
// That makes this the same kind of check as 投資元本 + 含み益 = 評価額合計 on the
// PayPay 証券 side: the source checking itself, not this program supplying a
// rate it has no way to verify. A rate fetched from anywhere else would be a
// number with nothing to hold it against.
func (r Reading) Amount() (int64, error) {
	if !r.HasYen {
		return 0, fmt.Errorf("%s was not found on the contract page — the session is "+
			"probably not authenticated, or the contract is not a foreign-currency one",
			selector.LabelSurrenderYen)
	}
	if !r.HasFCY || !r.HasRate {
		// Refused rather than accepted unchecked. The figure is about to be
		// written into a financial record, and an unverified one there is
		// indistinguishable from a correct one.
		return 0, fmt.Errorf("%s was read but %s and %s were not, so nothing checks it",
			selector.LabelSurrenderYen, selector.LabelSurrenderFCY, selector.LabelRate)
	}
	if err := money.AgreesWithConversion(r.Yen, r.FCYHundredths, r.RateHundredths); err != nil {
		return 0, fmt.Errorf("%s and %s disagree: %w",
			selector.LabelSurrenderYen, selector.LabelSurrenderFCY, err)
	}
	return r.Yen, nil
}

// ReadCard opens one contract from the list and reads its figures.
//
// It navigates by clicking, because a contract has no address: the detail page
// is reached with a token minted for this rendering of the list. Which is also
// why the page that opens is checked against the card that was clicked — the
// URL cannot say which contract it is showing, so the page has to.
func ReadCard(ctx context.Context, card Card) (Reading, error) {
	reading := Reading{Card: card}

	wanted, err := card.Number()
	if err != nil {
		return reading, stepErr(StepOpenContract, fmt.Errorf("the list card for %q: %w", card.Title, err))
	}

	js, err := selector.ExtractPolicy()
	if err != nil {
		return reading, stepErr(StepReadContract, err)
	}

	// The handler first, on its own budget. A card is server-rendered and its
	// opener is not, so a visible card is not a card that can be opened — and
	// clicking one that cannot be says nothing at all.
	//
	// A step rather than something inside the click, because a budget nested
	// inside a shorter one is a budget that never applies: this wait was 30
	// seconds inside a 20-second click, so it was 20, and its own reason for
	// being 30 was silently discarded.
	if err := withTimeout(ctx, readyTimeout, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Poll(selector.ContractOpenerReady, nil))
	}); err != nil {
		return reading, stepErr(StepOpenContract, browser.PageOf(ctx).WithLocation(
			fmt.Errorf("waiting for the list's own click handler to be defined: %w", err)))
	}

	// The card is addressed by its number rather than by position. The list is
	// re-rendered on every visit, and a positional click would open whichever
	// contract had moved into that slot — silently, since the page that opens
	// looks like a contract either way.
	if err := withTimeout(ctx, clickTimeout, func(ctx context.Context) error {
		return clickCardFor(ctx, wanted)
	}); err != nil {
		return reading, stepErr(StepOpenContract, browser.PageOf(ctx).WithLocation(
			fmt.Errorf("clicking the contract's card: %w", err)))
	}

	// The slow one, and on its own so that a slow navigation is reported as a
	// slow navigation. The click posts back, redirects and renders through an
	// edge that sometimes has opinions.
	if err := withTimeout(ctx, navigateTimeout, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.WaitVisible(selector.PolicySummary, chromedp.ByQuery))
	}); err != nil {
		return reading, stepErr(StepOpenContract, browser.PageOf(ctx).WithLocation(
			fmt.Errorf("waiting for the contract page after clicking its card: %w", err)))
	}

	var page policyPage
	if err := withTimeout(ctx, extractTimeout, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Evaluate(js, &page))
	}); err != nil {
		return reading, stepErr(StepReadContract, browser.PageOf(ctx).WithLocation(
			fmt.Errorf("reading the contract page: %w", err)))
	}

	got, err := field(page.Summary, selector.LabelPolicyNumber)
	if err != nil {
		return reading, stepErr(StepReadContract, fmt.Errorf("the contract page: %w", err))
	}
	if got != wanted {
		// Not a mismatch to report and carry on from: every figure below would
		// be recorded against the wrong contract.
		return reading, stepErr(StepReadContract, errors.New(
			"the contract page that opened is not the one that was clicked — its "+
				"種類-証券番号 differs from the card's"))
	}
	reading.Number = got

	if reading.PolicyType, err = field(page.Rows, selector.DetailPolicyType); err != nil {
		return reading, stepErr(StepReadContract, fmt.Errorf("the contract page: %w", err))
	}
	if err := reading.readFigures(page.Rows); err != nil {
		return reading, stepErr(StepReadContract, err)
	}
	return reading, nil
}

// readFigures pulls the three figures out and parses them.
//
// A label that is absent is not an error here — [Reading.Amount] decides what
// its absence means, and it can say something more useful than this can.
func (r *Reading) readFigures(rows []Pair) error {
	var err error
	if r.YenText, err = optionalField(rows, selector.LabelSurrenderYen); err != nil {
		return err
	}
	if r.FCYText, err = optionalField(rows, selector.LabelSurrenderFCY); err != nil {
		return err
	}
	if r.RateText, err = optionalField(rows, selector.LabelRate); err != nil {
		return err
	}

	if r.YenText != "" {
		yen, perr := money.ParseYen(r.YenText)
		switch {
		case errors.Is(perr, money.ErrNoValue):
		case perr != nil:
			return fmt.Errorf("%s: %w", selector.LabelSurrenderYen, perr)
		default:
			r.Yen, r.HasYen = yen, true
		}
	}
	if r.FCYText != "" {
		amount, perr := parseForeignAmount(r.FCYText)
		switch {
		case errors.Is(perr, money.ErrNoValue):
		case perr != nil:
			return fmt.Errorf("%s: %w", selector.LabelSurrenderFCY, perr)
		default:
			r.FCYHundredths, r.HasFCY = amount, true
		}
	}
	if r.RateText != "" {
		rate, perr := parseRate(r.RateText)
		switch {
		case errors.Is(perr, money.ErrNoValue):
		case perr != nil:
			return fmt.Errorf("%s: %w", selector.LabelRate, perr)
		default:
			r.RateHundredths, r.HasRate = rate, true
		}
	}
	return nil
}

// parseForeignAmount reads "12,345.67 米ドル" as hundredths of the unit.
//
// The currency is dropped rather than checked, because nothing downstream does
// anything with it: what makes the figure meaningful is the rate stated beside
// it, and that rate names the same currency. Checking the name here would be
// checking the page against itself.
func parseForeignAmount(s string) (int64, error) {
	amount, _, ok := strings.Cut(strings.TrimSpace(s), " ")
	if !ok {
		return 0, fmt.Errorf("parse %q: expected an amount and a currency", s)
	}
	return money.ParseHundredths(amount)
}

// parseRate reads "1 米ドル=150.25 円" as hundredths of a yen.
//
// The leading count is checked rather than skipped. A rate quoted per hundred
// units is a normal way to write one for a weak currency, and reading "100
// テスト=1.50 円" as 1.50 per unit would understate the holding by a factor of a
// hundred — a wrong figure that looks entirely plausible.
func parseRate(s string) (int64, error) {
	left, right, ok := strings.Cut(s, "=")
	if !ok {
		return 0, fmt.Errorf("parse rate %q: expected <count> <currency>=<amount> 円", s)
	}
	count, _, ok := strings.Cut(strings.TrimSpace(left), " ")
	if !ok {
		return 0, fmt.Errorf("parse rate %q: expected a count and a currency before the =", s)
	}
	if count != "1" {
		return 0, fmt.Errorf("parse rate %q: quoted per %s units, and this reads it "+
			"as a rate per unit", s, count)
	}
	yen, _, ok := strings.Cut(strings.TrimSpace(right), "円")
	if !ok {
		return 0, fmt.Errorf("parse rate %q: expected 円 after the amount", s)
	}
	return money.ParseHundredths(yen)
}

// withTimeout bounds one wait. Deriving a sub-context cancels the actions, not
// the browser, so the caller's Chrome stays alive for a page dump afterwards.
func withTimeout(ctx context.Context, d time.Duration, fn func(context.Context) error) error {
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return fn(tctx)
}

// clickCardFor opens the contract with this number, through the page's own
// click handler.
//
// Two other ways were tried against the live site and both did nothing while
// reporting success — see mark_contract.js for which and why. This is the one
// that has been observed to work, and it was replaced twice on reasoning rather
// than on evidence.
//
// The budgets around it are what changed for good: a failure here now names the
// wait it was in and the page the browser was on, so the next one will not have
// to be reasoned about at all.
func clickCardFor(ctx context.Context, number string) error {
	js, err := selector.MarkContract(number)
	if err != nil {
		return err
	}
	var res struct {
		Matched int `json:"matched"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &res)); err != nil {
		return fmt.Errorf("looking for the contract in the list: %w", err)
	}
	switch {
	case res.Matched == 0:
		return errors.New("the contract is no longer in the list under the number the " +
			"card gave for it")
	case res.Matched > 1:
		return fmt.Errorf("%d cards carry the same 種類-証券番号, so it does not identify "+
			"a contract", res.Matched)
	}

	return nil
}

// field returns the one value under label, refusing anything else.
//
// Refusing a second match is the point. This page carries sections for products
// the customer does not hold, and 積立金額 appears in more than one of them —
// so a lookup that took the first would be reporting whichever section the
// markup happened to put first. The extraction already drops what the browser
// is not showing; this catches the case where two are.
func field(pairs []Pair, label string) (string, error) {
	value, err := optionalField(pairs, label)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("no visible row labelled %q", label)
	}
	return value, nil
}

// optionalField is [field] but treats absence as an empty string.
func optionalField(pairs []Pair, label string) (string, error) {
	var found string
	var count int
	for _, p := range pairs {
		if selector.TrimLabel(p.Label) != label {
			continue
		}
		count++
		found = p.Value
	}
	if count > 1 {
		return "", fmt.Errorf("%d visible rows are labelled %q, so there is no way to "+
			"tell which contract's figure this is", count, label)
	}
	return found, nil
}
