package pagescan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec/selector"
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
	if err := chromedp.Run(tctx, chromedp.Evaluate(expr, &detail)); err != nil {
		return detail, fmt.Errorf("extract from %s: %w", url, err)
	}
	return detail, nil
}
