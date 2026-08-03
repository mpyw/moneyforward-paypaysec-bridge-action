package pagescan

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// holdingTimeout bounds loading one 銘柄's detail page.
const holdingTimeout = 30 * time.Second

// Detail is one 銘柄's own page, as text. Mirrors extract_holding.js.
//
// AcquisitionPresent is false for products where the site does not render that
// element at all — the cost then has to come from subtracting the profit, which
// is unrounded here even though the list abbreviates it.
type Detail struct {
	ValuePresent       bool   `json:"valuePresent"`
	ValueRaw           string `json:"valueRaw"`
	AcquisitionPresent bool   `json:"acquisitionPresent"`
	AcquisitionRaw     string `json:"acquisitionRaw"`
	GainRaw            string `json:"gainRaw"`

	// LandedURL is where the browser ended up, which is not always where it was
	// sent. Filled by [LoadHolding]; not part of the page script's output.
	LandedURL string `json:"-"`

	// RequestedURL is where it was sent.
	RequestedURL string `json:"-"`
}

// OnRequestedPage says whether the browser is on the page it was asked for.
//
// Compared by path, because a redirect to a different 銘柄 is the failure this
// exists for and a query string is not part of that. A mismatch means the
// figures below describe something other than the holding they were fetched
// for — most importantly the acquisition cost, which is the only thing the
// detail page is actually consulted for.
func (d Detail) OnRequestedPage() bool {
	landed, err := neturl.Parse(d.LandedURL)
	if err != nil {
		return false
	}
	requested, err := neturl.Parse(d.RequestedURL)
	if err != nil {
		return false
	}
	return landed.Path == requested.Path
}

// LoadHolding loads one 銘柄's page, by the ref its row linked to.
func LoadHolding(ctx context.Context, ref string) (Detail, error) {
	tctx, cancel := context.WithTimeout(ctx, holdingTimeout)
	defer cancel()

	expr, err := selector.ExtractHolding()
	if err != nil {
		return Detail{}, err
	}

	url := ref
	if strings.HasPrefix(ref, "/") {
		url = selector.Origin + ref
	}

	var detail Detail
	if err := browser.PageOf(tctx).Open(url); err != nil {
		return detail, err
	}
	// This page's own figure, not the list page's total.
	if err := settle(tctx, selector.HoldingValue); err != nil {
		return detail, err
	}
	var landed string
	if err := chromedp.Run(tctx,
		chromedp.Evaluate(expr, &detail),
		chromedp.Location(&landed),
	); err != nil {
		return detail, fmt.Errorf("extract from %s: %w", url, err)
	}
	detail.RequestedURL, detail.LandedURL = url, landed
	return detail, nil
}
