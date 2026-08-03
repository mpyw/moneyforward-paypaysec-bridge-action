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
	"errors"
	"fmt"
	"io"
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

// Client reads the 投資信託 endpoints with an authenticated session.
type Client struct {
	// HTTP carries the session cookies. Built from the browser's jar; see
	// [cookiestore.Store.HTTPClient].
	HTTP *http.Client

	// miniSeqNo is the mini bucket's client number, fetched once on demand.
	miniSeqNo string

	// Trace, when set, is handed every reply before it is judged: the path, the
	// fields sent, and the body verbatim.
	//
	// For the debug command only. The service answers a request it does not like
	// with STATUS 9 and システムの不具合, which names nothing — three releases were
	// spent guessing at that from CI, one guess per run, at a login and a full
	// scrape each. A body in front of a person settles it in one.
	//
	// Never set in the scheduled job: a body here holds the account's balances.
	Trace func(path string, fields map[string]string, body []byte)
}

// ErrNoMiniApp says the page's own test reports no ミニアプリ 投資信託 for this
// account.
//
// Not a failure, and treated as neither a failure nor an empty portfolio. The two
// need opposite handling — a read that failed must stop the run, an empty bucket
// licenses deleting everything recorded under it — and this is a third thing: a
// bucket that was never asked about. Skipping the target leaves the category
// uncovered, which is what [portfolio.Plan.CheckCoverage] refuses to delete from.
//
// The condition comes from the page, not from the service: what these endpoints
// answer for an account without the bucket has not been observed, because the
// account this was built against has it. Deciding not to ask is the part that can
// be got right without that observation.
var ErrNoMiniApp = errors.New("the account has no ミニアプリ 投資信託")

// Info is what pc_invest_info says about the account, beyond the client number.
//
// InvTrustUsable and PPKYC are the page's own test for whether the ミニアプリ
// bucket exists at all: it shows that tab only when the client number and
// InvTrustUsable are both set. An account failing that has no ミニアプリ 投資信託
// to read, which is a different thing from a read that failed.
type Info struct {
	MiniClientSeqNo string
	InvTrustUsable  string
	PPKYC           string
}

// HasMiniApp is the page's own test, kept in its terms.
//
// Verbatim from the bundle: `"" != (MINI_CLIENT_SEQ_NO && INV_TRUST_USABLE)`,
// which in JavaScript is "both are truthy". Spelled out rather than paraphrased,
// because the values arrive as text and the shapes differ — the client number is a
// number, and INV_TRUST_USABLE has been seen as the string "true".
//
// PPKYC is deliberately not part of this. The bundle gates the tab menu on
// `hasMiniApp && PP_KYC` and blocks the app-side portfolio route when PP_KYC is 0,
// which says something about the アプリ bucket rather than this one — and says it
// about screens, not about what the endpoints return. Reading a bucket out of that
// would be a guess, so PPKYC is carried for the debug command to show and nothing
// here acts on it.
func (i Info) HasMiniApp() bool {
	return truthy(i.MiniClientSeqNo) && truthy(i.InvTrustUsable)
}

// truthy reads one of these text-carried flags the way the page would.
func truthy(v string) bool {
	switch v {
	case "", "0", "false":
		return false
	default:
		return true
	}
}

// ReadInfo reports what the account is, asking as the ミニアプリ.
func (c *Client) ReadInfo(ctx context.Context) (Info, error) {
	fields := miniInfoFields()
	var info infoResponse
	if err := c.post(ctx, appInfo, fields, &info); err != nil {
		return Info{}, err
	}
	return Info{
		MiniClientSeqNo: string(info.MiniClientSeqNo),
		InvTrustUsable:  string(info.InvTrustUsable),
		PPKYC:           string(info.PPKYC),
	}, nil
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
	Status laxInt64 `json:"STATUS"`

	// LoginStatus is 1 when the session is no longer signed in.
	LoginStatus laxInt64 `json:"LOGIN_STATUS"`

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

// laxInt64 is a number the service may send either as a JSON number or as a
// decimal string.
//
// Both were observed on one run: INVEST_BRAND_ARRAY changed shape between two
// buckets, and MINI_CLIENT_SEQ_NO came back as a number where the page's own
// bundle treats it as a string. That is what a PHP service over a database driver
// looks like from outside — whether a scalar keeps its quotes depends on where the
// value came from, not on what it means — so a field's JSON type is not something
// to design around one observation of.
//
// This is not a guess at a value: the string form is parsed strictly and refused
// if it is not a number. A missing field stays zero, as it would without this.
type laxInt64 int64

func (n *laxInt64) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var quoted string
		if err := json.Unmarshal(data, &quoted); err != nil {
			return err
		}
		text = strings.TrimSpace(quoted)
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a number", text)
	}
	*n = laxInt64(v)
	return nil
}

// laxString is an identifier the service may send either quoted or bare.
//
// The digits are kept exactly as they arrived rather than parsed and reprinted:
// this is a name for something, not a quantity, and a round trip through an
// integer is a chance to lose a leading zero or overflow a width nobody
// promised.
type laxString string

