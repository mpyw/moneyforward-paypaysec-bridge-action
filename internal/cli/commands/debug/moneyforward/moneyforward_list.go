package moneyforward

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
)

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "show the portfolio entries currently recorded",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runList(ctx, session.From(cmd))
		},
	}
}

func runList(ctx context.Context, opts *session.Options) error {
	acct, err := account(opts)
	if err != nil {
		return err
	}
	entries, err := acct.Entries(ctx)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "no entries recorded")
		return nil
	}
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tYEN\tSUBCLASS\tID\tHASH")
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n", e.Name, e.Yen, e.Subclass, e.ID, e.Hash[:12]+"…")
	}
	return w.Flush()
}
