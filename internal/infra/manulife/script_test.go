package manulife

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
)

// The page scripts, driven by a real browser over a data: URL. Nothing here
// touches the network, so it runs on every push alongside the other browser
// tests.
//
// Every figure, name and number below is invented. Real ones do not belong in a
// repository, and this file would otherwise be a copy of somebody's policy.
//
// The fixtures are shaped around what actually went wrong on the live site
// rather than around what the markup looks like at rest: sections the customer
// does not hold, the same label twice, and one label punctuated two ways.

// listHTML is the contract list, including a card the page is not showing.
//
// The site renders parts of its chrome twice — once for wide screens and once
// for narrow — so "in the markup" and "on the page" are different questions,
// and a hidden card would otherwise be offered as a contract to open.
//
// Note the full-width colon after 種類-証券番号 and the space before the
// half-width one after 契約状況. Both are as the live page writes them, and both
// are why labels are compared through TrimLabel.
const listHTML = `<!doctype html><meta charset="utf-8"><title>fixture</title><body>
<div class="c-card-wrap">
  <div class="c-card c-card--v1">
    <div class="c-card__head"><div class="c-card__brand">
      <div class="c-card__title">テスト終身保険</div>
    </div></div>
    <div class="c-card__body"><div class="c-desc-table"><table><tbody>
      <tr><th>種類-証券番号：</th><td class="tdCss">000-0000000</td></tr>
      <tr><th>契約日：</th><td class="tdCss">2020年01月01日</td></tr>
      <tr><th>契約状況 :</th><td class="tdCss">契約継続中</td></tr>
    </tbody></table></div></div>
  </div>
  <div class="c-card c-card--v1" style="display:none">
    <div class="c-card__head"><div class="c-card__brand">
      <div class="c-card__title">表示されていない契約</div>
    </div></div>
    <div class="c-card__body"><div class="c-desc-table"><table><tbody>
      <tr><th>種類-証券番号：</th><td class="tdCss">999-9999999</td></tr>
      <tr><th>契約状況 :</th><td class="tdCss">契約継続中</td></tr>
    </tbody></table></div></div>
  </div>
</div>
</body>`

func TestExtractContracts(t *testing.T) {
	var list contractList
	runScript(t, listHTML, mustExtractContracts(t), &list)

	if len(list.Cards) != 1 {
		t.Fatalf("cards = %d, want 1 — the hidden card is in the markup and not on the page: %+v",
			len(list.Cards), list.Cards)
	}
	card := list.Cards[0]
	if card.Title != "テスト終身保険" {
		t.Errorf("title = %q", card.Title)
	}

	number, err := card.Number()
	if err != nil {
		t.Fatalf("Number() = %v — the label carries a full-width colon here", err)
	}
	if number != "000-0000000" {
		t.Errorf("Number() = %q", number)
	}
	if !card.InForce() {
		t.Error("InForce() = false — the label is written 契約状況 with a space before its colon")
	}
}

// TestExtractContractsOnALapsedContract: the status is read, not assumed.
func TestExtractContractsOnALapsedContract(t *testing.T) {
	html := strings.Replace(listHTML, "<td class=\"tdCss\">契約継続中</td>",
		"<td class=\"tdCss\">消滅</td>", 1)

	var list contractList
	runScript(t, html, mustExtractContracts(t), &list)
	if len(list.Cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(list.Cards))
	}
	if list.Cards[0].InForce() {
		t.Error("InForce() = true for a contract the page says is 消滅")
	}
}

// policyHTML is a contract's detail page, including a section for a product
// this contract is not.
//
// That section is real: the live page carried a 変額保険 block, with a
// zero-balance and a non-zero-balance panel both display:none, on a contract
// that is not a 変額保険. Its 積立金額 parses perfectly. If the extraction
// returned it, the reader would see two rows under one label and — before it
// learned to refuse that — record whichever came first.
//
// Note the half-width colons in the summary. The list writes the same label
// with a full-width one.
const policyHTML = `<!doctype html><meta charset="utf-8"><title>fixture</title><body>
<div class="policySummary c-box-v1 clearfix">
  <div class="row">
    <div class="pull-left col-md-6 col-xs-12">
      <div class="row row-margin">
        <div class="col-md-5 col-xs-4">商品名:</div>
        <div class="bold col-md-7 col-xs-8">テスト終身保険</div>
      </div>
      <div class="row row-margin">
        <div class="col-md-5 col-xs-4">種類-証券番号:</div>
        <div class="bold col-md-7 col-xs-8">000-0000000</div>
      </div>
    </div>
  </div>
</div>

<div id="somePanelZero" style="display:none;">
  <table><tbody>
    <tr class="row"><th class="col-md-3"><p>積立金額</p></th>
      <td class="col-md-9"><span class="customerCareLink">9,999,999 円</span></td></tr>
    <tr class="row"><th class="col-md-3"><p>解約時お支払金額（円支払）</p></th>
      <td class="col-md-9"><span class="customerCareLink">8,888,888 円</span></td></tr>
  </tbody></table>
</div>

<table><tbody>
  <tr class="row"><th class="col-md-3"><p>保険種類</p></th>
    <td class="col-md-9">通貨選択型一時払終身保険</td></tr>
  <tr class="row"><th class="col-md-3"><p>解約時お支払金額（契約通貨支払）</p></th>
    <td class="col-md-9"><span class="customerCareLink">10,000.00 米ドル</span></td></tr>
  <tr class="row"><th class="col-md-3"><p>解約時お支払金額（円支払）</p></th>
    <td class="col-md-9"><span class="customerCareLink">1,500,000 円</span></td></tr>
  <tr class="row"><th class="col-md-3"><p>円換算レート</p></th>
    <td class="col-md-9"><span class="customerCareLink">1 米ドル=150.00 円</span></td></tr>
</tbody></table>
</body>`

