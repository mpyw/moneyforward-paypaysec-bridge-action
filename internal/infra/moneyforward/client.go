// Package moneyforward drives MoneyForward Me: authenticate with ID/PW plus the
// new-device email OTP, then update a manual asset's balance.
//
// File layout:
//
//	client.go     — the Client and its step vocabulary
//	login.go      — authentication
//	asset.go      — the balance write
//	selectors.go  — every DOM selector and URL
package moneyforward

import (
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

// Client holds the credentials for one MoneyForward account. The zero value is
// not usable; call Validate before driving a browser with it.
type Client struct {
	Email    string
	Password string

	// AssetID is the account_id_hash of the manual asset to update. Only needed
	// by UpdateAssetBalance, so login-only callers may leave it empty.
	AssetID string
}

// Validate reports missing credentials up front, rather than letting a browser
// launch and an empty form submission produce a confusing failure three steps
// later.
func (c *Client) Validate() error {
	var missing []string
	if c.Email == "" {
		missing = append(missing, "Email")
	}
	if c.Password == "" {
		missing = append(missing, "Password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("moneyforward: missing credentials: %v", missing)
	}
	return nil
}

// Step names used by StepError. They double as page-dump labels, so keep them
// filename-safe.
const (
	StepNavigate          = "navigate"
	StepFillCredentials   = "fill-credentials"
	StepSubmitCredentials = "submit-credentials"
	StepAwaitChallenge    = "await-challenge"
	StepFetchOTP          = "fetch-otp"
	StepSubmitOTP         = "submit-otp"
	StepAwaitHome         = "await-home"
)

// stepErr marks err as having failed at the named step. The error type itself
// lives in internal/browser because the PayPay flow needs the same thing, and
// cmd/sync inspects failures from both.
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }

// StepOf returns the failing step name, or "" if err carries no step marker.
// It re-exports [browser.StepOf] so callers of this package need not import the
// browser layer just to read an error.
func StepOf(err error) string { return steperr.Of(err) }
