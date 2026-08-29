//go:build wireinject

// The build tag keeps this file out of the binary: it is the specification wire
// reads, not code that runs.
//
//	go run github.com/google/wire/cmd/wire ./internal/cli/commands/debug/manulife

package manulife

import (
	"context"

	"github.com/google/wire"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
)

// newSignIn assembles the site client and where its one-time code comes from.
func newSignIn(ctx context.Context, opts *session.Options) (signIn, error) {
	wire.Build(
		providerSet,
		wire.Struct(new(signIn), "*"),
	)
	return signIn{}, nil
}
