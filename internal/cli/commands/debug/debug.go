// Package debug holds the step-by-step development harness.
//
// The pipeline has four independent ways to break — the login form, the OTP
// hand-off, the balance scrape, and the MoneyForward write — and running them
// end-to-end makes every failure look the same. These subcommands take them
// apart:
//
//	mfpp debug paypaysec selectors  check the login-page selectors (no credentials)
//	mfpp debug paypaysec login      log in, persisting the session to the profile
//	mfpp debug paypaysec balance    read every target using that session
//	mfpp debug paypaysec probe      inspect one URL in detail
//	mfpp debug mf login             log in to MoneyForward
//	mfpp debug mf portfolio         show what the account page reveals about writing
//	mfpp debug mf list              list the entries currently recorded
//	mfpp debug mf add               create one entry
//	mfpp debug mf sync              reconcile the entries against name=yen pairs
//	mfpp debug gmail authorize      obtain a Gmail credential through OAuth consent
//	mfpp debug gmail search         list recent messages matching a query
//
// One directory per group, under this one, matching the command tree.
//
// Two things make step-by-step work possible:
//
//   - --otp file takes the code from a path instead of the mailbox, so a browser
//     problem can be isolated from a mail problem: with that flag, a failure can
//     only be the browser.
//   - The Chrome profile persists between runs (--profile), so you log in once
//     and then iterate on the scrape without burning a one-time code per
//     attempt.

package debug

import (
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/gmail"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/moneyforward"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/paypaysec"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
	"github.com/urfave/cli/v3"
)

// Command groups the step-by-step development subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "debug",
		Usage: "development harness: verify one pipeline step at a time",
		// Declared here and read back per invocation, so nothing in the tree
		// holds a pointer to a struct the parser writes into.
		Flags: session.Flags(),
		Commands: []*cli.Command{
			paypaysec.Command(),
			moneyforward.Command(),
			gmail.Command(),
		},
	}
}
