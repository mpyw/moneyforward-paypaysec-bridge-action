package manualasset

import (
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/v3/internal/application/domain/asset"
)

// TestEveryKindMapsBothWays holds the translation between this site's
// numbering and the domain's instrument kinds.
//
// Exhaustive over [asset.Kinds], so adding a kind without a 資産クラス for it
// fails here rather than at a write — where the site answers a rejected entry
// with a 200 and a re-rendered page, and the holding simply never appears.
func TestEveryKindMapsBothWays(t *testing.T) {
	seen := map[AssetSubclass]asset.Kind{}
	for _, kind := range asset.Kinds() {
		subclass, err := SubclassFor(kind)
		if err != nil {
			t.Errorf("SubclassFor(%v) = %v — every kind this program can produce "+
				"needs a 資産クラス to be filed under", kind, err)
			continue
		}
		if other, dup := seen[subclass]; dup {
			t.Errorf("%v and %v both map to 資産クラス %d; one of them is filed wrong",
				other, kind, subclass)
		}
		seen[subclass] = kind

		if back := KindOf(subclass); back != kind {
			t.Errorf("KindOf(SubclassFor(%v)) = %v — the account cannot be read back "+
				"in the terms it is written in", kind, back)
		}
	}
}

// TestSubclassForRefusesTheZeroValue: an unrecognised kind must not quietly
// become whatever the zero value files as.
func TestSubclassForRefusesTheZeroValue(t *testing.T) {
	if got, err := SubclassFor(asset.KindUnknown); err == nil {
		t.Errorf("SubclassFor(KindUnknown) = %d, want an error", got)
	}
}

// TestKindOfIsForgivingWhereSubclassForIsNot.
//
// Reading is not the place to refuse: the account may hold a row somebody
// created by hand under a 資産クラス this program has no kind for, and the read
// has to report the rest of the account rather than fail.
func TestKindOfIsForgiving(t *testing.T) {
	if got := KindOf(AssetSubclass(9999)); got != asset.KindUnknown {
		t.Errorf("KindOf(unknown) = %v, want the zero Kind", got)
	}
}

// TestSavingsInsuranceIsFiledAsInsurance pins a choice rather than a fact.
//
// The create form offers sixty-odd 資産クラス, several of which would take this
// holding without complaint: 外債 describes what the contract is invested in,
// 国債 would be wrong outright (MoneyForward means Japanese government bonds by
// it), and その他 takes anything. 積立型保険 was chosen, and a mapping nothing
// pins is one that gets changed by whoever next reads the list.
func TestSavingsInsuranceIsFiledAsInsurance(t *testing.T) {
	got, err := SubclassFor(asset.SavingsInsurance)
	if err != nil {
		t.Fatalf("SubclassFor(SavingsInsurance) = %v", err)
	}
	if got != SubclassSavingsInsurance {
		t.Errorf("SubclassFor(SavingsInsurance) = %d, want %d (積立型保険)",
			got, SubclassSavingsInsurance)
	}
}
