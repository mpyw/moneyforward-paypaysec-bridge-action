package otp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/gmail"
)

// Gmail waits for the OTP mail to arrive and reads the code out of it.
//
// Read here rather than relayed. A watcher that PATCHes the code into a GitHub
// variable needs a third-party service, a personal access token and write
// permission, and leaves the job asking whether the variable holds this run's
// code or the last one's. Reading the mailbox in-process turns that into one
// timestamp comparison — see [Gmail.Fetch].
type Gmail struct {
	// Mail is the mailbox to search, normally a *gmail.Client.
	Mail MailSearcher

	// Spec is everything specific to the service being logged into.
	Spec MailSpec

	// Timeout caps the total wait; Interval is the poll cadence. Zero means the
	// package defaults.
	Timeout  time.Duration
	Interval time.Duration

	// Warn, if set, receives transient problems the wait should survive: a
	// failed search, a message whose code does not validate. Defaults to the
	// standard logger.
	//
	// A field rather than a package variable that tests reassign: two sources
	// polling at once would have shared it, and a test silencing it would have
	// silenced the other.
	Warn func(format string, args ...any)
}

// warn reports something the poll should carry on past.
func (g *Gmail) warn(format string, args ...any) {
	if g.Warn != nil {
		g.Warn(format, args...)
		return
	}
	log.Printf(format, args...)
}

const (
	// maxMessages is how many recent matches to examine per poll. The mail we
	// want is the newest, but a couple of near-simultaneous messages are
	// possible.
	maxMessages = 5

	// recencyBound trims the search. A day rather than an hour because Gmail's
	// unit suffixes are d/m/y — "1h" is not an hour, and "20m" is twenty months.
	recencyBound = "newer_than:1d"
)

// Describe names this source for log lines and failure messages.
func (g *Gmail) Describe() string {
	if g.Spec.Label != "" {
		return "Gmail (" + g.Spec.Label + ")"
	}
	return "Gmail"
}

// Fetch polls the mailbox until a message no older than since yields a code.
//
// Freshness is judged on the server's receive time, and the previous run's mail
// is still sitting in the mailbox looking identical — so a message from before
// since is skipped even when it parses perfectly.
//
// "Before since" is decided at one-second resolution, because that is the
// resolution the answer has. internalDate is a millisecond field carrying a
// whole second, while since comes from time.Now() and carries nanoseconds, so a
// mail that genuinely arrived after the login was submitted is stamped with the
// floor of that second and compares as older. Requiring strictly-newer rejected
// it permanently: on 2026-08-01 the code polled for 100 seconds against a
// message already in every result set, stamped 18:06:46 against a cutoff of
// 18:06:46, and would have run to its timeout.
//
// Accepting the whole second admits a mail that arrived within it but before
// the submit. That needs two login attempts to the same service inside one
// second, which the sequential phases and the workflow's concurrency group do
// not allow.
func (g *Gmail) Fetch(ctx context.Context, since time.Time) (string, error) {
	if g.Mail == nil {
		return "", fmt.Errorf("otp: Gmail source has no mailbox configured")
	}
	since = since.Truncate(time.Second)

	// Trim the search to recent mail, then compare exactly in Go.
	//
	// Deliberately not `after:<epoch>`: Gmail accepts that syntax and then
	// returns nothing, which is indistinguishable from "the mail has not arrived
	// yet" — the poll simply spins until it times out. newer_than is coarse but
	// it works, and the precise cutoff is enforced on Received below anyway.
	query := g.Spec.Query + " " + recencyBound

	p := newPoller(g.Timeout, g.Interval, 5*time.Second)
	code, err := p.run(ctx, func() (string, error) {
		msgs, serr := g.Mail.Search(ctx, query, maxMessages)
		if serr != nil {
			// One failed request should not waste the whole window.
			g.warn("WARN: %s: %v", g.Describe(), serr)
			return "", nil
		}
		found, ok := g.extract(msgs, since)
		if !ok {
			g.warn("  %s: %d recent message(s), none at or after %s yet",
				g.Describe(), len(msgs), since.Format("15:04:05"))
			return "", nil
		}
		return found, nil
	})
	if errors.Is(err, errWaitedTooLong) {
		return "", fmt.Errorf("%s: no message at or after %s matching %q within %s",
			g.Describe(), since.Format(time.RFC3339), g.Spec.Query, p.timeout)
	}
	return code, err
}

// extract returns the code from the newest qualifying message. since must
// already be floored to the second.
func (g *Gmail) extract(msgs []gmail.Message, since time.Time) (string, bool) {
	pattern := g.Spec.pattern()
	var best gmail.Message
	var bestCode string
	var found bool

	for _, m := range msgs {
		// since has already been floored to the second by Fetch; see there.
		if m.Received.Before(since) {
			continue
		}
		match := pattern.FindStringSubmatch(m.Body)
		if len(match) < 2 {
			continue
		}
		code := match[1]
		if err := validateDigits(code, g.Spec.Digits); err != nil {
			g.warn("WARN: %s: message %s: %v", g.Describe(), m.ID, err)
			continue
		}
		if !found || m.Received.After(best.Received) {
			best, bestCode, found = m, code, true
		}
	}
	return bestCode, found
}
