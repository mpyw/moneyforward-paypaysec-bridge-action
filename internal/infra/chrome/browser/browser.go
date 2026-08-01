// Package browser wraps chromedp launch options for this project. Both the
// sync workflow (headless, CI) and the local debugging commands (headed) come
// through here so flag drift is impossible.
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Options controls how Chrome is launched.
type Options struct {
	// Headless runs Chrome without a window. Local debugging wants false;
	// CI has no display and must use true.
	Headless bool

	// NoSandbox drops Chrome's sandbox. GitHub-hosted Linux runners restrict
	// unprivileged user namespaces via AppArmor, so without this the zygote host
	// aborts at startup with "No usable sandbox!" before a single page loads.
	//
	// Deliberately not inferred from Headless: this is a real reduction in
	// isolation for a browser pointed at banking sites, so local runs should
	// leave it off. See DefaultsFor.
	NoSandbox bool

	// UserDataDir persists the Chrome profile — and therefore the session — at
	// this path. Empty means a throwaway profile, which is what CI wants: a
	// fresh device every run, and no session left on disk.
	//
	// Local development wants the opposite. Logging in costs a one-time code,
	// so iterating on the scraping step against a fresh profile would burn an
	// OTP per attempt. With a persisted profile you log in once and re-run the
	// read as often as you like.
	//
	// SECURITY: the directory holds live brokerage session cookies. Keep it
	// under .debug/ (gitignored, created 0700) and delete it when finished.
	UserDataDir string
}

// DefaultsFor returns the options appropriate to where this is running:
// headless and unsandboxed on a hosted runner, headed and sandboxed otherwise.
//
// Told rather than reading the environment itself. Dropping the sandbox is a
// real reduction in isolation for a browser pointed at brokerage sites, and a
// package this low should not be the one deciding that from an ambient
// variable.
func DefaultsFor(ci bool) Options {
	return Options{Headless: ci, NoSandbox: ci}
}

// New launches Chrome and returns its context plus a cancel function. The
// caller owns the cancel and must call it.
func New(parent context.Context, opts Options) (context.Context, context.CancelFunc, error) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", opts.Headless),
		// MF and PayPay 証券 both run bot-detection JS. These reduce the most
		// obvious automation tells.
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "AutomationControlled"),
		chromedp.WindowSize(1280, 900),
		// How long to wait for Chrome to announce its DevTools socket.
		//
		// chromedp allows 20s, which a shared CI runner under -race does not
		// always manage — it cost one red build here, and the scheduled job runs
		// on the same kind of machine and gets one attempt a day. Waiting longer
		// costs nothing when the browser starts promptly, and the job's own
		// deadline is the real bound.
		chromedp.WSURLReadTimeout(90*time.Second),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"),
	)
	if opts.NoSandbox {
		allocOpts = append(allocOpts, chromedp.NoSandbox)
	}
	if opts.UserDataDir != "" {
		// 0700: the profile contains session cookies for financial accounts.
		if err := os.MkdirAll(opts.UserDataDir, 0o700); err != nil {
			return nil, nil, fmt.Errorf("create profile dir: %w", err)
		}
		allocOpts = append(allocOpts, chromedp.UserDataDir(opts.UserDataDir))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, allocOpts...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	cancel := func() {
		ctxCancel()
		allocCancel()
	}
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("start chrome: %w", err)
	}
	return ctx, cancel, nil
}

const probeInterval = 300 * time.Millisecond

// Page drives whatever the browser currently has open.
//
// A type bound to the chromedp context, rather than a family of functions each
// taking one: every operation here acts on the same page, and threading the
// context through argument lists said nothing while letting any of them be
// called with the wrong one.
type Page struct {
	ctx context.Context
}

// PageOf binds to a chromedp context.
func PageOf(ctx context.Context) Page { return Page{ctx: ctx} }

