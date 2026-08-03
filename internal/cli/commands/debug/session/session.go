package session

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/config"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/cookiestore"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

// Session is one running Chrome plus the context bounding its use.
type Session struct {
	ctx   context.Context
	close func()
	opts  *Options
}

// Start loads the environment and launches Chrome against the persisted
// profile. The caller must call [Session.Finish].
//
// Persisting the profile is what lets the scrape be iterated on separately from
// the login: `debug paypaysec login` once, then `debug paypaysec balance` as often as
// needed without another one-time code.
func (o *Options) Start(ctx context.Context) (*Session, error) {
	ctx, cancelTimeout := context.WithTimeout(ctx, o.timeout)

	bopts := browser.DefaultsFor(config.IsCI())
	// --headless forces it on for a local run; a hosted runner has no display
	// and DefaultsFor has already said so.
	bopts.Headless = bopts.Headless || o.headless
	bopts.UserDataDir = o.ProfileDir()
	_, _ = fmt.Fprintf(os.Stderr, "→ Chrome (headless=%v, profile=%s)\n", bopts.Headless, bopts.UserDataDir)

	bctx, closeBrowser, err := browser.New(ctx, bopts)
	if err != nil {
		cancelTimeout()
		return nil, err
	}

	s := &Session{
		ctx:  bctx,
		opts: o,
		close: func() {
			closeBrowser()
			cancelTimeout()
		},
	}

	n, err := cookiestore.Store{Path: o.CookieFile()}.Load(bctx)
	if err != nil {
		// A bad cookie file should not block a fresh login.
		_, _ = fmt.Fprintf(os.Stderr, "  (could not restore session: %v)\n", err)
	} else if n > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "  restored %d cookies from %s\n", n, o.CookieFile())
	}
	return s, nil
}

// Context is the browser context, for the site packages that drive it.
func (s *Session) Context() context.Context { return s.ctx }

// SaveSession writes the current cookies so a later process can reuse the
// login. Called after a successful sign-in.
func (s *Session) SaveSession() {
	n, err := cookiestore.Store{Path: s.opts.CookieFile()}.Save(s.ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  (could not save session: %v)\n", err)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "  saved %d cookies to %s\n", n, s.opts.CookieFile())
	_, _ = fmt.Fprintf(os.Stderr, "  ⚠ that file is a live session for the account — delete it when done\n")
}

// Finish honours --keep-open, then shuts Chrome down.
func (s *Session) Finish() {
	if s.opts.keepOpen && !s.opts.headless {
		s.waitForEnter()
	}
	s.close()
}

// waitForEnter parks so the browser stays inspectable in DevTools.
func (s *Session) waitForEnter() {
	_, _ = fmt.Fprintf(os.Stderr, "\nChrome is still open. Press Enter to close it.\n")
	done := make(chan struct{})
	go func() {
		defer close(done)
		var discard string
		_, _ = fmt.Scanln(&discard)
	}()
	select {
	case <-done:
	case <-s.ctx.Done():
	}
}

// navigateTimeout bounds a single page load.
//
// Without one, a page that never settles consumes the command's whole deadline
// in silence — which is generous for an OTP wait and useless for a page load.
const navigateTimeout = 45 * time.Second

// Open loads a URL into this session's browser and waits for the document, with
// a settle pause because these pages render client-side.
func (s *Session) Open(url string) error {
	_, _ = fmt.Fprintf(os.Stderr, "→ loading %s\n", url)

	tctx, cancel := context.WithTimeout(s.ctx, navigateTimeout)
	defer cancel()
	if err := browser.PageOf(tctx).Open(url); err != nil {
		return err
	}
	// These pages render client-side; give the content a moment to appear.
	return chromedp.Run(tctx, chromedp.Sleep(2*time.Second))
}

// Report prints where the browser currently is.
//
// Every subcommand ends up wanting this, and a redirect to a sign-in page is the
// single most common explanation for a page not looking the way it should.
func (s *Session) Report() {
	info, err := browser.PageOf(s.ctx).Info()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  (page info unavailable: %v)\n", err)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "  url:   %s\n  title: %s\n", info.URL, info.Title)
}

// ReportInteractive lists what the page offers to click or type into.
//
// The first thing anyone wants when a selector has stopped resolving, and the
// reason both site commands have a probe. It lived in each of them, down to a
// column width that differed between the two for no reason.
func (s *Session) ReportInteractive() error {
	els, err := browser.PageOf(s.ctx).FindInteractive()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nInteractive controls (%d):\n", len(els))
	for _, e := range els {
		_, _ = fmt.Fprintf(os.Stderr, "  %-44s tag=%-6s role=%-4s visible=%-5v %q\n",
			e.Selector, e.Tag, e.Role, e.Visible, e.Text)
	}
	return nil
}

// DumpPage writes the page out and says where it went.
//
// Never an error: the dump is a convenience beside whatever the command was
// actually reporting, and failing the command over it would lose that instead.
func (s *Session) DumpPage(label string) {
	path, err := browser.PageOf(s.ctx).Dump(s.opts.debugDir, label)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "\n(could not dump page: %v)\n", err)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "\ndump: %s\n", path)
	_, _ = fmt.Fprintf(os.Stderr, "  ⚠ may contain personal data — delete it when done\n")
}

// ReportStepFailure dumps the page the flow died on.
//
// The dump is the actual deliverable when a selector is wrong, so a failure to
// write it is reported but never masks the original error.
func (s *Session) ReportStepFailure(err error) {
	step := steperr.Of(err)
	if step == "" {
		step = "unknown-step"
	}
	_, _ = fmt.Fprintf(os.Stderr, "\n✗ failed at step %q\n", step)

	if info, ierr := browser.PageOf(s.ctx).Info(); ierr == nil {
		_, _ = fmt.Fprintf(os.Stderr, "  page:  %s\n  title: %s\n", info.URL, info.Title)
	}

	path, derr := browser.PageOf(s.ctx).Dump(s.opts.debugDir, step)
	if derr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  (could not write page dump: %v)\n", derr)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "  dump:  %s\n", path)
	_, _ = fmt.Fprintf(os.Stderr, "  ⚠ may contain personal data — delete it once the selector is found\n")
}
