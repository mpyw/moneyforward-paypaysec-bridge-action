package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/credential"
)

// ConsentFlow obtains a Gmail credential by asking a person to grant one.
type ConsentFlow interface {
	Obtain(ctx context.Context) (credential.Gmail, error)
}

// CredentialStore keeps a credential where a later run will find it.
type CredentialStore interface {
	Store(ctx context.Context, cred credential.Gmail) error
}

// MailboxOpener reports which mailbox a credential opens.
//
// The cheapest way to find out both that a credential works and that it belongs
// to the account that was meant — which the consent screen cannot tell anyone,
// since it names the application rather than the mailbox.
type MailboxOpener interface {
	OpenMailbox(ctx context.Context, cred credential.Gmail) (string, error)
}
