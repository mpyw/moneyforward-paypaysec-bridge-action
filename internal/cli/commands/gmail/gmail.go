// Package gmail inspects the mailbox OTP mail arrives in, and mints the
// credential that reads it.
//
// One file per subcommand, each holding its own definition, its own flags and
// what it does.
package gmail

import (
	"github.com/urfave/cli/v3"
)

// Command groups the Gmail debug subcommands.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "gmail",
		Usage: "Gmail steps",
		Commands: []*cli.Command{
			authorizeCommand(),
			checkCommand(),
			searchCommand(),
		},
	}
}
