package manulife

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
)

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
//
//declscope:package // every phase of the flow marks its failures with this
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }

// StepOf returns the failing step name, or "" if err carries no step marker.
func StepOf(err error) string { return steperr.Of(err) }
