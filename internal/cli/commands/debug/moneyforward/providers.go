package moneyforward

import (
	"github.com/google/wire"

	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/assetname"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/portfolio"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/port"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/usecase/syncassets"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/adapter"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/moneyforward/manualasset"
)

// providerSet is what `debug mf sync` is built from.
var providerSet = wire.NewSet(
	provideBridges,
	provideReporter,
	provideAllowEmpty,
)

// provideBridges is the one bridge this command has: what was typed, against
// the account the flags named.
func provideBridges(desired []asset.Asset, acct manualasset.Account) []syncassets.Bridge {
	return []syncassets.Bridge{{
		Source: typedHoldings(desired),
		Ledger: &ledgerOn{account: acct},
	}}
}

func provideReporter() port.Reporter { return planPrinter{} }

// provideAllowEmpty lets `--empty` mean it.
//
// The scheduled job never sets this: a scrape returning nothing is a bug, and
// acting on it empties the account. Here it is what was asked for.
func provideAllowEmpty(desired []asset.Asset) bool { return len(desired) == 0 }

// typedHoldings is a source that reports what the command line said.
type typedHoldings []asset.Asset

// ID names it as what it is, so a plan printed by this command cannot be
// mistaken for one produced by reading a site.
func (t typedHoldings) ID() string { return "typed" }

func (t typedHoldings) SignIn(context.Context) error { return nil }

// Holdings reports every category the typed entries mention as covered.
//
// Which is the honest answer for this command: the operator typed the entries,
// so the categories they named are exactly the ones this run "read". An entry
// in the ledger under some other category is one this invocation knows nothing
// about, and the coverage check will refuse to delete it — which is what should
// happen when somebody tests the write path with two entries against a ledger
// holding five.
func (t typedHoldings) Holdings(context.Context) (asset.Holdings, error) {
	seen := map[string]bool{}
	var categories []string
	for _, a := range t {
		if c, ok := assetname.CategoryOf(a.Name); ok && !seen[c] {
			seen[c] = true
			categories = append(categories, c)
		}
	}
	return asset.Holdings{Assets: t, Categories: categories}, nil
}

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

func (planPrinter) Phase(string)                     {}
func (planPrinter) ReadResult(string, []asset.Asset) {}

// Applied says nothing: this command already prints the whole plan, and the
// table is the useful output. What it must not do is print a second summary
// that looks like the first.
func (planPrinter) Applied(string, portfolio.Plan) {}

// Failed is likewise silent. This command has one bridge, so a failure is the
// error the command returns — printing it here would say it twice.
func (planPrinter) Failed(string, error) {}

func (planPrinter) Planned(_ string, plan portfolio.Plan) {
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "\nACTION\tNAME\tWAS\tNOW")
	for _, step := range plan.Steps {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", step.Action, step.Name, step.Was, step.Now)
	}
	_ = w.Flush()
}
