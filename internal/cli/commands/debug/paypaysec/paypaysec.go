// Package paypaysec verifies the PayPay 証券 steps one at a time.
//
// One file per subcommand, each holding its own definition, its own flags and
// what it does.
package paypaysec

import (
	"github.com/urfave/cli/v3"
)

// Command groups the PayPay 証券 debug subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "paypaysec",
		Usage: "PayPay 証券 steps",
		Commands: []*cli.Command{
			selectorsCommand(),
			loginCommand(),
			balanceCommand(),
			probeCommand(),
		},
	}
}
