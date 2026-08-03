package investapi

import (
	"context"
	"strconv"

	"github.com/samber/lo"
)

// The paths, constants and field names below are the page's own, read out of
// investment_trust/app.*.js rather than guessed. CONFIRMED 2026-08-04.
//
// The two buckets are different major versions of the same three endpoints, which
// is what makes them separable at all: as views of one page they were not.

// origin is the host these endpoints live on. A variable so a test can point the
// client at a stub; nothing else reassigns it.
var origin = "https://www.paypay-sec.co.jp"

func setOrigin(u string) { origin = u }

const (
	// pagePath is the screen these endpoints belong to. Sent as the Referer, and
	// the ミニアプリ bucket will not answer without it: the same body that is
	// accepted from inside the document is refused from a client that does not say
	// which page it is on, with STATUS 9 and システムの不具合 — a message that names
	// nothing. Measured against the live service, that header alone is the
	// difference; a browser User-Agent makes none.
	//
	// Sent on every call, not just the ミニアプリ ones. The v2 endpoints do not ask
	// for it, but one rule is easier to keep true than an exception, and the claim
	// is accurate either way: this program really is on that page, in a browser,
	// borrowing its session.
	pagePath = "/investment_trust/"

	appTop   = "/v2/invest/brand/pc_invest_top"
	appInit  = "/v2/invest/brand/pc_invest_init"
	appInfo  = "/v2/invest/brand/pc_invest_info"
	miniTop  = "/v3/invest/brand/pc_invest_top"
	miniInit = "/v3/invest/brand/pc_invest_init"

	// appIDApp and appIDMiniApp select the bucket. The path already implies it;
	// the field is sent anyway because the page sends it.
	appIDApp     = 3
	appIDMiniApp = 6
)

// commonFields are what every one of these calls carries.
//
// The values are literal in the bundle — "uuid_pc" and "device_token" are the
// strings themselves, not placeholders for something a client generates. Nothing
// here identifies the account; the session cookie does that.
var commonFields = map[string]string{
	"APP_VERSION":  "",
	"UUID":         "uuid_pc",
	"DEVICE_TOKEN": "device_token",
	"OS":           "pc",
}

// endpoints is one bucket's pair of paths.
type endpoints struct {
	top  string
	init string
}

// endpointsFor is the whole of what distinguishes the two buckets on the wire.
//
// Worth being a function rather than two variables at a call site: this is the
// answer to "which bucket am I reading", and when that answer lived in the page it
// was a tab whose state could not be observed.
func endpointsFor(bucket Bucket) endpoints {
	if bucket == MiniApp {
		return endpoints{top: miniTop, init: miniInit}
	}
	return endpoints{top: appTop, init: appInit}
}

// fieldsFor is the form body for one bucket.
//
// The ミニアプリ body needs the account's client number, which is fetched on first
// use; see [Client.miniClientSeqNo].
func (c *Client) fieldsFor(ctx context.Context, bucket Bucket) (map[string]string, error) {
	if bucket == App {
		return lo.Assign(commonFields, map[string]string{
			"APP_ID": strconv.Itoa(appIDApp),
		}), nil
	}

	seq, err := c.miniClientSeqNo(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Assign(commonFields, map[string]string{
		"APP_ID":             strconv.Itoa(appIDMiniApp),
		"MINI_CLIENT_SEQ_NO": seq,
	}), nil
}

// miniInfoFields is the body the page sends when it asks pc_invest_info as the
// ミニアプリ.
//
// The path is the アプリ bucket's, but these fields are not: the page has one
// transport per bucket, each with its own default fields, and it reaches this one
// endpoint through whichever is current. Asking as the アプリ bucket is what it does
// for the アプリ tab, and it is a different question — the ミニアプリ endpoints refuse
// the answer to it outright.
//
// MINI_CLIENT_SEQ_NO is sent empty because the ミニアプリ defaults declare it, so
// every ミニアプリ call carries it, including the call that exists to find out what
// it is.
func miniInfoFields() map[string]string {
	return lo.Assign(commonFields, map[string]string{
		"APP_ID":             strconv.Itoa(appIDMiniApp),
		"MINI_CLIENT_SEQ_NO": "",
	})
}
