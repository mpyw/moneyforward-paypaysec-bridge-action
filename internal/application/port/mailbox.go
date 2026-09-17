package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/credential"
)

// MailboxOpener reports which mailbox a credential opens.
//
// The cheapest way to find out both that a credential works and that it belongs
// to the account that was meant — which the consent screen cannot tell anyone,
// since it names the application rather than the mailbox.
type MailboxOpener interface {
	OpenMailbox(ctx context.Context, cred credential.Gmail) (string, error)
}
