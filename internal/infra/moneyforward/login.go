package moneyforward

import (
	"context"
	"fmt"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/selector"
	"maps"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/chrome/browser"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/otp"
)

const (
	// loginFormTimeout covers ordinary page transitions.
	loginFormTimeout = 30 * time.Second

	// loginChallengeTimeout covers the fork after submitting credentials: MF either
	// shows the OTP challenge or drops straight to the home page.
	loginChallengeTimeout = 45 * time.Second

	// loginSubmitProbeTimeout is short on purpose — the OTP button either is already
	// on the page or we fall back to pressing Enter.
	loginSubmitProbeTimeout = 3 * time.Second

	// loginHomeTimeout is generous because the post-OTP SSO round-trip between
	// id.moneyforward.com and moneyforward.com involves several redirects.
	loginHomeTimeout = 60 * time.Second

	// loginSettleTimeout is how long the redirect chain gets to finish on its own
	// before the app is opened explicitly.
	loginSettleTimeout = 25 * time.Second
)

// Keys mixed into the OTP candidate probe. They must not collide with a key in
// [selector.OTPInputCandidates].
const (
	loginHomeCandidateKey       = "__home__"
	loginIDPortalCandidateKey   = "__id_portal__"
	loginSignInFormCandidateKey = "__sign_in_form__"
)

// LoginResult reports what actually happened, so callers can log it and so the
// dev command can print which unconfirmed selector matched.
type LoginResult struct {
	// AlreadyAuthenticated reports that the browser arrived with a live session
	// and no credentials were submitted.
	AlreadyAuthenticated bool

	// OTPRequired is false when MF accepted the session without a challenge.
	OTPRequired bool

	// OTPInputKey and OTPSubmitKey are the keys from selector.OTPInputCandidates and
	// selector.OTPSubmitCandidates that matched. Empty when no OTP was required, and
	// OTPSubmitKey is empty when the Enter-key fallback was used.
	OTPInputKey  string
	OTPSubmitKey string
}

// Login performs the full ID/PW + email-OTP sequence and returns once the
// authenticated home page is visible.
//
// src supplies the one-time code: [otp.Gmail] reads it out of the mailbox
// unattended, [otp.File] takes it from a path someone writes it to. The second
// is what makes this callable from a laptop while the selectors are still being
// pinned down.
func (c *Client) Login(ctx context.Context, src otp.Source) (LoginResult, error) {
	var res LoginResult
	if err := c.Validate(); err != nil {
		return res, err
	}

	if err := runWithLoginTimeout(ctx, loginFormTimeout, chromedp.Navigate(selector.SignInURL)); err != nil {
		return res, stepErr(stepNavigate, err)
	}

	// The sign-in page is not necessarily what comes back. An existing session
	// redirects straight past it, and then waiting for the email field times out
	// on a page that is perfectly fine — a confusing failure for the most benign
	// possible state.
	landing, err := browser.PageOf(ctx).WaitForAny(loginFormTimeout, map[string]string{
		loginSignInFormCandidateKey: selector.EmailInput,
		loginHomeCandidateKey:       selector.HomeAnchor,
		loginIDPortalCandidateKey:   selector.IDPortalMarker,
	})
	if err != nil {
		return res, stepErr(stepNavigate, browser.PageOf(ctx).WithLocation(err))
	}
	if landing != loginSignInFormCandidateKey {
		res.AlreadyAuthenticated = true
		return res, c.enterAppAfterLogin(ctx)
	}

	// WaitVisible on the password field rather than assuming it renders with the
	// email field: if MF has since split this into two steps, the failure names
	// the password field instead of silently typing into nothing.
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.SendKeys(selector.EmailInput, c.Email, chromedp.ByQuery),
		chromedp.WaitVisible(selector.PasswordInput, chromedp.ByQuery),
		chromedp.SendKeys(selector.PasswordInput, c.Password, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(stepFillCredentials, err)
	}

	// Capture the instant before the click: MF sends the OTP email in response
	// to it, so any code stamped earlier belongs to a previous attempt.
	submittedAt := time.Now()
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Click(selector.SignInSubmit, chromedp.ByQuery),
	); err != nil {
		return res, stepErr(stepSubmitCredentials, err)
	}

	var hit string

	// Race the OTP challenge against the two places a successful sign-in can
	// land. Waiting on only one would hang for the full timeout in the other
	// cases and look like a broken selector.
	probe := make(map[string]string, len(selector.OTPInputCandidates)+2)
	maps.Copy(probe, selector.OTPInputCandidates)
	probe[loginHomeCandidateKey] = selector.HomeAnchor
	probe[loginIDPortalCandidateKey] = selector.IDPortalMarker

	hit, err = browser.PageOf(ctx).WaitForAny(loginChallengeTimeout, probe)
	if err != nil {
		// None of the three appearing usually means the credentials were
		// rejected, which leaves the browser on the sign-in page.
		return res, stepErr(StepAwaitChallenge, browser.PageOf(ctx).WithLocation(err))
	}

	// Anything that is not one of those two landings is the challenge.
	if hit != loginHomeCandidateKey && hit != loginIDPortalCandidateKey {
		res.OTPRequired = true
		res.OTPInputKey = hit

		code, err := src.Fetch(ctx, submittedAt)
		if err != nil {
			return res, stepErr(stepFetchOTP, fmt.Errorf("via %s: %w", src.Describe(), err))
		}
		if err := c.submitLoginOTP(ctx, selector.OTPInputCandidates[hit], code, &res); err != nil {
			return res, err
		}
	}

	return res, c.enterAppAfterLogin(ctx)
}

