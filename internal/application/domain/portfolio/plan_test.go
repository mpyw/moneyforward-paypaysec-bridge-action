package portfolio

import (
	"errors"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
	"strings"
	"testing"
)

// planActionLabels summarises a plan as "action:name" pairs, in order.
func planActionLabels(p Plan) []string {
	out := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		out = append(out, string(s.Action)+":"+s.Name)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name    string
		current []asset.Asset
		desired []asset.Asset
		want    []string
	}{
		{
			name:    "empty account creates everything",
			current: nil,
			desired: []asset.Asset{{Name: "A", Yen: 100}, {Name: "B", Yen: 200}},
			want:    []string{"create:A", "create:B"},
		},
		{
			name:    "identical state changes nothing",
			current: []asset.Asset{{Name: "A", Yen: 100}},
			desired: []asset.Asset{{Name: "A", Yen: 100}},
			want:    []string{"unchanged:A"},
		},
		{
			name:    "a changed value is an update",
			current: []asset.Asset{{Name: "A", Yen: 100}},
			desired: []asset.Asset{{Name: "A", Yen: 150}},
			want:    []string{"update:A"},
		},
		{
			// A sold holding is not a holding worth nothing. Zeroing it would
			// leave a position in the portfolio that no longer exists.
			name:    "a holding that is gone is deleted, not zeroed",
			current: []asset.Asset{{Name: "A", Yen: 100}, {Name: "B", Yen: 200}},
			desired: []asset.Asset{{Name: "A", Yen: 100}},
			want:    []string{"unchanged:A", "delete:B"},
		},
		{
			name:    "deletes come after the rest",
			current: []asset.Asset{{Name: "gone", Yen: 1}, {Name: "A", Yen: 100}},
			desired: []asset.Asset{{Name: "A", Yen: 150}, {Name: "new", Yen: 5}},
			want:    []string{"update:A", "create:new", "delete:gone"},
		},
		{
			// The valuation can be unchanged while the cost basis is corrected,
			// and MoneyForward shows the difference as 評価損益.
			name:    "a changed acquisition cost alone is an update",
			current: []asset.Asset{{Name: "A", Yen: 100, AcquisitionYen: 80, HasAcquisition: true}},
			desired: []asset.Asset{{Name: "A", Yen: 100, AcquisitionYen: 90, HasAcquisition: true}},
			want:    []string{"update:A"},
		},
		{
			name:    "everything gone empties the account",
			current: []asset.Asset{{Name: "A", Yen: 100}, {Name: "B", Yen: 200}},
			desired: nil,
			want:    []string{"delete:A", "delete:B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planActionLabels(Reconcile(tt.current, tt.desired))
			if !sameStrings(got, tt.want) {
				t.Errorf("Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReconcileDeleteOrderIsStable keeps a plan readable and diffable: the same
// inputs must produce the same order, whatever order the account listed them in.
func TestReconcileDeleteOrderIsStable(t *testing.T) {
	current := []asset.Asset{{Name: "zebra"}, {Name: "alpha"}, {Name: "mike"}}
	got := planActionLabels(Reconcile(current, nil))
	want := []string{"delete:alpha", "delete:mike", "delete:zebra"}
	if !sameStrings(got, want) {
		t.Errorf("Reconcile() = %v, want deletes in name order %v", got, want)
	}
}

// TestReconcileCarriesValues checks the figures a caller needs to log and to act
// on: Was for what is recorded, Now for what it should become.
func TestReconcileCarriesValues(t *testing.T) {
	plan := Reconcile(
		[]asset.Asset{{Name: "A", Yen: 100}, {Name: "gone", Yen: 7}},
		[]asset.Asset{{Name: "A", Yen: 150}, {Name: "new", Yen: 5}},
	)

	byName := map[string]Step{}
	for _, s := range plan.Steps {
		byName[s.Name] = s
	}

	if s := byName["A"]; s.Was != 100 || s.Now != 150 {
		t.Errorf("update step = was %d now %d, want 100 -> 150", s.Was, s.Now)
	}
	if s := byName["new"]; s.Now != 5 {
		t.Errorf("create step Now = %d, want 5", s.Now)
	}
	if s := byName["gone"]; s.Was != 7 {
		t.Errorf("delete step Was = %d, want 7", s.Was)
	}
}

func TestPlanCounts(t *testing.T) {
	plan := Reconcile(
		[]asset.Asset{{Name: "keep", Yen: 1}, {Name: "change", Yen: 1}, {Name: "drop", Yen: 1}},
		[]asset.Asset{{Name: "keep", Yen: 1}, {Name: "change", Yen: 2}, {Name: "add", Yen: 1}},
	)
	counts := plan.Counts()

	want := map[Action]int{
		ActionUnchanged: 1,
		ActionUpdate:    1,
		ActionCreate:    1,
		ActionDelete:    1,
	}
	for action, n := range want {
		if counts[action] != n {
			t.Errorf("Counts()[%s] = %d, want %d", action, counts[action], n)
		}
	}
}

// TestReconcileMatchesOnName documents why name is the key: neither side knows
// the other's identifiers, so nothing else is shared.
func TestReconcileMatchesOnName(t *testing.T) {
	current := []asset.Asset{{Name: "[米国株] テスト電機", Yen: 100}}
	desired := []asset.Asset{{Name: "[米国株] テスト電機", Yen: 100}}

	got := planActionLabels(Reconcile(current, desired))
	if !sameStrings(got, []string{"unchanged:[米国株] テスト電機"}) {
		t.Errorf("Reconcile() = %v, want it matched on name", got)
	}
}

// TestDuplicateName covers the assumption every part of a plan makes.
func TestDuplicateName(t *testing.T) {
	if got := DuplicateName([]asset.Asset{{Name: "a"}, {Name: "b"}}); got != "" {
		t.Errorf("DuplicateName() = %q on distinct names", got)
	}
	if got := DuplicateName([]asset.Asset{{Name: "a"}, {Name: "b"}, {Name: "a"}}); got != "a" {
		t.Errorf("DuplicateName() = %q, want the repeated name", got)
	}
	if got := DuplicateName(nil); got != "" {
		t.Errorf("DuplicateName(nil) = %q", got)
	}
}

// TestStepConfirm is the rule for what counts as a write having worked.
//
// Reading back is the only reliable verdict: the ledger answers a rejected write
// with 200 and the page re-rendered, so nothing about the response distinguishes
// it from success.
func TestStepConfirm(t *testing.T) {
	want := asset.Asset{Name: "テスト電機", Yen: 456789, AcquisitionYen: 400000, HasAcquisition: true}
	landed := want

	tests := map[string]struct {
		step    Step
		found   *asset.Asset
		wantErr string
	}{
		"a create that landed":   {Step{Action: ActionCreate, Name: want.Name}, &landed, ""},
		"an update that landed":  {Step{Action: ActionUpdate, Name: want.Name}, &landed, ""},
		"a delete that happened": {Step{Action: ActionDelete, Name: want.Name}, nil, ""},
		"unchanged is nothing to confirm": {
			Step{Action: ActionUnchanged, Name: want.Name}, &landed, "",
		},
		"a create that did not appear": {
			Step{Action: ActionCreate, Name: want.Name}, nil, "not recorded",
		},
		"a delete that left it behind": {
			Step{Action: ActionDelete, Name: want.Name}, &landed, "still recorded",
		},
		"a value that did not land": {
			Step{Action: ActionUpdate, Name: want.Name},
			&asset.Asset{Name: want.Name, Yen: 1, AcquisitionYen: 400000, HasAcquisition: true},
			"recorded value is 1",
		},
		// The one the ledger's own response can never reveal, and the reason the
		// whole detail-page walk on the broker side exists.
		//
		// Matched on "recorded=false" rather than just "cost": a dropped cost and
		// a wrong cost are different failures, and the looser assertion passed
		// while this check was removed, because the next one caught it and said
		// "cost" too.
		"a cost that was dropped": {
			Step{Action: ActionUpdate, Name: want.Name},
			&asset.Asset{Name: want.Name, Yen: want.Yen},
			"recorded=false",
		},
		"a cost that landed wrong": {
			Step{Action: ActionUpdate, Name: want.Name},
			&asset.Asset{Name: want.Name, Yen: want.Yen, AcquisitionYen: 1, HasAcquisition: true},
			"recorded cost is 1",
		},
		"an action nothing knows": {
			Step{Action: Action("sideways"), Name: want.Name}, &landed, "unknown action",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.step.Confirm(want, tt.found)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Confirm() error = %v, want none", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Confirm() accepted %s", name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Confirm() error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

// TestCheckCoverage refuses to delete an entry from a category the run never
// looked at.
//
// This replaced a limit on the share of the ledger one run could delete, which
// could not tell a bad scrape from a sale and so refused both. Every failure it
// stood in for now fails at the source with its own complaint, and the whole
// read fails with it — so a plan that exists rests on verified pages, and the
// question worth asking is not how many deletes there are but whether the run
// covered where they come from.
func TestCheckCoverage(t *testing.T) {
	del := func(names ...string) Plan {
		var p Plan
		for _, n := range names {
			p.Steps = append(p.Steps, Step{Action: ActionDelete, Name: n})
		}
		return p
	}

	tests := map[string]struct {
		plan    Plan
		covered []string
		wantErr bool
	}{
		"the category was read": {
			del("[米国株] テスト電機"), []string{"米国株", "ミニ"}, false,
		},
		// The case the old limit refused: somebody sold out of a category. Every
		// page was read and one of them is now empty. Nothing is wrong.
		"everything in a covered category, sold": {
			del("[米国株] テスト電機", "[米国株] テスト商事", "[ミニ] テスト電機"),
			[]string{"米国株", "ミニ"}, false,
		},
		// The case worth catching: a target dropped out of the list, so its
		// entries are unverified rather than stale.
		"a category nobody read": {
			del("[投信ミ] テストAIファンド"), []string{"米国株", "ミニ"}, true,
		},
		"one covered, one not": {
			del("[米国株] テスト電機", "[投信ミ] テストAIファンド"),
			[]string{"米国株"}, true,
		},
		// A row somebody typed in by hand. Reconcile does plan its deletion —
		// the account is managed wholesale — and coverage has nothing to say
		// about it, because there is no category to have failed to read.
		//
		// This asserts no error, which means the delete goes ahead. That is the
		// contract the setup instructions warn about.
		"no prefix at all, deleted without complaint": {
			del("手で足した何か"), []string{"米国株"}, false,
		},
		"nothing deleted": {Plan{}, nil, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.plan.CheckCoverage(tt.covered)
			if tt.wantErr != (err != nil) {
				t.Fatalf("CheckCoverage() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrUnverifiedDeletes) {
				t.Errorf("error = %v, want ErrUnverifiedDeletes", err)
			}
		})
	}
}

// TestReconcileNoticesAChangedKind is what merging Position into Asset was for.
//
// The comparison used to run on a type with the instrument kind stripped off,
// so a row filed as 投資信託 when it is 米国株 read as unchanged and stayed wrong
// for as long as the figures did not move. Nobody looking at the amounts would
// ever notice.
func TestReconcileNoticesAChangedKind(t *testing.T) {
	same := func(k asset.Kind) []asset.Asset {
		return []asset.Asset{{Name: "テスト電機", Yen: 456789, Kind: k}}
	}

	plan := Reconcile(same(asset.MutualFund), same(asset.USStock))
	if n := plan.Counts()[ActionUpdate]; n != 1 {
		t.Errorf("plan = %+v, want the wrongly filed row corrected", plan.Counts())
	}

	// And leaves it alone when nothing differs.
	if n := Reconcile(same(asset.USStock), same(asset.USStock)).Counts()[ActionUnchanged]; n != 1 {
		t.Error("an identical position was not left alone")
	}
}

// TestCheckCategoryEmptied is the guard added after the same category lost the
// same two 銘柄 twice, on two different reading bugs.
func TestCheckCategoryEmptied(t *testing.T) {
	entry := func(names ...string) []asset.Asset {
		out := make([]asset.Asset, 0, len(names))
		for _, n := range names {
			out = append(out, asset.Asset{Name: n, Yen: 1})
		}
		return out
	}

	tests := map[string]struct {
		recorded, held []asset.Asset
		wantErr        bool
	}{
		// The failure. Both 投信ミ rows gone, the category read as empty.
		"a category read as empty": {
			entry("[米国株] テスト電機", "[投信ミ] A", "[投信ミ] B"),
			entry("[米国株] テスト電機"),
			true,
		},
		// The case refusing which started all of this. Four of five sold, and
		// every category still has something.
		"most of a category sold": {
			entry("[米国株] A", "[米国株] B", "[米国株] C", "[米国株] D", "[ミニ] E"),
			entry("[米国株] A", "[ミニ] E"),
			false,
		},
		"nothing removed":  {entry("[米国株] A"), entry("[米国株] A"), false},
		"a category added": {entry("[米国株] A"), entry("[米国株] A", "[ミニ] B"), false},
		// Hand-typed rows carry no category, so there is nothing to compare.
		"unprefixed rows are not categories": {
			entry("手で足した何か"), entry("[米国株] A"), false,
		},
		"an empty ledger": {nil, entry("[米国株] A"), false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckCategoryEmptied(tt.recorded, tt.held)
			if tt.wantErr != (err != nil) {
				t.Fatalf("CheckCategoryEmptied() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrCategoryEmptied) {
				t.Errorf("error = %v, want ErrCategoryEmptied", err)
			}
		})
	}
}
