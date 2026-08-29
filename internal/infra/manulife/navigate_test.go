package manulife

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
)

// Clicking a card and arriving at the contract behind it, against a stand-in
// site rather than a single page.
//
// The fixture tests next door drive the extraction scripts over a data: URL,
// which cannot cover this: a click there reaches an element and goes nowhere,
// so "the right card was matched" is all they can say. Whether the click opens
// a contract, and whether the wait afterwards sees it, was covered by one live
// run and nothing else — and the first real sync failed exactly there, with a
// message that named neither the wait nor the page.
//
// A loopback server rather than the in-memory one: Chrome dials this from
// another process, so there has to be a port.

// listPage is a contract list shaped like the real one in the way that matters:
// the card opens itself through a named function defined by one of the page's
// scripts, and that script arrives after the cards do.
//
// The delay is the point. The live page renders its cards server-side and
// defines the opener later; clicking in between raises a ReferenceError inside
// the handler, which goes to window.onerror rather than to whoever dispatched
// the click — so the click reports success and nothing happens. Three attempts
// at fixing the click were really chasing this.
const listPage = `<!doctype html><meta charset="utf-8"><title>list</title><body>
<div class="c-card-wrap">
  <div class="c-card c-card--v1"
       onclick="RedirectToPageOrFFFModal('/policyinquiry?id=fresh-token');return false;">
    <div class="c-card__head"><div class="c-card__brand">
      <div class="c-card__title">テスト終身保険</div>
    </div></div>
    <div class="c-card__body"><div class="c-desc-table"><table><tbody>
      <tr><th>種類-証券番号：</th><td class="tdCss">000-0000000</td></tr>
      <tr><th>契約状況 :</th><td class="tdCss">契約継続中</td></tr>
    </tbody></table></div></div>
  </div>
</div>
<script>
setTimeout(function () {
  window.RedirectToPageOrFFFModal = function (url) { location.href = url; };
}, 800);
</script>
</body>`

// detailPage is what that click has to land on. The figures are invented and
// chosen so the two routes agree: 10,000.00 at 150.00 is 1,500,000.
const detailPage = `<!doctype html><meta charset="utf-8"><title>契約詳細</title><body>
<div class="policySummary c-box-v1 clearfix">
  <div class="row row-margin">
    <div class="col-md-5">種類-証券番号:</div><div class="bold col-md-7">000-0000000</div>
  </div>
</div>
<table><tbody>
  <tr class="row"><th><p>保険種類</p></th><td>通貨選択型一時払終身保険</td></tr>
  <tr class="row"><th><p>解約時お支払金額（契約通貨支払）</p></th>
    <td><span class="customerCareLink">10,000.00 米ドル</span></td></tr>
  <tr class="row"><th><p>解約時お支払金額（円支払）</p></th>
    <td><span class="customerCareLink">1,500,000 円</span></td></tr>
  <tr class="row"><th><p>円換算レート</p></th>
    <td><span class="customerCareLink">1 米ドル=150.00 円</span></td></tr>
</tbody></table>
</body>`