// WaitForAny polls until one of the named selectors matches a visible element
// and returns that name.
//
// chromedp has no built-in "race these waits", and every login flow here forks:
// the OTP challenge appears on a new device, but a trusted session lands
// straight on the home page. Waiting on only one of those hangs until timeout in
// the other case, which reads as a broken selector when nothing is broken.
func (p Page) WaitForAny(timeout time.Duration, candidates map[string]string) (string, error) {
	ctx := p.ctx
	if len(candidates) == 0 {
		return "", fmt.Errorf("WaitForAny: no candidates given")
	}
	js, err := pageScripts.Call("wait_for_any.js", candidates)
	if err != nil {
		return "", fmt.Errorf("WaitForAny: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		var hit string
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &hit)); err != nil {
			return "", fmt.Errorf("WaitForAny: probe: %w", err)
		}
		if hit != "" {
			return hit, nil
		}
		if time.Now().After(deadline) {
			pairs := make([]string, 0, len(candidates))
			for name, sel := range candidates {
				pairs = append(pairs, fmt.Sprintf("%s=%q", name, sel))
			}
			sort.Strings(pairs)
			return "", fmt.Errorf("none of [%s] became visible within %s",
				strings.Join(pairs, ", "), timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(probeInterval):
		}
	}
}

// Open navigates without waiting for the load event.
//
// chromedp.Navigate resolves only once the frame has fully loaded, which makes
// it hostage to whatever third-party resource a page happens to embed: a single
// analytics script that never answers hangs the whole call, on a document that
// has been usable for seconds. This starts the navigation, then waits for the
// document — which is the thing callers actually need.
func (p Page) Open(url string) error {
	ctx := p.ctx
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := page.Navigate(url).Do(ctx)
		return err
	})); err != nil {
		return fmt.Errorf("navigate to %s: %w", url, err)
	}
	if err := chromedp.Run(ctx, chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		return fmt.Errorf("waiting for %s to render: %w", url, err)
	}
	return nil
}

// PageInfo is a cheap snapshot for log lines.
type PageInfo struct {
	URL   string
	Title string
}

// Info reads the current URL and title.
func (p Page) Info() (PageInfo, error) {
	var info PageInfo
	err := chromedp.Run(p.ctx,
		chromedp.Location(&info.URL),
		chromedp.Title(&info.Title),
	)
	if err != nil {
		return PageInfo{}, fmt.Errorf("read page info: %w", err)
	}
	return info, nil
}

// WithLocation annotates err with the page the browser is actually on.
//
// A timeout waiting for a selector says only that the selector did not appear.
// Where it did not appear is the difference between "the credentials were
// rejected and we are still on the login page" and "we were redirected
// somewhere unexpected" — and on the scheduled job there is no second attempt to
// go and look. This costs one CDP round trip on a path that has already failed.
//
// Best effort: if the page cannot be read, the original error is returned
// unchanged rather than replaced by a complaint about reading it.
func (p Page) WithLocation(err error) error {
	if err == nil {
		return nil
	}
	info, ierr := p.Info()
	if ierr != nil {
		return err
	}
	return fmt.Errorf("%w (the browser was on %s, %q)", err, withoutQuery(info.URL), info.Title)
}

// withoutQuery keeps the scheme, host and path and drops everything after.
//
// These messages go to a workflow log. Query strings on the sign-in paths carry
// OAuth state, nonces and code challenges; none of that helps identify a page,
// and URL parameters are a well-worn way to leak something that should not be
// in a log.
func withoutQuery(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i]
	}
	return raw
}

// Dump writes the current page's HTML to dir for offline selector hunting and
// returns the file path.
//
// CAUTION: on an authenticated page this file contains personal data — account
// names, balances, and session-bearing markup. dir must stay out of git (.debug/
// is gitignored) and the file should be deleted once the selector is found.
func (p Page) Dump(dir, label string) (string, error) {
	ctx := p.ctx
	info, err := p.Info()
	if err != nil {
		return "", err
	}
	var html string
	if err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("read outer HTML: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create dump dir: %w", err)
	}

	name := fmt.Sprintf("%s-%s.html", time.Now().Format("20060102-150405"), sanitizeDumpLabel(label))
	path := filepath.Join(dir, name)
	header := fmt.Sprintf("<!-- url:   %s\n     title: %s\n     note:  may contain personal data; delete when done -->\n",
		info.URL, info.Title)
	if err := os.WriteFile(path, []byte(header+html), 0o600); err != nil {
		return "", fmt.Errorf("write dump: %w", err)
	}
	return path, nil
}

func sanitizeDumpLabel(s string) string {
	if s == "" {
		return "page"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}
