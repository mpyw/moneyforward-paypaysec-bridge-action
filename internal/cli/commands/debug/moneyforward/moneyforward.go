// Package moneyforward verifies the MoneyForward steps one at a time.
//
// One file per subcommand, each holding its own definition, its own flags and
// what it does. They shared a var block before, and two of them shared a
// --subclass destination: harmless while only one runs per invocation, and
// exactly the kind of thing that stops being harmless quietly.
package moneyforward

import (
	"github.com/urfave/cli/v3"
)

// Command groups the MoneyForward debug subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "mf",
		Usage: "MoneyForward steps",
		Commands: []*cli.Command{
			loginCommand(),
			portfolioCommand(),
			addCommand(),
			syncCommand(),
			listCommand(),
			fetchCommand(),
			probeCommand(),
		},
	}
}
