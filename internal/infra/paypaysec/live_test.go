//go:build live

// Live checks against the real PayPay 証券 site.
//
//	go test -tags=live -v ./internal/paypay
//
// These are regression guards, not exploration. DOM drift is this project's main
// source of breakage and the workflow only runs once a day, so being able to ask
// "do the confirmed selectors still resolve?" on demand is worth a build tag.
// Discovering new selectors is `mfpp debug paypaysec probe`, which can log in;
// these run unauthenticated and never submit anything.
package paypaysec_test

import (
	"context"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/pagescript"
)

const (
	liveTimeout  = 2 * time.Minute
	probeTimeout = 20 * time.Second
)

// TestLoginPageSelectors checks the CONFIRMED login-form selectors against the
// live page. No credentials, nothing submitted.
func TestLoginPageSelectors(t *testing.T) {
	bctx, done := newLiveBrowser(t)
	defer done()

	if err := chromedp.Run(bctx, chromedp.Navigate(selector.LoginURL)); err != nil {
		t.Fatalf("navigate to %s: %v", selector.LoginURL, err)
	}
	logCurrentPage(t, bctx)

	for name, selector := range map[string]string{
		"selector.MemberIDInput": selector.MemberIDInput,
		"selector.PasswordInput": selector.PasswordInput,
		"selector.LoginSubmit":   selector.LoginSubmit,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := browser.PageOf(bctx).WaitForAny(probeTimeout, map[string]string{name: selector}); err != nil {
				t.Errorf("%s = %s no longer resolves: %v", name, selector, err)
			}
		})
	}
}

// TestTargetsRequireAuth confirms every target still refuses an anonymous
// client.
//
// This guards the read path's central assumption: that a page without the
// balance markup means "not logged in" rather than "zero". If one of these ever
// started serving a partial page anonymously, a scrape could record an empty
// account as a real balance.
func TestTargetsRequireAuth(t *testing.T) {
	bctx, done := newLiveBrowser(t)
	defer done()

	seen := map[string]bool{}
	for _, target := range selector.Targets {
		if seen[target.URL] {
			continue // 投資信託 appears twice, once per tab
		}
		seen[target.URL] = true

		t.Run(target.Key, func(t *testing.T) {
			if err := chromedp.Run(bctx,
				chromedp.Navigate(target.URL),
				chromedp.WaitReady("body", chromedp.ByQuery),
				chromedp.Sleep(2*time.Second),
			); err != nil {
				t.Fatalf("load %s: %v", target.URL, err)
			}

			info, err := browser.PageOf(bctx).Info()
			if err != nil {
				t.Fatalf("page info: %v", err)
			}
			t.Logf("%s -> %s", target.URL, info.URL)

			if strings.HasPrefix(info.URL, target.URL) {
				t.Errorf("%s served itself to an anonymous client; the read path treats "+
					"missing balance markup as 'not authenticated', which would no longer be safe",
					target.URL)
			}
			present, err := valueTotalPresent(bctx)
			if err != nil {
				t.Fatalf("probe %s: %v", selector.ValueTotal, err)
			}
			if present {
				t.Errorf("%s is present without a session", selector.ValueTotal)
			}
		})
	}
}

// valueTotalPresent reports whether the 評価額合計 element exists on the current page.
// It takes the selector from the package itself, so this cannot drift away from
// what the scraper actually looks for.
func valueTotalPresent(ctx context.Context) (bool, error) {
	js, err := pagescript.Apply(`(sel) => !!document.querySelector(sel)`, selector.ValueTotal)
	if err != nil {
		return false, err
	}
	var found bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &found)); err != nil {
		return false, err
	}
	return found, nil
}

func newLiveBrowser(t *testing.T) (context.Context, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), liveTimeout)
	bctx, closeBrowser, err := browser.New(ctx, browser.Options{Headless: true})
	if err != nil {
		cancel()
		t.Fatalf("launch chrome: %v", err)
	}
	return bctx, func() {
		closeBrowser()
		cancel()
	}
}

func logCurrentPage(t *testing.T, ctx context.Context) {
	t.Helper()
	info, err := browser.PageOf(ctx).Info()
	if err != nil {
		t.Fatalf("page info: %v", err)
	}
	t.Logf("landed on %s (%q)", info.URL, info.Title)
}
