package moneyforward

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
)

func portfolioCommand() *cli.Command {
	return &cli.Command{
		Name:  "portfolio",
		Usage: "show what the account page reveals about writing to it",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runPortfolio(ctx, session.From(cmd))
		},
	}
}

func runPortfolio(ctx context.Context, opts *session.Options) error {
	acct, err := account(opts)
	if err != nil {
		return err
	}
	w, err := acct.Writer(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "sub-account: %s (%q)\n", w.SubAssetID, w.SubAccountLabel)
	_, _ = fmt.Fprintf(os.Stderr, "csrf token:  %d chars\n", len(w.Token))
	return nil
}
