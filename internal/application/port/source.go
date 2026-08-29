package port

import (
	"context"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
)

// Source is an account holdings are read from.
//
// There is more than one, and they are read independently: PayPay 証券 moves
// every weekday, an insurance contract's surrender value monthly. Failing the
// whole run because one of them could not be signed into would stop the other
// being recorded for no reason.
type Source interface {
	// ID names this source in log lines and in the error a failed read
	// produces. Short and stable — "paypaysec", "manulife" — because it is also
	// how a person is told which half of a run went wrong.
	ID() string

	// SignIn establishes a session, obtaining a one-time code if one is asked
	// for.
	//
	// Separate from reading because when it happens matters. Every service here
	// mails a code, and a code stamped before its own request belongs to a
	// previous attempt — so which sign-in happens when is not an implementation
	// detail.
	SignIn(ctx context.Context) error

	// Holdings reads one entry per 銘柄, and reports which categories it
	// covered — including the ones that turned out to hold nothing. Without
	// that, an empty category and an unread one are the same answer.
	Holdings(ctx context.Context) (asset.Holdings, error)
}
