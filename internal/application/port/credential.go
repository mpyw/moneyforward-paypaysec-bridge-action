package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/credential"
)

// CredentialStore keeps a credential where a later run will find it.
type CredentialStore interface {
	Store(ctx context.Context, cred credential.Gmail) error
}