// enterAppAfterLogin opens moneyforward.com and confirms the session reached it.
//
// Signing in at id.moneyforward.com authenticates the account but leaves the
// browser on the ID portal, which links nowhere into the app and carries none of
// its markers. Waiting there for the app's home anchor simply times out — which
// is exactly what happened before this step existed, and it read as a broken
// selector rather than a browser sitting in the wrong place.
//
// Harmless when already in the app: the navigation is a no-op and the marker is
// already present.
func (c *Client) enterAppAfterLogin(ctx context.Context) error {
	// Signing in through the app's own entry point already lands in the app, via
	// a redirect chain that may still be in flight. Navigating on top of that
	// cancels it — ERR_ABORTED — so wait for the marker first and only steer if
	// it never arrives.
	if err := runWithLoginTimeout(ctx, loginSettleTimeout,
		chromedp.WaitVisible(selector.HomeAnchor, chromedp.ByQuery),
	); err == nil {
		return nil
	}

	if err := runWithLoginTimeout(ctx, loginHomeTimeout,
		chromedp.Navigate(selector.HomeURL),
		chromedp.WaitVisible(selector.HomeAnchor, chromedp.ByQuery),
	); err != nil {
		return stepErr(stepAwaitHome, browser.PageOf(ctx).WithLocation(
			fmt.Errorf("opening %s after authentication: %w", selector.HomeURL, err)))
	}
	return nil
}

// runWithLoginTimeout bounds a chromedp action group. Deriving a sub-context is the
// supported way to time-box actions: it cancels the actions, not the browser, so
// the caller's Chrome stays alive for a page dump afterwards.
func runWithLoginTimeout(ctx context.Context, d time.Duration, actions ...chromedp.Action) error {
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return chromedp.Run(tctx, actions...)
}

// submitLoginOTP types the code and confirms it, preferring a real button and
// falling back to Enter. The fallback matters because one-time-code forms
// frequently auto-submit and may render no button at all.
func (c *Client) submitLoginOTP(ctx context.Context, otpSelector, code string, res *LoginResult) error {
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.SendKeys(otpSelector, code, chromedp.ByQuery),
	); err != nil {
		return stepErr(stepSubmitOTP, fmt.Errorf("type code into %s: %w", otpSelector, err))
	}

	buttonKey, err := browser.PageOf(ctx).WaitForAny(loginSubmitProbeTimeout, selector.OTPSubmitCandidates)
	if err != nil {
		if err := runWithLoginTimeout(ctx, loginFormTimeout, chromedp.KeyEvent(kb.Enter)); err != nil {
			return stepErr(stepSubmitOTP, fmt.Errorf("no submit button matched and Enter failed: %w", err))
		}
		return nil
	}

	res.OTPSubmitKey = buttonKey
	if err := runWithLoginTimeout(ctx, loginFormTimeout,
		chromedp.Click(selector.OTPSubmitCandidates[buttonKey], chromedp.ByQuery),
	); err != nil {
		return stepErr(stepSubmitOTP, fmt.Errorf("click %s: %w", selector.OTPSubmitCandidates[buttonKey], err))
	}
	return nil
}
