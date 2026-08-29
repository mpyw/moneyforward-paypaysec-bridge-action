// Package manulife verifies the マニュライフ生命 steps one at a time.
//
// One file per subcommand, each holding its own definition, its own flags and
// what it does — the same shape as the PayPay 証券 group next door.
//
// Both subcommands read only; nothing here writes anywhere.
//
// Every attempt costs a one-time code and the supply is not unlimited, so a
// failure is expected to leave enough behind to fix the cause without another
// one — which is what [session.Session.ReportStepFailure] is for. The page dump
// from a failed run is how the challenge step's real selectors were found,
// after a set of guesses had matched the wrong element and reported success.
package manulife

import (
	"github.com/urfave/cli/v3"
)

// Command groups the マニュライフ生命 debug subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "manulife",
		Usage: "マニュライフ生命 マイページ steps",
		Commands: []*cli.Command{
			loginCommand(),
			readCommand(),
		},
	}
}
