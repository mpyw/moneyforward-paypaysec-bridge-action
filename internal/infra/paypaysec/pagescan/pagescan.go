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

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

const (
	// targetTimeout bounds loading and reading one target, and has to be larger
	// than everything it contains or it becomes the thing that fires.
	//
	//	settle           20s   the figures on arrival
	//	figures API      30s   the tab's own response (tab targets only)
	//	settle           20s   the figures after the swap
	//	navigate/extract  —    seconds
	//
	// An outer deadline firing first replaces a specific complaint — "the page
	// was still loading", "that request did not respond" — with a context
	// deadline, which says nothing about which wait was the problem.
	targetTimeout = 75 * time.Second

	// tabSettleDelay gives a tab's panel a moment to swap in. The tabs are
	// client-side, so there is no navigation to wait on.
	tabSettleDelay = 750 * time.Millisecond

	// settleTimeout bounds waiting for the figures to finish loading.
	settleTimeout = 20 * time.Second

	// settlePoll is the cadence of that wait, and stableReads is how many
	// consecutive identical readings count as settled.
	settlePoll = 400 * time.Millisecond

	// figuresTimeout bounds the wait for a tab's own figures request, and
	// figuresPoll is how often the watch is checked. Measured: the response lands
	// about a second after the click.
	//
	// Generous, because exceeding it is now an error rather than a silent read of
	// the wrong bucket. A slow runner should make the job late, not wrong.
	figuresTimeout = 30 * time.Second
	figuresPoll    = 200 * time.Millisecond
	stableReads    = 3
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

// Load navigates to one target, activates its tab if it has one, waits for the
// figures to settle, and reads them off.
func Load(ctx context.Context, t selector.Target) (Figures, error) {
	tctx, cancel := context.WithTimeout(ctx, targetTimeout)
	defer cancel()

	page := targetPage{ctx: tctx, target: t}

	var figures Figures

	// Armed before navigating, not before the click.
	//
	// The default tab's request happens during the page's own load, and clicking
	// the tab that is already active is a no-op — the framework has no state
	// change to react to, so nothing is fetched. Watching from the click onwards
	// therefore waited forever for 投資信託（アプリ）, whose figures had already
	// arrived.
	//
	// Listening from before the navigation cannot pick up the wrong tab's answer
	// either, because the two tabs call different paths and each target waits for
	// its own.
	var watch *apiWatch
	if t.FiguresAPI != "" {
		if err := chromedp.Run(tctx, network.Enable()); err != nil {
			return figures, fmt.Errorf("%s: enable network events: %w", t.Key, err)
		}
		watch = watchFiguresAPI(tctx, t.FiguresAPI)
	}

	if err := chromedp.Run(tctx,
		chromedp.Navigate(t.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return figures, fmt.Errorf("%s: load %s: %w", t.Key, t.URL, err)
	}

	if err := page.settle(); err != nil {
		return figures, fmt.Errorf("%s: %w", t.Key, err)
	}

	if t.TabLabel != "" {
		if err := page.selectTab(); err != nil {
			return figures, err
		}
		if err := page.awaitFigures(watch); err != nil {
			return figures, fmt.Errorf("%s: after tab %q: %w", t.Key, t.TabLabel, err)
		}
		// The response arriving is not the same as the page having rendered it.
		if err := page.settle(); err != nil {
			return figures, fmt.Errorf("%s: after tab %q: %w", t.Key, t.TabLabel, err)
		}
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
// A type so that settling and tab selection are methods on the page they act
// on. As package-level functions taking a ctx they read as though they applied
// to any page, including a 銘柄 detail page, where selecting a tab is meaningless.
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

// awaitFigures waits for this tab's own figures to arrive.
//
// An error when they do not, which is the whole change. The previous version
// waited a while and then read whatever was on screen; on a slow runner that was
// the other bucket's data, and an empty category is a plan to delete everything
// in it. The request fires whether or not the two tabs hold the same numbers, so
// not seeing it means something is wrong — not that there was nothing to see.
func (p targetPage) awaitFigures(watch *apiWatch) error {
	deadline := time.Now().Add(figuresTimeout)
	for {
		if watch.arrived() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not respond within %s, so the figures on screen "+
				"are still the previously selected tab's",
				p.target.FiguresAPI, figuresTimeout)
		}
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		case <-time.After(figuresPoll):
		}
	}
}

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

// tabClick is what select_tab.js reports back.
type tabClick struct {
	Clicked   bool     `json:"clicked"`
	Active    string   `json:"active"`
	Available []string `json:"available"`
}

// activeTab is what active_tab.js reports back.
//
// Not the same type as [tabClick], though it was once: that script attempts no
// click, so it has nothing to say about one, and a shared Clicked field decoded
// to false on every confirmation. Nothing read it, which is exactly the problem
// — the type claimed an answer the page never gave.
type activeTab struct {
	Active    string   `json:"active"`
	Available []string `json:"available"`
}

// selectTab activates the target's tab and verifies it took.
//
// 投資信託 puts both buckets at one URL, so without this the same figure would
// be read twice and one bucket silently double-counted — the kind of error that
// produces a believable total rather than a failure.
func (p targetPage) selectTab() error {
	t := p.target

	expr, err := selector.SelectTab(t.TabLabel)
	if err != nil {
		return fmt.Errorf("%s: build tab script: %w", t.Key, err)
	}

	var res tabClick
	if err := chromedp.Run(p.ctx, chromedp.Evaluate(expr, &res)); err != nil {
		return fmt.Errorf("%s: select tab %q: %w", t.Key, t.TabLabel, err)
	}
	if !res.Clicked {
		return fmt.Errorf("%s: no tab labelled %q under %s (found %v)",
			t.Key, t.TabLabel, selector.TabMenu, res.Available)
	}

	// The click is client-side, so there is no navigation to wait on. Give the
	// panel a moment, then confirm the intended tab really is the active one:
	// reading the wrong bucket is worse than failing.
	if err := chromedp.Run(p.ctx, chromedp.Sleep(tabSettleDelay)); err != nil {
		return err
	}

	confirmExpr, err := selector.ActiveTab()
	if err != nil {
		return fmt.Errorf("%s: build tab-confirm script: %w", t.Key, err)
	}
	var after activeTab
	if err := chromedp.Run(p.ctx, chromedp.Evaluate(confirmExpr, &after)); err != nil {
		return fmt.Errorf("%s: confirm active tab: %w", t.Key, err)
	}
	if after.Active != t.TabLabel {
		return fmt.Errorf("%s: clicked %q but %q is active", t.Key, t.TabLabel, after.Active)
	}
	return nil
}
