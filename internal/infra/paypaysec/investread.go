package paypaysec

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/cookiestore"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/investapi"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/pagescan"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// readInvestmentTrust reads one 投資信託 bucket over HTTP instead of driving the
// page.
//
// The two buckets share a URL and are swapped with a tab, and every reading
// failure this scraper has had lived in that swap. See [investapi] for why no
// amount of waiting closed it: the click is instant, the figures are not, and
// nothing in the document distinguishes the two states. This reads the requests
// the page itself makes, where a bucket is an endpoint rather than a view.
//
// The result is shaped as a [Reading] so that everything downstream — the
// three-route reconciliation, the masker, the per-target log line — is unchanged.
// What changes is where the numbers came from.
func readInvestmentTrust(ctx context.Context, t selector.Target) (Reading, error) {
	bucket := investapi.App
	if t.Bucket == selector.BucketMiniApp {
		bucket = investapi.MiniApp
	}

	reading := Reading{Target: t}

	api, err := investAPIFor(ctx)
	if err != nil {
		return reading, fmt.Errorf("%s: %w", t.Key, err)
	}
	figures, err := api.Read(ctx, bucket)
	if err != nil {
		return reading, fmt.Errorf("%s: %w", t.Key, err)
	}

	reading.TotalYen, reading.HasTotal = figures.Total, true
	reading.AcquisitionYen, reading.HasAcquisition = figures.Acquisition, true
	reading.GainYen, reading.HasGain = figures.Gain, true

	// The API is unambiguous about there being a list, which the page was not:
	// a rendered section that failed to populate and a category holding nothing
	// arrived identically. An answer with no holdings in it is an empty
	// category, full stop.
	// The raw fields are what the page said, and here nothing said anything —
	// these are the API's numbers written out. Filled rather than left blank
	// because they are what [Reading.Texts] hands the masker, and because the
	// debug table reads them: an amount this program holds should be maskable in
	// every form it can appear in.
	reading.Figures = pagescan.Figures{
		TotalPresent:       true,
		TotalRaw:           yen(figures.Total),
		AcquisitionPresent: true,
		AcquisitionRaw:     yen(figures.Acquisition),
		GainPresent:        true,
		GainRaw:            yen(figures.Gain),
		HoldingsSection:    true,
	}

	names := make(map[string]bool, len(figures.Holdings))
	for _, h := range figures.Holdings {
		// Two brands under one name would collapse into one ledger entry, and
		// the ledger keys on the name. Caught here rather than downstream,
		// because here is where the pair is still visible.
		if names[h.Name] {
			return reading, fmt.Errorf("%s: two holdings are both called %q", t.Key, h.Name)
		}
		names[h.Name] = true

		reading.Holdings = append(reading.Holdings, Holding{
			Name:           h.Name,
			InvestText:     yen(h.Yen),
			Yen:            h.Yen,
			HasYen:         true,
			AcquisitionYen: h.Acquisition,
			HasAcquisition: true,
		})
		reading.HoldingsSumYen += h.Yen
		reading.HoldingsParsed++
	}
	return reading, nil
}

// yen renders an amount as the raw text of a figure that had none.
func yen(n int64) string { return strconv.FormatInt(n, 10) + "円" }

// investAPIFor borrows the browser's session for the HTTP calls.
//
// Per read rather than per run: the only state a client accumulates is the
// ミニアプリ client number, and exactly one target asks for it, so caching it
// across targets saves nothing. Late rather than early, because the borrow reads
// the live cookie jar and a session captured before the login steps is a session
// from before the login steps.
func investAPIFor(ctx context.Context) (*investapi.Client, error) {
	http, err := cookiestore.HTTPClientFor(ctx)
	if err != nil {
		return nil, fmt.Errorf("borrow session for the 投資信託 API: %w", err)
	}
	return &investapi.Client{HTTP: http}, nil
}
