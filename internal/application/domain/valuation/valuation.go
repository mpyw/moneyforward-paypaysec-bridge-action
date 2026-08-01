// Package valuation decides whether a set of reported figures may be believed.
//
// The most consequential judgement this program makes, and it is arithmetic
// about money — not about pages, browsers or brokers. It lived in the scraper
// because that is where the figures arrive, which meant the rule and the
// scraping of one particular site could not be reasoned about separately.
//
// Nothing here knows where a figure came from. The caller supplies amounts and
// gets back either a total it may record or a refusal saying which two figures
// disagree, and adds its own context — which page, which element — on the way
// out.
package valuation

import (
	"errors"
	"fmt"
)

// Amount is a figure and how much is known about it.
//
// Three states, because they mean three different things and conflating any two
// of them has already produced a wrong balance here:
//
//   - not reported: the source had no such figure. On a scraped page that
//     usually means the session is not authenticated.
//   - reported, unknown: the source showed a placeholder. A legitimate state for
//     an empty position.
//   - known: there is a number.
type Amount struct {
	Yen int64

	// Reported says the source had this figure at all.
	Reported bool

	// Known says it resolved to a number rather than a placeholder.
	Known bool
}

// Yen returns a known amount.
func Yen(yen int64) Amount { return Amount{Yen: yen, Reported: true, Known: true} }

// Placeholder is a figure the source showed without a value.
func Placeholder() Amount { return Amount{Reported: true} }

// Absent is a figure the source did not have.
func Absent() Amount { return Amount{} }

// Position is one holding's contribution to a total.
type Position struct {
	// Name identifies it in a refusal, which is where anyone will start looking.
	Name string

	// Value is what it is worth now.
	Value Amount

	// Cost is what it cost, which the ledger derives profit from.
	Cost Amount
}

// Labels are what the source calls these figures.
//
// Carried so a refusal can be read against the page it came from: an operator
// looking for why a total was rejected wants the words that are actually on the
// screen. This package still does not know what they mean — they are only ever
// printed.
type Labels struct {
	Total       string
	Acquisition string
	Gain        string
}

// or returns the label, or a neutral word when the source gave none.
func or(label, fallback string) string {
	if label == "" {
		return fallback
	}
	return label
}

// Figures is everything one source said about a set of positions.
type Figures struct {
	// Labels is optional; see [Labels].
	Labels Labels

	// Total is the source's own headline figure.
	Total Amount

	// Acquisition and Gain are the other route to it: by definition they sum to
	// Total, so a disagreement means one of the three was misread.
	Acquisition Amount
	Gain        Amount

	Positions []Position
}

// ErrNotReported is returned when the source had no total at all.
//
// A sentinel because the caller knows what that means in its own terms — for a
// scraped page it is almost always an unauthenticated session — and can say so
// better than this package can.
var ErrNotReported = errors.New("no total was reported")

// Reconciled returns the figure to record, once the routes agree.
//
// It refuses rather than guesses. Recording a wrong balance is worse than
// recording none: the figure lands in a financial history that is then trusted,
// and nothing downstream can tell it was wrong. Every check below exists
// because its absence produced exactly that.
func (f Figures) Reconciled() (int64, error) {
	if !f.Total.Reported {
		return 0, ErrNotReported
	}
	if !f.Total.Known {
		return 0, fmt.Errorf("%s is a placeholder, not an amount", or(f.Labels.Total, "the total"))
	}

	// 投資元本 + 含み益 = 評価額合計 holds by definition.
	if f.Acquisition.Known && f.Gain.Known {
		if sum := f.Acquisition.Yen + f.Gain.Yen; sum != f.Total.Yen {
			return 0, fmt.Errorf(
				"%s is %d but %s %d + %s %d = %d — one of the three is misread",
				or(f.Labels.Total, "the total"), f.Total.Yen,
				or(f.Labels.Acquisition, "cost"), f.Acquisition.Yen,
				or(f.Labels.Gain, "gain"), f.Gain.Yen, sum)
		}
	}

	// A source holding money must say what it holds.
	//
	// Without this, a list that failed to render leaves the total set and no
	// positions — and every check below is guarded on having them, so all of
	// them are skipped and the read succeeds with nothing to record. Downstream
	// that is not an error but a deletion.
	valued := f.valuedPositions()
	if f.Total.Yen != 0 && valued == 0 {
		return 0, fmt.Errorf("%s is %d but nothing was listed under it — the "+
			"positions are missing, not the holdings", or(f.Labels.Total, "the total"), f.Total.Yen)
	}

	// The positions are what gets recorded, so their sum agreeing with the
	// source's own total is the check that matters most: a missed or misread one
	// would otherwise go unnoticed until someone compared the two by hand.
	if valued > 0 {
		if sum := f.valueSum(); sum != f.Total.Yen {
			return 0, fmt.Errorf("%s is %d but its %d holdings sum to %d",
				or(f.Labels.Total, "the total"), f.Total.Yen, valued, sum)
		}
	}

	if err := f.checkCosts(); err != nil {
		return 0, err
	}
	return f.Total.Yen, nil
}

// checkCosts holds the positions' costs against the reported cost basis.
//
// This one matters especially: a cost is gathered per position, often from a
// separate page, and the ledger's own read-back confirms the cost that was sent
// rather than the cost that was right. Nothing downstream catches a wrong one.
func (f Figures) checkCosts() error {
	if !f.Acquisition.Known || len(f.Positions) == 0 {
		return nil
	}
	// One position without a cost used to disable this for all of them, so a
	// single misread page passed unnoticed among others that were fine.
	for _, p := range f.Positions {
		if !p.Cost.Known {
			return fmt.Errorf("%s is %d but %q has no cost, so it cannot be checked "+
				"against the holdings", or(f.Labels.Acquisition, "the cost basis"),
				f.Acquisition.Yen, p.Name)
		}
	}
	var sum int64
	for _, p := range f.Positions {
		sum += p.Cost.Yen
	}
	if sum != f.Acquisition.Yen {
		return fmt.Errorf("%s is %d but its %d holdings' costs sum to %d",
			or(f.Labels.Acquisition, "the cost basis"), f.Acquisition.Yen, len(f.Positions), sum)
	}
	return nil
}

// valuedPositions counts the positions that resolved to a figure.
func (f Figures) valuedPositions() int {
	n := 0
	for _, p := range f.Positions {
		if p.Value.Known {
			n++
		}
	}
	return n
}

// valueSum totals the positions that resolved to a figure.
func (f Figures) valueSum() int64 {
	var sum int64
	for _, p := range f.Positions {
		if p.Value.Known {
			sum += p.Value.Yen
		}
	}
	return sum
}

// Amounts is every yen figure these could name in a message, including the sums
// computed only to report a disagreement.
//
// Here, beside the messages that use them: a figure added to a refusal without
// being added here reaches a log in the clear.
func (f Figures) Amounts() []int64 {
	amounts := []int64{
		f.Total.Yen, f.Acquisition.Yen, f.Gain.Yen,
		f.valueSum(),
		f.Acquisition.Yen + f.Gain.Yen,
	}
	var costs int64
	for _, p := range f.Positions {
		amounts = append(amounts, p.Value.Yen, p.Cost.Yen)
		costs += p.Cost.Yen
	}
	return append(amounts, costs)
}
