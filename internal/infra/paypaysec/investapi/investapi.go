// Package investapi reads the 投資信託 holdings from the endpoints the page
// itself calls, instead of from the page.
//
// The 投資信託 screen puts two buckets behind one URL and swaps between them with
// a tab. Every failure this scraper has had came from that swap: the click is
// client-side and instant, the figures arrive about a second later, and nothing
// in the document distinguishes "this tab's data" from "the previous tab's data
// still on screen". Measured against the live page, the tab's own class and the
// nav's bucket markers flip 8ms after the click while the numbers change at
// ~1000ms.
//
// Three fixes were tried against that gap — waiting for the DOM to hold still,
// waiting a fixed floor, waiting for the tab's own network response — and the
// first two let a category holding two 銘柄 read as empty, twice, and delete
// both. Each fix closed one gap and left the next one.
//
// There is no gap here. A response is the data or it is an error; there is no
// state to observe and no moment at which the wrong bucket is plausible. The two
// buckets are different requests, not different views of one.
//
// The contract is the page's own, read out of its script bundle rather than
// guessed: an unauthenticated GET of /investment_trust/ names the bundle, and the
// bundle names the paths, the constants and where the ミニアプリ bucket's client
// number comes from. The bundle is not the whole contract, though — see [pagePath]
// and [decode.go] for two things the live service insisted on that no amount of
// reading it would have shown.
//
// The files:
//
//	contract.go   paths, constants and request bodies — the bundle's half
//	transport.go  one POST, and the header the ミニアプリ bucket demands
//	response.go   what a reply is, and what must be true before it is read
//	decode.go     the service's refusal to commit to any JSON type
//	account.go    whether a bucket is there to read at all
//	holdings.go   joining what is held to what it is called
//
// The Client, its buckets and its figures are the unit the package is named
// for; every exported name here is read as investapi.X.
//
//declscope:core
package investapi

import (
	"context"
	"net/http"
)

// Bucket is one of the two 投資信託 views.
type Bucket int

const (
	// App is the PayPay 証券 app's own holdings, and MiniApp is the ones held
	// through the mini app inside PayPay.
	App Bucket = iota
	MiniApp
)

// Client reads the 投資信託 endpoints with an authenticated session.
type Client struct {
	// HTTP carries the session cookies. Built from the browser's jar; see
	// [cookiestore.HTTPClientFor].
	HTTP *http.Client

	// miniSeqNo is the ミニアプリ bucket's client number, fetched once on demand.
	miniSeqNo string

	// Trace, when set, is handed every reply before it is judged: the path, the
	// fields sent, and the body verbatim.
	//
	// For the debug command only. The service answers a request it does not like
	// with STATUS 9 and システムの不具合, which names nothing — three releases were
	// spent guessing at that from CI, one guess per run, at a login and a full
	// scrape each. A body in front of a person settled it in one.
	//
	// Never set in the scheduled job: a body here holds the account's balances.
	Trace func(path string, fields map[string]string, body []byte)
}

// Figures is one bucket's holdings and the totals it reports for them.
type Figures struct {
	Total       int64
	Acquisition int64
	Gain        int64
	Holdings    []Holding
}

// Read returns one bucket's holdings and totals.
//
// Returns [ErrNoMiniAppAccount] when the account has no ミニアプリ bucket, which callers
// must tell apart from a failure: it is not one.
func (c *Client) Read(ctx context.Context, bucket Bucket) (Figures, error) {
	fields, err := c.fieldsFor(ctx, bucket)
	if err != nil {
		return Figures{}, err
	}
	paths := endpointsFor(bucket)

	// init before top, which is the order the page loads them in. Nothing here
	// needs the catalogue first, but this is a stateful PHP service and the
	// ミニアプリ bucket has already refused a top call that arrived out of the
	// page's order.
	var catalogue initResponse
	if err := c.post(ctx, paths.init, fields, &catalogue); err != nil {
		return Figures{}, err
	}
	var top topResponse
	if err := c.post(ctx, paths.top, fields, &top); err != nil {
		return Figures{}, err
	}

	holdings, err := nameHoldings(top, catalogue, paths.init)
	if err != nil {
		return Figures{}, err
	}
	return Figures{
		Total:       int64(top.SecuritiesValueTotal),
		Acquisition: int64(top.TotalAcquisitionFeeTaxTotal),
		Gain:        int64(top.SumGrossProfitTotal),
		Holdings:    holdings,
	}, nil
}

// miniClientSeqNo is the account's ミニアプリ client number, asked for once.
//
// The page's own availability test is applied here rather than by the caller,
// because this is the only place the answer is known and the only place a caller
// could act on it too late. What the ミニアプリ endpoints do for an account without
// the bucket is not known — this account has it, and the other case cannot be
// observed from here — so the site's judgement is used as the judgement and the
// bucket is not asked about at all.
func (c *Client) miniClientSeqNo(ctx context.Context) (string, error) {
	if c.miniSeqNo != "" {
		return c.miniSeqNo, nil
	}

	account, err := c.ReadAccountInfo(ctx)
	if err != nil {
		return "", err
	}
	if !account.hasMiniApp() {
		return "", ErrNoMiniAppAccount
	}
	c.miniSeqNo = account.MiniClientSeqNo
	return c.miniSeqNo, nil
}
