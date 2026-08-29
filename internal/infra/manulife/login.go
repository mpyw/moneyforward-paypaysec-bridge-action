package manulife

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife/selector"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

const (
	// formTimeout covers ordinary page transitions.
	formTimeout = 30 * time.Second

	// challengeTimeout covers the fork after submitting credentials: the code
	// field appearing, or straight through to the contract list.
	challengeTimeout = 2 * time.Minute

	// digitInterval paces the key events that enter the code.
	digitInterval = 120 * time.Millisecond

	// homeTimeout allows for the post-OTP redirect chain.
	homeTimeout = 60 * time.Second
)

// otpCandidateKey names the challenge in the race against the contract list.
const otpCandidateKey = "otp"

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
	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Navigate(selector.LoginURL),
		chromedp.WaitVisible(selector.UsernameInput, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepNavigate, err)
	}

	if err := runWithTimeout(ctx, formTimeout,
		chromedp.SendKeys(selector.UsernameInput, c.Username, chromedp.ByQuery),
		chromedp.WaitVisible(selector.PasswordInput, chromedp.ByQuery),
		chromedp.SendKeys(selector.PasswordInput, c.Password, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepFillCredentials, err)
	}

	// Captured before the click: the code is mailed in response to it, so
	// anything stamped earlier belongs to a previous attempt. The same address
	// also sends a login notice thirteen seconds later, which is why the mail
	// spec discriminates on the body rather than on time alone.
	submittedAt := time.Now()
	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Click(selector.LoginSubmit, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepSubmitCredentials, err)
	}

	// Raced because the two outcomes are both normal: the form posts back to
	// itself with the code field revealed, or a recognised session goes
	// straight through. Waiting on only one hangs for the whole timeout in the
	// other case, which reads as a broken selector when nothing is broken.
	candidates := map[string]string{otpCandidateKey: selector.OTPInput}
	for name, sel := range selector.HomeCandidates {
		candidates[name] = sel
	}
	hit, err := browser.PageOf(ctx).WaitForAny(challengeTimeout, candidates)
	if err != nil {
		// Neither appearing usually means the credentials were rejected, which
		// leaves the browser on the sign-in page and is worth saying.
		return res, stepErr(StepAwaitChallenge, browser.PageOf(ctx).WithLocation(err))
	}
	if hit != otpCandidateKey {
		return res, nil
	}
	res.OTPRequired = true

	code, err := src.Fetch(ctx, submittedAt)
	if err != nil {
		return res, stepErr(StepFetchOTP, fmt.Errorf("via %s: %w", src.Describe(), err))
	}
	if err := c.submitOTP(ctx, code); err != nil {
		return res, err
	}

	if err := runWithTimeout(ctx, homeTimeout,
		chromedp.WaitVisible(selector.ContractCard, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(StepAwaitHome, browser.PageOf(ctx).WithLocation(err))
	}
	return res, nil
}

// submitOTP types the code, checks it landed, and submits.
//
// The digits go in as individual key events rather than as a value assignment.
// The field is a single one here, so either would work today — but the page
// filters input through its own handlers, and a form that ignores a
// programmatic write is a thing this project has already met on the PayPay 証券
// challenge, where the submit control stays hidden until its keypress handler
// has seen every digit.
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
		chromedp.Click(selector.OTPInput, chromedp.ByQuery),
	); err != nil {
		return stepErr(StepSubmitOTP, fmt.Errorf("focus %s: %w", selector.OTPInput, err))
	}
	for i, r := range code {
		if err := runWithTimeout(ctx, formTimeout,
			chromedp.KeyEvent(string(r)),
			chromedp.Sleep(digitInterval),
		); err != nil {
			return stepErr(StepSubmitOTP, fmt.Errorf("type digit %d of %d: %w", i+1, selector.OTPDigits, err))
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
	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Value(selector.OTPInput, &got, chromedp.ByQuery),
	); err != nil {
		return stepErr(StepSubmitOTP, fmt.Errorf("read %s back: %w", selector.OTPInput, err))
	}
	if got != code {
		// Neither value is logged: one is the code, and the other is whatever
		// the field holds, which is no safer to print.
		return stepErr(StepSubmitOTP, fmt.Errorf(
			"%s holds %d character(s) after typing %d — the digits did not reach the "+
				"field, so submitting now would send an empty code",
			selector.OTPInput, len([]rune(got)), len(code)))
	}

	if err := runWithTimeout(ctx, formTimeout,
		chromedp.Click(selector.OTPSubmit, chromedp.ByQuery),
	); err != nil {
		return stepErr(StepSubmitOTP, fmt.Errorf("click %s: %w", selector.OTPSubmit, err))
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
