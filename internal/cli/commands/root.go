// Package commands assembles the mfpp command tree.
//
// Everything this project does is one binary with subcommands, rather than a
// directory of near-identical mains: they share the same credential
// conventions and the same repository, so splitting them apart only duplicated
// the wiring.
//
//	mfpp sync               the scheduled job
//	mfpp gmail authorize    obtain the Gmail credential the job reads OTPs with
//	mfpp debug …            step-by-step development harness
//
// Setting the repository secrets is not among them, deliberately: `gh secret
// set` already does it, encrypting locally before upload. Anyone using this as
// an action sets them in their own repository.
//
// Each group lives in its own package under this one, so a subcommand's
// helpers cannot reach another's, and the directories are the tree: sync,
// secrets and debug sit here, and what runs as `debug paypaysec` lives in
// debug/paypaysec. They were all flat under commands/ before, which said the
// three debug groups were siblings of the job.
//
// What stays in this file is the tree itself.
//
// The definitions live under internal rather than cmd/ so they can be
// constructed and exercised from tests; cmd/mfpp is a thin entry point.
package commands

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/gmail"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/sync"
	"github.com/urfave/cli/v3"
)

// Root builds the full command tree.
func Root() *cli.Command {
	return &cli.Command{
		Name:  "mfpp",
		Usage: "sync the PayPay 証券 balance into a MoneyForward manual asset",
		Commands: []*cli.Command{
			sync.Command(),
			gmail.Command(),
			debug.Command(),
		},
	}
}