func TestReadCardOpensTheContractBehindTheCard(t *testing.T) {
	ctx, url := servingSite(t, nil)
	openList(t, ctx, url)

	cards, err := Cards(ctx)
	if err != nil {
		t.Fatalf("Cards() = %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(cards))
	}

	reading, err := ReadCard(ctx, cards[0])
	if err != nil {
		t.Fatalf("ReadCard() = %v — the click has to reach the contract page", err)
	}
	if reading.Number != "000-0000000" {
		t.Errorf("Number = %q", reading.Number)
	}
	if reading.PolicyType != "通貨選択型一時払終身保険" {
		t.Errorf("PolicyType = %q", reading.PolicyType)
	}
	yen, err := reading.Amount()
	if err != nil {
		t.Fatalf("Amount() = %v", err)
	}
	if yen != 1500000 {
		t.Errorf("Amount() = %d, want 1500000", yen)
	}
}

// TestReadCardRefusesTheWrongContract is why the click is followed by a check
// rather than trusted.
//
// A contract has no address: the token in its URL is minted per rendering of
// the list, so the URL cannot say which contract opened. If the page that
// arrives belongs to a different one, every figure below it would be recorded
// against the wrong contract.
func TestReadCardRefusesTheWrongContract(t *testing.T) {
	swapped := strings.Replace(detailPage, "000-0000000", "999-9999999", 1)
	ctx, url := servingSite(t, map[string]string{"/policyinquiry": swapped})
	openList(t, ctx, url)

	cards, err := Cards(ctx)
	if err != nil {
		t.Fatalf("Cards() = %v", err)
	}
	if _, err := ReadCard(ctx, cards[0]); err == nil {
		t.Fatal("ReadCard() accepted a page belonging to another contract")
	}
}

// TestReadCardSaysWhatItWasWaitingFor is the failure the first real sync
// produced: a bare "context deadline exceeded", naming neither the wait nor the
// page the browser was on.
//
// Here the click goes nowhere, so the wait for the contract page is what
// expires — and it has to say so.
func TestReadCardSaysWhatItWasWaitingFor(t *testing.T) {
	// The handler exists and does nothing, which is a click that lands and goes
	// nowhere — as distinct from a handler that is not there yet.
	stuck := strings.Replace(listPage,
		`RedirectToPageOrFFFModal('/policyinquiry?id=fresh-token')`, "void 0", 1)
	ctx, url := servingSite(t, map[string]string{"/": stuck})
	openList(t, ctx, url)

	cards, err := Cards(ctx)
	if err != nil {
		t.Fatalf("Cards() = %v", err)
	}

	// Shortened so the test does not sit through the real budget; what is being
	// checked is the wording, not the duration.
	short, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err = ReadCard(short, cards[0])
	if err == nil {
		t.Fatal("ReadCard() succeeded with a card that opens nothing")
	}
	if step := StepOf(err); step != StepOpenContract {
		t.Errorf("step = %q, want %q", step, StepOpenContract)
	}
	if !strings.Contains(err.Error(), "waiting for the contract page") {
		t.Errorf("error = %v, want it to name the wait that expired", err)
	}
	// Deliberately not asserting the page here. The outer context is what was
	// cut short, so by the time the error is worded the browser cannot be asked
	// anything — WithLocation is best-effort and correctly gives up. In a real
	// run the outer deadline is the job's and one of the inner budgets expires
	// first, which is the case below.
}

// TestReadCardSaysWhereTheBrowserWas is the other half, with a live context.
//
// The contract is gone from the list by the time it is clicked — the list is
// re-rendered on every visit, so this is a real thing that happens rather than
// a contrived one. What it has to produce is an error naming the page, because
// "the contract is not in the list" is only actionable alongside which list was
// being looked at.
func TestReadCardSaysWhereTheBrowserWas(t *testing.T) {
	other := strings.Replace(listPage, "000-0000000", "111-1111111", 1)
	ctx, url := servingSite(t, map[string]string{"/relisted": other})
	openList(t, ctx, url)

	cards, err := Cards(ctx)
	if err != nil {
		t.Fatalf("Cards() = %v", err)
	}
	// The list is re-rendered, and this contract is no longer on it.
	openList(t, ctx, url+"/relisted")

	_, err = ReadCard(ctx, cards[0])
	if err == nil {
		t.Fatal("ReadCard() succeeded against a list the contract had left")
	}
	if step := StepOf(err); step != StepOpenContract {
		t.Errorf("step = %q, want %q", step, StepOpenContract)
	}
	for _, want := range []string{"no longer in the list", "the browser was on"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

// TestReadCardWaitsForTheListsOwnClickHandler is the failure that took three
// attempts to see.
//
// The opener is never defined here, so the card can never be opened. What the
// run has to do is say that — rather than click something that cannot respond
// and then blame the wait for the page that never came.
func TestReadCardWaitsForTheListsOwnClickHandler(t *testing.T) {
	never := strings.Replace(listPage, "}, 800);", "}, 600000);", 1)
	ctx, url := servingSite(t, map[string]string{"/": never})
	openList(t, ctx, url)

	cards, err := Cards(ctx)
	if err != nil {
		t.Fatalf("Cards() = %v", err)
	}

	short, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err = ReadCard(short, cards[0])
	if err == nil {
		t.Fatal("ReadCard() clicked a card whose handler does not exist yet")
	}
	if !strings.Contains(err.Error(), "click handler to be defined") {
		t.Errorf("error = %v, want it to name what it was waiting for — clicking "+
			"anyway is what made three earlier failures unreadable", err)
	}
}

// servingSite starts a loopback site and a browser, with overrides replacing
// the default page for a path.
func servingSite(t *testing.T, overrides map[string]string) (context.Context, string) {
	t.Helper()
	if !chromeAvailable() {
		t.Skip("no Chrome on PATH")
	}

	pages := map[string]string{"/": listPage, "/policyinquiry": detailPage}
	for path, body := range overrides {
		pages[path] = body
	}

	srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	// Loopback rather than the in-memory network: Chrome dials this itself,
	// from outside the test binary, so there has to be a port.
	srv.Start()

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	t.Cleanup(cancel)

	bctx, closeBrowser, err := browser.New(ctx, browser.DefaultsFor(true))
	if err != nil {
		t.Fatalf("launch chrome: %v", err)
	}
	t.Cleanup(closeBrowser)
	return bctx, srv.URL
}

// openList puts the browser where a completed sign-in leaves it.
func openList(t *testing.T, ctx context.Context, url string) {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
}
