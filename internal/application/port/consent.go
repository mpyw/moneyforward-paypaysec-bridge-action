package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/credential"
)

// ConsentFlow obtains a Gmail credential by asking a person to grant one.
type ConsentFlow interface {
	Obtain(ctx context.Context) (credential.Gmail, error)
}
