package investapi

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// The figures are invented. Real ones do not belong in a repository.

// stub answers the three endpoints and records what it was asked.
type stub struct {
	mu       []string // paths, in order
	referers map[string]string
	fields   map[string]map[string]string
	loginOut bool
	status   int
	noName   bool

	// asArray answers INVEST_BRAND_ARRAY as a bare array instead of an object
	// keyed by brand id. Both are one PHP array on the far side, and the live
	// service sent each of them on the same run.
	asArray bool

	// noHoldings is a bucket that holds nothing, which is the array shape too.
	noHoldings bool

	// quoted sends every number as a decimal string, the other way a PHP service
	// can hand over a scalar.
	quoted bool

	// brokenTotal sends a total that is not a number in either form.
	brokenTotal bool

	// noMiniClient is an account with no ミニアプリ, which reports its client
	// number as zero rather than as absent.
	noMiniClient bool
}

func (s *stub) handler(t *testing.T) http.Handler {
	t.Helper()
	if s.fields == nil {
		s.fields = map[string]map[string]string{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu = append(s.mu, r.URL.Path)
		s.fields[r.URL.Path] = readFields(t, r)
		if s.referers == nil {
			s.referers = map[string]string{}
		}
		s.referers[r.URL.Path] = r.Header.Get("Referer")

		w.Header().Set("Content-Type", "application/json")
		login := 0
		if s.loginOut {
			login = 1
		}

		switch r.URL.Path {
		case appInfo:
			seq := `900000001`
			if s.noMiniClient {
				seq = `0`
			}
			// A bare number, which is what the live service sends — the page's
			// own bundle treats it as a string.
			_, _ = w.Write([]byte(`{"STATUS":0,"MINI_CLIENT_SEQ_NO":` + seq + `}`))
		case appTop, miniTop:
			held := `{"7":{"BRAND_ID":7,"SECURITIES_VALUE":345678,"SUM_GROSS_PROFIT":45678}}`
			if s.noHoldings {
				held = `[]`
			} else if s.asArray {
				held = `[{"BRAND_ID":7,"SECURITIES_VALUE":345678,"SUM_GROSS_PROFIT":45678}]`
			}
			if s.quoted {
				held = `{"7":{"BRAND_ID":"7","SECURITIES_VALUE":"345678","SUM_GROSS_PROFIT":"45678"}}`
			}
			total := `345678`
			if s.quoted {
				total = `"345678"`
			}
			if s.brokenTotal {
				total = `"-"`
			}
			_, _ = w.Write([]byte(`{"STATUS":` + strconv.Itoa(s.status) + `,"LOGIN_STATUS":` + strconv.Itoa(login) + `,
				"SECURITIES_VALUE_TOTAL":` + total + `,
				"TOTAL_ACQUISITION_FEE_TAX_TOTAL":300000,
				"SUM_GROSS_PROFIT_TOTAL":45678,
				"MESSAGE_ARRAY":[{"MESSAGE":"だめ"}],
				"INVEST_BRAND_ARRAY":` + held + `}`))
		case appInit, miniInit:
			if s.noName {
				_, _ = w.Write([]byte(`{"STATUS":0,"INVEST_BRAND_ARRAY":[]}`))
				return
			}
			catalogue := `{"7":{"BRAND_ID":7,"BRAND_NM":"テスト・グローバル・ファンド"},
				 "9":{"BRAND_ID":9,"BRAND_NM":"持っていない銘柄"}}`
			if s.quoted {
				catalogue = `[{"BRAND_ID":"7","BRAND_NM":"テスト・グローバル・ファンド"},
				 {"BRAND_ID":"9","BRAND_NM":"持っていない銘柄"}]`
			}
			if s.asArray {
				catalogue = `[{"BRAND_ID":7,"BRAND_NM":"テスト・グローバル・ファンド"},
				 {"BRAND_ID":9,"BRAND_NM":"持っていない銘柄"}]`
			}
			_, _ = w.Write([]byte(`{"STATUS":0,"INVEST_BRAND_ARRAY":` + catalogue + `}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func readFields(t *testing.T, r *http.Request) map[string]string {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content type: %v", err)
	}
	out := map[string]string{}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			return out
		}
		b, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part %q: %v", part.FormName(), err)
		}
		out[part.FormName()] = string(b)
	}
}

func serve(t *testing.T, s *stub) *Client {
	t.Helper()
	srv := httptest.NewServer(s.handler(t))
	t.Cleanup(srv.Close)
	// The package addresses the real host; point it at the stub for the test.
	old := origin
	setOrigin(srv.URL)
	t.Cleanup(func() { setOrigin(old) })
	return &Client{HTTP: srv.Client()}
}

func TestReadApp(t *testing.T) {
	s := &stub{}
	got, err := serve(t, s).Read(t.Context(), App)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if got.Total != 345678 || got.Acquisition != 300000 || got.Gain != 45678 {
		t.Errorf("totals = %+v", got)
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one held", got.Holdings)
	}
	h := got.Holdings[0]
	if h.Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q", h.Name)
	}
	// Derived, not fetched: value minus unrealised gain.
	if h.Acquisition != 300000 {
		t.Errorf("acquisition = %d, want value - gain", h.Acquisition)
	}
	// The catalogue lists a 銘柄 the account does not hold. Only what the top
	// call reported is a holding — reading the catalogue as the portfolio would
	// invent hundreds.
	if s.fields[appTop]["APP_ID"] != "3" {
		t.Errorf("APP_ID = %q, want the app bucket", s.fields[appTop]["APP_ID"])
	}
	if _, asked := s.fields[appInfo]; asked {
		t.Error("the app bucket asked for a MINI_CLIENT_SEQ_NO it does not need")
	}
}

// TestReadMiniAppFetchesItsClientNumber pins the one piece of state these calls
// need, and where it comes from.
func TestReadMiniAppFetchesItsClientNumber(t *testing.T) {
	s := &stub{}
	c := serve(t, s)
	if _, err := c.Read(t.Context(), MiniApp); err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if s.fields[miniTop]["MINI_CLIENT_SEQ_NO"] != "900000001" {
		t.Errorf("MINI_CLIENT_SEQ_NO = %q, want the one info returned",
			s.fields[miniTop]["MINI_CLIENT_SEQ_NO"])
	}
	if s.fields[miniTop]["APP_ID"] != "6" {
		t.Errorf("APP_ID = %q, want the mini bucket", s.fields[miniTop]["APP_ID"])
	}

	// Asked as the mini bucket, not as the app bucket whose path it borrows. The
	// page has one transport per bucket and reaches this endpoint through the mini
	// one whenever the mini tab is showing; asking as the app bucket is a
	// different question, and the answer to it made the service refuse the top
	// call outright.
	if got := s.fields[appInfo]["APP_ID"]; got != "6" {
		t.Errorf("info APP_ID = %q, want the mini bucket's", got)
	}
	if _, sent := s.fields[appInfo]["MINI_CLIENT_SEQ_NO"]; !sent {
		t.Error("the mini defaults declare MINI_CLIENT_SEQ_NO, so even the call " +
			"that asks for it carries it")
	}

	// Asked once, then remembered: a second bucket read must not re-fetch it.
	before := len(s.mu)
	if _, err := c.Read(t.Context(), MiniApp); err != nil {
		t.Fatalf("second Read() error = %v", err)
	}
	for _, p := range s.mu[before:] {
		if p == appInfo {
			t.Error("the client number was fetched again")
		}
	}
}

// TestReadRefusesASignedOutSession is the reason the envelope is checked.
//
// A session that has expired answers with LOGIN_STATUS 1 and no holdings. Taken
// at face value that is a category that emptied — and this program deletes those,
// which it has done twice for other reasons. An expired cookie must not be able
// to look like a sale.
func TestReadRefusesASignedOutSession(t *testing.T) {
	_, err := serve(t, &stub{loginOut: true}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() accepted a signed-out session's empty portfolio")
	}
	if !strings.Contains(err.Error(), "signed out") {
		t.Errorf("error = %v, want it to say the session is signed out", err)
	}
}

// TestReadRefusesAnErrorStatus keeps a declared field from going unread, which
// is how the acquisition checks in this project used to go quiet.
func TestReadRefusesAnErrorStatus(t *testing.T) {
	_, err := serve(t, &stub{status: 9}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() ignored a non-zero STATUS")
	}
	if !strings.Contains(err.Error(), "だめ") {
		t.Errorf("error = %v, want the service's own message", err)
	}
}

// TestReadRefusesAHoldingItCannotName guards the ledger's key. An entry recorded
// under an empty name cannot be matched again, so the next run creates another.
func TestReadRefusesAHoldingItCannotName(t *testing.T) {
	_, err := serve(t, &stub{noName: true}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() returned a holding with no name")
	}
	if !strings.Contains(err.Error(), "does not name it") {
		t.Errorf("error = %v", err)
	}
}

// TestReadAcceptsTheArrayShape is the shape the live service sent on the first
// real run, and the shape this package refused.
//
// INVEST_BRAND_ARRAY is a PHP array: json_encode writes an object when its keys
// are sparse and an array when they are dense, so the shape follows the account's
// holdings rather than the endpoint. One run saw both — the ミニアプリ bucket keyed
// by brand id, the アプリ bucket a bare array — and there is no shape to assume.
//
// The array form carries no key, so the join to the catalogue has to fall back to
// BRAND_ID. Both stub responses use the array form here, which is what makes that
// fallback the thing under test.
func TestReadAcceptsTheArrayShape(t *testing.T) {
	got, err := serve(t, &stub{asArray: true}).Read(t.Context(), App)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one held", got.Holdings)
	}
	// Named, not just counted: an unnamed holding is refused, so a count alone
	// would pass on a join that silently matched the wrong 銘柄.
	if got.Holdings[0].Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q, want the catalogue entry with the held BRAND_ID",
			got.Holdings[0].Name)
	}
	if got.Holdings[0].Acquisition != 300000 {
		t.Errorf("acquisition = %d, want value - gain", got.Holdings[0].Acquisition)
	}
}

// TestReadAcceptsAnEmptyBucket separates "holds nothing" from "could not be read".
//
// An empty PHP array is `[]`, which is the array shape again — and refusing to
// decode it would turn every genuinely empty category into a failed run. There is
// no danger in accepting it here: an empty answer that is actually a lost session
// is caught by the envelope, and one that is actually a mis-read is caught by the
// ledger's own refusal to empty a category.
func TestReadAcceptsAnEmptyBucket(t *testing.T) {
	got, err := serve(t, &stub{noHoldings: true}).Read(t.Context(), App)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(got.Holdings) != 0 {
		t.Errorf("holdings = %+v, want none", got.Holdings)
	}
	// The totals still have to arrive: "no holdings" is not "no answer".
	if got.Total != 345678 {
		t.Errorf("total = %d, want the reported total", got.Total)
	}
}

// TestReadAcceptsQuotedNumbers is the other half of what a PHP service does to
// scalars.
//
// Two of these arrived unquoted where the page's bundle implied a string, and the
// reverse is the same coin: a value's JSON type here reflects where it came from
// on the far side, not what it means. Every number this package reads is parsed
// from either form, so finding out costs a test rather than a failed sync.
func TestReadAcceptsQuotedNumbers(t *testing.T) {
	got, err := serve(t, &stub{quoted: true}).Read(t.Context(), MiniApp)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Total != 345678 || got.Acquisition != 300000 || got.Gain != 45678 {
		t.Errorf("totals = %+v, want them parsed out of the quoted form", got)
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one held", got.Holdings)
	}
	// Joined on a quoted BRAND_ID, which is the part a lenient total would not
	// have covered.
	if got.Holdings[0].Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q", got.Holdings[0].Name)
	}
	if got.Holdings[0].Yen != 345678 {
		t.Errorf("yen = %d", got.Holdings[0].Yen)
	}
}

// TestReadRefusesANumberItCannotParse keeps the leniency from becoming a guess: a
// scalar that is neither a number nor a number in quotes is refused, not zeroed.
// A zero here would be an amount, and this program acts on amounts.
func TestReadRefusesANumberItCannotParse(t *testing.T) {
	_, err := serve(t, &stub{brokenTotal: true}).Read(t.Context(), App)
	if err == nil {
		t.Fatal("Read() accepted a total that is not a number")
	}
	if !strings.Contains(err.Error(), "not a number") {
		t.Errorf("error = %v, want it to say what was wrong with the value", err)
	}
}

// TestReadAsksForTheCatalogueFirst pins the order the page loads these in.
//
// Nothing here needs the catalogue before the holdings — this is a stateful PHP
// service, and the mini bucket has already refused a top call that arrived out of
// the page's order once.
func TestReadAsksForTheCatalogueFirst(t *testing.T) {
	s := &stub{}
	if _, err := serve(t, s).Read(t.Context(), App); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	var init, top int
	for i, path := range s.mu {
		switch path {
		case appInit:
			init = i
		case appTop:
			top = i
		}
	}
	if init > top {
		t.Errorf("call order = %v, want %s before %s", s.mu, appInit, appTop)
	}
}

// TestReadRefusesAMiniAccountThatHasNoClientNumber covers an account without the
// ミニアプリ at all.
//
// The field arrives as a number, so "absent" reads as zero rather than as an empty
// string. Passed on, it is a client number with no client behind it, and the
// service answers with a system error that names nothing — a day spent on the
// wrong end of the call.
func TestReadRefusesAMiniAccountThatHasNoClientNumber(t *testing.T) {
	_, err := serve(t, &stub{noMiniClient: true}).Read(t.Context(), MiniApp)
	if err == nil {
		t.Fatal("Read() sent a client number of zero")
	}
	if !strings.Contains(err.Error(), "MINI_CLIENT_SEQ_NO") {
		t.Errorf("error = %v, want it to name the field that was missing", err)
	}
}

// TestReadSaysWhichPageItIsOn is the header the ミニアプリ bucket will not answer
// without.
//
// Measured against the live service: the identical body is accepted from inside
// the document and refused from a client that sends no Referer, with STATUS 9 and
// システムの不具合 — which names nothing, and cost three releases of guessing from
// CI before the call was made from a terminal instead. A browser User-Agent makes
// no difference; this header is the whole of it.
func TestReadSaysWhichPageItIsOn(t *testing.T) {
	s := &stub{}
	if _, err := serve(t, s).Read(t.Context(), MiniApp); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	// Every call, not only the ミニアプリ ones: the v2 endpoints do not ask, and an
	// exception is a thing to keep true.
	for _, path := range []string{appInfo, miniInit, miniTop} {
		got := s.referers[path]
		if got == "" {
			t.Errorf("%s was sent no Referer", path)
			continue
		}
		if !strings.HasSuffix(got, "/investment_trust/") {
			t.Errorf("%s Referer = %q, want the 投資信託 screen", path, got)
		}
	}
}
