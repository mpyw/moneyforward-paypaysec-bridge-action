package session

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// RecordNetwork starts recording the page's own XHR and fetch traffic, and
// returns the function that stops it.
//
// Separate from the page dumps because it answers a different question. A dump
// says what the page ended up showing; this says where those figures came from,
// and whether there is an address that could be asked directly. On a site whose
// elements are numbered by position in a component tree — nothing addressable,
// nothing stable — that is not a nicety, it is the only lead there is.
//
// The recording holds whatever the responses held, so it is personal data in the
// same way a dump is.
func (s *Session) RecordNetwork(label string) (stop func(), err error) {
	rec, err := browser.Record(s.ctx, s.opts.debugDir, label)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(os.Stderr, "→ recording XHR/fetch to %s\n", rec.Path())
	return func() {
		n, err := rec.Stop()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "  (network log: %v)\n", err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "\nnetwork: %d exchanges in %s\n", n, rec.Path())
		if n > 0 {
			_, _ = fmt.Fprintf(os.Stderr, "  ⚠ holds whatever the responses held — delete it when done\n")
		}
	}, nil
}

// holdInterval is how often a held session is written back.
//
// Short enough that forgetting the terminal costs at most this much of a login,
// long enough not to churn the file while somebody reads a page.
const holdInterval = 10 * time.Second

// HoldSession keeps writing the captured session out while a person works in
// the browser, and returns the function that stops it.
//
// The session is the expensive thing in a manual run: it costs a one-time code,
// and both services already known here stop mailing those after about five
// attempts in quick succession. Saving only at the end makes the whole run
// all-or-nothing on somebody remembering to come back to the terminal — which
// is exactly how the first Manulife attempt was lost, the deadline expiring
// while the browser sat there signed in.
//
// So it is written repeatedly, from the first moment there is anything to
// write. Whatever ends the run — a forgotten terminal, an expired deadline, a
// Ctrl-C — the next one starts from the last snapshot instead of from another
// code.
//
// The returned stop waits for the writer to finish, so no CDP call from here
// overlaps whatever the caller does next.
func (s *Session) HoldSession() (stop func()) {
	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(holdInterval)
		defer ticker.Stop()

		announced := false
		for {
			select {
			case <-done:
				return
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
			n, err := cookiestore.Store{Path: s.opts.CookieFile()}.Save(s.ctx)
			// Silent about failures and about every repeat. A page mid-navigation
			// answers badly and the next tick gets it right, so a line per attempt
			// would be noise over the one thing worth saying: that there is now a
			// session on disk, and that it is a live one.
			if err != nil || n == 0 || announced {
				continue
			}
			announced = true
			_, _ = fmt.Fprintf(os.Stderr, "  session captured to %s and kept current — "+
				"a lost run will not cost another one-time code\n", s.opts.CookieFile())
			_, _ = fmt.Fprintf(os.Stderr, "  ⚠ that file is a live session for the account — delete it when done\n")
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

// Finish honours --keep-open, then shuts Chrome down.
func (s *Session) Finish() {
	if s.opts.keepOpen && !s.opts.headless {
		_, _ = s.waitForEnter("\nChrome is still open. Press Enter to close it.\n")
	}
	s.close()
}

// Pause hands the browser to whoever is at the terminal and waits, reporting
// what they typed and whether anyone was actually waited for.
//
// For a service this program has no selectors for yet. Reaching an
// authenticated page needs a sign-in, and writing one needs the selectors that
// are the thing being looked for — so the first pass is a person signing in by
// hand, and everything after it inspects what they landed on.
//
// waited is false when nobody could have answered: headless, or a deadline that
// expired while waiting. A caller looping on this needs to know the difference,
// because a Pause that returns instantly and reports nothing is otherwise
// indistinguishable from an empty line, and the loop spins.
func (s *Session) Pause(what string) (typed string, waited bool) {
	if s.opts.headless {
		_, _ = fmt.Fprintf(os.Stderr, "  (headless — not pausing for: %s)\n", what)
		return "", false
	}
	return s.waitForEnter("\n" + what + "\n")
}

// waitForEnter parks so the browser stays usable and inspectable in DevTools.
func (s *Session) waitForEnter(prompt string) (typed string, waited bool) {
	_, _ = fmt.Fprint(os.Stderr, prompt)
	done := make(chan struct{})
	var line string
	go func() {
		defer close(done)
		// Scanln reports an error for a bare newline, which is the ordinary case
		// here: line stays empty and that is the answer.
		_, _ = fmt.Scanln(&line)
	}()
	select {
	case <-done:
		return strings.TrimSpace(line), true
	case <-s.ctx.Done():
		return "", false
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
