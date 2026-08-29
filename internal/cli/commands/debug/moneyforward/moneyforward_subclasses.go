package moneyforward

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/cli/commands/debug/session"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/manualasset"
)

func subclassesCommand() *cli.Command {
	return &cli.Command{
		Name:  "subclasses",
		Usage: "list the 資産クラス options the create form offers, and what maps to them",
		Description: "Reads only. MoneyForward's 資産クラス identifiers are stated nowhere\n" +
			"but in this select, so adding a mapping for a new instrument kind means\n" +
			"looking at it — a guessed identifier files a holding under the wrong\n" +
			"資産クラス with no error anywhere.\n\n" +
			"Uses the session from `mfpp debug mf login`.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSubclasses(ctx, session.From(cmd))
		},
	}
}

func runSubclasses(ctx context.Context, opts *session.Options) error {
	acct, err := account(opts)
	if err != nil {
		return err
	}
	options, err := acct.Subclasses(ctx)
	if err != nil {
		return err
	}

	// What each option is currently mapped from, so the gaps are visible rather
	// than having to be worked out by comparing two lists by eye.
	mapped := map[manualasset.AssetSubclass]asset.Kind{}
	for _, kind := range asset.Kinds() {
		if id, err := manualasset.SubclassFor(kind); err == nil {
			mapped[id] = kind
		}
	}

	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tLABEL\tMAPPED FROM")
	for _, o := range options {
		from := "—"
		if kind, ok := mapped[o.ID]; ok {
			from = kind.String()
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", o.ID, o.Label, from)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// The other direction: a kind this program knows about that the form does
	// not offer would be a write that fails at the site rather than here.
	offered := map[manualasset.AssetSubclass]bool{}
	for _, o := range options {
		offered[o.ID] = true
	}
	for id, kind := range mapped {
		if !offered[id] {
			_, _ = fmt.Fprintf(os.Stderr,
				"\n⚠ %s maps to %d, which this form does not offer\n", kind, id)
		}
	}
	return nil
}
