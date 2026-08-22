package investapi

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Every figure and 銘柄 name here is invented. Real ones do not belong in a
// repository — this one is public, and the account's contents would be in it.
//
// The shapes, though, are not invented: each is one the live service actually
// sent. That is the point of the flags below.

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

	// miniNotUsable is the other half of the page's test: a client number exists
	// but the bucket is not on offer.
	miniNotUsable bool
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
			// The number is bare and the flag is a quoted "true": both are what the
			// live service sends, where the page's own bundle treats the first as a
			// string.
			usable := `"true"`
			if s.miniNotUsable {
				usable = `"false"`
			}
			_, _ = w.Write([]byte(`{"STATUS":0,"PP_KYC":1,
				"INV_TRUST_USABLE":` + usable + `,
				"MINI_CLIENT_SEQ_NO":` + seq + `}`))
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
	// The package addresses the real host and keeps addressing it: on the
	// in-memory network the client sends every request here whatever host it
	// names, so nothing is redirected and no global is swapped out from under a
	// parallel test.
	srv := httptest.NewTestServer(t, s.handler(t))
	return &Client{HTTP: srv.Client()}
}
