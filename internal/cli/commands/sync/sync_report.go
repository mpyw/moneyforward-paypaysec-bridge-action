package sync

import (
	"log"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/portfolio"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/actionslog"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/manulife"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec"
	ppsel "github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/infra/paypaysec/selector"
)

// What the run says as it goes. The figures are masked by the time any of this
// prints, so what is worth saying is counts and flags — a masked amount is a row
// of asterisks and says nothing.

// reporter prints progress, masking every figure before it can appear.
type reporter struct {
	masker actionslog.Masker
}

func (r reporter) Phase(name string) { log.Printf("=== %s ===", name) }

func (r reporter) ReadResult(source string, assets []asset.Asset) {
	// The per-holding figures were registered by maskFigures as they were read;
	// the total is new here. Registered before the line that prints it.
	var total int64
	for _, a := range assets {
		total += a.Yen
	}
	r.masker.MaskAmount(total)
	log.Printf("→ %s: %d holdings, %d yen in total", source, len(assets), total)
}

// Planned reports what the run intends to do, before it does any of it.
//
// Worded as a plan, because it is one. It used to end with a ✓ and a tally,
// which is what a finished run looks like — and the checks that can still
// refuse the whole thing run after this line. A refusal therefore printed a
// tick and a delete count and then failed, and the log read as though the
// deletes had happened. They had not.
func (r reporter) Planned(source string, plan portfolio.Plan) {
	log.Printf("   %s:", source)
	for _, step := range plan.Steps {
		// Names are 銘柄, not balances, so they are not masked.
		log.Printf("   %-9s %s", step.Action, step.Name)
	}
	counts := plan.Counts()
	log.Printf("→ %s planned: create=%d update=%d unchanged=%d delete=%d (nothing written yet)",
		source, counts[portfolio.ActionCreate], counts[portfolio.ActionUpdate],
		counts[portfolio.ActionUnchanged], counts[portfolio.ActionDelete])
}

// Applied reports what actually happened, once it has.
func (r reporter) Applied(source string, plan portfolio.Plan) {
	counts := plan.Counts()
	log.Printf("✓ %s: created=%d updated=%d unchanged=%d deleted=%d",
		source, counts[portfolio.ActionCreate], counts[portfolio.ActionUpdate],
		counts[portfolio.ActionUnchanged], counts[portfolio.ActionDelete])
}

// Failed says which source was given up on, as it happens.
//
// The run carries on with the others and fails at the end, so this is the only
// line that appears where the failure actually occurred. Without it the account
// of what went wrong arrives after everything that succeeded, about something
// minutes earlier.
func (r reporter) Failed(source string, err error) {
	log.Printf("✗ %s: %v", source, err)
	log.Printf("   entries in %s's account are left as they are", source)
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
	log.Printf("   %-16s %d 銘柄  section=%v total=%v cost=%v gain=%v",
		r.Target.Key, r.HoldingCount(), r.Figures.HoldingsSection,
		r.HasTotal, r.HasAcquisition, r.HasGain)
}

// reportContractSkip says out loud that a contract was not read.
//
// Otherwise it is invisible in the one way that matters: the list was read, so
// the category counts as covered, so the row this program wrote for that
// contract is deleted rather than left alone. A deletion nobody was told the
// reason for is the failure this project keeps closing.
//
// The contract's own name, not its number: the number identifies a person's
// policy and the name is what the recorded entry is called.
func reportContractSkip(card manulife.Card) {
	log.Printf("   %s is in the list but not in force; its entry will be removed",
		card.Title)
}

// logChallenge reports whether a service asked for a one-time code.
func logChallenge(service string) func(bool) {
	return func(challenged bool) {
		if !challenged {
			log.Printf("→ %s presented no OTP challenge", service)
		}
	}
}

// reportSkip says out loud that a category was not read.
//
// Otherwise it is invisible: the guards keep the category's recorded entries from
// being deleted, so the run succeeds and those entries quietly stop being updated.
// A stale figure that nobody is told about is worse than a failure, because a
// failure sends mail.
func reportSkip(t ppsel.Target, why error) {
	log.Printf("   %-16s skipped — %v; entries under %s are left as they are",
		t.Key, why, t.Category())
}
