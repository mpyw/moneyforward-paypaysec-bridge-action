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
// The contract below is the page's own, read out of its script bundle rather
// than guessed: an unauthenticated GET of /investment_trust/ names the bundle,
// and the bundle names the paths, the constants and where the mini bucket's
// client number comes from.
package investapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Bucket is one of the two 投資信託 views.
type Bucket int

const (
	// App is the PayPay 証券 app's own holdings, and MiniApp is the ones held
	// through the mini app inside PayPay.
	App Bucket = iota
	MiniApp
)

// The paths and constants the page uses. CONFIRMED 2026-08-04 from
// investment_trust/app.*.js.
//
// The mini bucket is a different major version of the same three endpoints,
// which is why watching the network could tell them apart at all.
// origin is the host these endpoints live on. A variable so a test can point
// the client at a stub; nothing else reassigns it.
var origin = "https://www.paypay-sec.co.jp"

func setOrigin(u string) { origin = u }

const (
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

// Client reads the 投資信託 endpoints with an authenticated session.
type Client struct {
	// HTTP carries the session cookies. Built from the browser's jar; see
	// [cookiestore.Store.HTTPClient].
	HTTP *http.Client

	// miniSeqNo is the mini bucket's client number, fetched once on demand.
	miniSeqNo string
}

// Holding is one 銘柄 as the API reports it.
//
// Acquisition is derived, not fetched: the per-brand payload carries the current
// value and the unrealised gain, and the cost is the difference. Exact integer
// arithmetic, where the page had it rounded to one decimal place in 万.
type Holding struct {
	BrandID     int
	Name        string
	Yen         int64
	Acquisition int64
}

// Figures is one bucket's holdings and the totals it reports for them.
type Figures struct {
	Total       int64
	Acquisition int64
	Gain        int64
	Holdings    []Holding
}

// envelope is what every one of these replies carries, and what has to be true
// before anything else in it is worth reading.
//
// Both fields are checked because the page checks both, and because of what the
// second one costs to miss. A signed-out session answers with LOGIN_STATUS 1 and
// no holdings — which, taken at face value, is a category that emptied, and this
// program deletes those. An expired cookie must not be able to look like a sale.
type envelope struct {
	// Status is 0 on success. Anything else is an error, described in Messages.
	Status int `json:"STATUS"`

	// LoginStatus is 1 when the session is no longer signed in.
	LoginStatus int `json:"LOGIN_STATUS"`

	Messages []struct {
		Message string `json:"MESSAGE"`
	} `json:"MESSAGE_ARRAY"`
}

// check reports whatever is wrong with the reply, before its numbers are used.
func (e envelope) check(path string) error {
	if e.LoginStatus == 1 {
		return fmt.Errorf("%s reports the session is signed out; its empty holdings "+
			"are not a portfolio", path)
	}
	if e.Status != 0 {
		detail := ""
		for _, m := range e.Messages {
			if m.Message != "" {
				detail = ": " + m.Message
				break
			}
		}
		return fmt.Errorf("%s returned STATUS %d%s", path, e.Status, detail)
	}
	return nil
}

// brandList is INVEST_BRAND_ARRAY, which arrives in either of two shapes.
//
// Observed live: the ミニアプリ bucket answered with an object keyed by brand id,
// and the アプリ bucket with a bare array. Both are one PHP array on the far side
// — json_encode writes an object when the keys are sparse and an array when they
// are dense — so which one arrives is a property of the account's holdings, not of
// the endpoint. Neither shape can be assumed even once.
//
// Keys is the object's keys where there were any, aligned with Items, and empty
// strings where the array form left none. Nothing here decides what a key means;
// see [Client.Read] for how a holding is joined to its name.
type brandList[T any] struct {
	Keys  []string
	Items []T
}

func (b *brandList[T]) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return nil
	}

	if text[0] == '[' {
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		b.Items = items
		b.Keys = make([]string, len(items))
		return nil
	}

	var byKey map[string]T
	if err := json.Unmarshal(data, &byKey); err != nil {
		return err
	}
	// Sorted so a log line and an error name the holdings in the same order
	// twice running. Map order would be a new order every run.
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.Keys = append(b.Keys, k)
		b.Items = append(b.Items, byKey[k])
	}
	return nil
}

