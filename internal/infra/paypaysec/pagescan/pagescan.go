// Package pagescan drives PayPay 証券's pages and reports what they said.
//
// It returns text, never numbers. Turning "78万9012円" into 789012 and
// checking that figure against the page's other two routes happens in the
// parent package. Keeping the two apart also keeps chromedp, the settle timings
// and the JS mirror structs out of a namespace where a reconciliation function
// could reach for them.
//
// The scripts themselves are driven by script_test.go, against fixture pages in
// a real browser. That was long assumed impossible — the wire checks here settle
// for asserting a script mentions the keys it returns, which cannot notice a
// renamed "name" or "ref" — and it is not.
//
// Everything here was CONFIRMED against the live site on 2026-08-01.
package pagescan

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

const (
	// targetTimeout bounds loading and reading one target, and has to be larger
	// than everything it contains or it becomes the thing that fires.
	//
	//	settle           20s   the figures on arrival
	//	navigate/extract  —    seconds
	//
	// An outer deadline firing first replaces a specific complaint — "the page
	// was still loading" — with a context deadline, which says nothing about
	// which wait was the problem.
	targetTimeout = 45 * time.Second

	// settleTimeout bounds waiting for the figures to finish loading.
	settleTimeout = 20 * time.Second

	// settlePoll is the cadence of that wait, and stableReads is how many
	// consecutive identical readings count as settled.
	settlePoll = 400 * time.Millisecond

	stableReads = 3
)

// Row is one 銘柄 as the page listed it, in the page's own words.
type Row struct {
	// Name is the 銘柄 as the site labels it, e.g. "テスト電機".
	Name string `json:"name"`

	// Ref is the site's own link for the holding, "/trade/brand/35/0" — present
	// on the 株 template, empty on 投資信託. It decides which of the two ways of
	// establishing an acquisition cost applies; see [LoadHolding].
	Ref string `json:"ref"`

	InvestText string `json:"investText"`

	// GainText is the profit as shown. On the 株 template it is abbreviated
	// ("+3.7万"), which is why that template's cost is taken from the detail
	// page instead of by subtracting this.
	GainText string `json:"gainText"`
}

// Figures is everything one target's page offered, all of it as text.
//
// The *Present flags say whether the element existed at all, which is a
// different thing from it being empty: an absent 評価額合計 means the session is
// not authenticated, while an empty one is a placeholder.
type Figures struct {
	TotalPresent       bool   `json:"totalPresent"`
	TotalRaw           string `json:"totalRaw"`
	AcquisitionPresent bool   `json:"acquisitionPresent"`
	AcquisitionRaw     string `json:"acquisitionRaw"`
	GainPresent        bool   `json:"gainPresent"`
	GainRaw            string `json:"gainRaw"`

	// HoldingsSection says the page had a 保有銘柄 section, whether or not it
	// listed anything.
	//
	// Distinct from an empty Holdings, because a page that did not render its
	// section and a category that holds nothing arrive identically otherwise —
	// and the first, paired with a zero total, satisfies every check there is.
	HoldingsSection bool `json:"holdingsSection"`

	// Holdings is every 銘柄 row on the page.
	Holdings []Row `json:"holdings"`
}

// Load navigates to one target, waits for the figures to settle, and reads them
// off.
//
// No tab handling. 投資信託 was the only screen that needed it, and it is read
// through its own endpoints now — see [investapi]. Every failure this scraper had
// lived in that click, so the machinery went with it rather than staying around
// for a caller that no longer exists.
func Load(ctx context.Context, t selector.Target) (Figures, error) {
	tctx, cancel := context.WithTimeout(ctx, targetTimeout)
	defer cancel()

	page := targetPage{ctx: tctx, target: t}

	var figures Figures
	if err := chromedp.Run(tctx,
		chromedp.Navigate(t.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return figures, fmt.Errorf("%s: load %s: %w", t.Key, t.URL, err)
	}

	if err := page.settle(); err != nil {
		return figures, fmt.Errorf("%s: %w", t.Key, err)
	}

	expr, err := selector.ExtractBalance()
	if err != nil {
		return figures, fmt.Errorf("%s: build extraction script: %w", t.Key, err)
	}
	if err := chromedp.Run(tctx, chromedp.Evaluate(expr, &figures)); err != nil {
		return figures, fmt.Errorf("%s: extract: %w", t.Key, err)
	}
	return figures, nil
}

// targetPage is one target's page in one browser context.
//
// A type so that settling is a method on the page it acts on. As a
// package-level function taking a ctx it reads as though it applied to any page,
// where in fact it watches the element this target's figure lives in.
type targetPage struct {
	ctx    context.Context
	target selector.Target
}

// pageState mirrors page_state.js.
type pageState struct {
	Loading bool   `json:"loading"`
	Present bool   `json:"present"`
	Text    string `json:"text"`
}

// settle blocks until the page's total has stopped changing.
//
// Waiting for the document alone is not enough: the 投資信託 view fetches its
// numbers afterwards and shows 0円 until they arrive. That placeholder parses
// cleanly, so an early read produces a confident zero for an account that holds
// real money — the worst kind of failure this program can have.
func (p targetPage) settle() error { return settle(p.ctx, selector.ValueTotal) }

// settle is the wait itself, told which element carries the figure to wait for.
func settle(ctx context.Context, value string) error {
	expr, err := selector.PageState(value)
	if err != nil {
		return fmt.Errorf("build page-state script: %w", err)
	}

	deadline := time.Now().Add(settleTimeout)
	var last string
	var stable int
	var latest pageState
	for {
		var st pageState
		if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &st)); err != nil {
			return fmt.Errorf("probe page state: %w", err)
		}
		latest = st

		switch {
		case st.Loading || !st.Present:
			stable = 0
		case st.Text == last:
			stable++
			if stable >= stableReads {
				return nil
			}
		default:
			stable = 1
		}
		last = st.Text

		if time.Now().After(deadline) {
			return settleTimedOut(latest)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(settlePoll):
		}
	}
}

// settleTimedOut decides whether running out of time was a failure.
//
// It used to be waved through, on the reasoning that the caller cross-checks the
// figures against each other anyway. That reasoning does not hold. While the
// 投資信託 view is fetching, the total, 投資元本, 含み益 and every row all read
// 0円 — a state that is internally consistent, so every cross-check passes and
// the caller records real holdings as zero.
//
// A page that never had the element is a different matter, and left alone: the
// read has its own, better-worded complaint about not being authenticated.
func settleTimedOut(last pageState) error {
	switch {
	case last.Loading:
		return fmt.Errorf("the page was still loading after %s; anything read now is "+
			"a placeholder, not an amount", settleTimeout)
	case last.Present:
		// Deliberately not quoting the value: this message reaches a workflow
		// log, and the value is a balance.
		return fmt.Errorf("the figures were still changing after %s, so no reading "+
			"of them is trustworthy", settleTimeout)
	default:
		return nil
	}
}
