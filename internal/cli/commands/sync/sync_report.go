package sync

import (
	"log"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/portfolio"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/actionslog"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/paypaysec"
)

// What the run says as it goes. The figures are masked by the time any of this
// prints, so what is worth saying is counts and flags — a masked amount is a row
// of asterisks and says nothing.

// reporter prints progress, masking every figure before it can appear.
type reporter struct {
	masker actionslog.Masker
}

func (r reporter) Phase(name string) { log.Printf("=== %s ===", name) }

func (r reporter) ReadResult(assets []asset.Asset) {
	// The per-holding figures were registered by maskFigures as they were read;
	// the total is new here. Registered before the line that prints it.
	var total int64
	for _, a := range assets {
		total += a.Yen
	}
	r.masker.MaskAmount(total)
	log.Printf("→ %d holdings, %d yen in total", len(assets), total)
}

func (r reporter) Planned(plan portfolio.Plan) {
	for _, step := range plan.Steps {
		// Names are 銘柄, not balances, so they are not masked.
		log.Printf("   %-9s %s", step.Action, step.Name)
	}
	counts := plan.Counts()
	log.Printf("✓ created=%d updated=%d unchanged=%d deleted=%d",
		counts[portfolio.ActionCreate], counts[portfolio.ActionUpdate],
		counts[portfolio.ActionUnchanged], counts[portfolio.ActionDelete])
}

// reportTarget logs one line per page read.
//
// Counts and flags only — the amounts are masked by the time this runs, but a
// masked figure in a log is a row of asterisks, which says nothing. What is
// worth saying is how many 銘柄 a page listed and whether it had a figure at
// all, because the failure this exists for is a page that came back empty while
// looking entirely healthy: a zero total and no rows agree with each other, and
// with every cross-check there is.
func reportTarget(r paypaysec.Reading) {
	tab := ""
	if r.Tab != "" {
		tab = " tab=" + r.Tab
	}
	log.Printf("   %-16s %d 銘柄  section=%v total=%v cost=%v gain=%v%s",
		r.Target.Key, r.HoldingCount(), r.Figures.HoldingsSection,
		r.HasTotal, r.HasAcquisition, r.HasGain, tab)
}

// logChallenge reports whether a service asked for a one-time code.
func logChallenge(service string) func(bool) {
	return func(challenged bool) {
		if !challenged {
			log.Printf("→ %s presented no OTP challenge", service)
		}
	}
}
