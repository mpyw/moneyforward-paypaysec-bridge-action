package manulife

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

const (
	// loginFormTimeout covers ordinary page transitions.
	loginFormTimeout = 30 * time.Second

	// loginChallengeTimeout covers the fork after submitting credentials: the code
	// field appearing, or straight through to the contract list.
	loginChallengeTimeout = 2 * time.Minute

	// loginDigitInterval paces the key events that enter the code.
	loginDigitInterval = 120 * time.Millisecond

	// loginHomeTimeout allows for the post-OTP redirect chain.
	loginHomeTimeout = 60 * time.Second
)

// loginOTPCandidateKey names the challenge in the race against the contract list.
const loginOTPCandidateKey = "otp"

// LoginResult reports what actually happened during a sign-in.
type LoginResult struct {
	// OTPRequired is false when the site accepted the session without a
	// challenge.
	OTPRequired bool
}

// Login performs the full ID/PW + email-OTP sequence and returns once the
// contract list is visible.
//
// src supplies the one-time code: [otp.Gmail] reads it out of the mailbox
// unattended, [otp.File] takes it from a path someone writes it to. The second
// tells a browser fault apart from a mail fault, which is worth having on a
// service that mails a code per attempt and will not do so indefinitely.
func (c *Client) Login(ctx context.Context, src otp.Source) (LoginResult, error) {
	var res LoginResult
	if err := c.Validate(); err != nil {
		return res, err
	}

	// The sign-in form, not the site root: the root's only control is a button
	// running window.location='/auth', so a run pointed at it waits for inputs
	// that are not there.
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Navigate(selector.LoginURL),
		chromedp.WaitVisible(selector.UsernameInput, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(stepNavigate, err)
	}

	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.SendKeys(selector.UsernameInput, c.Username, chromedp.ByQuery),
		chromedp.WaitVisible(selector.PasswordInput, chromedp.ByQuery),
		chromedp.SendKeys(selector.PasswordInput, c.Password, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(stepFillCredentials, err)
	}

	// Captured before the click: the code is mailed in response to it, so
	// anything stamped earlier belongs to a previous attempt. The same address
	// also sends a login notice thirteen seconds later, which is why the mail
	// spec discriminates on the body rather than on time alone.
	submittedAt := time.Now()
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Click(selector.LoginSubmit, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(stepSubmitCredentials, err)
	}

	// Raced because the two outcomes are both normal: the form posts back to
	// itself with the code field revealed, or a recognised session goes
	// straight through. Waiting on only one hangs for the whole timeout in the
	// other case, which reads as a broken selector when nothing is broken.
	candidates := map[string]string{loginOTPCandidateKey: selector.OTPInput}
	maps.Copy(candidates, selector.HomeCandidates)
	hit, err := browser.PageOf(ctx).WaitForAny(loginChallengeTimeout, candidates)
	if err != nil {
		// Neither appearing usually means the credentials were rejected, which
		// leaves the browser on the sign-in page and is worth saying.
		return res, stepErr(stepAwaitChallenge, browser.PageOf(ctx).WithLocation(err))
	}
	if hit != loginOTPCandidateKey {
		return res, nil
	}
	res.OTPRequired = true

	code, err := src.Fetch(ctx, submittedAt)
	if err != nil {
		return res, stepErr(stepFetchOTP, fmt.Errorf("via %s: %w", src.Describe(), err))
	}
	if err := c.submitLoginOTP(ctx, code); err != nil {
		return res, err
	}

	if err := runWithLoginTimeout(ctx, loginHomeTimeout,
		chromedp.WaitVisible(selector.ContractCard, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(stepAwaitHome, browser.PageOf(ctx).WithLocation(err))
	}
	return res, nil
}

// submitLoginOTP types the code, checks it landed, and submits.
//
// The digits go in as individual key events rather than as a value assignment.
// The field is a single one here, so either would work today — but the page
// filters input through its own handlers, and a form that ignores a
// programmatic write is a thing this project has already met on the PayPay 証券
// challenge, where the submit control stays hidden until its keypress handler
// has seen every digit.
func (c *Client) submitLoginOTP(ctx context.Context, code string) error {
	if len(code) != selector.OTPDigits {
		return stepErr(stepSubmitOTP, fmt.Errorf("expected %d digits, got %d", selector.OTPDigits, len(code)))
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return stepErr(stepSubmitOTP, fmt.Errorf("code contains a non-digit: %q", code))
		}
	}

	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Click(selector.OTPInput, chromedp.ByQuery),
	); err != nil {
		return stepErr(stepSubmitOTP, fmt.Errorf("focus %s: %w", selector.OTPInput, err))
	}
	for i, r := range code {
		if err := runWithLoginTimeout(ctx, loginFormTimeout,
			chromedp.KeyEvent(string(r)),
			chromedp.Sleep(loginDigitInterval),
		); err != nil {
			return stepErr(stepSubmitOTP, fmt.Errorf("type digit %d of %d: %w", i+1, selector.OTPDigits, err))
		}
	}

	// Read the field back before submitting anything.
	//
	// This exists because of what happened without it. A selector matched the
	// submit button instead of the field, so the "focus" click submitted the
	// form with the code empty, the digits went to no element at all, and the
	// page re-rendered looking exactly as it had. Every step reported success
	// and the one-time code was spent.
	//
	// Submitting a form whose contents were never confirmed is the same mistake
	// as trusting a write that was never read back, which the ledger side of
	// this program refuses to make. It costs one round trip on a path that is
	// about to spend a credential.
	var got string
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Value(selector.OTPInput, &got, chromedp.ByQuery),
	); err != nil {
		return stepErr(stepSubmitOTP, fmt.Errorf("read %s back: %w", selector.OTPInput, err))
	}
	if got != code {
		// Neither value is logged: one is the code, and the other is whatever
		// the field holds, which is no safer to print.
		return stepErr(stepSubmitOTP, fmt.Errorf(
			"%s holds %d character(s) after typing %d — the digits did not reach the "+
				"field, so submitting now would send an empty code",
			selector.OTPInput, len([]rune(got)), len(code)))
	}

	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Click(selector.OTPSubmit, chromedp.ByQuery),
	); err != nil {
		return stepErr(stepSubmitOTP, fmt.Errorf("click %s: %w", selector.OTPSubmit, err))
	}
	return nil
}

// runWithLoginTimeout bounds a chromedp action group. Deriving a sub-context cancels
// the actions, not the browser, so the caller's Chrome stays alive for a page
// dump afterwards.
func runWithLoginTimeout(ctx context.Context, d time.Duration, actions ...chromedp.Action) error {
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return chromedp.Run(tctx, actions...)
}
