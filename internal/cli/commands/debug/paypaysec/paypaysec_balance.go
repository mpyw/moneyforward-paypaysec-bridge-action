package paypaysec

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	ppsite "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

func balanceCommand() *cli.Command {
	return &cli.Command{
		Name:  "balance",
		Usage: "read every target using the persisted session",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runBalance(ctx, session.From(cmd))
		},
	}
}

// runBalance reads every target and prints the breakdown.
//
// Amounts are printed in full: this is a local dev tool, and the whole point is
// to eyeball the figures. cmd/sync masks them instead.
func runBalance(ctx context.Context, opts *session.Options) error {
	s, err := opts.Start(ctx)
	if err != nil {
		return err
	}
	defer s.Finish()

	// Accumulated as a Balances so the per-銘柄 assets can be derived exactly the
	// way the sync job derives them.
	var balances ppsite.Balances
	var failures int

	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "\nTARGET\tBUCKET\t評価額合計\t投資元本\t含み益\tHOLDINGS\tAMOUNT")

	for _, target := range ppsel.Targets {
		reading, rerr := ppsite.Read(s.Context(), target)
		if rerr != nil {
			_, _ = fmt.Fprintf(w, "%s\t%s\t\t\t\t\tERROR\n", target.Key, target.Bucket)
			failures++
			_, _ = fmt.Fprintf(os.Stderr, "  ! %v\n", rerr)
			continue
		}

		total := reading.Figures.TotalRaw
		if !reading.Figures.TotalPresent {
			total = "-"
		}
		var amount string
		yen, aerr := reading.Amount()
		if aerr != nil {
			amount = "MISMATCH"
			failures++
			_, _ = fmt.Fprintf(os.Stderr, "  ! %v\n", aerr)
		} else {
			amount = fmt.Sprintf("%d", yen)
			balances.Readings = append(balances.Readings, reading)
			if target.Bucket == ppsel.BucketMiniApp {
				balances.MiniApp += yen
			} else {
				balances.App += yen
			}
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d/%d\t%s\n",
			target.Key, target.Bucket, total,
			reading.Figures.AcquisitionRaw, reading.Figures.GainRaw,
			reading.HoldingsParsed, reading.HoldingCount(), amount)
	}

	// Totals are a cross-check, not a record: only the per-銘柄 assets below get
	// written to MoneyForward.
	_, _ = fmt.Fprintf(w, "\napp\t\t\t\t\t\t%d\n", balances.App)
	_, _ = fmt.Fprintf(w, "miniapp\t\t\t\t\t\t%d\n", balances.MiniApp)
	_, _ = fmt.Fprintf(w, "TOTAL (debug only)\t\t\t\t\t\t%d\n", balances.Total())
	if err := w.Flush(); err != nil {
		return err
	}

	if failures > 0 {
		return fmt.Errorf("%d target(s) could not be read — see the notes above", failures)
	}

	assets, err := balances.Assets()
	if err != nil {
		return err
	}
	aw := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(aw, "\nMoneyForward assets (%d)\tYEN\t取得価額\tSOURCE\n", len(assets))
	for _, a := range assets {
		acquisition := "-"
		if a.HasAcquisition {
			acquisition = fmt.Sprintf("%d", a.AcquisitionYen)
		}
		_, _ = fmt.Fprintf(aw, "%s\t%d\t%s\t%s\n", a.Name, a.Yen, acquisition, a.Source)
	}
	if err := aw.Flush(); err != nil {
		return err
	}
	return nil
}
