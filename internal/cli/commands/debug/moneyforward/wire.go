//go:build wireinject

// The build tag keeps this file out of the binary: it is the specification wire
// reads, not code that runs.
//
//	go run github.com/google/wire/cmd/wire ./internal/cli/commands/debug/moneyforward

package moneyforward

import (
	"github.com/google/wire"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/usecase/syncassets"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/manualasset"
)

// newSyncFromEntries assembles the real sync use case against a broker that
// reports what was typed and a ledger already holding a session.
//
// The same use case the scheduled job runs — deliberately, because this command
// used to reconcile on its own, which meant the thing it was for testing was
// not the thing being tested.
//
// Not the same graph, though, and not shareable with the job's: the broker, the
// ledger and the reporter are all different implementations, so an injector
// covering both would take those three as arguments and be a struct literal
// with extra steps. Named for what it builds rather than left as a second
// newSync, since the two mean different things.
func newSyncFromEntries(desired []asset.Asset, acct manualasset.Account) syncassets.Sync {
	wire.Build(
		providerSet,
		// Named fields rather than "*": AllowEmpty is the one field this command
		// writes to the real account, and the limit on how much one run may
		// remove is not something a debug helper should quietly widen.
		wire.Struct(new(syncassets.Sync), "Broker", "Ledger", "Reporter", "AllowEmpty"),
	)
	return syncassets.Sync{}
}
