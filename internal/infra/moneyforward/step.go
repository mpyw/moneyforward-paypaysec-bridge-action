package moneyforward

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

// Step names used by StepError. They double as page-dump labels, so keep them
// filename-safe.
//
//declscope:shared // login.go marks each failure with the step it reached
const (
	StepAwaitChallenge    = "await-challenge"
	stepNavigate          = "navigate"
	stepFillCredentials   = "fill-credentials"
	stepSubmitCredentials = "submit-credentials"
	stepFetchOTP          = "fetch-otp"
	stepSubmitOTP         = "submit-otp"
	stepAwaitHome         = "await-home"
)

// StepOf returns the failing step name, or "" if err carries no step marker.
// It re-exports [browser.StepOf] so callers of this package need not import the
// browser layer just to read an error.
func StepOf(err error) string { return steperr.Of(err) }

// stepErr marks err as having failed at the named step. The error type itself
// lives in internal/browser because the PayPay flow needs the same thing, and
// cmd/sync inspects failures from both.
//
//declscope:shared // every phase of the flow marks its failures with this
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }
