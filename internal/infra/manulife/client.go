// Package manulife drives the マニュライフ生命 マイページ: authenticate with
// ID/PW plus an email OTP, then read a contract's valuation.
//
// The site is Salesforce Visualforce, which decides the shape of everything
// here. Three facts are worth stating once, because each was established by
// recording what the browser actually did rather than by reading markup:
//
//   - There is no API to prefer. Of the traffic a signed-in session produces,
//     the only calls carrying the site's own data are Visualforce partial
//     postbacks — and their bodies carry no figures at all. The numbers arrive
//     server-rendered in the document. So unlike PayPay 証券's 投資信託, there is
//     no endpoint to move to; the page is the only source there is.
//
//   - A session cannot be moved between browsers. Saving a signed-in session's
//     cookies and restoring them into a fresh Chrome lands on /error_page rather
//     than on the contract list — twice, and the second time with Akamai Bot
//     Manager's own cookies (_abck, ak_bmsc, bm_sz) deliberately left out, on the
//     theory that a sensor verdict issued to one browser cannot speak for
//     another. It made no difference, so whatever binds the session is something
//     else and remains unidentified.
//
//     The practical consequence is that every debug run against this service
//     costs a one-time code, and services stop mailing those after a handful in
//     quick succession. It is why the harness captures as much as it can per
//     sign-in rather than one page at a time.
//
//   - A contract has no address. Its detail page is reached as
//     /policyinquiry?id=<token>, and the token is minted per rendering of the
//     list — two sign-ins produced two values for one contract. Nothing here may
//     store one, and every read starts from the list.
//
// File layout mirrors the PayPay 証券 package:
//
//	client.go    — the Client and its step vocabulary
//	login.go     — authentication
//	selector/    — every selector, URL, label and the OTP mail spec
package manulife

import (
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

// Client holds the credentials for one マイページ account. The zero value is not
// usable; call Validate before driving a browser with it.
type Client struct {
	Username string
	Password string
}

// Validate reports missing credentials up front, rather than letting an empty
// form submission fail confusingly several steps later.
func (c *Client) Validate() error {
	var missing []string
	if c.Username == "" {
		missing = append(missing, "Username")
	}
	if c.Password == "" {
		missing = append(missing, "Password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("manulife: missing credentials: %v", missing)
	}
	return nil
}

// Step names used with [steperr.Error]. They double as page-dump labels, so
// keep them filename-safe.
const (
	StepNavigate          = "navigate"
	StepFillCredentials   = "fill-credentials"
	StepSubmitCredentials = "submit-credentials"
	StepAwaitChallenge    = "await-challenge"
	StepFetchOTP          = "fetch-otp"
	StepSubmitOTP         = "submit-otp"
	StepAwaitHome         = "await-home"
	StepReadList          = "read-list"
	StepOpenContract      = "open-contract"
	StepReadContract      = "read-contract"
)

// stepErr marks err as having failed at the named step.
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }

// StepOf returns the failing step name, or "" if err carries no step marker.
func StepOf(err error) string { return steperr.Of(err) }