// topResponse is pc_invest_top. Only the fields this program uses.
type topResponse struct {
	envelope
	SecuritiesValueTotal        int64 `json:"SECURITIES_VALUE_TOTAL"`
	TotalAcquisitionFeeTaxTotal int64 `json:"TOTAL_ACQUISITION_FEE_TAX_TOTAL"`
	SumGrossProfitTotal         int64 `json:"SUM_GROSS_PROFIT_TOTAL"`

	// InvestBrandArray is the holdings, and only the holdings. Absent or empty
	// when the bucket holds nothing.
	InvestBrandArray brandList[struct {
		BrandID         int   `json:"BRAND_ID"`
		SecuritiesValue int64 `json:"SECURITIES_VALUE"`
		SumGrossProfit  int64 `json:"SUM_GROSS_PROFIT"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// initResponse is pc_invest_init: the catalogue of every 銘柄 the bucket offers,
// which is where names come from.
type initResponse struct {
	envelope
	InvestBrandArray brandList[struct {
		BrandID int    `json:"BRAND_ID"`
		BrandNM string `json:"BRAND_NM"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// infoResponse is pc_invest_info, consulted only for the mini bucket's client
// number. The app bucket does not need one.
type infoResponse struct {
	envelope
	MiniClientSeqNo string `json:"MINI_CLIENT_SEQ_NO"`
}

// Read returns one bucket's holdings and totals.
func (c *Client) Read(ctx context.Context, bucket Bucket) (Figures, error) {
	var figures Figures

	fields, err := c.fieldsFor(ctx, bucket)
	if err != nil {
		return figures, err
	}

	topPath, initPath := appTop, appInit
	if bucket == MiniApp {
		topPath, initPath = miniTop, miniInit
	}

	var top topResponse
	if err := c.post(ctx, topPath, fields, &top); err != nil {
		return figures, err
	}
	var catalogue initResponse
	if err := c.post(ctx, initPath, fields, &catalogue); err != nil {
		return figures, err
	}

	figures.Total = top.SecuritiesValueTotal
	figures.Acquisition = top.TotalAcquisitionFeeTaxTotal
	figures.Gain = top.SumGrossProfitTotal

	// Indexed under both the object key and BRAND_ID, because a holding can
	// arrive with either and they agree wherever both are present. Keying on one
	// alone would work against whichever shape the account happened to produce
	// the day it was written.
	names := map[string]string{}
	for i, entry := range catalogue.InvestBrandArray.Items {
		if key := catalogue.InvestBrandArray.Keys[i]; key != "" {
			names[key] = entry.BrandNM
		}
		if entry.BrandID != 0 {
			names[strconv.Itoa(entry.BrandID)] = entry.BrandNM
		}
	}

	for i, brand := range top.InvestBrandArray.Items {
		id := top.InvestBrandArray.Keys[i]
		if id == "" {
			id = strconv.Itoa(brand.BrandID)
		}
		name := names[id]
		if name == "" {
			// Refused rather than recorded under a blank. A holding with no name
			// has nothing to record it against, and the ledger keys on the name.
			return Figures{}, fmt.Errorf("brand %s is held but %s does not name it",
				id, initPath)
		}
		figures.Holdings = append(figures.Holdings, Holding{
			BrandID:     brand.BrandID,
			Name:        name,
			Yen:         brand.SecuritiesValue,
			Acquisition: brand.SecuritiesValue - brand.SumGrossProfit,
		})
	}
	return figures, nil
}

// fieldsFor is the form body for one bucket.
func (c *Client) fieldsFor(ctx context.Context, bucket Bucket) (map[string]string, error) {
	fields := map[string]string{}
	for k, v := range commonFields {
		fields[k] = v
	}

	if bucket == App {
		fields["APP_ID"] = strconv.Itoa(appIDApp)
		return fields, nil
	}

	fields["APP_ID"] = strconv.Itoa(appIDMiniApp)
	if c.miniSeqNo == "" {
		seq, err := c.fetchMiniSeqNo(ctx)
		if err != nil {
			return nil, err
		}
		c.miniSeqNo = seq
	}
	fields["MINI_CLIENT_SEQ_NO"] = c.miniSeqNo
	return fields, nil
}

// fetchMiniSeqNo asks the app bucket's info endpoint for the mini bucket's
// client number, which is what the page does before its first mini call.
func (c *Client) fetchMiniSeqNo(ctx context.Context) (string, error) {
	fields := map[string]string{"APP_ID": strconv.Itoa(appIDApp)}
	for k, v := range commonFields {
		fields[k] = v
	}

	var info infoResponse
	if err := c.post(ctx, appInfo, fields, &info); err != nil {
		return "", err
	}
	if info.MiniClientSeqNo == "" {
		return "", fmt.Errorf("%s returned no MINI_CLIENT_SEQ_NO, so the ミニアプリ "+
			"holdings cannot be requested", appInfo)
	}
	return info.MiniClientSeqNo, nil
}

// post sends one multipart form and decodes the reply.
//
// Multipart because that is what the page sends. A form-urlencoded body was not
// tried: matching the client the server already serves is one fewer thing to be
// wrong about.
// checked is anything this package decodes: every reply carries the envelope.
type checked interface{ check(path string) error }

func (c *Client) post(ctx context.Context, path string, fields map[string]string, out checked) error {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			return fmt.Errorf("build %s body: %w", path, err)
		}
	}
	if err := form.Close(); err != nil {
		return fmt.Errorf("build %s body: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+path, &body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("post %s: %s", path, res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w — an HTML body here means the session is "+
			"not authenticated", path, err)
	}
	return out.check(path)
}
