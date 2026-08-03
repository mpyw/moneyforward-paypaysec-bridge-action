// Package portfolio decides what a set of holdings should become.
//
// Pure: it knows nothing about MoneyForward, browsers or HTTP. What it takes is
// two lists of names and amounts, and what it produces is the sequence of
// changes that turns one into the other. The infrastructure that performs those
// changes carries its own identifiers and tokens; none of that belongs to the
// decision.
package portfolio

import (
	"errors"
	"fmt"
	"sort"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/asset"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/assetname"
)

// A position here is an [asset.Asset].
//
// There is no separate type for it, and there was: an Asset with Kind and
// Source dropped. Two shapes of one thing, converted at every boundary, and the
// comparison below could not see the instrument kind because the type it
// compared did not carry it — so a row filed under the wrong 資産クラス was never
// corrected.
//
// Name is the key, because it is the only thing both sides share: a broker
// knows its own identifiers and a ledger knows its own, and neither has heard
// of the other's. That is also why names have to be built to be unique — a
// collision here silently merges two holdings into one.

// sameFigures reports whether two positions record the same thing.
//
// The cost matters as much as the value: correcting only the cost basis is a
// real change, and it is the difference a ledger shows as profit. So does the
// kind — a holding filed as 投資信託 when it is 米国株 is wrong in a way nobody
// looking at the figures would notice.
func sameFigures(a, b asset.Asset) bool {
	return a.Yen == b.Yen &&
		a.AcquisitionYen == b.AcquisitionYen &&
		a.HasAcquisition == b.HasAcquisition &&
		a.Kind == b.Kind
}

// Action is what reconciliation decided to do with one position.
type Action string

const (
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionUnchanged Action = "unchanged"
	ActionDelete    Action = "delete"
)

// Step is one decision, recorded whether it is carried out.
type Step struct {
	Action Action
	Name   string

	// Was and Now are the recorded and intended valuations. Was is meaningless
	// for a creation, Now for a deleted.
	Was int64
	Now int64
}

// Plan is the full set of decisions for one reconciliation.
type Plan struct {
	Steps []Step
}

// Counts summarises the plan by action.
func (p Plan) Counts() map[Action]int {
	out := map[Action]int{}
	for _, s := range p.Steps {
		out[s.Action]++
	}
	return out
}

// Reconcile works out how to turn current into desired.
//
// A position that is no longer desired is deleted rather than zeroed. A holding
// that has been sold is not a holding worth nothing; leaving it at zero would
// keep a position in the portfolio that no longer exists.
//
// Deletes come last and in name order, so a plan reads the same twice and two
// runs diff cleanly against each other.
func Reconcile(current, desired []asset.Asset) Plan {
	recorded := make(map[string]asset.Asset, len(current))
	for _, p := range current {
		recorded[p.Name] = p
	}

	var plan Plan
	wanted := make(map[string]bool, len(desired))

	for _, want := range desired {
		wanted[want.Name] = true
		have, exists := recorded[want.Name]
		switch {
		case !exists:
			plan.Steps = append(plan.Steps, Step{Action: ActionCreate, Name: want.Name, Now: want.Yen})
		case sameFigures(have, want):
			plan.Steps = append(plan.Steps, Step{
				Action: ActionUnchanged, Name: want.Name, Was: have.Yen, Now: want.Yen})
		default:
			plan.Steps = append(plan.Steps, Step{
				Action: ActionUpdate, Name: want.Name, Was: have.Yen, Now: want.Yen})
		}
	}

	var stale []asset.Asset
	for _, p := range current {
		if !wanted[p.Name] {
			stale = append(stale, p)
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Name < stale[j].Name })
	for _, p := range stale {
		plan.Steps = append(plan.Steps, Step{Action: ActionDelete, Name: p.Name, Was: p.Yen})
	}
	return plan
}

// DuplicateName returns a name held by more than one position, or "".
//
// Everything about a plan is indexed by name: what to change, what to leave, and
// what the ledger holds under it afterward. Two positions sharing one make all
// of that address whichever the map kept, so the other is invisible — never
// updated, never deleted, and quietly contributing its own figure to the total
// for as long as it exists.
func DuplicateName(positions []asset.Asset) string {
	seen := make(map[string]bool, len(positions))
	for _, p := range positions {
		if seen[p.Name] {
			return p.Name
		}
		seen[p.Name] = true
	}
	return ""
}

