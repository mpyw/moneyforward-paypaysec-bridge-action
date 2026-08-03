//go:build live

// The live tests in this file load the real MoneyForward sign-in page to check
// that the selectors marked CONFIRMED in selectors.go still resolve. DOM drift
// is this project's main source of breakage and the workflow only runs once a
// day, so being able to check the selectors on demand is worth a build tag.
//
// They are excluded from the default build because they need network access and
// a local Chrome. Run them deliberately:
//
//	go test -tags=live ./internal/moneyforward -v
//
// No credentials are required and nothing is submitted: the page is fetched
// unauthenticated and only inspected.
package moneyforward_test

import (
	"context"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/selector"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward"
)

const (
	// liveTimeout bounds the whole test, including the browser launch.
	liveTimeout = 90 * time.Second

	// probeTimeout is how long any one selector gets to appear.
	probeTimeout = 20 * time.Second
)

// TestSignInPageSelectors checks the sign-in form against the live page.
//
// selector.PasswordInput is treated separately from the other two: MoneyForward
// could plausibly split the form into an email step followed by a password step,
// and Client.Login assumes both fields render together. If that assumption ever
// breaks, this test should say so in as many words rather than leaving it to be
// discovered as a mysterious timeout during a real run.
func TestSignInPageSelectors(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), liveTimeout)
	defer cancel()

	bctx, closeBrowser, err := browser.New(ctx, browser.Options{Headless: true})
	if err != nil {
		t.Fatalf("launch chrome: %v", err)
	}
	defer closeBrowser()

	if err := chromedp.Run(bctx, chromedp.Navigate(selector.SignInURL)); err != nil {
		t.Fatalf("navigate to %s: %v", selector.SignInURL, err)
	}

	info, err := browser.PageOf(bctx).Info()
	if err != nil {
		t.Fatalf("read page info: %v", err)
	}
	t.Logf("landed on %s (%q)", info.URL, info.Title)

	required := map[string]string{
		"selector.EmailInput":   selector.EmailInput,
		"selector.SignInSubmit": selector.SignInSubmit,
	}
	for name, selector := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := waitForSelector(bctx, name, selector); err != nil {
				t.Errorf("%s = %s no longer resolves on the sign-in page: %v", name, selector, err)
			}
		})
	}

	t.Run("selector.PasswordInput", func(t *testing.T) {
		if _, err := waitForSelector(bctx, "password", selector.PasswordInput); err != nil {
			t.Errorf("%s = %s is not on the first page.\n"+
				"If MoneyForward has split sign-in into two steps, Client.Login must click\n"+
				"through the email step before filling the password: %v",
				"selector.PasswordInput", selector.PasswordInput, err)
		}
	})
}

// TestDumpWritesPage exercises the failure-path diagnostic on a real page.
//
// Dump only ever runs when something has already gone wrong, so a bug in it
// would surface at the worst possible moment — with no page dump to debug from.
// This keeps it honest against live markup.
func TestDumpWritesPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), liveTimeout)
	defer cancel()

	bctx, closeBrowser, err := browser.New(ctx, browser.Options{Headless: true})
	if err != nil {
		t.Fatalf("launch chrome: %v", err)
	}
	defer closeBrowser()

	if err := chromedp.Run(bctx, chromedp.Navigate(selector.SignInURL)); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	// Where the browser ended up, which is not where it was sent: this URL
	// redirects to id.moneyforward.com carrying an OAuth challenge.
	info, err := browser.PageOf(bctx).Info()
	if err != nil {
		t.Fatalf("page info: %v", err)
	}

	dir := t.TempDir()
	path, err := browser.PageOf(bctx).Dump(dir, moneyforward.StepAwaitChallenge)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	dump := string(body)

	// The URL header is what makes a dump identifiable after the fact, and it
	// has to be the landing URL rather than the requested one. This assertion
	// used to name selector.SignInURL and so could only ever have passed if the
	// sign-in stopped redirecting — while a dump recording the requested URL
	// would hide the single thing these dumps exist to show.
	if !strings.Contains(dump, info.URL) {
		t.Errorf("dump %s does not record %s, the URL the browser is actually on", path, info.URL)
	}
	if strings.HasPrefix(info.URL, selector.SignInURL) {
		t.Errorf("%s no longer redirects; the sign-in flow assumed by this package has changed",
			selector.SignInURL)
	}
	// And it has to contain the markup we would go hunting through.
	const emailFieldMarkup = `mfid_user[email]`
	if !strings.Contains(dump, emailFieldMarkup) {
		t.Errorf("dump %s does not contain the sign-in form markup (%s)", path, emailFieldMarkup)
	}
	t.Logf("wrote %d bytes to %s", len(body), filepath.Base(path))
}

// waitForSelector waits for a single selector, reusing the same visibility rule the login
// flow relies on so the test cannot pass on an element the real code would miss.
func waitForSelector(ctx context.Context, name, selector string) (string, error) {
	return browser.PageOf(ctx).WaitForAny(probeTimeout, map[string]string{name: selector})
}
