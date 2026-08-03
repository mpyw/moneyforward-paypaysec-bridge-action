package gmail

import (
	"github.com/google/wire"

	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/credential"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/port"
	gmailapi "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/gmail"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/gmail/consent"
)

// providerSet is what the consent flow is built from.
var providerSet = wire.NewSet(
	provideConsent,
	provideCredentialStore,
	provideMailboxOpener,
)

// Two paths, both strings, and wire matches by type — so they are two types.
// The distinction is one a reader wants anyway: one is the OAuth client granted
// by the console, the other is the credential this run produces.
type (
	// clientFile is the OAuth client downloaded from the Google Cloud console.
	clientFile string

	// credentialFile is where the resulting credential is written.
	credentialFile string
)

func provideConsent(client clientFile) port.ConsentFlow {
	return consent.Flow{ClientFile: string(client)}
}

func provideCredentialStore(out credentialFile) port.CredentialStore {
	return consent.File{Path: string(out)}
}

func provideMailboxOpener() port.MailboxOpener { return mailboxOpener{} }

// mailboxOpener proves a credential works, and says which account granted it.
//
// The consent screen names the application, not the mailbox, so this is the
// only point at which anyone finds out they authorized the wrong account.
type mailboxOpener struct{}

func (mailboxOpener) OpenMailbox(ctx context.Context, cred credential.Gmail) (string, error) {
	client, err := gmailapi.NewFromCredential(ctx, cred)
	if err != nil {
		return "", err
	}
	return client.Profile(ctx)
}
