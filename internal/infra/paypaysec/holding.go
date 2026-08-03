package paypaysec

import (
	"context"
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/pagescan"
)

// fillAcquisition looks up each holding's acquisition cost.
//
// MoneyForward derives 評価損益 from the acquisition price, and treats a blank
// one as "cost equals current value" — every holding then reports a profit of
// exactly zero. So the cost has to be supplied, and it has to be right.
//
// Two sources, because the templates differ. A 株 row links to its own page,
// which states the acquisition amount outright; the list would only allow it to
// be derived from a rounded profit figure. A 投資信託 row has no such link, but
// its profit is not rounded, so the subtraction is exact there.
func (r *Reading) fillAcquisition(ctx context.Context) error {
	for i := range r.Holdings {
		h := &r.Holdings[i]
		if !h.HasYen {
			continue
		}
		if err := r.fillHolding(ctx, h); err != nil {
			return fmt.Errorf("%s: %q: %w", r.Target.Key, h.Name, err)
		}
	}
	return nil
}

// fillHolding establishes one holding's cost, by whichever route its template
// allows.
func (r *Reading) fillHolding(ctx context.Context, h *Holding) error {
	if h.Ref == "" {
		// No detail page. The profit on this template is exact, so the
		// subtraction is too.
		gain, ok, err := parseAmountCell(h.GainText)
		if err != nil {
			return fmt.Errorf("profit %q: %w", h.GainText, err)
		}
		if ok {
			h.AcquisitionYen, h.HasAcquisition = h.Yen-gain, true
		}
		return nil
	}

	detail, err := pagescan.LoadHolding(ctx, h.Ref)
	if err != nil {
		return err
	}
	return h.applyDetail(detail)
}

// applyDetail is what the 銘柄's own page means, separated from fetching it.
//
// A method on the holding rather than inline in the caller because this is
// where every silent skip in the acquisition path lived, and none of it was
// reachable by a test while it sat behind a browser.
func (h *Holding) applyDetail(detail pagescan.Detail) error {
	// Am I on the page I asked for? That is what this check is for, and until
	// 2026-08-04 it asked instead whether the two pages agreed on the valuation.
	//
	// They cannot be relied on to. The list figure and the detail figure are
	// fetched seconds apart, and a price that moves in between makes them differ
	// by a few yen — which failed the whole run, twice in one day, for a reason
	// that is not a fault. A moving quantity cannot answer a question about
	// identity.
	//
	// The URL can. A redirect to a different 銘柄 is the failure worth catching,
	// because the acquisition cost below is the only thing this page is
	// consulted for and attributing another holding's cost is silent and wrong.
	if !detail.OnRequestedPage() {
		return fmt.Errorf("asked for %s and landed on %s, so these figures are "+
			"some other 銘柄's", detail.RequestedURL, detail.LandedURL)
	}

	// Still recorded, and still reported when the routes are compared, but no
	// longer a reason to fail on its own. Recorded before anything can return,
	// so the figure reaches the caller on the error path too. See
	// [Reading.Amounts].
	value, ok, verr := parseAmountCell(detail.ValueRaw)
	if verr != nil {
		return fmt.Errorf("its own page's valuation %q: %w", detail.ValueRaw, verr)
	}
	if ok {
		h.DetailYen, h.HasDetailYen = value, true
	}

	// Prefer a stated acquisition amount, but the site only renders that element
	// for some products.
	//
	// A present-but-unparsable one is an error, not a reason to fall through to
	// the subtraction: the two routes should agree, and silently preferring the
	// other one after failing to read this is how a rendering change becomes a
	// wrong 評価損益 rather than a failed run.
	if detail.AcquisitionPresent {
		acq, ok, perr := parseAmountCell(detail.AcquisitionRaw)
		if perr != nil {
			return fmt.Errorf("its own page's acquisition amount %q: %w", detail.AcquisitionRaw, perr)
		}
		if ok {
			h.AcquisitionYen, h.HasAcquisition = acq, true
			return nil
		}
	}

	// The profit figure is always there and is unrounded on the detail page, so
	// the subtraction is exact either way.
	gain, ok, err := parseAmountCell(detail.GainRaw)
	if err != nil {
		return fmt.Errorf("profit %q on its own page: %w", detail.GainRaw, err)
	}
	if ok {
		h.AcquisitionYen, h.HasAcquisition = h.Yen-gain, true
	}
	return nil
}
