package moneyforward

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/manualasset"
)

func syncCommand() *cli.Command {
	var entries []string
	var empty bool
	var subclass int

	return &cli.Command{
		Name:  "sync",
		Usage: "reconcile the portfolio against name=yen pairs (writes to MoneyForward)",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "entry",
				Usage:       `desired entry as "name=yen"; repeat per holding. Anything not listed is deleted`,
				Destination: &entries,
			},
			&cli.BoolFlag{
				Name: "empty",
				Usage: "reconcile against nothing, deleting every entry — required to be " +
					"explicit, since passing no entries by mistake would otherwise wipe the account",
				Destination: &empty,
			},
			&cli.IntFlag{Name: "subclass", Value: 15, Destination: &subclass},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if len(entries) == 0 && !empty {
				return fmt.Errorf("no entries given; pass --empty to mean it")
			}
			return runSync(ctx, session.From(cmd), entries, subclass)
		},
	}
}

func runSync(ctx context.Context, opts *session.Options, pairs []string, subclass int) error {
	acct, err := account(opts)
	if err != nil {
		return err
	}

	desired := make([]asset.Asset, 0, len(pairs))
	for _, p := range pairs {
		name, amount, ok := strings.Cut(p, "=")
		if !ok {
			return fmt.Errorf("entry %q is not name=yen", p)
		}
		yen, cerr := strconv.ParseInt(strings.TrimSpace(amount), 10, 64)
		if cerr != nil {
			return fmt.Errorf("entry %q: %w", p, cerr)
		}
		desired = append(desired, asset.Asset{
			Name:   strings.TrimSpace(name),
			Yen:    yen,
			Kind:   manualasset.KindOf(manualasset.AssetSubclass(subclass)),
			Source: "debug mf sync",
		})
	}

	_, err = newSyncFromEntries(desired, acct).Run(ctx)
	return err
}