func (l *laxString) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var quoted string
		if err := json.Unmarshal(data, &quoted); err != nil {
			return err
		}
		*l = laxString(strings.TrimSpace(quoted))
		return nil
	}
	*l = laxString(text)
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
	SecuritiesValueTotal        laxInt64 `json:"SECURITIES_VALUE_TOTAL"`
	TotalAcquisitionFeeTaxTotal laxInt64 `json:"TOTAL_ACQUISITION_FEE_TAX_TOTAL"`
	SumGrossProfitTotal         laxInt64 `json:"SUM_GROSS_PROFIT_TOTAL"`

	// InvestBrandArray is the holdings, and only the holdings. Absent or empty
	// when the bucket holds nothing.
	InvestBrandArray brandList[struct {
		BrandID         laxInt64 `json:"BRAND_ID"`
		SecuritiesValue laxInt64 `json:"SECURITIES_VALUE"`
		SumGrossProfit  laxInt64 `json:"SUM_GROSS_PROFIT"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// initResponse is pc_invest_init: the catalogue of every 銘柄 the bucket offers,
// which is where names come from.
type initResponse struct {
	envelope
	InvestBrandArray brandList[struct {
		BrandID laxInt64 `json:"BRAND_ID"`
		BrandNM string   `json:"BRAND_NM"`
	}] `json:"INVEST_BRAND_ARRAY"`
}

// infoResponse is pc_invest_info, consulted only for the mini bucket's client
// number. The app bucket does not need one.
type infoResponse struct {
	envelope
	MiniClientSeqNo laxString `json:"MINI_CLIENT_SEQ_NO"`

	// InvTrustUsable and PPKYC decide whether the ミニアプリ bucket exists for
	// this account; see [Info].
	InvTrustUsable laxString `json:"INV_TRUST_USABLE"`
	PPKYC          laxString `json:"PP_KYC"`
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

	// init before top, which is the order the page loads them in. Nothing here
	// needs the catalogue first, but this is a stateful PHP service and the mini
	// bucket has already refused a top call that arrived out of the page's order.
	var catalogue initResponse
	if err := c.post(ctx, initPath, fields, &catalogue); err != nil {
		return figures, err
	}
	var top topResponse
	if err := c.post(ctx, topPath, fields, &top); err != nil {
		return figures, err
	}

	figures.Total = int64(top.SecuritiesValueTotal)
	figures.Acquisition = int64(top.TotalAcquisitionFeeTaxTotal)
	figures.Gain = int64(top.SumGrossProfitTotal)

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
			names[strconv.FormatInt(int64(entry.BrandID), 10)] = entry.BrandNM
		}
	}

	for i, brand := range top.InvestBrandArray.Items {
		id := top.InvestBrandArray.Keys[i]
		if id == "" {
			id = strconv.FormatInt(int64(brand.BrandID), 10)
		}
		name := names[id]
		if name == "" {
			// Refused rather than recorded under a blank. A holding with no name
			// has nothing to record it against, and the ledger keys on the name.
			return Figures{}, fmt.Errorf("brand %s is held but %s does not name it",
				id, initPath)
		}
		figures.Holdings = append(figures.Holdings, Holding{
			BrandID:     int(brand.BrandID),
			Name:        name,
			Yen:         int64(brand.SecuritiesValue),
			Acquisition: int64(brand.SecuritiesValue - brand.SumGrossProfit),
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

// fetchMiniSeqNo asks for the ミニアプリ client number.
//
// The path is the app bucket's, but the fields are the mini bucket's: APP_ID 6
// with an empty MINI_CLIENT_SEQ_NO. That is not a detail — the page has one
// transport per bucket, each with its own default fields, and it reaches this one
// endpoint through the mini transport whenever the mini tab is showing. Asking as
// the app bucket is what the page does for the app tab, and it is a different
// question.
//
// Sending the field empty is the page's own doing too: the mini defaults declare
// MINI_CLIENT_SEQ_NO, so every mini call carries it, including the call that
// exists to find out what it is.
func (c *Client) fetchMiniSeqNo(ctx context.Context) (string, error) {
	fields := miniInfoFields()

	var info infoResponse
	if err := c.post(ctx, appInfo, fields, &info); err != nil {
		return "", err
	}
	// The page's own test for whether this bucket exists, applied before anything
	// is asked of it.
	//
	// What the endpoints do for an account that does not have it is not known: this
	// account has it, and there is no way to observe the other case from here. So
	// the site's own judgement is used as the judgement, and the bucket is not
	// asked about at all rather than asked and second-guessed.
	account := Info{
		MiniClientSeqNo: string(info.MiniClientSeqNo),
		InvTrustUsable:  string(info.InvTrustUsable),
		PPKYC:           string(info.PPKYC),
	}
	if !account.HasMiniApp() {
		return "", ErrNoMiniApp
	}
	return account.MiniClientSeqNo, nil
}

// miniInfoFields is the body the page sends when it asks this as the ミニアプリ.
func miniInfoFields() map[string]string {
	fields := map[string]string{
		"APP_ID":             strconv.Itoa(appIDMiniApp),
		"MINI_CLIENT_SEQ_NO": "",
	}
	for k, v := range commonFields {
		fields[k] = v
	}
	return fields
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
	req.Header.Set("Referer", origin+pagePath)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("post %s: %s", path, res.Status)
	}

	// Read whole rather than streamed into the decoder, so that Trace has
	// something to show when the decode is what failed.
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if c.Trace != nil {
		c.Trace(path, fields, payload)
	}

	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode %s: %w — an HTML body here means the session is "+
			"not authenticated", path, err)
	}
	return out.check(path)
}