func TestExtractPolicy(t *testing.T) {
	var page policyPage
	runScript(t, policyHTML, mustExtractPolicy(t), &page)

	number, err := field(page.Summary, selector.LabelPolicyNumber)
	if err != nil {
		t.Fatalf("the summary's %s: %v", selector.LabelPolicyNumber, err)
	}
	if number != "000-0000000" {
		t.Errorf("種類-証券番号 = %q", number)
	}

	// The hidden section carries this label too. One visible row, or the reader
	// has no way to tell which product's figure it is holding.
	yen, err := optionalField(page.Rows, selector.LabelSurrenderYen)
	if err != nil {
		t.Fatalf("%s: %v — the hidden section's row should not have been returned",
			selector.LabelSurrenderYen, err)
	}
	if yen != "1,500,000 円" {
		t.Errorf("%s = %q, want the visible section's figure", selector.LabelSurrenderYen, yen)
	}

	// A row whose value has no span of its own, which is how 保険種類 renders.
	kind, err := field(page.Rows, selector.DetailPolicyType)
	if err != nil {
		t.Fatalf("%s: %v", selector.DetailPolicyType, err)
	}
	if kind != "通貨選択型一時払終身保険" {
		t.Errorf("%s = %q", selector.DetailPolicyType, kind)
	}
}

// TestExtractPolicyRefusesTwoVisibleRowsUnderOneLabel is the case the
// visibility filter does not cover.
//
// If the site ever shows both sections at once, the reader must stop rather
// than pick. Which figure it would have picked depends on the order of the
// markup, and both are plausible amounts.
func TestExtractPolicyRefusesTwoVisibleRowsUnderOneLabel(t *testing.T) {
	shown := strings.Replace(policyHTML, `style="display:none;"`, "", 1)

	var page policyPage
	runScript(t, shown, mustExtractPolicy(t), &page)

	if _, err := optionalField(page.Rows, selector.LabelSurrenderYen); err == nil {
		t.Errorf("two visible rows under %s were accepted", selector.LabelSurrenderYen)
	}
}

// TestMarkContractPicksTheRightCard drives the marking script over a list with
// two cards and checks it matched exactly one.
//
// Marking only. Whether the click that follows reaches the contract is
// navigate_test.go's job, against a site rather than a page — a click here goes
// nowhere, so this file cannot say anything about it.
func TestMarkContractPicksTheRightCard(t *testing.T) {
	twoCards := strings.Replace(listHTML, `style="display:none"`, "", 1)

	expr, err := selector.MarkContract("999-9999999")
	if err != nil {
		t.Fatalf("build the marking script: %v", err)
	}
	var res struct {
		Matched int `json:"matched"`
	}
	runScript(t, twoCards, expr, &res)
	if res.Matched != 1 {
		t.Errorf("matched = %d, want 1", res.Matched)
	}
}

// TestMarkContractFindsNothingForAnUnknownNumber: the list is re-rendered on
// every visit, so a contract can be gone by the time it is opened. That has to
// be a refusal, not a click on whatever is nearest.
func TestMarkContractFindsNothingForAnUnknownNumber(t *testing.T) {
	expr, err := selector.MarkContract("111-1111111")
	if err != nil {
		t.Fatalf("build the marking script: %v", err)
	}
	var res struct {
		Matched int `json:"matched"`
	}
	runScript(t, listHTML, expr, &res)
	if res.Matched != 0 {
		t.Errorf("matched = %d, want 0", res.Matched)
	}
}

func mustExtractContracts(t *testing.T) string {
	t.Helper()
	expr, err := selector.ExtractContracts()
	if err != nil {
		t.Fatalf("build the contracts script: %v", err)
	}
	return expr
}

func mustExtractPolicy(t *testing.T) string {
	t.Helper()
	expr, err := selector.ExtractPolicy()
	if err != nil {
		t.Fatalf("build the policy script: %v", err)
	}
	return expr
}

// runScript loads html into a real browser and decodes what expr returns.
func runScript(t *testing.T, html, expr string, into any) {
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

	if err := chromedp.Run(bctx,
		chromedp.Navigate("data:text/html;charset=utf-8,"+strings.NewReplacer(
			"#", "%23", "%", "%25",
		).Replace(html)),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(expr, into),
	); err != nil {
		t.Fatalf("run the script: %v", err)
	}
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
