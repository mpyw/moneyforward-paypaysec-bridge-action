// Package credential describes what this program needs a credential to be.
//
// Not what one looks like on the wire — that is Google's business and lives in
// the infrastructure that speaks to them. What is here is the one property that
// decides whether a credential is any use: it has to keep working with nobody
// watching.
package credential

import "errors"

// Gmail is a user credential for reading one mailbox.
//
// A user credential and not a service account: domain-wide delegation is a
// Workspace feature, so a service account — including one reached from GitHub
// Actions through Workload Identity Federation — cannot read a consumer inbox
// at all.
type Gmail struct {
	ClientID     string
	ClientSecret string

	// RefreshToken is what makes it usable tomorrow.
	RefreshToken string
}

// ErrNotUnattended reports a credential that cannot be used without a person.
var ErrNotUnattended = errors.New("credential has no refresh token")

// UsableUnattended reports whether this will still work on a schedule.
//
// Google will complete a consent flow without issuing a refresh token when the
// account has consented before, and everything about that looks like success:
// a token comes back, the mailbox opens, the command prints a credential. It
// then expires within the hour and every scheduled run after that fails on
// authentication, which is a long way from where the mistake was made.
func (g Gmail) UsableUnattended() error {
	if g.RefreshToken == "" {
		return ErrNotUnattended
	}
	return nil
}