// Confirm reports whether the ledger, read back, reflects this step.
//
// want is what the step was aiming at; found is what is recorded under that name
// now, or nil if nothing is.
//
// Reading back is the only reliable verdict available: MoneyForward answers a
// rejected write with 200 and the page re-rendered, so the response says nothing
// and the flash blocks say it ambiguously. This is the rule for what counts as
// having worked, and it lives here rather than beside the HTTP because it is a
// statement about positions, not about a site.
func (s Step) Confirm(want asset.Asset, found *asset.Asset) error {
	switch s.Action {
	case ActionUnchanged:
		return nil

	case ActionDelete:
		if found != nil {
			return fmt.Errorf("delete %q reported no error but it is still recorded", s.Name)
		}
		return nil

	case ActionCreate, ActionUpdate:
		if found == nil {
			return fmt.Errorf("%s %q reported no error but it is not recorded", s.Action, s.Name)
		}
		if found.Yen != want.Yen {
			return fmt.Errorf("%s %q reported no error but the recorded value is %d, not %d",
				s.Action, s.Name, found.Yen, want.Yen)
		}
		// The cost as well as the value. A ledger that derives profit from it
		// treats a blank one as "cost equals value", so writing that landed the
		// valuation and lost the cost reports a profit of exactly zero — a
		// plausible figure nothing downstream can question.
		if found.HasAcquisition != want.HasAcquisition {
			return fmt.Errorf("%s %q reported no error but its cost is recorded=%v, "+
				"want recorded=%v — the profit would be wrong",
				s.Action, s.Name, found.HasAcquisition, want.HasAcquisition)
		}
		if want.HasAcquisition && found.AcquisitionYen != want.AcquisitionYen {
			return fmt.Errorf("%s %q reported no error but the recorded cost is %d, not %d",
				s.Action, s.Name, found.AcquisitionYen, want.AcquisitionYen)
		}
		return nil
	}
	return fmt.Errorf("unknown action %q for %q", s.Action, s.Name)
}

// ErrUnverifiedDeletes reports deletes the read cannot account for.
var ErrUnverifiedDeletes = errors.New("the plan deletes entries from categories this run did not read")

// CheckCoverage refuses to delete an entry belonging to a category the run did
// not actually read.
//
// Not a limit on how much may be deleted. A share of the ledger is a proxy for
// "the scrape went wrong", and it cannot tell that from somebody selling — so it
// refuses both, and a real sale fails the job every weekday.
//
// It is also unnecessary. Every way a read can go wrong quietly is closed at the
// source with the complaint it deserves: a page still showing its loading
// placeholder, a total with no holdings under it, three routes that disagree, a
// tab that did not activate. Any of them fails the whole read. By the time there
// is a plan, every page it rests on has been verified.
//
// What is worth checking is not how much is being deleted but whether the run
// looked where it is deleting from. A category the ledger has entries under and
// the read never covered is a target dropped from the list, or a bucket that
// stopped being scraped — and its entries are unverified, not stale.
//
// Entries with no category prefix are deleted, not spared. The account named by
// MONEYFORWARD_ASSET_ID is managed wholesale — anything in it that does not
// correspond to a holding goes — and a row somebody typed by hand is
// indistinguishable from a row this program wrote before a rename.
//
// That is the documented contract, and the reason the setup instructions say to
// create a new account rather than point at an existing one. Sparing unprefixed
// rows would be the friendlier rule right up to the first run after a category
// is renamed, when the old rows would stay forever.
func (p Plan) CheckCoverage(covered []string) error {
	seen := make(map[string]bool, len(covered))
	for _, c := range covered {
		seen[c] = true
	}

	var unverified []string
	for _, step := range p.Steps {
		if step.Action != ActionDelete {
			continue
		}
		category, ok := assetname.CategoryOf(step.Name)
		if !ok || seen[category] {
			continue
		}
		unverified = append(unverified, step.Name)
	}
	if len(unverified) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %v — the read covered %v", ErrUnverifiedDeletes, unverified, covered)
}
