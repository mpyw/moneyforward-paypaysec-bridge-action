// Package otp abstracts "give me the one-time code that was just emailed".
//
// Two implementations, because the two situations are genuinely different:
//
//   - Gmail — unattended. Reads the mailbox the code was sent to. This is what
//     the scheduled job uses, and what local runs use once credentials exist.
//   - File — a code handed over by whoever has the mail, written to a path. Used
//     while developing, and in any context with no terminal to prompt at.
//
// The split exists so a broken scraper can be debugged without the mail path
// also being in play: when a run fails with File, the fault is in the browser
// automation and nowhere else.
package otp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/gmail"
)

// DefaultCodePattern finds a run of exactly six digits.
//
// The word boundaries matter: PayPay 証券 mail carries a two-letter prefix, so
// the body reads "AB-123456", and an unanchored \d{6} against a longer run of
// digits elsewhere in the mail would happily return the wrong six.
var DefaultCodePattern = regexp.MustCompile(`\b(\d{6})\b`)

// MailSpec describes where one service's code arrives and how to read it.
//
// This is the whole of what differs between PayPay 証券 and MoneyForward. The
// sources are shared; each site package exposes one of these next to its
// selectors, so the facts about a service stay together rather than being
// threaded through call sites as loose arguments.
type MailSpec struct {
	// Label names the service, in prompts and log lines.
	Label string

	// Query is Gmail search syntax naming where this service's mail comes from.
	// A recency bound is added by the source.
	//
	// Deliberately not narrow. Deciding which of a sender's messages is an OTP
	// mail is Pattern's job, and a query that tries to do it as well has to name
	// the subject line — a localized string, which is a fact about the recipient
	// rather than about the service.
	//
	// MoneyForward sends its subject in English to a login from a US runner and
	// in Japanese to one from Tokyo. `subject:追加認証` therefore matched every
	// message a developer saw and none of the ones CI got, so the scheduled job
	// polled for its whole window while the mail sat in every result set.
	Query string

	// Pattern extracts the code from the body; the first capture group is used.
	// Nil falls back to [DefaultCodePattern].
	//
	// This is the discriminator, so it has to reject the sender's other mail —
	// both services send a login notice within seconds of the code, from the
	// same address and inside the same freshness window.
	Pattern *regexp.Regexp

	// Digits is the expected code length.
	Digits int
}

// pattern returns the spec's pattern, or the default.
func (m MailSpec) pattern() *regexp.Regexp {
	if m.Pattern != nil {
		return m.Pattern
	}
	return DefaultCodePattern
}

// service names the service, or a placeholder when the spec is bare.
func (m MailSpec) service() string {
	if m.Label != "" {
		return m.Label
	}
	return "the service"
}

// MailSearcher is the slice of [gmail.Client] the mail-backed source needs, kept
// as an interface so the polling and extraction logic — the part that can be
// subtly wrong — is testable without a network or a mailbox.
type MailSearcher interface {
	Search(ctx context.Context, query string, max int64) ([]gmail.Message, error)
}

// Source yields a one-time code issued after `since`.
//
// Implementations must not return a code they cannot justify as newer than
// `since`. A stale code fails the login silently and burns a whole run, and in
// CI that means a whole day.
type Source interface {
	// Fetch blocks until a fresh code is available, ctx is cancelled, or the
	// implementation's own timeout elapses.
	Fetch(ctx context.Context, since time.Time) (string, error)

	// Describe returns a short human-readable name, used in log lines and in
	// the error message when Fetch gives up.
	Describe() string
}

// validateDigits reports why code is not an acceptable one-time code, or nil if
// it is. digits of 0 accepts any run of 4-8 digits.
//
// Shared by every hand-supplied source so a typo is rejected the same way
// wherever it was typed.
func validateDigits(code string, digits int) error {
	if code == "" {
		return errors.New("empty input")
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return fmt.Errorf("expected digits only, got %q", code)
		}
	}
	if digits > 0 {
		if len(code) != digits {
			return fmt.Errorf("expected %d digits, got %d", digits, len(code))
		}
		return nil
	}
	if len(code) < 4 || len(code) > 8 {
		return fmt.Errorf("expected 4-8 digits, got %d", len(code))
	}
	return nil
}
