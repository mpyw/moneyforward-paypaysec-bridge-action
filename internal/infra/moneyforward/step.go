package moneyforward

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

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
//
//declscope:package // every phase of the flow marks its failures with this
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }

// StepOf returns the failing step name, or "" if err carries no step marker.
// It re-exports [browser.StepOf] so callers of this package need not import the
// browser layer just to read an error.
func StepOf(err error) string { return steperr.Of(err) }
