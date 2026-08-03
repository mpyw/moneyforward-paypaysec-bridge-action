package pagescan

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec/selector"
)

// extract_balance.js says of itself that "page script is the one place in this
// project that no test can reach". It was true, and it is the reason the wire
// checks in this package settled for asserting that a script mentions the keys
// it is supposed to return — which cannot catch a rename of "name" or "ref",
// because those are substrings of "selectors.name" and "href".
//
// A browser and a data: URL reach it. Nothing here touches the network, so this
// runs on every push alongside the other browser tests.
//
// The figures are invented. Real ones do not belong in a repository.

// listPageHTML is a target page shaped like the live one, including the parts
// that made naive extraction fail: amounts split across child elements, the
// catalogue of brands the account does not hold, and the 投資信託 template's
// padded cells and title-attribute names.
const listPageHTML = `<!doctype html><meta charset="utf-8"><title>fixture</title><body>
<div class="summary">
  <span id="SECURITIES_VALUE_TOTAL">12<span>万</span>3456<span>円</span></span>
  <span id="TOTAL_ACQUISITION_FEE_TAX_TOTAL">10<span>万</span>0000<span>円</span></span>
  <span id="gross_profit_total">+2<span>万</span>3456<span>円</span></span>
</div>

<h3>取扱銘柄</h3>
<div class="icon_lv1">
  <div class="mypage_brand_icon"><a href="/trade/brand/99/0"></a>
    <div class="brand_text">持っていない銘柄</div>
    <div class="brand_invest">99万9999円</div>
  </div>
</div>

<h3>保有銘柄</h3>
<div class="icon_lv1">
  <div class="mypage_brand_icon">
    <a href="/trade/brand/35/0"></a>
    <div class="brand_text">テスト電機</div>
    <div class="brand_invest">10<span>万</span>0000<span>円</span></div>
    <div class="brand_gain">+2.0万</div>
  </div>
  <div class="mypage_brand_icon">
    <a href="/investment_trust/detail/7" title="テスト・グローバル・ファンド">
      <p>テスト・グローバル・ファンド</p>
    </a>
    <div class="brand_invest"> 23,456円 </div>
    <div class="brand_gain"> +3,456円 </div>
  </div>
</div>
</body>`

// TestExtractBalanceAgainstAPage runs the real script over that markup and
// checks what Go decodes from it.
func TestExtractBalanceAgainstAPage(t *testing.T) {
	figures := runExtraction(t, listPageHTML)

	if !figures.TotalPresent || figures.TotalRaw != "12万3456円" {
		t.Errorf("total = %q (present=%v); the nested spans should come back as one token",
			figures.TotalRaw, figures.TotalPresent)
	}
	if !figures.AcquisitionPresent || figures.AcquisitionRaw != "10万0000円" {
		t.Errorf("acquisition = %q (present=%v)", figures.AcquisitionRaw, figures.AcquisitionPresent)
	}
	if !figures.GainPresent || figures.GainRaw != "+2万3456円" {
		t.Errorf("gain = %q (present=%v)", figures.GainRaw, figures.GainPresent)
	}

	// Two rows, and only two: the 取扱銘柄 catalogue above uses the same row class
	// and the account holds none of it. Reading it unscoped once walked hundreds
	// of brands.
	if len(figures.Holdings) != 2 {
		t.Fatalf("holdings = %d rows, want the 2 under 保有銘柄: %+v",
			len(figures.Holdings), figures.Holdings)
	}

	if !figures.HoldingsSection {
		t.Error("the 保有銘柄 section was not reported as present")
	}

	stock := figures.Holdings[0]
	if stock.Name != "テスト電機" {
		t.Errorf("name = %q — a renamed key in the script decodes to empty here, and "+
			"downstream to a holding with nothing to record it under", stock.Name)
	}
	if stock.Ref != "/trade/brand/35/0" {
		t.Errorf("ref = %q — this decides which of the two acquisition routes applies", stock.Ref)
	}
	if stock.InvestText != "10万0000円" {
		t.Errorf("investText = %q", stock.InvestText)
	}
	if stock.GainText != "+2.0万" {
		t.Errorf("gainText = %q", stock.GainText)
	}

	// The 投資信託 template: the name is in the anchor's title, and the cells are
	// padded.
	fund := figures.Holdings[1]
	if fund.Name != "テスト・グローバル・ファンド" {
		t.Errorf("name = %q, want the one from the title attribute", fund.Name)
	}
	if fund.InvestText != "23,456円" {
		t.Errorf("investText = %q, want the padding stripped", fund.InvestText)
	}
}

// TestExtractBalanceOnAPageWithoutTheHoldingsSection is the state that used to
// pass every cross-check with nothing recorded: a total and no rows.
//
// The script reports it faithfully; refusing it is Reading.Amount's job, and
// this pins the half that has to be true for that to work.
func TestExtractBalanceOnAPageWithoutTheHoldingsSection(t *testing.T) {
	figures := runExtraction(t, `<!doctype html><meta charset="utf-8"><body>
<span id="SECURITIES_VALUE_TOTAL">12万3456円</span>`)

	if !figures.TotalPresent {
		t.Error("the total was not found")
	}
	if len(figures.Holdings) != 0 {
		t.Errorf("holdings = %+v, want none", figures.Holdings)
	}
	// The distinction that matters: no section at all, rather than an empty one.
	// A category that holds nothing still renders the heading.
	if figures.HoldingsSection {
		t.Error("a page with no 保有銘柄 heading reported having the section")
	}
}

// TestExtractBalanceOnAnUnauthenticatedPage: no element at all is what a
// logged-out page looks like, and it must be distinguishable from a zero.
func TestExtractBalanceOnAnUnauthenticatedPage(t *testing.T) {
	figures := runExtraction(t, `<!doctype html><meta charset="utf-8"><body>ログインしてください`)

	if figures.TotalPresent {
		t.Error("a page with no total reported one; that reads downstream as a balance")
	}
}

// runExtraction loads html into a real browser and returns what the script says.
func runExtraction(t *testing.T, html string) Figures {
	t.Helper()
	if !chromeAvailable() {
		t.Skip("no Chrome on PATH")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	bctx, closeBrowser, err := browser.New(ctx, browser.DefaultsFor(true))
	if err != nil {
		t.Fatalf("launch chrome: %v", err)
	}
	defer closeBrowser()

	expr, err := selector.ExtractBalance()
	if err != nil {
		t.Fatalf("build extraction script: %v", err)
	}

	var figures Figures
	if err := chromedp.Run(bctx,
		chromedp.Navigate("data:text/html;charset=utf-8,"+strings.NewReplacer(
			"#", "%23", "%", "%25",
		).Replace(html)),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(expr, &figures),
	); err != nil {
		t.Fatalf("run the extraction: %v", err)
	}
	return figures
}

func chromeAvailable() bool {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}
