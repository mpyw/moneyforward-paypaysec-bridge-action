// Package syncassets performs one synchronisation: read the broker's holdings,
// then bring the ledger in line with them.
//
// Separate from the command layer because the two phases and the order they run
// in are the actual work, and nothing about them depends on flags, a terminal or
// an environment. What varies — where the browser comes from, how a one-time
// code arrives, what gets logged — arrives as dependencies.
package syncassets

import (
	"context"
	"errors"
	"fmt"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/portfolio"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/port"
)

// Sync is one configured run.
//
// The dependencies are [port] interfaces, so this package names no site, no
// browser and no mailbox.
type Sync struct {
	Broker   port.Broker
	Ledger   port.Ledger
	Reporter port.Reporter

	// AllowEmptyingCategories permits deleting every entry in a category.
	//
	// Off by default, because every mis-read has taken that shape. On when a
	// person says so for one run, because selling out of a category is real and a
	// stop nothing can lift is the mistake this replaced.
	AllowEmptyingCategories bool

	// AllowEmpty permits reconciling against no holdings at all, which deletes
	// everything the ledger holds.
	//
	// Off by default and never set by the scheduled job: a scrape returning
	// nothing is a bug, and acting on it empties the account. Only a caller that
	// meant it — `mfpp debug mf sync --empty` — turns it on.
	AllowEmpty bool
}

// Result is what a run did.
type Result struct {
	Assets []asset.Asset
	Plan   portfolio.Plan
}

// Run reads the broker and brings the ledger in line with it.
//
// The whole sequence is here — sign in, read, plan, write, read back — rather
// than behind a "reconcile" call on the ledger. Writing to a financial record
// is not something to delegate to whichever adapter happens to be plugged in:
// the order of the writes, the refusal to proceed on an empty read, and what
// counts as a write having worked are decisions, and this is where decisions
// go.
func (s Sync) Run(ctx context.Context) (Result, error) {
	var result Result

	s.phase("read holdings")
	if err := s.Broker.SignIn(ctx); err != nil {
		return result, err
	}
	held, err := s.Broker.Holdings(ctx)
	if err != nil {
		return result, err
	}
	// An empty read aborts before the ledger is touched. Reconciliation deletes
	// what is no longer held, so a scrape that silently returned nothing would
	// empty the account — a failure that looks like a successful run with no
	// holdings.
	if len(held.Assets) == 0 && !s.AllowEmpty {
		return result, errors.New("syncassets: the broker reported no holdings at all; " +
			"refusing to reconcile, which would empty the ledger")
	}
	result.Assets = held.Assets
	if s.Reporter != nil {
		s.Reporter.ReadResult(held.Assets)
	}

	s.phase("record holdings")
	if err := s.Ledger.SignIn(ctx); err != nil {
		return result, err
	}
	result.Plan, err = s.reconcile(ctx, held)
	return result, err
}

// reconcile plans against what the ledger holds, then applies the plan.
//
// The plan is returned even on failure, by the caller: these are writes to a
// financial record, and "what did it manage before it stopped?" is the first
// question worth answering.
func (s Sync) reconcile(ctx context.Context, held asset.Holdings) (portfolio.Plan, error) {
	recorded, err := s.Ledger.Recorded(ctx)
	if err != nil {
		return portfolio.Plan{}, err
	}
	if dup := portfolio.DuplicateName(recorded); dup != "" {
		return portfolio.Plan{}, fmt.Errorf(
			"the ledger holds more than one entry named %q; nothing here can tell them "+
				"apart, so remove the extra by hand before syncing", dup)
	}

	plan := portfolio.Reconcile(recorded, held.Assets)
	if s.Reporter != nil {
		s.Reporter.Planned(plan)
	}
	// Skipped when the caller asked for an empty read and got one: that is a
	// request to remove everything, and there are no categories to check it
	// against. An AllowEmpty run that read something still has to pass.
	askedToEmpty := s.AllowEmpty && len(held.Assets) == 0
	if !askedToEmpty {
		if err := plan.CheckCoverage(held.Categories); err != nil {
			return plan, err
		}
		// Coverage asks whether the category was looked at. This asks whether
		// anything was seen — a page was fetched, verified, and still handed back
		// a different view's figures, which is how a category holding two 銘柄
		// came to read as empty and lose both.
		if !s.AllowEmptyingCategories {
			if err := portfolio.CheckCategoryEmptied(recorded, held.Assets); err != nil {
				return plan, err
			}
		}
	}

	wanted := make(map[string]asset.Asset, len(held.Assets))
	for _, a := range held.Assets {
		wanted[a.Name] = a
	}
	for _, step := range plan.Steps {
		if step.Action == portfolio.ActionUnchanged {
			continue
		}
		if err := s.apply(ctx, step, wanted[step.Name]); err != nil {
			return plan, err
		}
	}
	if s.Reporter != nil {
		s.Reporter.Applied(plan)
	}
	return plan, nil
}

// apply performs one step and confirms it took effect.
func (s Sync) apply(ctx context.Context, step portfolio.Step, want asset.Asset) error {
	var err error
	switch step.Action {
	case portfolio.ActionCreate:
		err = s.Ledger.Create(ctx, want)
	case portfolio.ActionUpdate:
		err = s.Ledger.Update(ctx, want)
	case portfolio.ActionDelete:
		err = s.Ledger.Delete(ctx, step.Name)
	default:
		return fmt.Errorf("unknown action %q for %q", step.Action, step.Name)
	}
	if err != nil {
		return err
	}
	return s.confirm(ctx, step, want)
}

// confirm reads the ledger back rather than trusting what the write returned.
//
// A ledger reached over the web can report success for a write it did not
// apply — and has. What it holds afterwards is not ambiguous.
func (s Sync) confirm(ctx context.Context, step portfolio.Step, want asset.Asset) error {
	after, err := s.Ledger.Recorded(ctx)
	if err != nil {
		return fmt.Errorf("verify %s %q: %w", step.Action, step.Name, err)
	}

	var found *asset.Asset
	for i := range after {
		if after[i].Name == step.Name {
			found = &after[i]
			break
		}
	}

	verr := step.Confirm(want, found)
	if verr == nil {
		return nil
	}
	// Only now: the service's own message cannot decide whether a write worked,
	// but once something is known to have gone wrong it is worth quoting.
	if explainer, ok := s.Ledger.(port.Explainer); ok {
		if reason := explainer.LastRejection(); reason != "" {
			return fmt.Errorf("%w — the service said: %s", verr, reason)
		}
	}
	return verr
}

// phase announces a stage when there is anyone listening.
func (s Sync) phase(name string) {
	if s.Reporter != nil {
		s.Reporter.Phase(name)
	}
}
