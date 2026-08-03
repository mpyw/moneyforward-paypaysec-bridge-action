// Package paypaysec drives the PayPay 証券 PC trading site: authenticate with
// ID/PW plus an email OTP, then read the balance.
//
// The site has no single "account total". Each category view
// (/trade?country=japan, ?country=usa, ?reserve_mode=1 …) shows its own
// 評価額合計, and the account total is their sum. ミニアプリ is one of those
// views, kept separate so the orchestrator can cross-check it.
//
// File layout:
//
//	client.go     — the Client and its step vocabulary
//	login.go      — authentication
//	balance.go    — reading and summing the category totals
//	selectors.go  — every DOM selector, URL, and extraction snippet
package paypaysec

import (
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/helpers/steperr"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// Client holds the credentials for one PayPay 証券 account. The zero value is
// not usable; call Validate before driving a browser with it.
type Client struct {
	Username string
	Password string

	// OnRead, if set, is handed every reading as soon as it is taken —
	// including one that then fails to reconcile.
	//
	// It exists so the scheduled job can register the figures with the Actions
	// log masker before anything can print them. The reconciliation errors name
	// the amounts that disagree, which is what makes them useful and also what
	// would otherwise put a balance in a workflow log verbatim. Masking after
	// the fact does not work: ::add-mask:: only affects output that comes after
	// it.
	OnRead func(Reading)

	// OnSkip, if set, is told about a target the account does not have.
	//
	// Separate from OnRead because a skip is not a reading: there are no figures
	// to mask and no amounts to reconcile. What it needs is to be said out loud.
	// A category left out of a run is invisible otherwise — the guards keep its
	// recorded entries from being deleted, so the run succeeds and the entries
	// quietly stop being updated, which is worth a line in the log.
	OnSkip func(t selector.Target, why error)
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
		return fmt.Errorf("paypaysec: missing credentials: %v", missing)
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
	StepAwaitDashboard    = "await-dashboard"
	StepReadBalance       = "read-balance"
)

// stepErr marks err as having failed at the named step.
func stepErr(step string, err error) error { return steperr.Wrap(step, err) }

// StepOf returns the failing step name, or "" if err carries no step marker.
// It re-exports [browser.StepOf] so callers need not import the browser layer
// just to read an error.
func StepOf(err error) string { return steperr.Of(err) }
