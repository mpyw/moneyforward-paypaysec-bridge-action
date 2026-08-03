package paypaysec

import (
	"context"
	"fmt"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec/selector"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/otp"
)

const (
	// formTimeout covers ordinary page transitions.
	formTimeout = 30 * time.Second

	// challengeTimeout covers the fork after submitting credentials: the OTP
	// page on an unrecognised device, or straight to the dashboard.
	//
	// Generous because it also has to cover the debug flow, where the OTP
	// selectors are not yet confirmed and a human types the code into the
	// visible window — the dashboard is then what this ends up waiting for.
	challengeTimeout = 3 * time.Minute

	// digitInterval paces the six single-digit key events.
	digitInterval = 120 * time.Millisecond

	// otpSubmitTimeout is how long the real submit anchor gets to appear once
	// the digits are in.
	otpSubmitTimeout = 15 * time.Second

	// dashboardTimeout allows for the post-OTP redirect chain.
	dashboardTimeout = 60 * time.Second
)

// Keys for the challenge-vs-dashboard race.
const (
	otpCandidateKey       = "otp"
	dashboardCandidateKey = "dashboard"
)

// LoginResult reports what actually happened during a sign-in.
type LoginResult struct {
	// OTPRequired is false when the site accepted the session without a
	// challenge.
	OTPRequired bool

	// OTPPrefix is the two-letter prefix the challenge page displayed, e.g.
	// "QO-". The same prefix appears in the email, and the two matching is the
	// site's anti-phishing check. It is reported rather than discarded so a
	// caller can show it, or one day compare it against what arrived.
	OTPPrefix string
}

// Login performs the full ID/PW + email-OTP sequence and returns once the trade
// dashboard is visible.
//
// src supplies the one-time code: [otp.Gmail] reads it out of the mailbox
// unattended, [otp.File] takes it from a path someone writes it to. The second
// is what makes this callable from a laptop while the OTP selectors are still
// unconfirmed, and what tells a browser fault apart from a mail fault.
func (c *Client) Login(ctx context.Context, src otp.Source) (LoginResult, error) {
	var res LoginResult
	if err := c.Validate(); err != nil {
		return res, err
	}

	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Navigate(selector.LoginURL),
		chromedp.WaitVisible(selector.MemberIDInput, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepNavigate, err)
	}

	if err := runWithTimeout(ctx, formTimeout,
		chromedp.SendKeys(selector.MemberIDInput, c.Username, chromedp.ByQuery),
		chromedp.WaitVisible(selector.PasswordInput, chromedp.ByQuery),
		chromedp.SendKeys(selector.PasswordInput, c.Password, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepFillCredentials, err)
	}

	// Captured before the click: the OTP email is sent in response to it, so any
	// code stamped earlier belongs to a previous attempt.
	submittedAt := time.Now()
	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Click(selector.LoginSubmit, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepSubmitCredentials, err)
	}

	// Race the OTP challenge against the dashboard: a recognised device skips
	// the challenge entirely, and waiting on only one of the two would hang for
	// the full timeout in the other case.
	hit, err := browser.PageOf(ctx).WaitForAny(challengeTimeout, map[string]string{
		otpCandidateKey:       selector.OTPFirstDigit,
		dashboardCandidateKey: selector.PostLoginAnchor,
	})
	if err != nil {
		// Neither appearing usually means the credentials were rejected, which
		// leaves the browser on the login page and is worth saying.
		return res, stepErr(StepAwaitChallenge, browser.PageOf(ctx).WithLocation(err))
	}
	if hit == dashboardCandidateKey {
		return res, nil
	}

	res.OTPRequired = true
	// Best-effort: the prefix is informational, so a missing one must not fail
	// a login that is otherwise fine.
	_ = chromedp.Run(ctx, chromedp.Text(selector.OTPPrefix, &res.OTPPrefix,
		chromedp.ByQuery, chromedp.AtLeast(0)))

	code, err := src.Fetch(ctx, submittedAt)
	if err != nil {
		return res, stepErr(StepFetchOTP, fmt.Errorf("via %s: %w", src.Describe(), err))
	}

	if err := c.submitOTP(ctx, code); err != nil {
		return res, err
	}

	if err := runWithTimeout(ctx, dashboardTimeout,
		chromedp.WaitVisible(selector.PostLoginAnchor, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepAwaitDashboard, browser.PageOf(ctx).WithLocation(err))
	}
	return res, nil
}

// submitOTP types the six digits and confirms.
//
// The digits go in as individual key events starting from #code1, because the
// following fields are readonly until the page's own keypress handler unlocks
// and focuses them. Setting values directly would leave that state machine
// untouched and the submit button hidden.
func (c *Client) submitOTP(ctx context.Context, code string) error {
	if len(code) != selector.OTPDigits {
		return stepErr(StepSubmitOTP, fmt.Errorf("expected %d digits, got %d", selector.OTPDigits, len(code)))
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return stepErr(StepSubmitOTP, fmt.Errorf("code contains a non-digit: %q", code))
		}
	}

	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Click(selector.OTPFirstDigit, chromedp.ByQuery),
	); err != nil {
		return stepErr(StepSubmitOTP, fmt.Errorf("focus %s: %w", selector.OTPFirstDigit, err))
	}

	for i, r := range code {
		if err := runWithTimeout(ctx, formTimeout,
			chromedp.KeyEvent(string(r)),
			// The handler that unlocks and focuses the next field runs on
			// keypress; give it a beat before the next one.
			chromedp.Sleep(digitInterval),
		); err != nil {
			return stepErr(StepSubmitOTP, fmt.Errorf("type digit %d of %d: %w", i+1, selector.OTPDigits, err))
		}
	}

	// The real anchor is hidden until all six digits are in; the visible
	// look-alike carries no handler. Waiting for this one to appear doubles as
	// confirmation that the digits registered.
	if err := runWithTimeout(ctx, otpSubmitTimeout,
		chromedp.WaitVisible(selector.OTPSubmit, chromedp.ByQuery),
		chromedp.Click(selector.OTPSubmit, chromedp.ByQuery),
	); err != nil {
		return stepErr(StepSubmitOTP, fmt.Errorf(
			"%s never became clickable — the digits may not have registered: %w", selector.OTPSubmit, err))
	}
	return nil
}

// runWithTimeout bounds a chromedp action group. Deriving a sub-context cancels
// the actions, not the browser, so the caller's Chrome stays alive for a page
// dump afterwards.
func runWithTimeout(ctx context.Context, d time.Duration, actions ...chromedp.Action) error {
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return chromedp.Run(tctx, actions...)
}
