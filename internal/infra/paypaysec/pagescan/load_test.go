package pagescan

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec/selector"
)

// Load navigates, waits for the figures to settle, clicks a tab if the target
// has one, and reads. None of that was covered: the package was 4.8%, because
// everything in it drives a browser and the browser was assumed to need the
// real site.
//
// It needs a page, not that page. These serve fixtures over loopback, which
// exercises the same code the scheduled job runs — including the wait, which is
// where a slow site records a real balance as zero.

// investmentTrustHTML is the two-tab shape: one URL, two buckets, and the
// figures swapped in by script when a tab is clicked.
const investmentTrustHTML = `<!doctype html><meta charset="utf-8"><title>fixture</title><body>
<ul class="tab_menu">
  <li class="actived"><a href="#">PayPay証券アプリ</a></li>
  <li><a href="#">ミニアプリ</a></li>
</ul>

<div class="summary">
  <span id="SECURITIES_VALUE_TOTAL">0円</span>
  <span id="TOTAL_ACQUISITION_FEE_TAX_TOTAL">0円</span>
  <span id="gross_profit_total">0円</span>
</div>

<h3>保有銘柄</h3>
<div class="icon_lv1" id="rows"></div>

<script>
  const app = {total: "0円", cost: "0円", gain: "0円", rows: ""};
  const mini = {
    total: "25万1234円", cost: "22万0000円", gain: "+3万1234円",
    rows: '<div class="mypage_brand_icon">' +
          '<a href="/investment_trust/detail/7" title="テスト・グローバル・ファンド"></a>' +
          '<div class="brand_invest">25万1234円</div>' +
          '<div class="brand_gain">+3万1234円</div></div>',
  };
  const show = (d) => {
    document.getElementById("SECURITIES_VALUE_TOTAL").textContent = d.total;
    document.getElementById("TOTAL_ACQUISITION_FEE_TAX_TOTAL").textContent = d.cost;
    document.getElementById("gross_profit_total").textContent = d.gain;
    document.getElementById("rows").innerHTML = d.rows;
  };
  document.querySelectorAll(".tab_menu li").forEach((li, i) => {
    li.querySelector("a").addEventListener("click", (e) => {
      e.preventDefault();
      document.querySelectorAll(".tab_menu li").forEach((o) => o.classList.remove("actived"));
      li.classList.add("actived");
      show(i === 0 ? app : mini);
    });
  });
</script>
</body>`

// TestLoadReadsATargetWithATab is the 投資信託 case: two buckets behind one URL,
// which without the click would read the same figure twice and double-count one
// of them.
func TestLoadReadsATargetWithATab(t *testing.T) {
	url, bctx := serving(t, investmentTrustHTML)

	figures, err := Load(bctx, selector.Target{
		Key: "toushin-miniapp", URL: url, TabLabel: selector.TabLabelMiniApp,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if figures.TotalRaw != "25万1234円" {
		t.Errorf("total = %q — the ミニアプリ tab's figures should be the ones read", figures.TotalRaw)
	}
	if len(figures.Holdings) != 1 {
		t.Fatalf("holdings = %+v, want the one under that tab", figures.Holdings)
	}
	if figures.Holdings[0].Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q", figures.Holdings[0].Name)
	}
}

// TestLoadReadsTheOtherTab pins that the tab is what decides, not the page.
func TestLoadReadsTheOtherTab(t *testing.T) {
	url, bctx := serving(t, investmentTrustHTML)

	figures, err := Load(bctx, selector.Target{
		Key: "toushin-app", URL: url, TabLabel: selector.TabLabelApp,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if figures.TotalRaw != "0円" || len(figures.Holdings) != 0 {
		t.Errorf("the アプリ tab read %q with %d holdings, want the empty side",
			figures.TotalRaw, len(figures.Holdings))
	}
	// And it still found the section, which is what tells an empty category from
	// a page that did not render one.
	if !figures.HoldingsSection {
		t.Error("the 保有銘柄 section was not seen")
	}
}

// TestLoadRefusesATabItCannotFind guards against reading whatever happened to be
// on screen. 投資信託 puts both buckets at one URL, so a tab that silently did
// not activate means one bucket's figure is attributed to the other.
func TestLoadRefusesATabItCannotFind(t *testing.T) {
	url, bctx := serving(t, investmentTrustHTML)

	_, err := Load(bctx, selector.Target{
		Key: "toushin-miniapp", URL: url, TabLabel: "存在しないタブ",
	})
	if err == nil {
		t.Fatal("Load() read the page without activating the tab it was asked for")
	}
	if !strings.Contains(err.Error(), "存在しないタブ") {
		t.Errorf("error = %v, want it to name the tab", err)
	}
}

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	t.Cleanup(srv.Close)

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
