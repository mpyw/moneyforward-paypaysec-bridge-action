// Package authorizegmail obtains the credential the scheduled job reads mail
// with.
//
// Short, because the mechanism is not the decision: parsing an OAuth client,
// binding a loopback port, waiting for a redirect, exchanging a code. What is
// left is the part worth stating — a credential that cannot be
// used unattended is not one this program can accept, however successful the
// flow looked.
package authorizegmail

import (
	"context"
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/port"
)

// Authorize runs the consent flow and keeps what it produced.
type Authorize struct {
	Consent port.ConsentFlow
	Store   port.CredentialStore

	// Verify, if set, is asked to prove the credential opens a mailbox before it
	// is kept. Optional: a caller with no network may skip it.
	Verify port.MailboxOpener
}

// Result is what a run produced.
type Result struct {
	// Mailbox is the address the credential turned out to open, when it was
	// checked. Which account consented is the one thing a person cannot verify
	// from the consent screen alone — it says the application's name, not
	// theirs.
	Mailbox string
}

// Run obtains a credential, refuses it if it will not survive on its own, and
// stores it.
func (a Authorize) Run(ctx context.Context) (Result, error) {
	var result Result

	cred, err := a.Consent.Obtain(ctx)
	if err != nil {
		return result, err
	}
	if err := cred.UsableUnattended(); err != nil {
		return result, fmt.Errorf("%w — the scheduled job runs with nobody watching, so "+
			"a credential that needs a person is no credential at all", err)
	}

	// Before storing, not after: a credential written to disk is one somebody
	// will assume works.
	if a.Verify != nil {
		result.Mailbox, err = a.Verify.OpenMailbox(ctx, cred)
		if err != nil {
			return result, fmt.Errorf("the credential was granted but does not work: %w", err)
		}
	}

	if err := a.Store.Store(ctx, cred); err != nil {
		return result, err
	}
	return result, nil
}
