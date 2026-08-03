package moneyforward

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/manualasset"
)

func addCommand() *cli.Command {
	var name string
	var amount, subclass int

	return &cli.Command{
		Name:  "add",
		Usage: "create one portfolio entry (writes to MoneyForward)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "name", Required: true,
				Usage:       "the asset name, as it will read in MoneyForward",
				Destination: &name,
			},
			&cli.IntFlag{
				Name: "amount", Required: true,
				Usage:       "the 評価額, in yen",
				Destination: &amount,
			},
			&cli.IntFlag{
				Name: "subclass", Value: 15,
				Usage:       "asset_subclass_id; 15=米国株, 12=投資信託, 14=国内株",
				Destination: &subclass,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runAdd(ctx, session.From(cmd), name, int64(amount), subclass)
		},
	}
}

func runAdd(ctx context.Context, opts *session.Options, name string, yen int64, subclass int) error {
	acct, err := account(opts)
	if err != nil {
		return err
	}
	w, err := acct.Writer(ctx)
	if err != nil {
		return err
	}
	res, err := w.Create(ctx, manualasset.Entry{
		Name:     name,
		Yen:      yen,
		Subclass: manualasset.AssetSubclass(subclass),
	})
	if err != nil {
		return err
	}

	// A 200 with the form re-rendered is how this endpoint reports a rejection,
	// so the response is kept for inspection either way.
	path := filepath.Join(opts.DebugDir(), "mf-create-response.html")
	if werr := os.WriteFile(path, res.Body, 0o600); werr == nil {
		_, _ = fmt.Fprintf(os.Stderr, "  response: %d, %d bytes -> %s\n", res.StatusCode, len(res.Body), path)
	}
	_, _ = fmt.Fprintf(os.Stderr, "  landed on: %s\n", res.FinalURL)
	_, _ = fmt.Fprintf(os.Stderr, "→ submitted %q = %d yen to %q\n", name, yen, w.SubAccountLabel)
	return nil
}
