//go:build wireinject

// The build tag keeps this file out of the binary: it is the specification wire
// reads, not code that runs.
//
//	go run github.com/google/wire/cmd/wire ./internal/cli/commands/debug/paypaysec

package paypaysec

import (
	"context"

	"github.com/google/wire"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
)

// newClient assembles the site client and where its one-time code comes from.
//
// There is no use case here, and this package is the only one where that is
// true. Its two substantive commands are deliberately not the port: `login` is
// a single port operation, and `balance` exists precisely to do what the port
// will not — read all eight targets and show the ones that failed, rather than
// stopping at the first. Wrapping either would remove the reason it exists.
//
// The assembly is still assembly, though, and "where is this put together"
// should have the same answer here as everywhere else.
func newSignIn(ctx context.Context, opts *session.Options) (signIn, error) {
	wire.Build(
		providerSet,
		wire.Struct(new(signIn), "*"),
	)
	return signIn{}, nil
}
