//go:build wireinject

// The build tag keeps this file out of the binary: it is the specification wire
// reads, not code that runs. Its counterpart, wire_gen.go, carries the inverse
// tag and is what actually builds the graph.
//
// Regenerate after changing a provider:
//
//	go run github.com/google/wire/cmd/wire ./internal/cli/commands/sync

package sync

import (
	"context"

	"github.com/google/wire"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/usecase/syncassets"
)

// newSync assembles the scheduled job's dependencies.
//
// The list is the whole specification — wire works out the order and the
// plumbing from the providers' signatures. What that buys here is not saved
// keystrokes but a compile-time failure when the graph stops making sense: a
// provider that starts needing something nothing supplies, or two that supply
// the same type, is an error at generation rather than a nil at four in the
// afternoon on a Monday.
//
// The returned cleanup closes the browser.
func newSync(ctx context.Context) (syncassets.Sync, func(), error) {
	wire.Build(
		providerSet,
		// Named fields rather than "*", so AllowEmpty stays off. Wire refuses to
		// build the struct otherwise, having nothing to fill it from — which is
		// the right answer: the scheduled job must never be the caller that
		// empties the account.
		wire.Struct(new(syncassets.Sync), "Broker", "Ledger", "Reporter"),
	)
	return syncassets.Sync{}, nil, nil
}
