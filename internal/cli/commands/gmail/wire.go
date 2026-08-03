//go:build wireinject

// The build tag keeps this file out of the binary: it is the specification wire
// reads, not code that runs.
//
//	go run github.com/google/wire/cmd/wire ./internal/cli/commands/debug/gmail

package gmail

import (
	"github.com/google/wire"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/usecase/authorizegmail"
)

// newAuthorize assembles the consent flow, where the credential goes, and the
// check that it opens the mailbox somebody meant.
func newAuthorize(client clientFile, out credentialFile) authorizegmail.Authorize {
	wire.Build(
		providerSet,
		wire.Struct(new(authorizegmail.Authorize), "*"),
	)
	return authorizegmail.Authorize{}
}
