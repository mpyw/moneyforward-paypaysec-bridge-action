package moneyforward

import (
	"github.com/google/wire"

	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/portfolio"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/port"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/adapter"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/moneyforward/manualasset"
)

// providerSet is what `debug mf sync` is built from.
var providerSet = wire.NewSet(
	provideBroker,
	provideLedger,
	provideReporter,
	provideAllowEmpty,
)

func provideBroker(desired []asset.Asset) port.Broker { return typedHoldings(desired) }

func provideLedger(acct manualasset.Account) port.Ledger { return &ledgerOn{account: acct} }

func provideReporter() port.Reporter { return planPrinter{} }

// provideAllowEmpty lets `--empty` mean it.
//
// The scheduled job never sets this: a scrape returning nothing is a bug, and
// acting on it empties the account. Here it is what was asked for.
func provideAllowEmpty(desired []asset.Asset) bool { return len(desired) == 0 }

// typedHoldings is a broker that reports what the command line said.
type typedHoldings []asset.Asset

func (t typedHoldings) SignIn(context.Context) error { return nil }

func (t typedHoldings) Holdings(context.Context) ([]asset.Asset, error) { return t, nil }

// ledgerOn is the MoneyForward ledger against an already-signed-in account,
// which is what the saved session gives this command.
type ledgerOn struct {
	adapter.MoneyForwardLedger
	account manualasset.Account
}

func (l *ledgerOn) SignIn(context.Context) error {
	l.UseAccount(l.account)
	return nil
}

// planPrinter shows what the run decided.
type planPrinter struct{}

func (planPrinter) Phase(string)             {}
func (planPrinter) ReadResult([]asset.Asset) {}

func (planPrinter) Planned(plan portfolio.Plan) {
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "\nACTION\tNAME\tWAS\tNOW")
	for _, step := range plan.Steps {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", step.Action, step.Name, step.Was, step.Now)
	}
	_ = w.Flush()
}
