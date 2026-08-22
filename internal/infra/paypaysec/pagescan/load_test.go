package pagescan

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// Load navigates, waits for the figures to settle, and reads. None of that was
// covered: the package was 4.8%, because
// everything in it drives a browser and the browser was assumed to need the
// real site.
//
// It needs a page, not that page. These serve fixtures over loopback, which
// exercises the same code the scheduled job runs — including the wait, which is
// where a slow site records a real balance as zero.

// TestLoadRefusesAPageStillLoading is the fix for the worst failure this scraper
// can have.
//
// While the Vue app fetches, the total, the cost basis, the gain and every row
// all read 0円 — a state that is internally consistent, so every cross-check
// agrees and a real balance is recorded as zero. settle used to return nil on
// timeout for exactly the reason that does not hold.
func TestLoadRefusesAPageStillLoading(t *testing.T) {
	url, bctx := serving(t, `<!doctype html><meta charset="utf-8"><body>
<div class="loading_page" style="width:10px;height:10px">読み込み中</div>
<span id="SECURITIES_VALUE_TOTAL">0円</span>
<h3>保有銘柄</h3><div class="icon_lv1"></div>`)

	_, err := Load(bctx, selector.Target{Key: "toushin-miniapp", URL: url})
	if err == nil {
		t.Fatal("Load() read a page that never finished loading")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error = %v, want it to say the figures are not amounts", err)
	}
}

// TestLoadOnAnUnauthenticatedPage passes the state through rather than failing:
// the read's own complaint names the likely cause, and this is not the place to
// preempt it.
func TestLoadOnAnUnauthenticatedPage(t *testing.T) {
	url, bctx := serving(t, `<!doctype html><meta charset="utf-8"><body>ログインしてください`)

	figures, err := Load(bctx, selector.Target{Key: "usa", URL: url})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if figures.TotalPresent {
		t.Error("a page with no total reported one")
	}
}

// serving starts a loopback server for one page and a browser to read it with,
// and returns the URL and the context Load drives through.
func serving(t *testing.T, html string) (string, context.Context) {
	t.Helper()
	if !chromeAvailable() {
		t.Skip("no Chrome on PATH")
	}
	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The figures request, answered slowly — that delay is the race this
		// package exists to survive. A real one: the reader here is a browser in
		// another process, so the wait cannot be faked away.
		if strings.Contains(r.URL.Path, "pc_invest_top") {
			time.Sleep(time.Second)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"STATUS":0}`)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	// Loopback rather than the in-memory network: Chrome dials this itself, from
	// outside the test binary, so there has to be a port.
	srv.Start()

	// Generous: the settle window alone is 20s, and one case here waits it out.
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	t.Cleanup(cancel)

	bctx, closeBrowser, err := browser.New(ctx, browser.DefaultsFor(true))
	if err != nil {
		t.Fatalf("launch chrome: %v", err)
	}
	t.Cleanup(closeBrowser)
	return srv.URL, bctx
}
