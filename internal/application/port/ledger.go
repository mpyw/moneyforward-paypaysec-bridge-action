package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
)

// Ledger is the account the holdings are recorded in.
//
// Addressed entirely by name: whatever identifiers the service uses to tell one
// row from another are its own business, and there is no way for a use case to
// hold them that would not make it that service's.
type Ledger interface {
	SignIn(ctx context.Context) error

	// Recorded is what the ledger holds now.
	Recorded(ctx context.Context) ([]asset.Asset, error)

	Create(ctx context.Context, a asset.Asset) error
	Update(ctx context.Context, a asset.Asset) error
	Delete(ctx context.Context, name string) error
}
