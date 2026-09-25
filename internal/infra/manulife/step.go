package manulife

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

// Step names used with [steperr.Error]. They double as page-dump labels, so
// keep them filename-safe.
//
//declscope:package // login.go and read.go mark each failure with the step it reached
const (
	stepNavigate          = "navigate"
	stepFillCredentials   = "fill-credentials"
	stepSubmitCredentials = "submit-credentials"
	stepAwaitChallenge    = "await-challenge"
	stepFetchOTP          = "fetch-otp"
	stepSubmitOTP         = "submit-otp"
	stepAwaitHome         = "await-home"
	stepReadList          = "read-list"
	stepOpenContract      = "open-contract"
	stepReadContract      = "read-contract"
)

// stepErr marks err as having failed at the named step.
//
//declscope:package // every phase of the flow marks its failures with this
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }

// StepOf returns the failing step name, or "" if err carries no step marker.
func StepOf(err error) string { return steperr.Of(err) }
