package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/asset"
)

// Broker is the account the holdings are read from.
type Broker interface {
	// SignIn establishes a session, obtaining a one-time code if one is asked
	// for.
	//
	// Separate from reading because when it happens matters. Both services here
	// mail a code, and a code stamped before its own request belongs to a
	// previous attempt — so which sign-in happens when is not an implementation
	// detail.
	SignIn(ctx context.Context) error

	// Holdings reads one entry per 銘柄, and reports which categories it
	// covered — including the ones that turned out to hold nothing. Without
	// that, an empty category and an unread one are the same answer.
	Holdings(ctx context.Context) (asset.Holdings, error)
}
